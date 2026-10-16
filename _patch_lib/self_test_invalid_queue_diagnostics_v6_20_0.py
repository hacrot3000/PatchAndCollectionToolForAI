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
