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


class RemoteSSHError(ValueError):
    kind = "remote_ssh_rejected"


def run_remote_ssh(root: Path, remote: dict, *, dry_run: bool) -> dict[str, object]:
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
        result = subprocess.run(
            [executable, "patch-ssh-command", "--workspace", str(root.resolve(strict=True))],
            input=json.dumps(payload, ensure_ascii=False),
            cwd=root, capture_output=True, text=True,
            stdin=None, timeout=timeout + 20, check=False,
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
