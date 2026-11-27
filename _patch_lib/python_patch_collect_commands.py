#!/usr/bin/env python3
"""Bounded, whitelist-only COLLECT command evidence (local or saved SSH profile)."""
from __future__ import annotations
import os
import re
import json
import hashlib
import stat
import shutil
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


# Virtual command, not an executable supplied by the model. The only
# external receipt access is through the owner-pinned Nim gateway.
NIM_READ_RUN = frozenset({"nim-build-status", "nim-build-log", "nim-build-result", "nim-artifacts"})
NIM_READ_NOARG = frozenset({"nim-profiles", "nim-doctor", "nim-source-compare"})
NIM_PROFILES = frozenset({"debug", "release", "server", "server-probe", "server-verbose", "battle", "pvp", "arenaserver", "arenaserver-fake-data", "local", "release-copy"})


def _validate_nim_read_action(action: dict) -> None:
    argv = action["argv"]
    if action.get("transport", "local") != "local" or action.get("cwd", ".") != "." or action.get("profile_name") is not None:
        raise CollectCommandError("Nim gateway receipt access requires local project root")
    args = argv[1:]
    if len(args) == 2 and args[0] in NIM_READ_RUN and re.fullmatch(r"[0-9a-f]{32}", args[1]):
        return
    if len(args) == 1 and args[0] in NIM_READ_NOARG:
        return
    if len(args) == 2 and args[0] == "nim-profile-info" and args[1] in NIM_PROFILES:
        return
    raise CollectCommandError("Nim gateway COLLECT permits only read-only actions; receipts require exact 32-hex RUN_ID")


def _nim_read_argv(root: Path, action: dict) -> list[str]:
    _validate_nim_read_action(action)
    if os.environ.get("TASKDECK_NIM_GATEWAY_LOCAL") != "1":
        raise CollectCommandError("Nim gateway is not enabled for this local native worker")
    try:
        root = root.resolve(strict=True)
        cfg = Path.home() / ".config/taskdeck/nim-gateway.json"
        st = cfg.lstat()
        if not stat.S_ISREG(st.st_mode) or st.st_uid != os.getuid() or st.st_mode & 0o077 or st.st_size > 8192:
            raise ValueError("unsafe owner registration")
        g = json.loads(cfg.read_bytes())
        identity_path = root / ".python_patch_tool.json"
        st = identity_path.lstat()
        if not stat.S_ISREG(st.st_mode) or st.st_size > 1024 * 1024:
            raise ValueError("invalid project identity")
        project = json.loads(identity_path.read_bytes())["project"]
        rel = g["script"]
        sha = g["sha256"]
        if (g["project_root"] != str(root) or not isinstance(g["project_id"], str)
                or not g["project_id"] or g["project_id"] != project["id"]
                or not isinstance(rel, str) or not re.fullmatch(r"[A-Za-z0-9_./-]+", rel)
                or rel.startswith(("/", "-")) or ".." in Path(rel).parts
                or Path(rel).as_posix() != rel or Path(rel).name != "nim-agent.sh"
                or not isinstance(sha, str) or not re.fullmatch(r"[0-9a-f]{64}", sha)):
            raise ValueError("registration does not match project")
        script = root / rel
        st = script.lstat()
        if (not stat.S_ISREG(st.st_mode) or st.st_size > 1024 * 1024
                or script.resolve(strict=True) != script or hashlib.sha256(script.read_bytes()).hexdigest() != sha):
            raise ValueError("gateway digest or path changed")
        bash = shutil.which("bash")
        if not bash:
            raise ValueError("bash unavailable")
        return [bash, str(script), *action["argv"][1:]]
    except (OSError, ValueError, TypeError, KeyError, AttributeError) as exc:
        raise CollectCommandError("Nim gateway owner registration or script digest invalid") from exc


def validate_command_action(action: dict, label: str = "COLLECT command") -> None:
    argv = action.get("argv")
    if not isinstance(argv, list) or not argv or len(argv) > 24 or any(
        not isinstance(x, str) or not x or len(x) > 512 for x in argv
    ):
        raise CollectCommandError(f"{label}.argv must contain 1..24 bounded literal string tokens")
    if argv[0] == "nim-gateway":
        _validate_nim_read_action(action)
    elif argv[0] not in READONLY:
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



_COLLECT_GLOB = re.compile(r"[A-Za-z0-9_./*? -]{1,128}\Z")


