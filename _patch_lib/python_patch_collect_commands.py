#!/usr/bin/env python3
"""Bounded, whitelist-only COLLECT command evidence (local or saved SSH profile)."""
from __future__ import annotations
import os
from pathlib import Path
import signal
import subprocess
import threading
import time

from python_patch_package_schema import PatchSchemaError, _check_legacy_strict_command
from python_patch_remote_ssh import RemoteSSHError, run_remote_ssh

READONLY = frozenset({"pwd", "ls", "tree", "find", "grep", "rg",
                      "cat", "head", "tail", "wc", "nl", "sed"})
MAX_OUTPUT = 65536


class CollectCommandError(ValueError):
    pass


def validate_command_action(action: dict, label: str = "COLLECT command") -> None:
    argv = action.get("argv")
    if not isinstance(argv, list) or not argv or len(argv) > 24 or any(
        not isinstance(x, str) or not x or len(x) > 512 for x in argv
    ):
        raise CollectCommandError(f"{label}.argv must contain 1..24 bounded literal string tokens")
    if argv[0] not in READONLY:
        raise CollectCommandError(f"{label}: executable outside read-only whitelist; scripts and interpreters forbidden")
    transport = action.get("transport", "local")
    if transport not in {"local", "ssh"}:
        raise CollectCommandError(f"{label}.transport must be local or ssh")
    timeout = action.get("timeout_seconds", 30)
    if type(timeout) is not int or not 1 <= timeout <= 120:
        raise CollectCommandError(f"{label}.timeout_seconds must be 1..120")
    bound = action.get("max_output_bytes", MAX_OUTPUT)
    if type(bound) is not int or not 1024 <= bound <= MAX_OUTPUT:
        raise CollectCommandError(f"{label}.max_output_bytes must be 1024..65536")
    if transport == "ssh":
        if action.get("cwd") not in (None, "."):
            raise CollectCommandError(f"{label}: SSH cwd cannot be specified")
        name = action.get("profile_name")
        if not isinstance(name, str) or not name or name.strip() != name or len(name.encode()) > 128:
            raise CollectCommandError(f"{label}.profile_name must name an exact saved SSH profile")
    else:
        if action.get("profile_name") is not None:
            raise CollectCommandError(f"{label}: local mode must not specify SSH profile_name")
        cwd = action.get("cwd", ".")
        if not isinstance(cwd, str) or not cwd or len(cwd) > 240:
            raise CollectCommandError(f"{label}.cwd must be a project-relative directory")


def _capture_local(root: Path, action: dict) -> dict:
    from python_patch_package_schema import resolve_project_path
    argv = action["argv"]
    cwd_raw = action.get("cwd", ".")
    cwd = root if cwd_raw == "." else resolve_project_path(root, cwd_raw)
    if not cwd.is_dir():
        raise CollectCommandError("local command cwd is not a project directory")
    try:
        _check_legacy_strict_command(root, {"argv": argv, "cwd": cwd_raw}, 0)
    except PatchSchemaError as exc:
        raise CollectCommandError(f"local command denied: {exc}") from exc
    cap = int(action.get("max_output_bytes", MAX_OUTPUT))
    data = bytearray()
    total = [0]
    started = time.monotonic()
    kwargs = {"cwd": cwd, "stdin": subprocess.DEVNULL,
              "stdout": subprocess.PIPE, "stderr": subprocess.STDOUT}
    if os.name == "nt":
        kwargs["creationflags"] = getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
    else:
        kwargs["start_new_session"] = True
    proc = subprocess.Popen(argv, **kwargs)
    def pump() -> None:
        assert proc.stdout is not None
        while True:
            chunk = proc.stdout.read(4096)
            if not chunk:
                break
            total[0] += len(chunk)
            if len(data) < cap:
                data.extend(chunk[:cap-len(data)])
    reader = threading.Thread(target=pump, daemon=True)
    reader.start()
    timed_out = False
    try:
        proc.wait(timeout=int(action.get("timeout_seconds", 30)))
    except subprocess.TimeoutExpired:
        timed_out = True
        try:
            if os.name == "nt":
                subprocess.run(["taskkill", "/PID", str(proc.pid), "/T", "/F"],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                               timeout=3, check=False)
            else:
                os.killpg(proc.pid, signal.SIGKILL)
        except (OSError, subprocess.TimeoutExpired):
            proc.kill()
        proc.wait(timeout=5)
    finally:
        reader.join(timeout=2)
        if proc.stdout is not None:
            proc.stdout.close()
    return {"rc": 124 if timed_out else int(proc.returncode or 0),
            "timed_out": timed_out, "truncated": total[0] > cap,
            "elapsed_seconds": round(time.monotonic() - started, 3),
            "output": bytes(data).decode("utf-8", errors="replace")}


def execute_collect_command(root: Path, action: dict) -> dict:
    validate_command_action(action)
    argv = list(action["argv"])
    transport = action.get("transport", "local")
    cap = int(action.get("max_output_bytes", MAX_OUTPUT))
    if transport == "ssh":
        try:
            result = run_remote_ssh(root, {
                "profile_name": action["profile_name"],
                "argv": argv,
                "timeout_seconds": action.get("timeout_seconds", 30)
            }, dry_run=False)
        except RemoteSSHError as exc:
            raise CollectCommandError(f"SSH command could not run: {exc}") from exc
        content = (str(result.get("stdout") or "") + str(result.get("stderr") or "")).encode("utf-8", errors="replace")
        return {
            "transport": transport, "profile_name": action["profile_name"],
            "argv": argv, "rc": int(result.get("rc") or 0),
            "timed_out": False, "truncated": len(content) > cap,
            "output": content[:cap].decode("utf-8", errors="replace"),
        }
    return {"transport": transport, "argv": argv, **_capture_local(root, action)}


def command_report(action: dict, result: dict) -> str:
    header = [
        "# COLLECT read-only command", "",
        "- Transport: " + str(result["transport"]),
        "- Command argv: " + repr(result["argv"]),
        "- Exit code: " + str(result["rc"]),
        "- Timeout: " + str(result["timed_out"]),
        "- Output truncated: " + str(result["truncated"]),
    ]
    if result.get("profile_name"):
        header.append("- Saved SSH profile: " + str(result["profile_name"]))
    header.extend(["", "## Console output", "", "<pre>",
                   str(result.get("output") or "").replace("<", "&lt;").replace(">", "&gt;"),
                   "</pre>", ""])
    return "\n".join(header)
