#!/usr/bin/env python3
"""Canonical public entrypoint for Python Patch Tool.

All launchers and TaskDeck call this router. Business logic remains in _patch_lib;
this file only preserves the historical public command routing contract.
"""
from __future__ import annotations

import os
import signal
import subprocess
import sys
import threading
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
EVENT_FD_ENV = "TASKDECK_PATCH_EVENT_FD"
COMMAND_FD_ENV = "TASKDECK_PATCH_COMMAND_FD"


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
    if lib_dir not in sys.path:
        sys.path.insert(0, lib_dir)
    os.environ["PYTHONDONTWRITEBYTECODE"] = "1"


def classify_route(tool_args: Sequence[str]) -> str:
    if not tool_args:
        return "queue"
    first = tool_args[0].lower()
    if first in DISPATCH_COMMANDS:
        return first
    if first == "collect":
        return "collect"
    patch_arg_count = sum(1 for arg in tool_args if arg.lower() == "--patch")
    if patch_arg_count > 1 or any(arg.lower() in AUTOMATION_FLAGS for arg in tool_args):
        return "run"
    if first in UTILITY_COMMANDS:
        return "utility"
    return "direct"


def _protocol_writer_from_env():
    raw = os.environ.get(EVENT_FD_ENV, "").strip()
    if not raw:
        return None
    try:
        fd = int(raw, 10)
    except ValueError as exc:
        raise EntryError(f"{EVENT_FD_ENV} must be an integer file descriptor") from exc
    if fd < 3:
        raise EntryError(f"{EVENT_FD_ENV} must be >= 3 so protocol data never shares stdin/stdout/stderr")
    if os.name == "nt":
        raise EntryError("Patch protocol FD transport is not available on Windows yet")
    try:
        from python_patch_protocol import EventWriter
        return EventWriter(fd)
    except (ImportError, OSError, ValueError) as exc:
        raise EntryError(f"cannot open Patch protocol event channel fd={fd}: {exc}") from exc


def _command_fd_from_env(event_fd: int | None) -> int | None:
    raw = os.environ.get(COMMAND_FD_ENV, "").strip()
    if not raw:
        return None
    try:
        fd = int(raw, 10)
    except ValueError as exc:
        raise EntryError(f"{COMMAND_FD_ENV} must be an integer file descriptor") from exc
    if fd < 3:
        raise EntryError(f"{COMMAND_FD_ENV} must be >= 3 so commands never share stdin/stdout/stderr")
    if os.name == "nt":
        raise EntryError("Patch protocol command FD transport is not available on Windows yet")
    if event_fd is None:
        raise EntryError(f"{COMMAND_FD_ENV} requires {EVENT_FD_ENV}")
    if fd == event_fd:
        raise EntryError("Patch protocol event and command file descriptors must be different")
    try:
        os.fstat(fd)
    except OSError as exc:
        raise EntryError(f"cannot access Patch protocol command channel fd={fd}: {exc}") from exc
    return fd


def _normalized_return_code(return_code: int) -> int:
    return return_code if return_code >= 0 else 128 + (-return_code)


def _run_with_protocol(
    writer,
    child_argv: Sequence[str],
    project_root: str,
    tool_args: Sequence[str],
    command_fd: int | None = None,
) -> int:
    from python_patch_protocol import relay_event_fd

    route = classify_route(tool_args)
    capabilities = ["events_v1", "queue_snapshot_v1", "child_event_relay_v1", "progress_v1"]
    if command_fd is not None:
        capabilities.append("commands_v1")
    writer.emit(
        "hello",
        project_root=project_root,
        route=route,
        capabilities=capabilities,
    )
    if route in {"queue", "run", "resume", "plan"}:
        try:
            from python_patch_protocol import emit_queue_snapshot
            emit_queue_snapshot(writer, project_root)
        except Exception as exc:
            writer.emit("error", phase="queue_snapshot", message=f"{type(exc).__name__}: {exc}")
    writer.emit("run_started", route=route)

    child_event_read, child_event_write = os.pipe()
    child_env = os.environ.copy()
    child_env[EVENT_FD_ENV] = str(child_event_write)
    pass_fds = [child_event_write]
    if command_fd is not None:
        pass_fds.append(command_fd)

    relay_thread = None
    try:
        proc = subprocess.Popen(
            [sys.executable, *child_argv],
            pass_fds=tuple(pass_fds),
            env=child_env,
        )
        os.close(child_event_write)
        child_event_write = -1
        relay_thread = threading.Thread(
            target=relay_event_fd,
            args=(child_event_read, writer),
            name="taskdeck-patch-event-relay",
            daemon=True,
        )
        relay_thread.start()
        child_event_read = -1
    except OSError as exc:
        if child_event_read >= 0:
            os.close(child_event_read)
        if child_event_write >= 0:
            os.close(child_event_write)
        writer.emit("error", phase="spawn", message=str(exc))
        writer.emit("run_finished", route=route, status="failed", exit_code=2)
        writer.close()
        return 2

    previous_handlers: dict[int, object] = {}
    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            previous_handlers[int(sig)] = signal.getsignal(sig)
            signal.signal(sig, signal.SIG_IGN)
        except (OSError, RuntimeError, ValueError):
            pass
    try:
        return_code = proc.wait()
    finally:
        for sig_value, handler in previous_handlers.items():
            try:
                signal.signal(sig_value, handler)
            except (OSError, RuntimeError, ValueError):
                pass

    if relay_thread is not None:
        relay_thread.join(timeout=2.0)
        if relay_thread.is_alive():
            writer.emit("error", phase="child_event_relay", message="child event relay did not close after process exit")

    exit_code = _normalized_return_code(return_code)
    writer.emit(
        "run_finished",
        route=route,
        status="success" if exit_code == 0 else "failed",
        exit_code=exit_code,
    )
    writer.close()
    return exit_code


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
    writer = None
    command_fd = None
    try:
        writer = _protocol_writer_from_env()
        command_fd = _command_fd_from_env(writer.fd if writer is not None else None)
        project_root, tool_args = parse_entry_args(sys.argv[1:] if argv is None else argv)
        child_argv, required = build_exec_argv(project_root, tool_args, base_dir)
        for path in required:
            if not path.is_file():
                raise EntryError(f"Missing Patch Tool runtime file: {path}")
    except EntryError as exc:
        if writer is not None:
            writer.emit("error", phase="entry", message=str(exc))
            writer.close()
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2

    if writer is not None:
        return _run_with_protocol(writer, child_argv, project_root, tool_args, command_fd)

    # Critical compatibility invariant: without a machine event channel the
    # historical terminal path remains an exec, not a supervising subprocess.
    os.execv(sys.executable, [sys.executable, *child_argv])
    return 127


if __name__ == "__main__":
    raise SystemExit(main())
