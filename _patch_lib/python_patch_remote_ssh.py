#!/usr/bin/env python3
"""Strict bridge for a manifest-only PATCH invoking one read-only SSH command.

The TaskDeck executable resolves the saved SSH profile/credentials and
enforces the *authoritative* remote argv whitelist a second time. A standalone
portable Patch Tool has no SSH identity authority and fails closed.
"""
from __future__ import annotations

import json
import os
from pathlib import Path
import shutil
import subprocess
import signal
import threading
from typing import Callable


class RemoteSSHError(ValueError):
    kind = "remote_ssh_rejected"


def _run_streamed_bridge(args: list[str], payload: str, root: Path, timeout: int,
                         callback: Callable[[str, bytes], None]) -> subprocess.CompletedProcess:
    """Stream bounded CLI output without using an interactive shell/stdin."""
    kwargs = {"cwd": root, "stdin": subprocess.PIPE,
              "stdout": subprocess.PIPE, "stderr": subprocess.PIPE}
    if os.name != "nt":
        kwargs["start_new_session"] = True
    elif hasattr(subprocess, "CREATE_NEW_PROCESS_GROUP"):
        kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    proc = subprocess.Popen(args, **kwargs)
    collected: dict[str, bytearray] = {"stdout": bytearray(), "stderr": bytearray()}
    def drain(kind: str, stream) -> None:
        buffer = collected[kind]
        while True:
            chunk = os.read(stream.fileno(), 4096)
            if not chunk:
                break
            buffer.extend(chunk)
            if len(buffer) > 65536:
                del buffer[:len(buffer) - 65536]
            try:
                callback(kind, chunk)
            except Exception:
                # Console is observational; do not interrupt authenticated SSH.
                pass
    threads = []
    for kind, stream in (("stdout", proc.stdout), ("stderr", proc.stderr)):
        thread = threading.Thread(target=drain, args=(kind, stream), daemon=True)
        thread.start()
        threads.append(thread)
    try:
        if proc.stdin is not None:
            try:
                proc.stdin.write(payload.encode("utf-8"))
                proc.stdin.flush()
            except BrokenPipeError:
                pass
            finally:
                try:
                    proc.stdin.close()
                except BrokenPipeError:
                    pass
        try:
            rc = proc.wait(timeout=timeout + 20)
        except subprocess.TimeoutExpired as exc:
            try:
                if os.name != "nt":
                    os.killpg(proc.pid, signal.SIGKILL)
                else:
                    proc.kill()
            except ProcessLookupError:
                pass
            proc.wait(timeout=5)
            raise RemoteSSHError("TaskDeck SSH command bridge timed out") from exc
    finally:
        for thread in threads:
            thread.join(timeout=3)
        for stream in (proc.stdout, proc.stderr):
            if stream is not None:
                stream.close()
    return subprocess.CompletedProcess(args, rc,
                                       collected["stdout"].decode("utf-8", errors="replace"),
                                       collected["stderr"].decode("utf-8", errors="replace"))


def run_remote_ssh(root: Path, remote: dict, *, dry_run: bool,
                   output_callback: Callable[[str, bytes], None] | None = None) -> dict[str, object]:
    if not isinstance(remote, dict):
        raise RemoteSSHError("remote_ssh must be an object")
    name = remote.get("profile_name")
    argv = remote.get("argv")
    timeout = remote.get("timeout_seconds", 30)
    if not isinstance(name, str) or not name or name.strip() != name or len(name.encode("utf-8")) > 128:
        raise RemoteSSHError("remote_ssh.profile_name must be the exact unique saved SSH profile name")
    if not isinstance(argv, list) or not argv or any(not isinstance(v, str) for v in argv):
        raise RemoteSSHError("remote_ssh.argv must be an array of literal argument strings")
    if type(timeout) is not int or not 1 <= timeout <= 120:
        raise RemoteSSHError("remote_ssh.timeout_seconds must be 1..120")
    executable = (os.environ.get("PTV_TASKDECK_EXECUTABLE") or "").strip()
    if not executable:
        executable = shutil.which("taskdeck") or ""
    if not executable:
        raise RemoteSSHError("TaskDeck executable is required for SSH profile execution; standalone Patch Tool cannot run remote SSH")
    executable = os.path.abspath(executable)
    if not os.path.isfile(executable) or not os.access(executable, os.X_OK):
        raise RemoteSSHError("TaskDeck SSH profile bridge executable is unavailable")
    payload = {
        "profile_name": name,
        "argv": argv,
        "timeout_seconds": timeout,
        "dry_run": bool(dry_run),
    }
    try:
        bridge = [executable, "patch-ssh-command", "--workspace", str(root.resolve(strict=True))]
        serialized = json.dumps(payload, ensure_ascii=False)
        if output_callback is not None and not dry_run:
            result = _run_streamed_bridge(bridge, serialized, root, timeout, output_callback)
        else:
            result = subprocess.run(
                bridge, input=serialized, cwd=root, capture_output=True,
                text=True, timeout=timeout + 20, check=False,
            )
    except subprocess.TimeoutExpired as exc:
        raise RemoteSSHError("TaskDeck SSH command bridge timed out") from exc
    except OSError as exc:
        raise RemoteSSHError(f"TaskDeck SSH bridge could not start: {type(exc).__name__}") from exc
    report = {
        "profile_name": name,
        "argv": list(argv),
        "dry_run": dry_run,
        "rc": result.returncode,
        "status": "PASS" if result.returncode == 0 else "FAIL",
        "stdout": result.stdout[-65536:],
        "stderr": result.stderr[-65536:],
    }
    if dry_run and result.returncode:
        detail = result.stderr.strip() or result.stdout.strip() or "SSH remote profile/command rejected"
        raise RemoteSSHError(detail[:1000])
    return report
