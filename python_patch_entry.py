#!/usr/bin/env python3
"""Canonical public entrypoint for Python Patch Tool.

All launchers and TaskDeck call this router. Business logic remains in _patch_lib;
this file only preserves the historical public command routing contract.
"""
from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import Iterable, Sequence

MIN_PYTHON = (3, 10)
DISPATCH_COMMANDS = {"report", "run", "resume", "plan"}
AUTOMATION_FLAGS = {
    "--all",
    "-a",
    "--select",
    "--zip-failed",
    "--keep-failed-zip",
    "--move",
    "-y",
}
UTILITY_COMMANDS = {"paths", "health-search", "help", "--help", "-h", "version", "--version"}
PATCH_FILE_SUFFIXES = (".zip", ".py", ".tar.gz", ".tgz")


class EntryError(RuntimeError):
    pass


def _paths(base_dir: Path) -> dict[str, Path]:
    lib_dir = base_dir / "_patch_lib"
    return {
        "lib": lib_dir,
        "runner": lib_dir / "python_patch_runner.py",
        "collect_compat": lib_dir / "python_patch_collect_compat.py",
        "dispatcher": lib_dir / "python_patch_queue_dispatcher.py",
        "collect_progress": lib_dir / "python_patch_collect_progress_v6_7.py",
        "collect_regex_worker": lib_dir / "python_patch_collect_regex_worker.py",
    }


def _strip_legacy_transaction(args: Sequence[str]) -> tuple[list[str], bool]:
    filtered: list[str] = []
    skip_value = False
    stripped = False
    for raw in args:
        lower = raw.lower()
        if skip_value:
            skip_value = False
            if lower in {"off", "auto", "required"}:
                continue
        if lower == "--transaction":
            stripped = True
            skip_value = True
            continue
        if lower.startswith("--transaction="):
            stripped = True
            continue
        if lower == "--keep-failed-sandbox" or lower.startswith("--keep-failed-sandbox="):
            stripped = True
            continue
        filtered.append(raw)
    return filtered, stripped


def build_exec_argv(project_root: str, tool_args: Sequence[str], base_dir: Path | None = None) -> tuple[list[str], list[Path]]:
    """Return child argv and files that must exist.

    The returned argv excludes the Python executable itself. This function is pure
    enough for routing contract tests and intentionally contains no Patch Tool
    business logic.
    """
    base = (base_dir or Path(__file__).resolve().parent).resolve()
    paths = _paths(base)
    args = list(tool_args)

    if not args:
        return [str(paths["dispatcher"]), "--project-root", project_root], [paths["dispatcher"]]

    first = args[0]
    first_lower = first.lower()
    if first_lower in DISPATCH_COMMANDS:
        return [
            str(paths["dispatcher"]),
            "--project-root",
            project_root,
            first_lower,
            *args[1:],
        ], [paths["dispatcher"]]

    if first_lower == "collect":
        return [
            str(paths["collect_progress"]),
            "--project-root",
            project_root,
            "--collector",
            str(paths["collect_compat"]),
            "--",
            *args[1:],
        ], [paths["collect_compat"], paths["collect_progress"], paths["collect_regex_worker"]]

    patch_arg_count = sum(1 for arg in args if arg.lower() == "--patch")
    automation_route = patch_arg_count > 1 or any(arg.lower() in AUTOMATION_FLAGS for arg in args)
    if automation_route:
        filtered, _ = _strip_legacy_transaction(args)
        return [
            str(paths["dispatcher"]),
            "--project-root",
            project_root,
            "run",
            *filtered,
        ], [paths["dispatcher"]]

    filtered, stripped = _strip_legacy_transaction(args)
    force_inplace = False
    for arg in filtered:
        lower = arg.lower()
        if lower in {"--patch", "--all", "--select"} or lower.endswith(PATCH_FILE_SUFFIXES):
            force_inplace = True
            break

    if not force_inplace and filtered:
        if filtered[0].lower() not in UTILITY_COMMANDS:
            force_inplace = True

    if force_inplace:
        return [str(paths["runner"]), *filtered, "--transaction", "off"], [paths["runner"]]

    if stripped and not filtered:
        raise EntryError(
            "obsolete transaction/SANDBOX flags cannot be used as a standalone command.\n"
            "Use run_python_patches with no arguments for the normal queue."
        )

    return [str(paths["runner"]), *filtered], [paths["runner"]]


def parse_entry_args(argv: Sequence[str]) -> tuple[str, list[str]]:
    args = list(argv)
    project_root: str | None = None

    if args and args[0] == "--project-root":
        if len(args) < 2:
            raise EntryError("--project-root requires a directory")
        project_root = args[1]
        args = args[2:]
    elif args and args[0].startswith("--project-root="):
        project_root = args[0].split("=", 1)[1]
        args = args[1:]

    if args and args[0] == "--":
        args = args[1:]

    if project_root is None:
        # Compatibility default for the historical portable layout:
        # <project>/tools/python_patch_entry.py
        project_root = str(Path(__file__).resolve().parent.parent)

    return str(Path(project_root).expanduser().resolve()), args


def _prepare_environment(base_dir: Path) -> None:
    lib_dir = str((base_dir / "_patch_lib").resolve())
    current = os.environ.get("PYTHONPATH", "")
    os.environ["PYTHONPATH"] = lib_dir if not current else lib_dir + os.pathsep + current
    os.environ["PYTHONDONTWRITEBYTECODE"] = "1"


def main(argv: Sequence[str] | None = None) -> int:
    if sys.version_info < MIN_PYTHON:
        print(
            f"ERROR: Python {MIN_PYTHON[0]}.{MIN_PYTHON[1]}+ is required. "
            f"Current Python is {sys.version_info.major}.{sys.version_info.minor}.",
            file=sys.stderr,
        )
        return 2

    base_dir = Path(__file__).resolve().parent
    _prepare_environment(base_dir)
    try:
        project_root, tool_args = parse_entry_args(sys.argv[1:] if argv is None else argv)
        child_argv, required = build_exec_argv(project_root, tool_args, base_dir)
        for path in required:
            if not path.is_file():
                raise EntryError(f"Missing Patch Tool runtime file: {path}")
    except EntryError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2

    os.execv(sys.executable, [sys.executable, *child_argv])
    return 127


if __name__ == "__main__":
    raise SystemExit(main())
