#!/usr/bin/env python3
"""Read-only COLLECT command action: schema, runner, SSH bridge and report ZIP."""
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
from python_patch_collect_schema import CollectSchemaError, validate_request_data
from python_patch_collect_commands import (
    CollectCommandError, validate_command_action, execute_collect_command,
)

for argv in (
    ["bash", "scripts/read.sh"],
    ["python3", "-c", "print(1)"],
    ["sh", "-c", "id"],
    ["./tool.sh"],
):
    try:
        validate_command_action({"type": "command", "argv": argv})
    except CollectCommandError:
        pass
    else:
        raise AssertionError("COLLECT accepted executable/script: " + repr(argv))

for options in (
    {"type": "command", "argv": ["grep", "error", "."]},
    {"type": "command", "argv": ["rg", "-n", "--", "connection failed", "."]},
    {"type": "command", "transport": "ssh", "profile_name": "Production",
     "argv": ["find", ".", "-maxdepth", "2", "-type", "f"]},
):
    normalized = validate_request_data({"actions": [options]})
    assert normalized["actions"][0]["type"] == "command"

for options in (
    {"type": "command", "argv": ["bash", "script.sh"]},
    {"type": "command", "transport": "ssh", "argv": ["pwd"]},
    {"type": "command", "transport": "local", "profile_name": "remote", "argv": ["pwd"]},
    {"type": "command", "argv": ["pwd"], "timeout_seconds": 999},
    {"type": "command", "argv": ["pwd"], "unexpected": "bad"},
):
    try:
        validate_request_data({"actions": [options]})
    except CollectSchemaError:
        pass
    else:
        raise AssertionError("COLLECT accepted bad command action: " + repr(options))

