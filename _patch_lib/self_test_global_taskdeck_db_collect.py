#!/usr/bin/env python3
"""Regression: COLLECT resolves a saved global TaskDeck DB profile, not a project JSON.

A fake native TaskDeck bridge is used; no real database, secret or tunnel is opened.
"""
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parent))
from python_patch_database_select import (
    DatabaseSelectError, execute_database_select, validate_database_select_action,
)

with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)
    (root / ".python_patch_tool.json").write_text(
        '{"project":{"key":"ss_tool","id":"2a94ed57-5587-480b-adae-7c5c720f84f1"}}',
        encoding="utf-8",
    )
    fake = root / "taskdeck"
    fake.write_text(
        '#!/usr/bin/env python3\n'
        'import json,sys\n'
        'assert sys.argv[1:3]==["collect-db-select","--workspace"],sys.argv\n'
        'req=json.load(sys.stdin)\n'
        'assert req["profile"]=="HaiTacLocal"\n'
        'assert req["sql"].startswith("SELECT ") and "ep00_buff_columns" in req["sql"]\n'
        'assert req["max_rows"]==200 and req["timeout_sec"]==30\n'
        'print(json.dumps({"engine":"mysql","transport":"direct","columns":["id","csv_index"],'
        '"rows":[["1","0"],["2","1"]],"truncated":False}))\n',
        encoding="utf-8",
    )
    fake.chmod(0o700)
    old_helper = os.environ.get("PTV_TASKDECK_EXECUTABLE")
    old_profile = os.environ.pop("PTV_DB_PROFILES_FILE", None)
    os.environ["PTV_TASKDECK_EXECUTABLE"] = str(fake)
    request = {
        "type": "database_select", "id": "buff_columns_ep00",
        "profile": "HaiTacLocal",
        "select": [{"expr": {"column": "id"}}, {"expr": {"column": "csv_index"}}],
        "from": {"table": "ep00_buff_columns"},
        "order_by": [{"expr": {"column": "id"}, "direction": "ASC"}],
        "limit": 200, "max_rows": 200, "max_bytes": 800000,
        "timeout_sec": 30, "format": "jsonl",
    }
    cfg = validate_database_select_action(request)
    limits = {"max_file_bytes": 1024*1024, "max_total_bytes": 10*1024*1024}
    original_bytes = (root / ".python_patch_tool.json").read_bytes()
    try:
        result = execute_database_select(root, cfg, limits, root / "out")
        assert result["incomplete"] is False, result
        meta = result["meta"]
        assert meta["profile_source"] == "taskdeck-global", meta
        assert meta["rows_collected"] == 2, meta
        assert meta["safety"]["mysql_auth"] == "taskdeck-global-secretstore", meta
        values = (root / "out" / "database_select" / "result" / "result_0001.jsonl").read_text()
        assert '"id"' in values and '"1"' in values, values
        assert (root / ".python_patch_tool.json").read_bytes() == original_bytes
        assert not (root / "tools" / "db_profiles.local.json").exists()

        # Regression: the previous bridge incorrectly labeled any MySQL
        # 'Unknown column' as an outdated TaskDeck binary, hiding the cause.
        fake.write_text(
            '#!/bin/sh\\n'
            'echo "ERROR: Unknown column csv_index in field list" >&2\\n'
            'exit 2\\n',
            encoding="utf-8",
        )
        fake.chmod(0o700)
        try:
            execute_database_select(root, cfg, limits, root / "unknown_column")
        except DatabaseSelectError as exc:
            assert "Unknown column csv_index" in str(exc), exc
            assert "does not recognize collect-db-select" not in str(exc), exc
            assert str(fake) in str(exc), exc
        else:
            raise AssertionError("native MySQL unknown-column error was ignored")

        # Only a diagnostic explicitly rejecting collect-db-select is a
        # confirmed outdated binary error; include the binary path and stderr.
        fake.write_text(
            '#!/bin/sh\\n'
            'echo "ERROR: unknown subcommand collect-db-select" >&2\\n'
            'exit 2\\n',
            encoding="utf-8",
        )
        fake.chmod(0o700)
        try:
            execute_database_select(root, cfg, limits, root / "unsupported_subcommand")
        except DatabaseSelectError as exc:
            assert "does not recognize collect-db-select" in str(exc), exc
            assert "unknown subcommand collect-db-select" in str(exc), exc
            assert str(fake) in str(exc), exc
        else:
            raise AssertionError("unknown collect-db-select subcommand was ignored")

        # Explicit local-file override must not silently access global secrets.
        os.environ["PTV_DB_PROFILES_FILE"] = str(root / "nonexistent-custom-profile.json")
        try:
            execute_database_select(root, cfg, limits, root / "override")
        except DatabaseSelectError as exc:
            assert "profile file not found" in str(exc), exc
        else:
            raise AssertionError("missing explicit override should not fall back to global")
    finally:
        if old_helper is None:
            os.environ.pop("PTV_TASKDECK_EXECUTABLE", None)
        else:
            os.environ["PTV_TASKDECK_EXECUTABLE"] = old_helper
        if old_profile is not None:
            os.environ["PTV_DB_PROFILES_FILE"] = old_profile
print("PASS: COLLECT global TaskDeck profile fallback is read-only, source-preserving, and override-safe")