def _validate_collect_basic_args(cwd: Path, argv: list[str]) -> None:
    """Conservative argv grammar for COLLECT's four traversal commands.

    PATCH's historical legacy_strict compatibility still accepts additional
    read-only navigation options. COLLECT has no such compatibility promise:
    enforce a bounded Go-SSH-like grammar locally as well.
    """
    from python_patch_package_schema import _strict_relative_file_arg

    name, args = argv[0], argv[1:]

    def deny(reason: str) -> None:
        raise CollectCommandError(f"COLLECT {name}: {reason}")

    def path(value: str) -> None:
        try:
            _strict_relative_file_arg(cwd, value, label=f"COLLECT {name}")
        except PatchSchemaError as exc:
            deny(str(exc))

    if name == "pwd":
        if args:
            deny("pwd takes no operands")
        return

    if name == "ls":
        file_seen = False
        for token in args:
            if token.startswith("-"):
                if file_seen or token.startswith("--") or not token[1:] or not set(token[1:]) <= set("alh1"):
                    deny("ls accepts only -a/-l/-h/-1 and one relative path")
            else:
                if file_seen:
                    deny("ls accepts at most one relative path")
                path(token)
                file_seen = True
        return

    if name == "tree":
        file_seen = False
        depth_seen = False
        i = 0
        while i < len(args):
            token = args[i]
            if token in ("-a", "-d") and not file_seen:
                i += 1
            elif token == "-L" and not file_seen and not depth_seen:
                if i + 1 >= len(args) or args[i + 1] not in {"1", "2", "3"}:
                    deny("tree -L requires depth 1..3")
                depth_seen = True
                i += 2
            elif not token.startswith("-") and not file_seen:
                path(token)
                file_seen = True
                i += 1
            else:
                deny("tree allows -a/-d/-L 1..3 and one relative path only")
        return

    if name == "find":
        i = 0
        if i < len(args) and not args[i].startswith("-"):
            path(args[i])
            i += 1
        if i + 1 >= len(args) or args[i] != "-maxdepth" or args[i + 1] not in {"1", "2", "3"}:
            deny("find requires -maxdepth 1..3 (before filters)")
        max_depth = int(args[i + 1])
        i += 2
        seen: set[str] = set()
        while i < len(args):
            token = args[i]
            if token == "-type":
                if token in seen or i + 1 >= len(args) or args[i + 1] not in {"f", "d"}:
                    deny("find -type accepts only f or d once")
                seen.add(token)
                i += 2
            elif token in {"-name", "-iname"}:
                if "name" in seen or i + 1 >= len(args) or not _COLLECT_GLOB.fullmatch(args[i + 1]) or ".." in args[i + 1] or args[i + 1].startswith("-"):
                    deny("find -name/-iname requires one safe file glob")
                seen.add("name")
                i += 2
            elif token == "-mindepth":
                if token in seen or i + 1 >= len(args) or not args[i + 1].isdigit() or not 0 <= int(args[i + 1]) <= max_depth:
                    deny("find -mindepth must be between 0 and maxdepth")
                seen.add(token)
                i += 2
            elif token in {"-print", "-print0"}:
                i += 1
            else:
                # No command execution, file-writing actions, broad expressions
                # or hidden options such as -exec, -delete, -fprint, -D, -L.
                deny(f"find option/action outside read-only whitelist: {token}")
        return

    deny("unsupported basic command")


def collect_command_log_path(root: Path, index: int) -> tuple[Path | None, str]:
    """Project-scoped console evidence owned by the active TaskDeck run."""
    raw = os.environ.get("PTV_COLLECT_COMMAND_LOG_DIR", "").strip()
    if not raw or not 1 <= index <= 64:
        return None, ""
    try:
        candidate = Path(raw).resolve(strict=False)
        rel = candidate.relative_to(root.resolve(strict=True))
        parts = rel.parts
        if (len(parts) < 7 or parts[:3] != ("artifacts", "patch_tool", "runs")
                or parts[-3] != "command_items" or parts[-1] != "command_logs"):
            return None, ""
        candidate.mkdir(parents=True, exist_ok=True)
        dest = candidate / f"{index:03d}_collect_command.log"
        return dest, dest.relative_to(root).as_posix()
    except (OSError, ValueError):
        return None, ""


