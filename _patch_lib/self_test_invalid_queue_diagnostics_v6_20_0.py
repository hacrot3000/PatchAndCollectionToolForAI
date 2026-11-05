#!/usr/bin/env python3
"""Regression: invalid COLLECT explains the exact input issue end-to-end."""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import sys
import tempfile
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
spec = importlib.util.spec_from_file_location("ptv_invalid_collect_diagnostics", HERE / "python_patch_queue_dispatcher.py")
tool = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = tool
spec.loader.exec_module(tool)


def make_zip(target: Path, entries: dict[str, str]) -> None:
    with zipfile.ZipFile(target, "w") as zf:
        for name, value in entries.items():
            zf.writestr(name, value)


with tempfile.TemporaryDirectory() as directory:
    root = Path(directory)
    patchs = root / "patchs"
    patchs.mkdir()
    failures: list[tuple[str, str, str]] = [
        ("schema", json.dumps({"project": {"key": "server-patching", "id": "example-id"}, "actions": []}), "schema_error:"),
        ("syntax", '{"actions":[}', "invalid_request_json:JSONDecodeError"),
        ("unicode", "", "invalid_request_json:"),
    ]
    for name, body, prefix in failures:
        path = patchs / f"CODE_COLLECTION_REQUEST_{name}.zip"
        make_zip(path, {f"CODE_COLLECTION_REQUEST_{name}.json": body})
        valid, detail = tool.inspect_collect_zip(path)
        assert not valid, (name, detail)
        assert detail.startswith(prefix), (name, detail)

    no_request = patchs / "CODE_COLLECTION_REQUEST_missing.zip"
    make_zip(no_request, {"README.txt": "No JSON inside"})
    duplicate = patchs / "CODE_COLLECTION_REQUEST_duplicate.zip"
    make_zip(duplicate, {
        "CODE_COLLECTION_REQUEST_1.json": '{"actions":[{"type":"overview"}]}',
        "CODE_COLLECTION_REQUEST_2.json": '{"actions":[{"type":"overview"}]}',
    })
    bad_zip = patchs / "CODE_COLLECTION_REQUEST_broken.zip"
    bad_zip.write_bytes(b"not a zip file")

    items, _warnings = tool.discover_queue(root)
    invalid = {item.name: item for item in items if item.kind == "COLLECT INVALID"}
    for name, needle in [
        ("CODE_COLLECTION_REQUEST_schema.zip", "schema_error:"),
        ("CODE_COLLECTION_REQUEST_syntax.zip", "invalid_request_json:"),
        ("CODE_COLLECTION_REQUEST_missing.zip", "request_json_count=0"),
        ("CODE_COLLECTION_REQUEST_duplicate.zip", "request_json_count=2"),
        ("CODE_COLLECTION_REQUEST_broken.zip", "invalid_zip:"),
    ]:
        item = invalid.get(name)
        assert item is not None, (name, [(i.name,i.kind,i.detail) for i in items])
        assert needle in item.detail, (name, item.detail)
        diag = tool._invalid_queue_diagnosis(item)
        assert diag["kind"] == "invalid_queue_item", diag
        assert diag["message"] and diag["repair_hint"] and diag["discovery_detail"], diag
        assert diag["reason_code"] != "unknown_invalid_item", diag
        assert "No PATCH" not in diag["message"]
        if name.endswith("syntax.zip"):
            assert "line" in diag["message"] and "column" in diag["message"], diag
        if name.endswith("missing.zip"):
            assert "Không tìm thấy" in diag["message"], diag
        if name.endswith("duplicate.zip"):
            assert "2" in diag["message"], diag

    # Invalid requests remain diagnostic-only: handoff text is generated
    # without altering the input ZIP, with every unsupported action field
    # listed so a delegated AI can repair a new copy accurately.
    import hashlib
    multi = patchs / "CODE_COLLECTION_REQUEST_multi_fields.zip"
    make_zip(multi, {
        "CODE_COLLECTION_REQUEST_multi_fields.json": json.dumps({
            "actions": [
                {"type": "pack", "paths": ["."]},
                {"type": "find", "patterns": ["**/*.java"], "max_search_files": 125000},
                {"type": "find", "patterns": ["**/*.kt"], "max_search_files": 125000}
            ]
        })
    })
    before_hash = hashlib.sha256(multi.read_bytes()).hexdigest()
    multi_items, _ = tool.discover_queue(root)
    multi_item = next(row for row in multi_items if row.name == multi.name)
    assert multi_item.kind == "COLLECT INVALID", multi_item
    handoff = tool._invalid_collect_handoff(root, multi_item)
    assert handoff["handoff_name"].endswith(".md"), handoff
    assert "action[2] (find).max_search_files" in handoff["handoff"], handoff
    assert "action[3] (find).max_search_files" in handoff["handoff"], handoff
    assert "request.limits.max_search_files" in handoff["handoff"], handoff
    assert "Never overwrite, delete, rename or execute the original ZIP" in handoff["handoff"]
    assert hashlib.sha256(multi.read_bytes()).hexdigest() == before_hash
    assert list(patchs.glob("*_fixed.zip")) == []
    native = tool.protocol_queue_view(root)
    projected = next(row for row in native["items"] if row["name"] == multi.name)
    assert projected["selectable"] is False, projected
    assert projected["handoff"] == handoff["handoff"]
    assert tool._initial_selected([multi_item], "none") == set()
    assert tool._initial_selected([multi_item], "all") == set()
    assert tool._initial_selected([multi_item], "whatever") == set()

    # SQL collection handoff must enumerate nested schema errors, not only
    # the first rejected select item. Preserve original names and limits.
    nested = patchs / "CODE_COLLECTION_REQUEST_nested_database_select.zip"
    original_actions = []
    column_names = [
        "id", "csv_index", "key_name", "description", "datatype",
        "validator_class", "is_show_in_list", "xml_key"
    ]
    for name in ("ep00_buff_columns", "ep08_buff_columns", "ep11_buff_columns"):
        original_actions.append({
            "type": "database_select", "id": name, "profile": "HaiTacLocal",
            "select": [{"column": column} for column in column_names],
            "from": {"table": name},
            "order_by": [
                {"column": "csv_index", "direction": "asc"},
                {"column": "id", "direction": "asc"},
            ],
            "limit": 200, "max_rows": 200, "max_bytes": 800000,
            "timeout_sec": 30, "format": "jsonl",
        })
    nested_json = {
        "id": "ss-tool-mysql-buff-columns-haitaclocal-20261105",
        "project": {"key": "ss_tool", "id": "2a94ed57-5587-480b-adae-7c5c720f84f1"},
        "actions": original_actions,
    }
    make_zip(nested, {"CODE_COLLECTION_REQUEST_nested_database_select.json": json.dumps(nested_json)})
    before_nested = hashlib.sha256(nested.read_bytes()).hexdigest()
    local_config = root / ".python_patch_tool.json"
    local_config.write_text(json.dumps({
        "project": nested_json["project"], "other_setting": "must remain unchanged",
    }), encoding="utf-8")
    before_config = local_config.read_bytes()
    ok, reason = tool.inspect_collect_zip(nested)
    assert not ok and "action[1].select[0]" in reason, reason
    nested_item = tool.QueueItem(nested.name, "COLLECT INVALID", reason)
    nested_handoff = tool._invalid_collect_handoff(root, nested_item)["handoff"]
    assert "schema_error:action[1].select[0]" in nested_handoff, nested_handoff
    assert "Project identity declared in request" in nested_handoff, nested_handoff
    assert "key=ss_tool, id=2a94ed57-5587-480b-adae-7c5c720f84f1" in nested_handoff
    assert "MATCH on both fields" in nested_handoff
    assert "HaiTacLocal" in nested_handoff
    assert "ep00_buff_columns" in nested_handoff
    assert "ep08_buff_columns" in nested_handoff
    assert "ep11_buff_columns" in nested_handoff
    assert "max_bytes=800000" in nested_handoff
    assert "format=\"jsonl\"" in nested_handoff
    for action_idx in (1, 2, 3):
        for col_idx in range(8):
            assert f"action[{action_idx}].select[{col_idx}]" in nested_handoff, (action_idx, col_idx)
        for order_idx in range(2):
            assert f"action[{action_idx}].order_by[{order_idx}]" in nested_handoff, (action_idx, order_idx)
    assert "select=24, order_by=6" in nested_handoff, nested_handoff
    assert '{"column": "id"} -> {"expr": {"column": "id"}}' in nested_handoff
    assert '"direction": "asc"} -> {"expr": {"column": "id"}, "direction": "ASC"}' in nested_handoff
    assert "first reported error is not the only possible error" in nested_handoff
    assert "STOP if the original request ZIP" in nested_handoff
    assert "cannot safely be used to reconstruct it" in nested_handoff
    assert "SHA256 of the original request JSON member" in nested_handoff
    assert "Original request.id: ss-tool-mysql-buff-columns-haitaclocal-20261105" in nested_handoff
    assert "Never overwrite, delete, rename or execute the original ZIP" in nested_handoff
    assert hashlib.sha256(nested.read_bytes()).hexdigest() == before_nested
    assert local_config.read_bytes() == before_config
    assert not list(patchs.glob("*fixed*.zip"))
    local_config.write_text(json.dumps({
        "project": {"key": "different-project", "id": "00000000-0000-0000-0000-000000000000"}
    }), encoding="utf-8")
    mismatch_handoff = tool._invalid_collect_handoff(root, nested_item)["handoff"]
    assert "NOT VERIFIED / MISMATCH" in mismatch_handoff
    assert "key=ss_tool, id=2a94ed57-5587-480b-adae-7c5c720f84f1" in mismatch_handoff
    assert hashlib.sha256(nested.read_bytes()).hexdigest() == before_nested

    # Simulate the exact failing execution branch. This item must NEVER run
    # a PATCH/COLLECT payload, and its structured event must carry the reason.
    selected = invalid["CODE_COLLECTION_REQUEST_schema.zip"]
    events: list[tuple[str, dict[str, object]]] = []
    old_emitter = tool._emit_protocol_event
    tool._emit_protocol_event = lambda event_type, **data: (events.append((event_type, data)) or True)
    console = io.StringIO()
    try:
        with contextlib.redirect_stderr(console):
            outcome = tool.execute_items(root, [selected])
    finally:
        tool._emit_protocol_event = old_emitter
    assert outcome[0] == 2, outcome
    assert len(tool._LAST_EXECUTION_DETAILS) == 1
    row = tool._LAST_EXECUTION_DETAILS[0]
    assert row["status"] == "FAIL" and row["diagnosis"]["kind"] == "invalid_queue_item"
    assert "request.actions" in row["diagnosis"]["message"]
    assert row["diagnosis"]["repair_hint"] in row["output_tail"]
    assert "Discovery detail:" in row["output_tail"]
    assert "project unchanged" in row["output_tail"]
    assert "request.actions" in console.getvalue() and "Fix:" in console.getvalue()
    finished = [data for kind, data in events if kind == "item_finished"]
    assert len(finished) == 1, events
    assert finished[0]["diagnosis_kind"] == "invalid_queue_item"
    assert "request.actions" in finished[0]["failure_reason"]
    assert finished[0]["repair_hint"]
    assert "failed before execution" not in finished[0]["failure_reason"]

    # A COLLECT runner returning no result must not produce only a generic rc.
    missing_result = tool._collect_failure_diagnosis(3, None)
    assert missing_result["kind"] == "collect_result_missing", missing_result
    assert "runner" in missing_result["message"], missing_result
    assert missing_result["repair_hint"], missing_result
    failed_post = tool._collect_failure_diagnosis(
        3, {"status": "INCOMPLETE", "failure_reason": "incomplete action"},
        "COLLECT rc=0 but archived request is missing: patchs/patched/example.zip"
    )
    assert failed_post["kind"] == "collect_result_postcondition_failed", failed_post
    assert "archived request is missing" in failed_post["message"], failed_post
    reported = tool._collect_failure_diagnosis(2, {
        "status": "FAIL", "failure_reason": "action[2].path does not exist",
        "output_tail": "exact runner output"
    })
    assert reported["message"] == "action[2].path does not exist", reported
    assert reported["repair_hint"], reported

    # Structured PATCH classifications provide targeted advice without
    # replacing the runner's underlying error and source evidence.
    for kind, log, expected in [
        ("patch_payload_failed", "ERROR: src/example.nim: anchor not found", "anchor_mismatch"),
        ("patch_payload_failed", "ERROR: src/example.nim: checksum mismatch", "source_drift"),
        ("patch_payload_failed", "Traceback (most recent call last)", "python_exception"),
    ]:
        outcome = tool._enrich_patch_diagnosis(
            {"diagnosis": {"kind": kind, "message": "runner failure", "affected_paths": []}}, log
        )
        diag = outcome["diagnosis"]
        assert diag["kind"] == expected, (expected, diag)
        assert diag["repair_hint"], diag
        assert diag["message"] == "runner failure"

    # Legacy skipped-entry filename must remain recognizable by native Delete.
    (patchs / "CODE_COLLECTION_REQUEST_raw.json").write_text('{"actions":[]}', encoding="utf-8")
    _, warnings = tool.discover_queue(root)
    assert any(line == "RAW JSON REJECTED: patchs/CODE_COLLECTION_REQUEST_raw.json" for line in warnings)
    assert any("COLLECT REQUEST FIX:" in line for line in warnings)

print("PASS: invalid COLLECT queue/runtime/postcondition diagnostics include exact reason, hint, protocol and report")
