#!/usr/bin/env python3
"""Reject a PATCH that invokes a script it created during this same PATCH.

A script referenced by an automatic command must already exist when the PATCH
starts. This is a provenance check for direct script execution, not a sandbox
for arbitrary Python payload code or opaque build tools.
"""
from __future__ import annotations

import os
from pathlib import Path
import re


_SCRIPT_INTERPRETERS = {
    "bash", "sh", "python", "python3", "node", "pwsh", "powershell",
    "powershell.exe",
}
_PYTHON_VERSIONED = re.compile(r"^python3\.\d+(?:\.exe)?$", re.IGNORECASE)
_NODE_VERSIONED = re.compile(r"^node(?:\.exe)?$", re.IGNORECASE)
_INLINE_FLAGS = {"-c", "-m", "-e", "--eval", "--evaluate", "-command",
                 "-encodedcommand", "-enc"}
_TAKES_VALUE = {"-o", "-W", "-X", "--require", "--loader", "--import",
                "--input-type", "-executionpolicy", "-ep"}


class NewScriptExecutionError(RuntimeError):
    kind = "new_script_execution_forbidden"


def _basename(value: str) -> str:
    return value.replace("\\", "/").rsplit("/", 1)[-1].lower()


def _unwrap_env(argv: list[str]) -> list[str]:
    args = argv
    for _ in range(3):
        if not args or _basename(args[0]) not in {"env", "env.exe"}:
            return args
        offset = 1
        while offset < len(args):
            token = args[offset]
            if token == "--":
                offset += 1
                break
            if token in {"-i", "--ignore-environment"} or (
                "=" in token and not token.startswith("-")
            ):
                offset += 1
                continue
            if token.startswith("-"):
                # Do not misidentify an option argument as an executable.
                return []
            break
        args = args[offset:]
    return args


def command_script_path(argv: list[str], cwd: Path) -> Path | None:
    """Return a directly invoked script path, if one can be identified."""
    args = _unwrap_env(argv)
    if not args:
        return None
    executable = args[0]
    name = _basename(executable)
    is_interpreter = (
        name in _SCRIPT_INTERPRETERS
        or bool(_PYTHON_VERSIONED.fullmatch(name))
        or bool(_NODE_VERSIONED.fullmatch(name))
    )
    if is_interpreter:
        i = 1
        while i < len(args):
            token = args[i]
            lower = token.lower()
            if lower in _INLINE_FLAGS:
                return None
            if lower in {"-file", "-f"} and name in {"pwsh", "powershell", "powershell.exe"}:
                i += 1
                break
            if token == "--":
                i += 1
                break
            if lower in _TAKES_VALUE:
                i += 2
                continue
            if token.startswith("-"):
                i += 1
                continue
            break
        if i >= len(args):
            return None
        script = args[i]
        if script in {"-", "--"} or script.startswith("-"):
            return None
    elif "/" in executable or "\\" in executable:
        script = executable
    else:
        return None
    # Keep the spelling and path identity stable across the PATCH; avoid
    # following a symlink when recording a non-existent path.
    path = Path(script)
    return Path(os.path.abspath(path if path.is_absolute() else cwd / path))


class ScriptOriginGuard:
    """Snapshot referenced script existence before the PATCH payload runs."""

    def __init__(self, root: Path, commands: list[dict]):
        self.root = root
        self.before: dict[Path, bool] = {}
        for command in commands:
            argv = command.get("argv")
            if not isinstance(argv, list) or not argv or not all(
                isinstance(arg, str) and arg for arg in argv
            ):
                continue
            cwd_raw = command.get("cwd") or "."
            cwd = Path(os.path.abspath(root / cwd_raw))
            path = command_script_path(argv, cwd)
            if path is not None:
                self.before[path] = path.is_file()

    def missing_paths(self) -> list[Path]:
        """Direct command script inputs that did not exist at preflight."""
        return sorted((path for path, exists in self.before.items() if not exists),
                      key=str)

    def check(self, argv: list[str], cwd: Path) -> None:
        path = command_script_path(argv, cwd)
        if path is None:
            return
        if path not in self.before:
            raise NewScriptExecutionError(
                "new_script_execution_forbidden: command script was not "
                f"verified before this PATCH: {path}"
            )
        if not self.before[path] and path.is_file():
            raise NewScriptExecutionError(
                "new_script_execution_forbidden: this PATCH created the script "
                f"it is attempting to execute: {path}"
            )