def _capture_local(root: Path, action: dict, *, log_path: Path | None = None) -> dict:
    from python_patch_package_schema import resolve_project_path
    argv = action["argv"]
    cwd_raw = action.get("cwd", ".")
    cwd = root if cwd_raw == "." else resolve_project_path(root, cwd_raw)
    if not cwd.is_dir():
        raise CollectCommandError("local command cwd is not a project directory")
    if argv[0] == "nim-gateway":
        argv = _nim_read_argv(root, action)
    else:
        if argv[0] in {"pwd", "ls", "tree", "find"}:
            _validate_collect_basic_args(cwd, argv)
        try:
            _check_legacy_strict_command(cwd, {"argv": argv, "cwd": "."}, 0)
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
    # Limit the environment passed to COLLECT to public command inputs.
    # Internal handoff channels must not be passed to a file-search process.
    safe_env = dict(os.environ)
    for key in ("PTV_PATCH_RESULT_FILE", "PTV_COLLECT_RESULT_FILE",
                "PTV_PATCH_COMMAND_LOG_DIR", "PTV_PARENT_MUTATION_LOCK_TOKEN",
                "PTV_PARENT_MUTATION_LOCK_KEY", "TASKDECK_NIM_GATEWAY_LOCAL"):
        safe_env.pop(key, None)
    kwargs["env"] = safe_env
    log_stream = None
    if log_path is not None:
        log_path.parent.mkdir(parents=True, exist_ok=True)
        log_stream = log_path.open("wb")
        try:
            os.chmod(log_path, 0o600)
        except OSError:
            pass
    try:
        proc = subprocess.Popen(argv, **kwargs)
    except BaseException:
        if log_stream is not None:
            log_stream.close()
        raise
    def pump() -> None:
        assert proc.stdout is not None
        while True:
            chunk = os.read(proc.stdout.fileno(), 4096)
            if not chunk:
                break
            total[0] += len(chunk)
            remaining = max(0, cap - len(data))
            if remaining:
                part = chunk[:remaining]
                data.extend(part)
                if log_stream is not None:
                    log_stream.write(part)
                    log_stream.flush()  # View console while COLLECT is RUNNING
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
        if log_stream is not None:
            log_stream.close()
    return {"rc": 124 if timed_out else int(proc.returncode or 0),
            "timed_out": timed_out, "truncated": total[0] > cap,
            "elapsed_seconds": round(time.monotonic() - started, 3),
            "output": bytes(data).decode("utf-8", errors="replace")}


def execute_collect_command(root: Path, action: dict, *, log_path: Path | None = None) -> dict:
    validate_command_action(action)
    argv = list(action["argv"])
    transport = action.get("transport", "local")
    cap = int(action.get("max_output_bytes", MAX_OUTPUT))
    if transport == "ssh":
        log_stream = None
        captured = [0]
        remote_output_lock = threading.Lock()
        if log_path is not None:
            log_path.parent.mkdir(parents=True, exist_ok=True)
            log_stream = log_path.open("wb")
            try:
                os.chmod(log_path, 0o600)
            except OSError:
                pass
        def on_remote_output(channel: str, chunk: bytes) -> None:
            if log_stream is None:
                return
            with remote_output_lock:
                remaining = max(0, cap - captured[0])
                if remaining:
                    piece = chunk[:remaining]
                    log_stream.write(piece)
                    log_stream.flush()
                    captured[0] += len(piece)
        try:
            try:
                result = run_remote_ssh(root, {
                    "profile_name": action["profile_name"],
                    "argv": argv,
                    "timeout_seconds": action.get("timeout_seconds", 30)
                }, dry_run=False, output_callback=on_remote_output)
            except RemoteSSHError as exc:
                raise CollectCommandError(f"SSH command could not run: {exc}") from exc
        finally:
            if log_stream is not None:
                log_stream.close()
        content = (str(result.get("stdout") or "") + str(result.get("stderr") or "")).encode("utf-8", errors="replace")
        if log_path is not None and captured[0] == 0:
            log_path.write_bytes(content[:cap])
        return {
            "transport": transport, "profile_name": action["profile_name"],
            "argv": argv, "rc": int(result.get("rc") or 0),
            "timed_out": False, "truncated": len(content) > cap,
            "output": content[:cap].decode("utf-8", errors="replace"),
        }
    return {"transport": transport, "argv": argv,
            **_capture_local(root, action, log_path=log_path)}


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
