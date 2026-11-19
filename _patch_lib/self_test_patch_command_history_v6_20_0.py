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
        event_read, event_write = os.pipe()
        env["TASKDECK_PATCH_EVENT_FD"] = str(event_write)
        try:
            run = subprocess.run(
                [str(root / "tools" / "run_python_patches.sh"),
                 "run", "--patch", "patchs/commands.zip", "-y"],
                cwd=root, env=env, capture_output=True, text=True, timeout=90,
                pass_fds=(event_write,),
            )
        finally:
            os.close(event_write)
        import select
        event_parts = []
        try:
            while select.select([event_read], [], [], 0.3)[0]:
                chunk = os.read(event_read, 65536)
                if not chunk:
                    break
                event_parts.append(chunk)
        finally:
            os.close(event_read)
        emitted = [json.loads(x) for x in b"".join(event_parts).decode("utf-8").splitlines()
                   if x.strip()]
        live = [x for x in emitted if x.get("type") == "command_state"]
        assert [x["command"]["status"] for x in live] == ["QUEUE", "RUNNING", "PASS"], live
        assert [x["command"]["index"] for x in live] == [1, 1, 1], live
        assert all(x["item_index"] == 1 and x["item_name"] == "commands.zip" for x in live), live
        finished = [x for x in emitted if x.get("type") == "item_finished"
                    and x.get("name") == "commands.zip"]
        assert len(finished) == 1 and len(finished[0].get("commands") or []) == 1, finished
        assert finished[0]["commands"][0]["name"] == "Line one", finished
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

# A command refused before subprocess spawn must still leave History-readable
# console evidence; an existing partial console must not be overwritten.
from python_patch_runner import _write_command_failure_console
with tempfile.TemporaryDirectory(prefix="ptv_failed_command_log_") as td:
    log = Path(td) / "001_failed.log"
    _write_command_failure_console(log, "permission_denied: blocked executable")
    assert log.is_file() and "permission_denied" in log.read_text(), log
    _write_command_failure_console(log, "second failure")
    content = log.read_text()
    assert "permission_denied" in content and "second failure" in content
    if os.name != "nt":
        assert log.stat().st_mode & 0o077 == 0, "command log must be private"

# Live protocol emits all planned commands before the first starts and keeps
# their indices stable until terminal result, including SKIPPED remainder.
import python_patch_runner as runner
import python_patch_protocol as protocol
with tempfile.TemporaryDirectory(prefix="ptv_live_commands_") as td:
    root = Path(td)
    (root / "code.txt").write_text("first\nsecond\n", encoding="utf-8")
    log_dir = root / "artifacts" / "patch_tool" / "runs" / "test" / "command_items" / "01_test" / "command_logs"
    log_dir.mkdir(parents=True)
    prior_emit = protocol.emit_command_state
    previous_log_dir = runner._ACTIVE_COMMAND_LOG_DIR
    previous_commands = runner._ACTIVE_COMMAND_EVIDENCE
    events = []
    protocol.emit_command_state = lambda command: events.append(dict(command)) or True
    runner._ACTIVE_COMMAND_LOG_DIR = log_dir
    runner._ACTIVE_COMMAND_EVIDENCE = []
    try:
        result = runner._run_command_sequence(root, [
            {"name": "Head one", "argv": ["head", "-n", "1", "code.txt"], "cwd": ".", "timeout_seconds": 10},
            {"name": "Head two", "argv": ["head", "-n", "2", "code.txt"], "cwd": ".", "timeout_seconds": 10},
        ], label="POST PATCH")
        assert result["status"] == "PASS", result
        assert [e["status"] for e in events] == ["QUEUE", "QUEUE", "RUNNING", "PASS", "RUNNING", "PASS"], events
        assert [e["index"] for e in events] == [1, 2, 1, 1, 2, 2], events
        assert all(e["started_at"] and e["elapsed_seconds"] >= 0 for e in events if e["status"] == "PASS")
        assert len(runner._ACTIVE_COMMAND_EVIDENCE) == 2
        assert (log_dir / "001_post_patch.log").read_text().strip() == "first"
        assert "second" in (log_dir / "002_post_patch.log").read_text()
    finally:
        protocol.emit_command_state = prior_emit
        runner._ACTIVE_COMMAND_LOG_DIR = previous_log_dir
        runner._ACTIVE_COMMAND_EVIDENCE = previous_commands

print("PASS: each PATCH command owns a durable console artifact and History record")
