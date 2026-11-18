#!/usr/bin/env python3
"""PATCH individual command console persists across Current run and History."""
from __future__ import annotations

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
from python_patch_queue_dispatcher import protocol_history_report_view


def patch_archive(root: Path, name: str, manifest: dict, payload: str | None = None) -> None:
    archive = root / "patchs" / name
    with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("PATCH_TOOL_MANIFEST.json", json.dumps(manifest))
        if payload is not None:
            zf.writestr("patch_apply.py", payload)


if os.name != "nt":
    with tempfile.TemporaryDirectory(prefix="ptv_command_history_v6200_") as td:
        root = Path(td)
        shutil.copytree(HERE.parent, root / "tools")
        (root / "tools" / "run_python_patches.sh").chmod(0o755)
        (root / "patchs").mkdir()
        (root / "one.txt").write_text("alpha\nbeta\n", encoding="utf-8")
        manifest = {
            "schema_version": 1,
            "patch": {"id": "command-history"},
            "post_patch": {
                "run_when_no_changes": True,
                "no_change_reason": "Read a previously existing project file for console evidence.",
                "commands": [
                    {"name": "Line one", "argv": ["head", "-n", "1", "one.txt"],
                     "cwd": ".", "timeout_seconds": 12}
                ],
            },
        }
        patch_archive(root, "commands.zip", manifest)
        env = dict(os.environ)
        env["PYTHONDONTWRITEBYTECODE"] = "1"
        env["PTV_DISABLE_LIVE_STATUS"] = "1"
        run = subprocess.run(
            [str(root / "tools" / "run_python_patches.sh"),
             "--patch", "patchs/commands.zip"],
            cwd=root, env=env, capture_output=True, text=True, timeout=90,
        )
        assert run.returncode == 0, (run.stdout, run.stderr)
        assert "alpha" in run.stdout

        report = json.loads((root / "artifacts" / "patch_tool" / "LAST_RUN.json").read_text())
        rows = report.get("results") or report.get("details") or []
        own = next((x for x in rows if x.get("name") == "commands.zip"), None)
        assert own is not None, report
        patch_result = own.get("patch_result")
        assert isinstance(patch_result, dict), own
        commands = patch_result.get("commands")
        assert isinstance(commands, list) and len(commands) == 1, patch_result
        row = commands[0]
        assert row["status"] == "PASS" and row["name"] == "Line one", row
        log_path = root / row["log_path"]
        assert log_path.is_file() and log_path.read_text().strip() == "alpha", log_path
        assert log_path.stat().st_size <= 512 * 1024 + 100

        history = protocol_history_report_view(root, report["run_id"])
        assert history["status"] == "available", history
        record = next((x for x in history["items"] if x["name"] == "commands.zip"), None)
        assert record is not None, history
        exposed = record.get("commands")
        assert isinstance(exposed, list) and exposed[0]["log_path"] == row["log_path"], record
        assert exposed[0]["argv"] == ["head", "-n", "1", "one.txt"]
else:
    print("SKIP: POSIX command artifact integration; Go protocol tests cover Windows")

print("PASS: each PATCH command owns a durable console artifact and History record")