with tempfile.TemporaryDirectory(prefix="ptv_collect_command_v6200_") as td:
    root = Path(td)
    (root / "src").mkdir()
    (root / "patchs").mkdir()
    (root / "src" / "code.txt").write_text("alpha\nneedle string\nomega\n", encoding="utf-8")
    good = {"type": "command", "argv": ["grep", "-nF", "--", "needle string", "src/code.txt"],
            "timeout_seconds": 15, "max_output_bytes": 8192}
    result = execute_collect_command(root, good)
    assert result["rc"] == 0 and "needle string" in result["output"], result
    assert result["transport"] == "local"

    # Bounded local traversal has to use the same conservative grammar as
    # saved-profile SSH; the historical PATCH strict lane remains compatible.
    for argv in (
        ["pwd"],
        ["ls", "-lah", "src"],
        ["tree", "-L", "2", "."],
        ["find", ".", "-maxdepth", "2", "-type", "f", "-name", "*.txt"],
        ["find", ".", "-maxdepth", "2", "-mindepth", "1", "-iname", "*.TXT", "-print"],
    ):
        from python_patch_collect_commands import _validate_collect_basic_args
        _validate_collect_basic_args(root, argv)
    subdir_result = execute_collect_command(root, {
        "type": "command", "argv": ["grep", "-nF", "--", "needle string", "code.txt"],
        "cwd": "src",
    })
    assert subdir_result["rc"] == 0 and "needle string" in subdir_result["output"], subdir_result
    for argv in (
        ["pwd", "src"],
        ["ls", "-R", "src"],
        ["ls", "--recursive", "src"],
        ["tree", "-oout.log", "."],
        ["tree", "-o", "out.log"],
        ["find", "."],
        ["find", ".", "-name", "*.txt"],
        ["find", ".", "-maxdepth", "7"],
        ["find", ".", "-maxdepth", "2", "-printf", "%p"],
        ["find", ".", "-maxdepth", "2", "-fprint", "dump.txt"],
        ["find", ".", "-maxdepth", "2", "-execdir", "sh", "-c", "id", ";"],
    ):
        try:
            execute_collect_command(root, {"type": "command", "argv": argv})
        except CollectCommandError:
            pass
        else:
            raise AssertionError("COLLECT basic whitelist accepted unsafe argv: " + repr(argv))
    assert not (root / "out.log").exists()
    assert not (root / "dump.txt").exists()

    for argv in (
        ["find", ".", "-exec", "touch", "NEVER", ";"],
        ["find", ".", "-delete"],
        ["sed", "-i", "1d", "src/code.txt"],
        ["rg", "--pre", "scripts/read.sh", "needle", "src"],
    ):
        try:
            execute_collect_command(root, {"type": "command", "argv": argv})
        except (CollectCommandError, OSError):
            pass
        else:
            raise AssertionError("Unsafe COLLECT argv accepted: " + repr(argv))
    assert not (root / "NEVER").exists()
    assert (root / "src" / "code.txt").read_text().startswith("alpha\n")

    # An SSH action must route through the TaskDeck bridge, not a local shell.
    import python_patch_collect_commands as mod
    prior = mod.run_remote_ssh
    captured = []
    def stub_ssh(project, request, *, dry_run, output_callback=None):
        captured.append((project, request, dry_run))
        if output_callback is not None:
            output_callback("stdout", b"REMOTE FILE\n")
        return {"rc": 0, "stdout": "REMOTE FILE\n", "stderr": ""}
    mod.run_remote_ssh = stub_ssh
    try:
        response = execute_collect_command(root, {
            "type": "command", "transport": "ssh", "profile_name": "Saved SSH",
            "argv": ["pwd"], "timeout_seconds": 12,
        })
        assert response["output"] == "REMOTE FILE\n"
        assert captured == [(root, {"profile_name": "Saved SSH", "argv": ["pwd"], "timeout_seconds": 12}, False)]
    finally:
        mod.run_remote_ssh = prior

    # The public collector publishes console evidence as a normal report
    # inside the result ZIP, even when no source file was selected for packing.
    request = {"id": "collected-command", "actions": [good]}
    source = root / "patchs" / "CODE_COLLECTION_REQUEST_command.zip"
    with zipfile.ZipFile(source, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("CODE_COLLECTION_REQUEST_command.json", json.dumps(request))
    env = dict(os.environ)
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    command_log_dir = root / "artifacts" / "patch_tool" / "runs" / "fixture" / "command_items" / "001_collect" / "command_logs"
    command_state_file = root / "artifacts" / "patch_tool" / "runtime" / "collect_commands_fixture.json"
    env["PTV_COLLECT_COMMAND_LOG_DIR"] = str(command_log_dir)
    env["PTV_COLLECT_COMMAND_RESULT_FILE"] = str(command_state_file)
    cmd = [sys.executable, "-S", str(HERE / "python_patch_collect_compat.py"),
           "--project-root", str(root), "request", "patchs/CODE_COLLECTION_REQUEST_command.zip"]
    proc = subprocess.run(cmd, cwd=root, capture_output=True, text=True, env=env, timeout=60)
    assert proc.returncode == 0, (proc.stdout, proc.stderr)
    outputs = list((root / "artifacts" / "patch_tool_code_collections").glob("CODE_COLLECTION_RESULT_*.zip"))
    assert len(outputs) == 1, outputs
    with zipfile.ZipFile(outputs[0]) as zf:
        manifest = json.loads(zf.read("COLLECTION_MANIFEST.json"))
        assert manifest["collection_status"] == "PASS", manifest
        assert manifest["file_count"] == 0 and manifest["action_count"] == 1, manifest
        report = zf.read(manifest["reports"][0]["archive_path"]).decode("utf-8")
        assert "needle string" in report and "Exit code: 0" in report
    assert not source.exists()
    assert (root / "patchs" / "patched" / source.name).is_file()
    live = json.loads(command_state_file.read_text(encoding="utf-8"))
    assert len(live["commands"]) == 1 and live["commands"][0]["status"] == "PASS", live
    evidence = live["commands"][0]
    assert evidence["started_at"] and evidence["elapsed_seconds"] >= 0
    assert evidence["log_path"].endswith("001_collect_command.log"), evidence
    assert "needle string" in (root / evidence["log_path"]).read_text(encoding="utf-8")

print("PASS: COLLECT command schema, read-only local/SSH, result report and archive")


# Explicit owner-registered Nim gateway: no Docker dependency in this test.
# The stand-in records argv, demonstrating that the native collector invokes
# only the allowed read action with an exact RUN_ID and no shell interpolation.
if os.name != "nt" and shutil.which("bash"):
    import hashlib
    from unittest.mock import patch
    with tempfile.TemporaryDirectory(prefix="ptv_nim_gateway_") as td:
        home = Path(td).resolve()
        root = home / "project"
        root.mkdir()
        script = root / "nim-agent.sh"
        script.write_text('#!/bin/bash\nprintf "%s\\n" "$@"\n', encoding="utf-8")
        project_id = "01234567-89ab-cdef-0123-456789abcdef"
        (root / ".python_patch_tool.json").write_text(json.dumps({"project": {"key": "nim", "id": project_id}}))
        cfg = home / ".config/taskdeck/nim-gateway.json"
        cfg.parent.mkdir(parents=True)
        cfg.write_text(json.dumps({"project_root": str(root), "project_id": project_id,
                                   "script": "nim-agent.sh", "sha256": hashlib.sha256(script.read_bytes()).hexdigest()}))
        cfg.chmod(0o600)
        rid = "a" * 32
        action = {"type": "command", "transport": "local", "argv": ["nim-gateway", "nim-build-result", rid]}
        validate_request_data({"actions": [action]})
        with patch.dict(os.environ, {"HOME": str(home), "TASKDECK_NIM_GATEWAY_LOCAL": "1"}):
            result = execute_collect_command(root, action)
            assert result["rc"] == 0 and result["output"] == "nim-build-result\n" + rid + "\n", result
            for bad in (
                {"argv": ["nim-gateway", "nim-build-result"]},
                {"argv": ["nim-gateway", "nim-build-log", "latest"]},
                {"argv": ["nim-gateway", "nim-build-log", "../receipt.json"]},
                {"argv": ["nim-gateway", "nim-build", "server"]},
                {"argv": ["nim-gateway", "nim-delete", "test.nim", rid]},
                {"argv": ["nim-gateway", "nim-build-result", rid], "transport": "ssh", "profile_name": "Production"},
                {"argv": ["nim-gateway", "nim-build-result", rid], "cwd": "other"},
            ):
                try:
                    execute_collect_command(root, {"type": "command", **bad})
                except CollectCommandError:
                    pass
                else:
                    raise AssertionError("unsafe Nim gateway action accepted: " + repr(bad))
            with patch.dict(os.environ, {"TASKDECK_NIM_GATEWAY_LOCAL": "0"}):
                try:
                    execute_collect_command(root, action)
                except CollectCommandError:
                    pass
                else:
                    raise AssertionError("non-local worker read Nim gateway")
            script.write_text("exit 0\n")
            try:
                execute_collect_command(root, action)
            except CollectCommandError:
                pass
            else:
                raise AssertionError("changed Nim gateway accepted")
    print("PASS: owner-bound read-only Nim gateway COLLECT")
