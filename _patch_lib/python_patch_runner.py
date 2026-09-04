#!/usr/bin/env python3
from __future__ import annotations

import argparse
import contextlib
import difflib
import hashlib
import json
import os
import re
from pathlib import Path, PurePosixPath
import shutil
import signal
import stat
import subprocess
import sys
import tarfile
import tempfile
import zipfile
import time
import unicodedata
from datetime import datetime, timezone
from dataclasses import dataclass

from python_patch_utils import PatchFailure, diagnose_ops, finish_failure, run_ops
from python_patch_package_schema import PatchSchemaError, path_is_link_or_reparse, resolve_project_path, run_preflight, sha256_file

VERSION = "6.18.7"
_ACTIVE_TERMINATION_SIGNAL: int | None = None
MAX_ARCHIVE_ENTRIES = 10000
MAX_ARCHIVE_MEMBER_BYTES = 64 * 1024 * 1024
MAX_ARCHIVE_TOTAL_BYTES = 256 * 1024 * 1024
MAX_ARCHIVE_COMPRESSION_RATIO = 200


@dataclass(frozen=True)
class ManagedProcessResult:
    rc: int
    timed_out: bool = False
    lingering_descendants: bool = False

    @property
    def normalized_rc(self) -> int:
        value = int(self.rc)
        return 128 + abs(value) if value < 0 else value

    @property
    def effective_rc(self) -> int:
        rc = self.normalized_rc
        return 125 if self.lingering_descendants and rc == 0 else rc


def _sigterm_as_interrupt(signum, _frame):
    global _ACTIVE_TERMINATION_SIGNAL
    _ACTIVE_TERMINATION_SIGNAL = int(signum)
    raise KeyboardInterrupt


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def _safe_archive_name(name: str) -> PurePosixPath:
    if "\\" in name:
        raise ValueError(f"archive member must use POSIX '/' separators: {name}")
    text = name
    rel = PurePosixPath(text)
    if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts):
        raise ValueError(f"unsafe archive member: {name}")
    if any(":" in part for part in rel.parts):
        raise ValueError(f"Windows drive/ADS syntax is not allowed in archive member: {name}")
    return rel


def _archive_collision_key(rel: PurePosixPath) -> str:
    return unicodedata.normalize("NFC", rel.as_posix()).casefold()


def _safe_extract_zip(path: Path, dest: Path) -> None:
    with zipfile.ZipFile(path) as zf:
        infos = zf.infolist()
        if len(infos) > MAX_ARCHIVE_ENTRIES:
            raise ValueError(f"archive has too many entries ({len(infos)} > {MAX_ARCHIVE_ENTRIES})")
        total = 0
        seen: set[str] = set()
        for info in infos:
            if info.is_dir():
                continue
            rel = _safe_archive_name(info.filename)
            key = _archive_collision_key(rel)
            if key in seen:
                raise ValueError(f"duplicate/case-fold/Unicode-colliding archive member: {info.filename}")
            seen.add(key)
            if info.file_size > MAX_ARCHIVE_MEMBER_BYTES:
                raise ValueError(f"archive member too large: {info.filename} ({info.file_size})")
            total += info.file_size
            if total > MAX_ARCHIVE_TOTAL_BYTES:
                raise ValueError("archive expanded size exceeds tool limit")
            if info.file_size > 1024 * 1024 and info.compress_size == 0:
                raise ValueError(f"archive member has impossible compression size: {info.filename}")
            if info.compress_size > 0 and info.file_size / info.compress_size > MAX_ARCHIVE_COMPRESSION_RATIO:
                raise ValueError(f"archive member compression ratio is too high: {info.filename}")
            mode = (info.external_attr >> 16) & 0xFFFF
            if stat.S_ISLNK(mode):
                raise ValueError(f"archive symlink is not allowed: {info.filename}")
            target = dest.joinpath(*rel.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            copied = 0
            with zf.open(info) as src, target.open("wb") as out:
                for chunk in iter(lambda: src.read(1024 * 1024), b""):
                    copied += len(chunk)
                    if copied > info.file_size or copied > MAX_ARCHIVE_MEMBER_BYTES:
                        raise ValueError(f"archive member exceeded declared/safe size while extracting: {info.filename}")
                    out.write(chunk)
            if copied != info.file_size:
                raise ValueError(f"archive member size changed while extracting: {info.filename}")


def _safe_extract_tar(path: Path, dest: Path) -> None:
    with tarfile.open(path, "r:*") as tf:
        members = tf.getmembers()
        if len(members) > MAX_ARCHIVE_ENTRIES:
            raise ValueError(f"archive has too many entries ({len(members)} > {MAX_ARCHIVE_ENTRIES})")
        total = 0
        seen: set[str] = set()
        for member in members:
            if member.isdir():
                continue
            rel = _safe_archive_name(member.name)
            key = _archive_collision_key(rel)
            if key in seen:
                raise ValueError(f"duplicate/case-fold/Unicode-colliding archive member: {member.name}")
            seen.add(key)
            if not member.isfile():
                raise ValueError(f"archive non-regular member is not allowed: {member.name}")
            if member.size > MAX_ARCHIVE_MEMBER_BYTES:
                raise ValueError(f"archive member too large: {member.name} ({member.size})")
            total += member.size
            if total > MAX_ARCHIVE_TOTAL_BYTES:
                raise ValueError("archive expanded size exceeds tool limit")
            src = tf.extractfile(member)
            if src is None:
                raise ValueError(f"cannot read archive member: {member.name}")
            target = dest.joinpath(*rel.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            copied = 0
            with src, target.open("wb") as out:
                for chunk in iter(lambda: src.read(1024 * 1024), b""):
                    copied += len(chunk)
                    if copied > member.size or copied > MAX_ARCHIVE_MEMBER_BYTES:
                        raise ValueError(f"archive member exceeded declared/safe size while extracting: {member.name}")
                    out.write(chunk)
            if copied != member.size:
                raise ValueError(f"archive member size changed while extracting: {member.name}")


def _reject_duplicate_json_pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError(f"duplicate JSON key: {key}")
        out[key] = value
    return out


def _read_json(path: Path, label: str) -> dict:
    try:
        data = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
    except Exception as exc:
        raise ValueError(f"invalid {label}: {type(exc).__name__}: {exc}") from exc
    if not isinstance(data, dict):
        raise ValueError(f"{label} root must be an object")
    return data


def _python_payload_files(extracted: Path) -> list[Path]:
    rows: list[Path] = []
    for path in extracted.rglob("*.py"):
        rel = path.relative_to(extracted)
        if not path.is_file() or "__MACOSX" in rel.parts or "resources" in rel.parts:
            continue
        if path.name == ".ptv_legacy_multi_entry.py":
            continue
        rows.append(path)
    return sorted(rows, key=lambda x: x.relative_to(extracted).as_posix().lower())


def _legacy_python_candidates(extracted: Path, archive_name: str) -> list[Path]:
    py_files = _python_payload_files(extracted)
    named = [p for p in py_files if p.name.lower().startswith("patch_")]
    if named:
        return named
    marked: list[Path] = []
    for path in py_files:
        try:
            text = path.read_text(encoding="utf-8", errors="replace")[:262144]
        except OSError:
            continue
        if any(marker in text for marker in ("python_patch_utils", "run_patch", "PATCH_NAME")):
            marked.append(path)
    if marked:
        return marked
    lower = archive_name.lower()
    if lower.startswith("patch_") and not any(token in lower for token in ("handoff", "report", "python_patch_tool")):
        return py_files
    return []


def _legacy_multi_wrapper(extracted: Path, scripts: list[Path]) -> Path:
    wrapper = extracted / ".ptv_legacy_multi_entry.py"
    rels = [p.relative_to(extracted).as_posix() for p in scripts]
    wrapper.write_text(
        "import runpy, sys\n"
        "from pathlib import Path\n"
        "_base = Path(__file__).resolve().parent\n"
        f"_scripts = {rels!r}\n"
        "for _rel in _scripts:\n"
        "    _path = _base / _rel\n"
        "    _old = list(sys.argv)\n"
        "    try:\n"
        "        sys.argv = [str(_path), *_old[1:]]\n"
        "        try:\n"
        "            runpy.run_path(str(_path), run_name='__main__')\n"
        "        except SystemExit as _exc:\n"
        "            _code = _exc.code if isinstance(_exc.code, int) else (0 if _exc.code is None else 1)\n"
        "            if _code:\n"
        "                raise\n"
        "    finally:\n"
        "        sys.argv = _old\n",
        encoding="utf-8",
    )
    return wrapper


def _find_payload(extracted: Path, *, archive_name: str = "") -> tuple[dict, str, Path, list[str]]:
    manifest_path = extracted / "PATCH_TOOL_MANIFEST.json"
    manifest = _read_json(manifest_path, "PATCH_TOOL_MANIFEST.json") if manifest_path.is_file() else {}
    ops = extracted / "PATCH_TOOL_OPS.json"
    py_files = _python_payload_files(extracted)
    if ops.is_file() and py_files:
        raise ValueError("package contains both PATCH_TOOL_OPS.json and Python patch entrypoint")
    if ops.is_file():
        return manifest, "ops", ops, []
    if manifest:
        pp = manifest.get("post_patch") if isinstance(manifest.get("post_patch"), dict) else {}
        commands = pp.get("commands") if isinstance(pp, dict) else None
        if not py_files and isinstance(commands, list) and commands and bool(pp.get("run_when_no_changes")):
            return manifest, "command_only", manifest_path, []
        if len(py_files) != 1:
            raise ValueError(f"package must contain exactly one Python patch entrypoint when OPS is absent (found {len(py_files)})")
        return manifest, "python", py_files[0], []
    candidates = _legacy_python_candidates(extracted, archive_name)
    if not candidates:
        raise ValueError(f"legacy archive has no positively recognized Python patch entrypoint (found {len(py_files)} Python file(s))")
    if len(candidates) == 1:
        return {}, "python", candidates[0], [candidates[0].relative_to(extracted).as_posix()]
    wrapper = _legacy_multi_wrapper(extracted, candidates)
    return {}, "python", wrapper, [p.relative_to(extracted).as_posix() for p in candidates]


def _git_bytes(root: Path, args: list[str], *, timeout: int = 30) -> bytes | None:
    try:
        proc = subprocess.run(["git", *args], cwd=root, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout)
    except (subprocess.TimeoutExpired, OSError):
        return None
    if proc.returncode != 0:
        return None
    return proc.stdout


def _git_worktree_fingerprint(root: Path) -> str | None:
    if not (root / ".git").exists():
        return None
    # Disable repository-configured external diff/textconv/fsmonitor hooks for
    # state observation.  Fingerprinting must be read-only and bounded rather
    # than execute arbitrary helper programs merely to decide whether a PATCH
    # changed the project.
    diff = _git_bytes(root, ["-c", "core.fsmonitor=false", "diff", "--no-ext-diff", "--no-textconv", "--binary", "HEAD", "--", "."], timeout=60)
    raw = _git_bytes(root, ["-c", "core.fsmonitor=false", "ls-files", "--others", "--exclude-standard", "-z"], timeout=30)
    if diff is None or raw is None:
        return None
    h = hashlib.sha256()
    h.update(diff)
    for name in raw.split(b"\0"):
        if not name:
            continue
        try:
            rel = name.decode("utf-8", errors="surrogateescape")
            path = root / rel
            if path.is_file() and not path.is_symlink():
                h.update(name); h.update(b"\0"); h.update(_sha256(path).encode()); h.update(b"\0")
        except OSError:
            continue
    return h.hexdigest()


def _dirty_paths(root: Path) -> dict[str, str]:
    if not (root / ".git").exists():
        return {}
    try:
        proc = subprocess.run(
            ["git", "-c", "core.fsmonitor=false", "status", "--porcelain=v1", "-z", "--untracked-files=all"],
            cwd=root, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30,
        )
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError("git status timed out while determining dirty paths") from exc
    except OSError as exc:
        raise RuntimeError(f"git status could not start: {type(exc).__name__}: {exc}") from exc
    if proc.returncode != 0:
        detail = proc.stderr.decode("utf-8", errors="replace").strip().replace("\n", " ")[:500]
        raise RuntimeError(f"git status failed rc={proc.returncode}" + (f": {detail}" if detail else ""))
    out: dict[str, str] = {}
    parts = proc.stdout.split(b"\0")
    i = 0
    while i < len(parts):
        entry = parts[i]
        i += 1
        if not entry:
            continue
        text = entry.decode("utf-8", errors="surrogateescape")
        if len(text) < 4:
            continue
        status = text[:2]
        name = text[3:]
        if any(code in {"R", "C"} for code in status) and i < len(parts):
            # With porcelain=v1 -z, `name` is the destination and the next
            # NUL field is the source/original path. Consume it but keep the
            # destination as the dirty path used for patch/Git policy.
            i += 1
        path = root / name
        try:
            digest = _sha256(path) if path.is_file() and not path.is_symlink() else "<nonfile>"
        except OSError:
            digest = "<unreadable>"
        out[name] = f"{status}:{digest}"
    return out


def _touched_paths(before: dict[str, str], after: dict[str, str]) -> list[str]:
    return sorted(name for name in set(before) | set(after) if before.get(name) != after.get(name))


def _windows_taskkill(proc: subprocess.Popen, *, force: bool) -> None:
    if os.name != "nt":
        return
    try:
        argv = ["taskkill", "/PID", str(proc.pid), "/T"]
        if force:
            argv.append("/F")
        cp = subprocess.run(argv, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5, check=False)
        if cp.returncode != 0 and proc.poll() is None:
            proc.kill() if force else proc.terminate()
    except Exception:
        try:
            if proc.poll() is None:
                proc.kill() if force else proc.terminate()
        except Exception:
            pass


def _signal_subprocess_group(proc: subprocess.Popen, signum: int) -> None:
    try:
        if os.name != "nt":
            # Do not gate this on proc.poll(). The process-group leader may
            # already have exited while descendants in the same group remain.
            os.killpg(proc.pid, signum)
        elif proc.poll() is None:
            # CREATE_NEW_PROCESS_GROUP enables CTRL_BREAK delivery when the
            # child shares a console. Fall back to terminate on unsupported hosts.
            ctrl_break = getattr(signal, "CTRL_BREAK_EVENT", None)
            if signum in {getattr(signal, "SIGINT", 2), getattr(signal, "SIGTERM", 15)} and ctrl_break is not None:
                try:
                    proc.send_signal(ctrl_break)
                    return
                except Exception:
                    pass
            proc.terminate()
    except (ProcessLookupError, PermissionError, OSError):
        pass


def _managed_group_alive(proc: subprocess.Popen) -> bool:
    if os.name == "nt":
        return proc.poll() is None
    try:
        os.killpg(proc.pid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    except OSError:
        return False


def _quiesce_managed_group(proc: subprocess.Popen, initial_signal: int, *, grace_seconds: float = 1.0) -> None:
    """Terminate the managed payload tree before rollback/result publication."""
    if os.name == "nt":
        # Enumerate/terminate the tree while the group leader is still known.
        # Signalling only the leader first can let it exit before `taskkill /T`
        # discovers descendants, leaving a child alive after timeout/rollback.
        _windows_taskkill(proc, force=False)
        if proc.poll() is None:
            _signal_subprocess_group(proc, initial_signal)
        deadline = time.monotonic() + max(0.1, grace_seconds)
        while time.monotonic() < deadline:
            if proc.poll() is not None:
                break
            time.sleep(0.05)
        # /F is the final containment barrier before rollback or result publish.
        _windows_taskkill(proc, force=True)
        try:
            proc.wait(timeout=1.0)
        except Exception:
            pass
        return

    _signal_subprocess_group(proc, initial_signal)
    deadline = time.monotonic() + max(0.1, grace_seconds)
    while time.monotonic() < deadline:
        if proc.poll() is None:
            try:
                proc.wait(timeout=0.05)
            except subprocess.TimeoutExpired:
                pass
        if not _managed_group_alive(proc):
            return
        time.sleep(0.03)

    term = signal.SIGTERM if hasattr(signal, "SIGTERM") else initial_signal
    _signal_subprocess_group(proc, term)
    deadline = time.monotonic() + 0.75
    while time.monotonic() < deadline:
        if not _managed_group_alive(proc):
            return
        time.sleep(0.03)

    kill_signal = signal.SIGKILL if hasattr(signal, "SIGKILL") else term
    _signal_subprocess_group(proc, kill_signal)
    deadline = time.monotonic() + 0.75
    while time.monotonic() < deadline:
        if not _managed_group_alive(proc):
            break
        time.sleep(0.03)
    if proc.poll() is None:
        try:
            proc.wait(timeout=0.5)
        except subprocess.TimeoutExpired:
            pass


_INTERNAL_CONTROL_ENV_VARS = frozenset({
    "PTV_PATCH_RESULT_FILE",
    "PTV_COLLECT_RESULT_FILE",
    "PTV_PARENT_MUTATION_LOCK_KEY",
    "PTV_PARENT_MUTATION_LOCK_TOKEN",
})


def _external_command_env(*, base: dict[str, str] | None = None) -> dict[str, str]:
    """Environment for untrusted/project commands, without Patch Tool control channels."""
    env = dict(os.environ if base is None else base)
    for name in _INTERNAL_CONTROL_ENV_VARS:
        env.pop(name, None)
    return env


def _run_managed_process(argv: list[str], *, cwd: Path, env: dict[str, str] | None = None, timeout: int) -> ManagedProcessResult:
    """Run one executable in an isolated process group and distinguish timeout from rc=124.

    Timeout/interruption terminates descendants before control returns to the
    rollback path. Returning an explicit ``timed_out`` bit avoids conflating a
    program that deliberately exits 124 with a tool-enforced timeout.
    """
    # Managed PATCH commands are deliberately non-interactive.  Keeping stdin
    # attached to the selector terminal lets an accidental input()/prompt steal
    # keystrokes or hang until the command timeout.
    kwargs: dict[str, object] = {"cwd": cwd, "env": env, "stdin": subprocess.DEVNULL}
    if os.name != "nt":
        kwargs["start_new_session"] = True
    elif hasattr(subprocess, "CREATE_NEW_PROCESS_GROUP"):
        kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    proc = subprocess.Popen(argv, **kwargs)
    try:
        rc = proc.wait(timeout=max(1, int(timeout)))
        lingering = False
        if os.name != "nt" and _managed_group_alive(proc):
            # Give very short-lived helpers a small bounded drain window.  A
            # process tree that remains alive beyond this point is asynchronous
            # work escaping the command contract and must be contained.
            drain_deadline = time.monotonic() + 0.25
            while time.monotonic() < drain_deadline and _managed_group_alive(proc):
                time.sleep(0.02)
            if _managed_group_alive(proc):
                lingering = True
                term = signal.SIGTERM if hasattr(signal, "SIGTERM") else signal.SIGINT
                _quiesce_managed_group(proc, term, grace_seconds=0.75)
        return ManagedProcessResult(rc, False, lingering)
    except subprocess.TimeoutExpired:
        term = signal.SIGTERM if hasattr(signal, "SIGTERM") else signal.SIGINT
        _quiesce_managed_group(proc, term, grace_seconds=1.0)
        return ManagedProcessResult(124, True)
    except KeyboardInterrupt:
        signum = _ACTIVE_TERMINATION_SIGNAL or signal.SIGINT
        _quiesce_managed_group(proc, signum, grace_seconds=1.0)
        raise


def _resolve_command_cwd(root: Path, cwd_raw: str, *, label: str) -> Path:
    if not isinstance(cwd_raw, str) or not cwd_raw:
        raise ValueError(f"{label} cwd must be a string")
    if cwd_raw == ".":
        return root
    try:
        cwd = resolve_project_path(root, cwd_raw)
    except PatchSchemaError as exc:
        raise ValueError(f"unsafe {label} cwd: {cwd_raw}: {exc}") from exc
    if not cwd.is_dir():
        raise ValueError(f"{label} cwd not found: {cwd_raw}")
    return cwd


def _run_argv(root: Path, cmd: dict, *, label: str = "POST PATCH") -> ManagedProcessResult:
    argv = cmd.get("argv")
    label_lower = label.lower().replace(" ", "_")
    if not isinstance(argv, list) or not argv or not all(isinstance(x, str) and x for x in argv):
        raise ValueError(f"{label_lower} command argv must be a non-empty string array")
    cwd_raw = cmd.get("cwd", ".")
    cwd = _resolve_command_cwd(root, cwd_raw, label=label_lower)
    timeout = int(cmd.get("timeout_seconds", 300))
    name = cmd.get("name") or " ".join(argv)
    print(f"{label}: {name}", flush=True)
    outcome = _run_managed_process(argv, cwd=cwd, env=_external_command_env(), timeout=max(1, timeout))
    if outcome.timed_out:
        print(f"ERROR: {label_lower} command timeout after {timeout}s", file=sys.stderr)
    return outcome


def _run_command_sequence(root: Path, commands: object, *, label: str) -> dict[str, object]:
    result: dict[str, object] = {"status": "PASS", "rc": 0, "commands": []}
    if not isinstance(commands, list) or not commands:
        return result
    rows: list[dict[str, object]] = []
    result["commands"] = rows
    for cmd in commands:
        if not isinstance(cmd, dict):
            rows.append({"name": "<invalid>", "status": "FAIL", "rc": 2, "timed_out": False})
            result.update({"status": "FAIL", "rc": 2})
            print(f"ERROR: invalid {label.lower()} command object", file=sys.stderr)
            break
        name = str(cmd.get("name") or " ".join(cmd.get("argv") or []) or "<unnamed>")
        try:
            outcome = _run_argv(root, cmd, label=label)
            rc = outcome.effective_rc
            if outcome.lingering_descendants:
                print(f"ERROR: {label.lower()} command left descendant processes after its leader exited; descendants were terminated", file=sys.stderr)
            rows.append({"name": name, "status": "PASS" if rc == 0 else "FAIL", "rc": rc, "timed_out": bool(outcome.timed_out), "lingering_descendants": bool(outcome.lingering_descendants)})
        except KeyboardInterrupt:
            rc = 143 if _ACTIVE_TERMINATION_SIGNAL == getattr(signal, "SIGTERM", None) else 130
            rows.append({"name": name, "status": "INTERRUPTED", "rc": rc, "timed_out": False})
            result.update({"status": "INTERRUPTED", "rc": rc})
            print(f"{label}: interrupted by user/termination signal", file=sys.stderr, flush=True)
            # User termination is a control-flow event, not an ordinary command
            # failure.  Propagate it so the runner/dispatcher globally stops the
            # batch after the managed process tree has been quiesced.
            raise
        except Exception as exc:
            print(f"ERROR: {label.lower()} command invalid: {exc}", file=sys.stderr)
            rows.append({"name": name, "status": "FAIL", "rc": 2, "timed_out": False, "error": f"{type(exc).__name__}: {exc}"})
            result.update({"status": "FAIL", "rc": 2})
            break
        if rc:
            result.update({"status": "FAIL", "rc": rc})
            break
    return result


def _run_post_patch(root: Path, manifest: dict, *, changed: bool) -> dict[str, object] | None:
    pp = manifest.get("post_patch")
    if not isinstance(pp, dict):
        return None
    commands = pp.get("commands")
    if not isinstance(commands, list) or not commands:
        return None
    if not changed and not bool(pp.get("run_when_no_changes", False)):
        print("POST PATCH: skipped because payload produced no detected project delta")
        return {"status": "SKIPPED", "rc": 0, "commands": [], "reason": "no_detected_project_delta"}
    return _run_command_sequence(root, commands, label="POST PATCH")


def _run_on_failure(root: Path, manifest: dict) -> dict[str, object] | None:
    node = manifest.get("on_failure")
    commands = node.get("commands") if isinstance(node, dict) else None
    if not isinstance(commands, list) or not commands:
        return None
    print("ON FAILURE: running failure-only command sequence after rollback/failure containment", flush=True)
    report = _run_command_sequence(root, commands, label="ON FAILURE")
    print(f"ON FAILURE SUMMARY: {report.get('status')} | rc={report.get('rc')}", flush=True)
    return report


def _apply_on_failure_commands(
    root: Path, manifest: dict, result: dict[str, object], *,
    before_fp: str | None, before_dirty: dict[str, str],
    before_targets: dict[str, dict[str, object]], target_paths: list[str],
    current_partial: dict[str, object],
) -> dict[str, object]:
    report = _run_on_failure(root, manifest)
    if report is None:
        return current_partial
    result["on_failure"] = report
    try:
        final_partial = _partial_state(
            root, before_fp=before_fp, before_dirty=before_dirty,
            before_targets=before_targets, target_paths=target_paths,
        )
    except Exception as exc:
        final_partial = {
            "detected": None, "changed_paths": [],
            "evidence": f"on_failure_final_state_unknown:{type(exc).__name__}",
        }
    report["final_project_delta"] = final_partial
    return final_partial


def _diagnostic_rerun_is_safe(argv: list[str]) -> tuple[bool, str | None]:
    joined = " ".join(argv).lower()
    match = re.search(
        r"(?:^|[^a-z0-9_])(flash|ota|deploy|push|publish|release|provision(?:ing)?|erase|esptool|dfu)(?:[^a-z0-9_]|$)",
        joined,
    )
    if match:
        return False, f"dangerous_action_hint:{match.group(1)}"
    return True, None


def _run_validation_profiles(root: Path, preflight: dict[str, object]) -> dict[str, object] | None:
    profiles = preflight.get("_resolved_validation_profiles") if isinstance(preflight, dict) else None
    if not isinstance(profiles, list) or not profiles:
        return None
    global_rerun = preflight.get("_resolved_validation_rerun_policy") if isinstance(preflight, dict) else None
    if not isinstance(global_rerun, dict):
        global_rerun = {"max_commands": 1, "on_timeout": False}
    max_reruns = int(global_rerun.get("max_commands") or 0)
    global_on_timeout = bool(global_rerun.get("on_timeout", False))
    reruns_used = 0
    report: dict[str, object] = {"status": "PASS", "rc": 0, "profiles": [], "diagnostic_reruns_used": 0}
    rows: list[dict[str, object]] = report["profiles"]  # type: ignore[assignment]
    for profile in profiles:
        if not isinstance(profile, dict):
            print("ERROR: invalid resolved validation profile", file=sys.stderr)
            rows.append({"name": "<invalid>", "status": "FAIL", "rc": 2, "timed_out": False})
            report.update({"status": "FAIL", "rc": 2})
            return report
        name = str(profile.get("name") or "unnamed")
        argv = profile.get("argv")
        cwd_raw = str(profile.get("cwd") or ".")
        timeout = int(profile.get("timeout_seconds") or 900)
        if not isinstance(argv, list) or not argv or any(not isinstance(x, str) or not x for x in argv):
            print(f"ERROR: validation profile {name} has invalid argv", file=sys.stderr)
            rows.append({"name": name, "status": "FAIL", "rc": 2, "timed_out": False, "error": "invalid_argv"})
            report.update({"status": "FAIL", "rc": 2})
            return report
        try:
            cwd = _resolve_command_cwd(root, cwd_raw, label=f"validation_profile_{name}")
        except ValueError as exc:
            print(f"ERROR: validation profile {name} has unsafe cwd: {exc}", file=sys.stderr)
            rows.append({"name": name, "status": "FAIL", "rc": 2, "timed_out": False, "error": "unsafe_cwd"})
            report.update({"status": "FAIL", "rc": 2})
            return report
        print(f"VALIDATION PROFILE: {name}", flush=True)
        try:
            outcome = _run_managed_process(argv, cwd=cwd, env=_external_command_env(), timeout=max(1, timeout))
        except KeyboardInterrupt:
            raise
        except Exception as exc:
            print(f"ERROR: validation profile {name} could not start: {type(exc).__name__}: {exc}", file=sys.stderr)
            rows.append({"name": name, "status": "FAIL", "rc": 2, "timed_out": False, "error": type(exc).__name__})
            report.update({"status": "FAIL", "rc": 2})
            return report
        rc = outcome.effective_rc
        row: dict[str, object] = {
            "name": name,
            "status": "PASS" if rc == 0 else "FAIL",
            "rc": rc,
            "timed_out": bool(outcome.timed_out),
            "lingering_descendants": bool(outcome.lingering_descendants),
        }
        rows.append(row)
        if rc != 0:
            rerun_cfg = profile.get("diagnostic_rerun")
            if isinstance(rerun_cfg, dict) and bool(rerun_cfg.get("enabled", True)):
                rerun_row: dict[str, object] = {"status": "SKIPPED"}
                row["diagnostic_rerun"] = rerun_row
                if not bool(rerun_cfg.get("safe", False)):
                    rerun_row["reason"] = "safe_true_required"
                elif reruns_used >= max_reruns:
                    rerun_row["reason"] = "global_max_commands_reached"
                elif outcome.timed_out and not (bool(rerun_cfg.get("on_timeout", False)) or global_on_timeout):
                    rerun_row["reason"] = "primary_timeout_rerun_disabled"
                else:
                    rerun_argv = [*argv, *list(rerun_cfg.get("append_args") or [])]
                    safe, reason = _diagnostic_rerun_is_safe(rerun_argv)
                    if not safe:
                        rerun_row["reason"] = reason or "dangerous_action"
                    else:
                        reruns_used += 1
                        report["diagnostic_reruns_used"] = reruns_used
                        rerun_name = str(rerun_cfg.get("name") or f"{name} diagnostic rerun")
                        rerun_timeout = int(rerun_cfg.get("timeout_seconds") or min(timeout, 600))
                        print(f"DIAGNOSTIC RERUN: {rerun_name}", flush=True)
                        try:
                            rerun_outcome = _run_managed_process(rerun_argv, cwd=cwd, env=_external_command_env(), timeout=max(1, rerun_timeout))
                            rerun_row.update({
                                "status": "PASS" if rerun_outcome.effective_rc == 0 else "FAIL",
                                "rc": rerun_outcome.effective_rc,
                                "timed_out": bool(rerun_outcome.timed_out),
                                "lingering_descendants": bool(rerun_outcome.lingering_descendants),
                                "name": rerun_name,
                            })
                        except Exception as exc:
                            rerun_row.update({"status": "FAIL", "rc": 2, "error": type(exc).__name__, "name": rerun_name})
                        # Historical contract: diagnostic rerun is evidence only.
                        # It never changes the primary validation failure result.
                        print(f"DIAGNOSTIC RERUN COMPLETE: {rerun_name} | primary validation remains FAIL", flush=True)
        if outcome.lingering_descendants:
            print(f"ERROR: validation profile {name} left descendant processes; descendants were terminated", file=sys.stderr)
            report.update({"status": "FAIL", "rc": rc})
            return report
        if outcome.timed_out:
            print(f"ERROR: validation profile {name} timeout after {timeout}s", file=sys.stderr)
            report.update({"status": "FAIL", "rc": rc})
            return report
        if rc:
            print(f"ERROR: validation profile {name} failed rc={rc}", file=sys.stderr)
            report.update({"status": "FAIL", "rc": rc})
            return report
        print(f"VALIDATION PROFILE: {name} PASS")
    return report

def _run_git_command(root: Path, args: list[str], *, timeout: int) -> ManagedProcessResult:
    env = _external_command_env()
    # Automated Git policy must never block waiting for credentials/input.
    env["GIT_TERMINAL_PROMPT"] = "0"
    return _run_managed_process(["git", *args], cwd=root, env=env, timeout=max(1, timeout))


def _run_git_policy(root: Path, manifest: dict, before_dirty: dict[str, str], after_dirty: dict[str, str]) -> int:
    policy = manifest.get("git")
    if not isinstance(policy, dict) or not (root / ".git").exists():
        return 0
    touched = _touched_paths(before_dirty, after_dirty)
    fail_on_error = bool(policy.get("fail_on_error", True))
    timeout = int(policy.get("timeout_seconds", 300))
    commit_mode = policy.get("commit")
    auto_commit = commit_mode == "auto" and bool(touched)
    staged_for_auto_commit = False
    commit_completed = False

    def run_git(args: list[str], label: str, *, command_timeout: int | None = None) -> int:
        outcome = _run_git_command(root, args, timeout=command_timeout or timeout)
        if outcome.timed_out:
            raise RuntimeError(f"{label} timeout after {command_timeout or timeout}s")
        rc = outcome.effective_rc
        if outcome.lingering_descendants:
            raise RuntimeError(f"{label} left descendant processes after leader exit; descendants were terminated")
        if rc:
            raise RuntimeError(f"{label} failed rc={rc}")
        return rc

    try:
        if auto_commit:
            message = policy.get("commit_message")
            if not isinstance(message, str) or not message.strip():
                raise RuntimeError("git.commit=auto requires commit_message")
            preexisting_dirty = sorted(name for name in touched if name in before_dirty)
            if preexisting_dirty:
                # This guard must run BEFORE git add. Otherwise a rejected
                # auto-commit silently changes the user's index by staging
                # pre-existing local edits on the same target path.
                raise RuntimeError(
                    "git.commit=auto refuses target paths that were already dirty before PATCH: "
                    + ", ".join(preexisting_dirty)
                )
        if policy.get("add") not in {None, "off", False} and touched:
            # Mark ownership before invoking Git: a failing `git add` may have
            # updated part of the index before returning non-zero.
            staged_for_auto_commit = auto_commit
            run_git(["add", "--", *touched], "git add")
        if auto_commit:
            message = str(policy.get("commit_message"))
            # --only confines the commit to paths touched by this patch run.
            run_git(["commit", "-m", message, "--only", "--", *touched], "git commit")
            commit_completed = True
            staged_for_auto_commit = False
        if policy.get("push") == "auto":
            run_git(["push"], "git push")
    except Exception as exc:
        # For auto-commit, touched paths are required to have been clean before
        # PATCH. Therefore any staged entries on them after our `git add` were
        # created by this policy and can be safely reset if commit never
        # completed. This restores the user's pre-policy Git index on failure.
        cleanup_error = None
        if staged_for_auto_commit and not commit_completed and touched:
            try:
                reset = _run_git_command(root, ["reset", "--quiet", "HEAD", "--", *touched], timeout=min(timeout, 30))
                if reset.timed_out:
                    cleanup_error = f"git index cleanup timeout after {min(timeout, 30)}s"
                elif reset.lingering_descendants:
                    cleanup_error = "git index cleanup left descendant processes; descendants were terminated"
                elif reset.effective_rc:
                    cleanup_error = f"git index cleanup failed rc={reset.effective_rc}"
            except Exception as cleanup_exc:
                cleanup_error = f"git index cleanup failed: {type(cleanup_exc).__name__}: {cleanup_exc}"
        print(f"ERROR: {exc}", file=sys.stderr)
        if cleanup_error:
            print(f"ERROR: {cleanup_error}", file=sys.stderr)
            return 1
        return 1 if fail_on_error else 0
    return 0



def _mutation_lock_key(root: Path) -> str:
    return hashlib.sha256(str(root.resolve()).encode("utf-8", errors="surrogateescape")).hexdigest()[:32]


def _safe_lock_directory(path: Path) -> Path:
    if path.exists() or path.is_symlink():
        st = path.lstat()
        attrs = getattr(st, "st_file_attributes", 0)
        reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
            raise RuntimeError(f"unsafe mutation lock directory: {path}")
    else:
        try:
            path.mkdir(mode=0o700, exist_ok=False)
        except FileExistsError:
            return _safe_lock_directory(path)
    return path


def _mutation_lock_path(root: Path) -> Path:
    top = _safe_lock_directory(Path(tempfile.gettempdir()) / "python_patch_tool_locks")
    base = _safe_lock_directory(top / _mutation_lock_key(root))
    return base / "mutation.lock"


def _open_mutation_lock_file(path: Path):
    if path.exists() or path.is_symlink():
        st = path.lstat()
        attrs = getattr(st, "st_file_attributes", 0)
        reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISREG(st.st_mode):
            raise RuntimeError(f"unsafe mutation lock file: {path}")
    flags = os.O_RDWR | os.O_CREAT | getattr(os, "O_BINARY", 0) | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
    fd = os.open(path, flags, 0o600)
    try:
        st = os.fstat(fd)
        attrs = getattr(st, "st_file_attributes", 0)
        reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if not stat.S_ISREG(st.st_mode) or reparse:
            raise RuntimeError(f"mutation lock descriptor is not a regular file: {path}")
        return os.fdopen(fd, "r+b")
    except Exception:
        os.close(fd)
        raise


def _inherited_mutation_lock_is_valid(root: Path, path: Path) -> bool:
    token = os.environ.get("PTV_PARENT_MUTATION_LOCK_TOKEN", "").strip()
    key = os.environ.get("PTV_PARENT_MUTATION_LOCK_KEY", "").strip()
    if not token or key != _mutation_lock_key(root):
        return False
    try:
        with _open_mutation_lock_file(path) as fh:
            fh.seek(0)
            current = fh.read(256).decode("ascii", errors="ignore").strip()
        return current == token
    except OSError:
        return False


def _acquire_project_mutation_lock(root: Path):
    path = _mutation_lock_path(root)
    # In batch-transaction mode the dispatcher owns this exact lock across the
    # whole snapshot -> PATCH sequence -> rollback/commit window. Children may
    # inherit only a matching random token written by that lock owner.
    if _inherited_mutation_lock_is_valid(root, path):
        return None
    fh = _open_mutation_lock_file(path)
    try:
        fh.seek(0, os.SEEK_END)
        if fh.tell() == 0:
            fh.write(b"0")
            fh.flush()
        fh.seek(0)
        started = time.monotonic()
        if os.name == "nt":
            import msvcrt
            msvcrt.locking(fh.fileno(), msvcrt.LK_LOCK, 1)
        else:
            import fcntl
            fcntl.flock(fh.fileno(), fcntl.LOCK_EX)
        waited = time.monotonic() - started
        if waited >= 0.25:
            print(f"PATCH MUTATION LOCK: acquired after waiting {waited:.1f}s for another PATCH process")
        return fh
    except Exception:
        fh.close()
        raise


def _release_project_mutation_lock(fh) -> None:
    if fh is None:
        return
    try:
        fh.seek(0)
        if os.name == "nt":
            import msvcrt
            msvcrt.locking(fh.fileno(), msvcrt.LK_UNLCK, 1)
        else:
            import fcntl
            fcntl.flock(fh.fileno(), fcntl.LOCK_UN)
    finally:
        fh.close()


def _snapshot_patch_input(source: Path) -> tuple[tempfile.TemporaryDirectory[str], Path, str]:
    """Capture the exact PATCH bytes that will be preflighted/executed.

    The queue file is user-controlled and other terminals are intentionally
    allowed to run concurrently.  Execution therefore never trusts that the
    pathname still contains the same bytes later in the run.
    """
    temp_dir = tempfile.TemporaryDirectory(prefix="ptv-input-")
    snapshot = Path(temp_dir.name) / source.name
    flags = os.O_RDONLY | getattr(os, "O_BINARY", 0) | getattr(os, "O_NOFOLLOW", 0)
    fd = None
    try:
        fd = os.open(source, flags)
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode):
            raise ValueError("PATCH input must be a regular non-symlink file")
        digest = hashlib.sha256()
        copied = 0
        with os.fdopen(os.dup(fd), "rb") as src, snapshot.open("wb") as dst:
            for chunk in iter(lambda: src.read(1024 * 1024), b""):
                dst.write(chunk)
                digest.update(chunk)
                copied += len(chunk)
            dst.flush()
            try: os.fsync(dst.fileno())
            except OSError: pass
        after = os.fstat(fd)
        identity_before = (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns)
        identity_after = (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns)
        if identity_before != identity_after or copied != before.st_size:
            raise ValueError("PATCH input changed while creating execution snapshot")
        return temp_dir, snapshot, digest.hexdigest()
    except Exception:
        temp_dir.cleanup()
        raise
    finally:
        if fd is not None:
            try: os.close(fd)
            except OSError: pass


def _queue_dirs(root: Path) -> tuple[Path, Path]:
    resolved_root = root.resolve(strict=True)
    queue = root / "patchs"
    if queue.is_symlink():
        raise ValueError("unsafe queue: project patchs/ must not be a symlink")
    if not queue.is_dir():
        raise ValueError("unsafe queue: project patchs/ is missing or not a directory")
    try:
        if queue.resolve(strict=True).parent != resolved_root:
            raise ValueError("unsafe queue: patchs/ escapes project root")
    except OSError as exc:
        raise ValueError("unsafe queue: patchs/ cannot be resolved") from exc
    history = queue / "patched"
    if history.exists() or history.is_symlink():
        if history.is_symlink() or not history.is_dir():
            raise ValueError("unsafe archive destination: patchs/patched/ must be a real directory")
    else:
        history.mkdir(parents=False, exist_ok=False)
    if history.resolve(strict=True).parent != queue.resolve(strict=True):
        raise ValueError("unsafe archive destination: patchs/patched/ escapes project queue")
    return queue, history


def _publish_executed_patch(snapshot: Path, dst: Path, expected_sha: str) -> None:
    """Publish exact executed bytes without overwriting a concurrent archive."""
    if dst.exists() or dst.is_symlink():
        if dst.is_symlink() or not dst.is_file():
            raise ValueError(f"unsafe archive destination: {dst}")
        if _sha256(dst) != expected_sha:
            raise ValueError(f"archive destination already exists with different content: {dst.name}")
        return
    fd, tmp_name = tempfile.mkstemp(prefix=".ptv-archive-", suffix=".tmp", dir=dst.parent)
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "wb") as out, snapshot.open("rb") as src:
            shutil.copyfileobj(src, out, length=1024 * 1024)
            out.flush()
            try: os.fsync(out.fileno())
            except OSError: pass
        if _sha256(tmp) != expected_sha:
            raise ValueError("executed PATCH snapshot failed archive hash verification")
        try:
            os.link(tmp, dst, follow_symlinks=False)
        except FileExistsError:
            if dst.is_symlink() or not dst.is_file() or _sha256(dst) != expected_sha:
                raise ValueError(f"archive destination raced with different content: {dst.name}")
        except OSError:
            # Hard links are unavailable on some removable/network filesystems.
            # Fall back to an exclusive copy while preserving no-overwrite
            # semantics, then verify the published bytes before success.
            out_fd = None
            try:
                out_fd = os.open(dst, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(out_fd, "wb") as out, tmp.open("rb") as src:
                    out_fd = None
                    for chunk in iter(lambda: src.read(1024 * 1024), b""):
                        out.write(chunk)
                    out.flush()
                    try: os.fsync(out.fileno())
                    except OSError: pass
            except FileExistsError:
                if dst.is_symlink() or not dst.is_file() or _sha256(dst) != expected_sha:
                    raise ValueError(f"archive destination raced with different content: {dst.name}")
            finally:
                if out_fd is not None:
                    try: os.close(out_fd)
                    except OSError: pass
            if dst.is_symlink() or not dst.is_file() or _sha256(dst) != expected_sha:
                try: dst.unlink()
                except OSError: pass
                raise ValueError("archive fallback copy failed hash verification")
        finally:
            try: tmp.unlink()
            except FileNotFoundError: pass
    finally:
        try: tmp.unlink()
        except FileNotFoundError: pass


def _remove_queue_input_if_executed(source: Path, expected_sha: str) -> str:
    """Remove the queue pathname only when it still names executed bytes.

    A concurrent/user replacement is restored and deliberately left queued.
    This avoids archiving or deleting a package that was never executed.
    """
    if not source.exists() and not source.is_symlink():
        return "already_absent"
    token = f".ptv-archive-guard-{os.getpid()}-{time.time_ns()}-{source.name}"
    guard = source.parent / token
    try:
        os.replace(source, guard)
    except FileNotFoundError:
        return "already_absent"
    try:
        if guard.is_symlink() or not guard.is_file():
            current_sha = None
        else:
            current_sha = _sha256(guard)
        if current_sha == expected_sha:
            guard.unlink()
            return "removed_executed_input"

        # The queue name was replaced while the PATCH was running. Restore the
        # replacement rather than deleting data that was never executed.
        if not source.exists() and not source.is_symlink():
            os.replace(guard, source)
            return "replacement_restored"

        # Another process recreated the queue name before restoration. Preserve
        # the displaced file under a visible non-runnable guard name.
        preserved = source.parent / f"PTV_UNEXPECTED_QUEUE_REPLACEMENT_{time.time_ns()}_{source.name}"
        os.replace(guard, preserved)
        return f"replacement_preserved:{preserved.name}"
    finally:
        if guard.exists() or guard.is_symlink():
            # Best effort: never silently delete an unexpected replacement.
            try:
                if not source.exists() and not source.is_symlink():
                    os.replace(guard, source)
            except OSError:
                pass


def _archive_success(root: Path, source: Path, executed_snapshot: Path, expected_sha: str) -> tuple[Path, str]:
    queue, out_dir = _queue_dirs(root)
    if source.parent.resolve(strict=True) != queue.resolve(strict=True):
        raise ValueError("PATCH input must be a direct file under project patchs/")
    dst = out_dir / source.name
    _publish_executed_patch(executed_snapshot, dst, expected_sha)
    lifecycle = _remove_queue_input_if_executed(source, expected_sha)
    return dst, lifecycle


def _execute_python(script: Path, root: Path, timeout: int) -> ManagedProcessResult:
    env = _external_command_env()
    lib = str(Path(__file__).resolve().parent)
    env["PYTHONPATH"] = lib + (os.pathsep + env["PYTHONPATH"] if env.get("PYTHONPATH") else "")
    return _run_managed_process([sys.executable, str(script)], cwd=root, env=env, timeout=max(1, timeout))


def _utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def _snapshot_declared_paths(root: Path, paths: list[str]) -> dict[str, dict[str, object]]:
    snap: dict[str, dict[str, object]] = {}
    for rel in sorted(set(paths)):
        path = root.joinpath(*PurePosixPath(rel).parts)
        try:
            if path.is_symlink():
                snap[rel] = {"kind": "symlink"}
            elif path.is_file():
                st = path.stat()
                snap[rel] = {"kind": "file", "size": st.st_size, "sha256": _sha256(path)}
            elif path.exists():
                snap[rel] = {"kind": "other"}
            else:
                snap[rel] = {"kind": "missing"}
        except OSError as exc:
            snap[rel] = {"kind": "unreadable", "error": type(exc).__name__}
    return snap


def _snapshot_changes(before: dict[str, dict[str, object]], after: dict[str, dict[str, object]]) -> list[str]:
    return sorted(path for path in set(before) | set(after) if before.get(path) != after.get(path))


def _write_patch_result(result: dict[str, object]) -> None:
    raw = os.environ.get("PTV_PATCH_RESULT_FILE", "").strip()
    if not raw:
        return
    path = Path(raw)
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        fd, temp_name = tempfile.mkstemp(prefix=".ptv-result-", suffix=".json", dir=path.parent)
        os.close(fd)
        temp = Path(temp_name)
        temp.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        os.replace(temp, path)
    except Exception as exc:
        print(f"[PTV v{VERSION} WARNING] could not write structured PATCH result: {type(exc).__name__}", file=sys.stderr)


def _failure_kind_from_patch_failure(exc: PatchFailure) -> str:
    text = str(exc).lower()
    if any(token in text for token in ("not found", "ambiguous", "expected block", "match failed", "anchor")):
        return "anchor_mismatch"
    if "does not exist" in text or "target" in text:
        return "source_drift"
    return "patch_operation_failed"


def _partial_state(
    root: Path,
    *,
    before_fp: str | None,
    before_dirty: dict[str, str],
    before_targets: dict[str, dict[str, object]],
    target_paths: list[str],
) -> dict[str, object]:
    after_fp = _git_worktree_fingerprint(root)
    after_dirty = _dirty_paths(root)
    after_targets = _snapshot_declared_paths(root, target_paths)
    target_changes = _snapshot_changes(before_targets, after_targets)
    changed = sorted(set(_touched_paths(before_dirty, after_dirty)) | set(target_changes))
    git_changed = (before_fp != after_fp) if before_fp is not None and after_fp is not None else None
    # A clean Git worktree fingerprint is not proof that the project is
    # unchanged when the PATCH declared no targets: Git intentionally omits
    # ignored files, so an unbounded/legacy Python payload could have changed
    # ignored source while leaving the fingerprint identical.  Treat that case
    # as unknown and force fail-safe continuation logic.
    if target_paths:
        if git_changed is not None:
            detected: bool | None = bool(git_changed or target_changes)
        else:
            detected = bool(target_changes)
    elif git_changed is True:
        detected = True
    else:
        detected = None
    if before_fp is not None and target_paths:
        evidence = "git_worktree+declared_targets"
    elif target_paths:
        evidence = "declared_targets"
    elif git_changed is True:
        evidence = "git_worktree_without_declared_targets"
    elif before_fp is not None:
        evidence = "git_clean_but_ignored_paths_unbounded_without_declared_targets"
    else:
        evidence = "insufficient_non_git_target_declaration"
    return {
        "detected": detected,
        "changed_paths": changed,
        "evidence": evidence,
    }




def _open_rollback_parent_fd(root: Path, rel: str) -> tuple[int | None, Path, str]:
    """Open the declared target parent without following symlink components.

    On POSIX the returned directory fd pins the exact directory inode, so a
    concurrent rename/symlink swap cannot redirect snapshot/restore outside the
    project. Windows falls back to the preflight-validated path checks.
    """
    pure = PurePosixPath(rel)
    parent_path = root.joinpath(*pure.parts[:-1]) if len(pure.parts) > 1 else root
    leaf = pure.name
    if os.name == "nt" or not hasattr(os, "O_DIRECTORY"):
        return None, parent_path, leaf
    flags = os.O_RDONLY | os.O_DIRECTORY | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
    fd = os.open(root, flags)
    try:
        for part in pure.parts[:-1]:
            next_fd = os.open(part, flags, dir_fd=fd)
            os.close(fd)
            fd = next_fd
        return fd, parent_path, leaf
    except Exception:
        try: os.close(fd)
        except OSError: pass
        raise


def _prepare_rollback_snapshot(root: Path, rollback: dict[str, object]) -> tuple[tempfile.TemporaryDirectory[str], dict[str, object]]:
    targets = rollback.get("targets")
    if not isinstance(targets, list) or not targets:
        raise PatchSchemaError("rollback snapshot requires targets", kind="rollback_contract_invalid")
    limit = int(rollback.get("max_total_bytes", 268435456))
    baselines = rollback.get("baselines")
    if not isinstance(baselines, dict):
        raise PatchSchemaError("rollback snapshot requires exact baselines", kind="rollback_contract_invalid")
    temp_dir = tempfile.TemporaryDirectory(prefix="ptv-rollback-")
    base = Path(temp_dir.name)
    entries: list[dict[str, object]] = []
    total = 0
    try:
        for index, rel in enumerate(targets):
            if not isinstance(rel, str):
                raise PatchSchemaError("rollback target must be a string", kind="rollback_contract_invalid")
            baseline = baselines.get(rel)
            if not isinstance(baseline, dict) or "exists" not in baseline:
                raise PatchSchemaError(f"rollback baseline missing at snapshot: {rel}", kind="rollback_contract_invalid", path=rel)
            parent_fd = None
            try:
                parent_fd, parent_path, leaf = _open_rollback_parent_fd(root, rel)
            except (OSError, ValueError) as exc:
                raise PatchSchemaError(
                    f"rollback target parent changed/unsafe before snapshot: {rel}",
                    kind="rollback_snapshot_race", path=rel,
                ) from exc
            try:
                if parent_fd is not None:
                    try:
                        lst = os.stat(leaf, dir_fd=parent_fd, follow_symlinks=False)
                    except FileNotFoundError:
                        if baseline.get("exists") is not False:
                            raise PatchSchemaError(f"rollback baseline changed after preflight: {rel} is now missing", kind="rollback_snapshot_race", path=rel)
                        entries.append({"path": rel, "kind": "missing"})
                        continue
                    if baseline.get("exists") is False:
                        raise PatchSchemaError(f"rollback baseline changed after preflight: {rel} now exists", kind="rollback_snapshot_race", path=rel)
                    if stat.S_ISLNK(lst.st_mode) or not stat.S_ISREG(lst.st_mode):
                        raise PatchSchemaError(f"rollback target baseline must be regular file or missing: {rel}", kind="rollback_snapshot_invalid", path=rel)
                    flags = os.O_RDONLY | getattr(os, "O_BINARY", 0) | getattr(os, "O_NOFOLLOW", 0)
                    fd = os.open(leaf, flags, dir_fd=parent_fd)
                else:
                    path = parent_path / leaf
                    try:
                        lst = path.lstat()
                    except FileNotFoundError:
                        if baseline.get("exists") is not False:
                            raise PatchSchemaError(f"rollback baseline changed after preflight: {rel} is now missing", kind="rollback_snapshot_race", path=rel)
                        entries.append({"path": rel, "kind": "missing"})
                        continue
                    if baseline.get("exists") is False:
                        raise PatchSchemaError(f"rollback baseline changed after preflight: {rel} now exists", kind="rollback_snapshot_race", path=rel)
                    if path_is_link_or_reparse(path) or not stat.S_ISREG(lst.st_mode):
                        raise PatchSchemaError(f"rollback target baseline must be regular non-reparse file or missing: {rel}", kind="rollback_snapshot_invalid", path=rel)
                    fd = os.open(path, os.O_RDONLY | getattr(os, "O_BINARY", 0) | getattr(os, "O_NOFOLLOW", 0))

                backup = base / f"{index:05d}.bin"
                digest = hashlib.sha256()
                copied = 0
                try:
                    before = os.fstat(fd)
                    if not stat.S_ISREG(before.st_mode):
                        raise PatchSchemaError(f"rollback target changed type before snapshot: {rel}", kind="rollback_snapshot_invalid", path=rel)
                    with os.fdopen(os.dup(fd), "rb") as src, backup.open("wb") as dst:
                        for chunk in iter(lambda: src.read(1024 * 1024), b""):
                            copied += len(chunk)
                            total += len(chunk)
                            if total > limit:
                                raise PatchSchemaError(
                                    f"rollback snapshot exceeds max_total_bytes={limit}",
                                    kind="rollback_snapshot_too_large",
                                )
                            dst.write(chunk)
                            digest.update(chunk)
                    after = os.fstat(fd)
                finally:
                    os.close(fd)
                identity_before = (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns)
                identity_after = (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns)
                if identity_before != identity_after or copied != before.st_size:
                    raise PatchSchemaError(f"rollback target changed while snapshotting: {rel}", kind="rollback_snapshot_race", path=rel)
                snapshot_sha = digest.hexdigest()
                expected_sha = str(baseline.get("sha256") or "").lower()
                if not expected_sha or snapshot_sha.lower() != expected_sha:
                    raise PatchSchemaError(
                        f"rollback baseline changed after preflight: {rel} expected={expected_sha or '<missing>'} actual={snapshot_sha}",
                        kind="rollback_snapshot_race", path=rel,
                    )
                entries.append({
                    "path": rel,
                    "kind": "file",
                    "backup": backup.name,
                    "size": copied,
                    "sha256": snapshot_sha,
                    "mode": stat.S_IMODE(before.st_mode),
                })
            finally:
                if parent_fd is not None:
                    try: os.close(parent_fd)
                    except OSError: pass
        return temp_dir, {"targets": list(targets), "entries": entries, "total_bytes": total, "on": list(rollback.get("on") or [])}
    except Exception:
        temp_dir.cleanup()
        raise


def _restore_rollback_snapshot(
    root: Path,
    snapshot_dir: tempfile.TemporaryDirectory[str],
    snapshot: dict[str, object],
    *,
    before_fp: str | None,
    before_dirty: dict[str, str],
    before_targets: dict[str, dict[str, object]],
    target_paths: list[str],
    trigger: str,
) -> dict[str, object]:
    entries = snapshot.get("entries")
    if not isinstance(entries, list):
        return {"status": "FAIL", "trigger": trigger, "message": "rollback snapshot is invalid", "restored_paths": [], "errors": ["missing entries"]}
    restored: list[str] = []
    errors: list[str] = []
    base = Path(snapshot_dir.name)
    for index, entry in enumerate(entries):
        if not isinstance(entry, dict) or not isinstance(entry.get("path"), str):
            errors.append("invalid rollback entry")
            continue
        rel = str(entry["path"])
        parent_fd = None
        try:
            parent_fd, parent_path, leaf = _open_rollback_parent_fd(root, rel)
            kind = entry.get("kind")
            if parent_fd is not None:
                if kind == "missing":
                    try:
                        st = os.stat(leaf, dir_fd=parent_fd, follow_symlinks=False)
                    except FileNotFoundError:
                        restored.append(rel)
                        continue
                    if stat.S_ISREG(st.st_mode) or stat.S_ISLNK(st.st_mode):
                        os.unlink(leaf, dir_fd=parent_fd)
                    else:
                        raise RuntimeError("rollback refuses to remove a directory/non-file created at target path")
                    restored.append(rel)
                    continue
                if kind != "file" or not isinstance(entry.get("backup"), str):
                    raise RuntimeError("invalid file rollback entry")
                backup = base / str(entry["backup"])
                if not backup.is_file() or path_is_link_or_reparse(backup):
                    raise RuntimeError("rollback backup missing")
                temp_name = f".ptv-restore-{os.getpid()}-{time.time_ns()}-{index}"
                flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
                out_fd = os.open(temp_name, flags, 0o600, dir_fd=parent_fd)
                try:
                    digest = hashlib.sha256()
                    with os.fdopen(os.dup(out_fd), "wb") as dst, backup.open("rb") as src:
                        for chunk in iter(lambda: src.read(1024 * 1024), b""):
                            dst.write(chunk); digest.update(chunk)
                        dst.flush()
                        try: os.fsync(dst.fileno())
                        except OSError: pass
                    if digest.hexdigest() != entry.get("sha256"):
                        raise RuntimeError("restored bytes failed snapshot hash verification")
                    if os.name != "nt":
                        os.fchmod(out_fd, int(entry.get("mode", 0o644)))
                finally:
                    os.close(out_fd)
                try:
                    os.replace(temp_name, leaf, src_dir_fd=parent_fd, dst_dir_fd=parent_fd)
                finally:
                    try: os.unlink(temp_name, dir_fd=parent_fd)
                    except FileNotFoundError: pass
                restored.append(rel)
                continue

            # Windows/fallback path: parent chain was validated during preflight
            # and is rechecked here before path-based replacement.
            path = parent_path / leaf
            if path_is_link_or_reparse(parent_path) or not parent_path.is_dir():
                raise RuntimeError("rollback target parent is missing/unsafe")
            if kind == "missing":
                if path_is_link_or_reparse(path) or path.is_file():
                    path.unlink()
                elif path.exists():
                    raise RuntimeError("rollback refuses to remove a directory/non-file created at target path")
                restored.append(rel)
                continue
            if kind != "file" or not isinstance(entry.get("backup"), str):
                raise RuntimeError("invalid file rollback entry")
            if path.exists() and not path_is_link_or_reparse(path) and not path.is_file():
                raise RuntimeError("rollback refuses to replace a directory/non-file target")
            backup = base / str(entry["backup"])
            if not backup.is_file() or path_is_link_or_reparse(backup):
                raise RuntimeError("rollback backup missing")
            fd, temp_name = tempfile.mkstemp(prefix=".ptv-restore-", dir=parent_path)
            temp_path = Path(temp_name)
            try:
                with os.fdopen(fd, "wb") as dst, backup.open("rb") as src:
                    shutil.copyfileobj(src, dst, length=1024 * 1024)
                    dst.flush()
                    try: os.fsync(dst.fileno())
                    except OSError: pass
                if os.name != "nt": os.chmod(temp_path, int(entry.get("mode", 0o644)))
                if _sha256(temp_path) != entry.get("sha256"):
                    raise RuntimeError("restored bytes failed snapshot hash verification")
                os.replace(temp_path, path)
            finally:
                try: temp_path.unlink()
                except FileNotFoundError: pass
            restored.append(rel)
        except Exception as exc:
            errors.append(f"{rel}: {type(exc).__name__}: {exc}")
        finally:
            if parent_fd is not None:
                try: os.close(parent_fd)
                except OSError: pass

    after_targets = _snapshot_declared_paths(root, target_paths)
    target_changes = _snapshot_changes(before_targets, after_targets)
    after_fp = _git_worktree_fingerprint(root)
    git_restored = None if before_fp is None or after_fp is None else before_fp == after_fp
    if errors:
        status = "FAIL"
    elif target_changes:
        status = "PARTIAL"
    elif git_restored is False:
        status = "PARTIAL"
    else:
        status = "PASS"
    remaining = _partial_state(
        root,
        before_fp=before_fp,
        before_dirty=before_dirty,
        before_targets=before_targets,
        target_paths=target_paths,
    )
    return {
        "status": status,
        "trigger": trigger,
        "restored_paths": sorted(restored),
        "errors": errors,
        "remaining_project_delta": remaining,
        "verification": "git_worktree+declared_targets" if git_restored is not None else "declared_targets_only",
    }


def _maybe_rollback(
    root: Path,
    *,
    preflight: dict[str, object],
    rollback_temp: tempfile.TemporaryDirectory[str] | None,
    rollback_snapshot: dict[str, object] | None,
    before_fp: str | None,
    before_dirty: dict[str, str],
    before_targets: dict[str, dict[str, object]],
    target_paths: list[str],
    trigger: str,
) -> dict[str, object] | None:
    config = preflight.get("rollback") if isinstance(preflight, dict) else None
    if not isinstance(config, dict) or rollback_temp is None or rollback_snapshot is None:
        return None
    allowed = config.get("on")
    if not isinstance(allowed, list) or trigger not in allowed:
        return {"status": "SKIPPED", "trigger": trigger, "reason": "trigger_not_enabled"}
    result = _restore_rollback_snapshot(
        root, rollback_temp, rollback_snapshot,
        before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets,
        target_paths=target_paths, trigger=trigger,
    )
    status = result.get("status")
    if status == "PASS":
        print(f"ROLLBACK: PASS | restored {len(result.get('restored_paths') or [])} declared target(s) to pre-patch state")
    else:
        print(f"ROLLBACK: {status} | automatic restore could not fully return the project to its pre-patch state", file=sys.stderr)
        for err in result.get("errors") or []:
            print(f"  rollback error: {err}", file=sys.stderr)
        remaining = result.get("remaining_project_delta")
        if isinstance(remaining, dict):
            for rel in remaining.get("changed_paths") or []:
                print(f"  remaining change: {rel}", file=sys.stderr)
    return result

def _base_result(source: Path, patch_sha256: str | None = None) -> dict[str, object]:
    return {
        "format": "python-patch-tool-patch-result",
        "format_version": 1,
        "tool_version": VERSION,
        "patch_file": source.name,
        "patch_sha256": patch_sha256,
        "started_at": _utc_now(),
        "finished_at": None,
        "status": "RUNNING",
        "rc": None,
        "stage": "start",
        "preflight": None,
        "diagnosis": None,
        "partial_modification": {"detected": False, "changed_paths": [], "evidence": "preflight_not_started"},
        "rollback": None,
    }


def _finish_result(result: dict[str, object], *, status: str, rc: int, stage: str, diagnosis: dict[str, object] | None = None, partial: dict[str, object] | None = None) -> int:
    pf = result.get("preflight")
    if isinstance(pf, dict) and "_resolved_validation_profiles" in pf:
        result["preflight"] = {k:v for k,v in pf.items() if not str(k).startswith("_")}
    result["status"] = status
    result["rc"] = int(rc)
    result["stage"] = stage
    result["finished_at"] = _utc_now()
    if diagnosis is not None:
        result["diagnosis"] = diagnosis
    if partial is not None:
        result["partial_modification"] = partial
    _write_patch_result(result)
    return int(rc)


def _prepare_package(root: Path, source: Path, *, skip_validation: bool = False):
    temp_dir: tempfile.TemporaryDirectory[str] | None = None
    manifest: dict = {}
    extracted: Path | None = None
    if source.suffix.lower() == ".py":
        kind = "python"
        payload = source
        preflight = {
            "status": "PASS",
            "tool_version": VERSION,
            "warnings": ["standalone legacy Python patch has no PATCH_TOOL_MANIFEST.json compatibility/schema metadata"],
            "checks": [{"kind": "legacy_standalone_python", "status": "PASS"}],
            "target_paths": [],
            "legacy_standalone": True,
            "package_format": "legacy_v4_standalone",
            "project_scope_verified": False,
        }
        return temp_dir, extracted, manifest, kind, payload, None, preflight
    if not (source.suffix.lower() == ".zip" or source.name.lower().endswith((".tar.gz", ".tgz"))):
        raise PatchSchemaError(f"unsupported PATCH extension: {source.name}", kind="package_invalid")
    temp_dir = tempfile.TemporaryDirectory(prefix="ptv-patch-")
    extracted = Path(temp_dir.name)
    if source.suffix.lower() == ".zip":
        _safe_extract_zip(source, extracted)
    else:
        _safe_extract_tar(source, extracted)
    manifest_path = extracted / "PATCH_TOOL_MANIFEST.json"
    manifest, kind, payload, legacy_scripts = _find_payload(extracted, archive_name=source.name)
    if not manifest_path.is_file():
        if kind != "python":
            raise PatchSchemaError(
                "legacy archive without PATCH_TOOL_MANIFEST.json must contain a positively recognized Python patch entrypoint",
                kind="manifest_missing",
            )
        preflight = {
            "status": "PASS",
            "tool_version": VERSION,
            "warnings": ["legacy v4 archive has no PATCH_TOOL_MANIFEST.json; project scope is unverified"],
            "checks": [{"kind": "legacy_archive_python", "status": "PASS", "script_count": len(legacy_scripts)}],
            "target_paths": [],
            "legacy_archive": True,
            "legacy_scripts": legacy_scripts,
            "package_format": "legacy_v4",
            "project_scope_verified": False,
        }
        return temp_dir, extracted, {}, kind, payload, None, preflight
    ops_data = _read_json(payload, "PATCH_TOOL_OPS.json") if kind == "ops" else None
    preflight = run_preflight(root, manifest, extracted=extracted, kind=kind, payload=payload, ops_data=ops_data, skip_validation=skip_validation)
    if kind == "ops" and isinstance(ops_data, dict):
        execution = manifest.get("execution") if isinstance(manifest.get("execution"), dict) else {}
        ops_timeout = int(execution.get("timeout_seconds", 900))
        ops_report = _diagnose_ops_managed(root, payload, ops_timeout)
        preflight["ops_dry_run"] = ops_report
        checks = preflight.get("checks")
        if isinstance(checks, list):
            checks.append({"kind": "ops_dry_run", **ops_report})
        if ops_report.get("status") != "PASS":
            issue = {
                "kind": str(ops_report.get("kind") or "patch_operation_failed"),
                "path": ops_report.get("path"),
                "field": f"PATCH_TOOL_OPS.json:{ops_report.get('operation') or '-'}",
                "message": str(ops_report.get("message") or "OPS dry-run failed"),
            }
            if ops_report.get("expected") is not None:
                issue["expected"] = ops_report.get("expected")
            preflight["status"] = "FAIL"
            preflight["issues"] = [issue]
            raise PatchSchemaError(
                f"OPS dry-run failed before payload: operation={ops_report.get('operation','-')} path={ops_report.get('path','-')}: {ops_report.get('message','')}",
                kind=str(ops_report.get("kind") or "patch_operation_failed"),
                path=str(ops_report.get("path")) if ops_report.get("path") else None,
                issues=[issue],
                report=preflight,
            )
    return temp_dir, extracted, manifest, kind, payload, ops_data, preflight


def _diagnose_ops_managed(root: Path, payload: Path, timeout: int) -> dict[str, object]:
    worker = Path(__file__).resolve().parent / "python_patch_ops_worker.py"
    fd, name = tempfile.mkstemp(prefix="ptv-ops-diagnose-", suffix=".json")
    os.close(fd)
    result_path = Path(name)
    try:
        try: result_path.unlink()
        except FileNotFoundError: pass
        outcome = _run_managed_process([
            sys.executable, str(worker), "--project-root", str(root),
            "--ops-json", str(payload), "--mode", "diagnose", "--result", str(result_path),
        ], cwd=root, timeout=max(1, timeout))
        rc = outcome.effective_rc
        if outcome.lingering_descendants:
            return {"status": "FAIL", "kind": "tool_error", "message": "OPS dry-run worker left descendant processes; descendants were terminated", "operations": 0}
        if outcome.timed_out:
            return {"status": "FAIL", "kind": "patch_operation_timeout", "message": f"OPS dry-run exceeded timeout_seconds={timeout}", "operations": 0}
        if result_path.is_file():
            try:
                data = json.loads(result_path.read_text(encoding="utf-8"))
                if isinstance(data, dict):
                    return data
            except Exception as exc:
                return {"status": "FAIL", "kind": "tool_error", "message": f"cannot parse OPS dry-run result: {type(exc).__name__}: {exc}"}
        return {"status": "FAIL", "kind": "tool_error", "message": f"OPS dry-run worker failed rc={rc} without result"}
    finally:
        try: result_path.unlink()
        except FileNotFoundError: pass


def _diagnostic_class(kind: str) -> str:
    kind = str(kind or "unknown")
    if kind in {"source_drift", "anchor_mismatch"}:
        return "SOURCE_DRIFT"
    if kind in {
        "schema_invalid", "manifest_missing", "package_invalid", "ops_invalid",
        "resource_missing", "tool_version_incompatible", "rollback_contract_invalid",
        "rollback_parent_missing", "rollback_path_unsafe", "command_missing",
        "worktree_requirement", "worktree_dirty", "patch_operation_failed",
        "project_identity_invalid", "project_identity_unconfigured", "project_mismatch",
        "project_config_invalid", "validation_profile_invalid", "validation_profile_missing",
    }:
        return "PATCH_INVALID"
    return "TOOL_ERROR"


def _print_issues(issues: object, *, indent: str = "  ") -> None:
    if not isinstance(issues, list) or not issues:
        return
    print(f"{indent}Issues ({len(issues)}):")
    for issue in issues[:80]:
        if not isinstance(issue, dict):
            continue
        field = issue.get("field") or issue.get("path") or "-"
        kind = issue.get("kind") or "issue"
        print(f"{indent}  - [{kind}] {field}: {issue.get('message','')}")
        if "expected" in issue or "actual" in issue:
            print(f"{indent}      expected={issue.get('expected','-')} actual={issue.get('actual','-')}")
        suggestion = issue.get("suggestion")
        if suggestion:
            print(f"{indent}      suggestion: {suggestion}")
    if len(issues) > 80:
        print(f"{indent}  ... {len(issues)-80} more")


def _preflight_failure_payload(exc: PatchSchemaError) -> dict[str, object]:
    affected = sorted({
        str(x.get("path")) for x in getattr(exc, "issues", [])
        if isinstance(x, dict) and x.get("path")
    })
    if not affected and exc.path:
        affected = [exc.path]
    report = getattr(exc, "report", None)
    preflight: dict[str, object] = dict(report) if isinstance(report, dict) else {}
    preflight.update({"status": "FAIL", "kind": exc.kind, "message": str(exc), "affected_paths": affected})
    if getattr(exc, "issues", None):
        preflight["issues"] = exc.issues
    return preflight


def _print_preflight_report(source: Path, manifest: dict, kind: str, preflight: dict[str, object], *, inspect_only: bool = False) -> None:
    patch = manifest.get("patch") if isinstance(manifest, dict) else None
    patch_id = patch.get("id") if isinstance(patch, dict) else source.stem
    targets = preflight.get("target_paths") or []
    prefix = "INSPECT" if inspect_only else "PREFLIGHT"
    print(f"{prefix}: PASS | {source.name} | id={patch_id} | payload={kind} | targets={len(targets)} | tool={VERSION}")
    compat = manifest.get("compatibility") if isinstance(manifest, dict) else None
    if isinstance(compat, dict) and compat:
        print(
            f"  Compatibility: min={compat.get('min_tool_version','-')} "
            f"max={compat.get('max_tool_version','-')} tested={compat.get('max_tested_version','-')}"
        )
    file_checks = [x for x in (preflight.get("checks") or []) if isinstance(x, dict) and x.get("kind") == "file"]
    if file_checks:
        print("  Source assumptions:")
        for check in file_checks[:80]:
            status = check.get("status", "PASS")
            rel = check.get("path", "-")
            print(f"    - [{status}] {rel} | exists expected={check.get('expected_exists','-')} actual={check.get('actual_exists','-')}")
            if check.get("expected_sha256") is not None:
                print(f"      sha256 expected={check.get('expected_sha256')} actual={check.get('actual_sha256','-')}")
            anchors = check.get("anchors")
            if isinstance(anchors, list):
                passed = sum(1 for a in anchors if isinstance(a, dict) and a.get("status") == "PASS")
                missing = sum(1 for a in anchors if isinstance(a, dict) and a.get("status") != "PASS")
                print(f"      anchors pass={passed} missing={missing}")
    if targets:
        print("  Targets:")
        for rel in targets[:40]:
            print(f"    - {rel}")
        if len(targets) > 40:
            print(f"    ... {len(targets)-40} more")
    rollback = preflight.get("rollback")
    if isinstance(rollback, dict):
        print(
            f"  Rollback: opt-in | targets={len(rollback.get('targets') or [])} "
            f"on={','.join(rollback.get('on') or [])} max_bytes={rollback.get('max_total_bytes')}"
        )
    for warning in preflight.get("warnings") or []:
        print(f"  WARNING: {warning}")


def _inspect_patch(root: Path, source: Path, *, verb: str = "INSPECT") -> int:
    result = _base_result(source)
    if path_is_link_or_reparse(source) or not source.is_file():
        print(f"{verb} RESULT: PATCH_INVALID — project unchanged | input is not a regular non-symlink file: {source}", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=2, stage="input", diagnosis={"kind": "package_invalid", "message": "input is not a regular non-symlink file", "affected_paths": []})
    temp_dir = None
    input_temp = None
    try:
        input_temp, execution_source, input_sha = _snapshot_patch_input(source)
        result["patch_sha256"] = input_sha
        temp_dir, _extracted, manifest, kind, _payload, ops_data, preflight = _prepare_package(root, execution_source)
        result["preflight"] = preflight
        result["manifest_patch"] = manifest.get("patch") if isinstance(manifest, dict) else None
        result["recovery"] = manifest.get("recovery") if isinstance(manifest, dict) else None
        _print_preflight_report(source, manifest, kind, preflight, inspect_only=True)
        if kind == "ops" and isinstance(preflight.get("ops_dry_run"), dict):
            ops_report = preflight["ops_dry_run"]
            print(
                f"  OPS dry-run: PASS | operations={ops_report.get('operations',0)} "
                f"patched={ops_report.get('patched',0)} created={ops_report.get('created',0)} unchanged={ops_report.get('unchanged',0)}"
            )
        pp = manifest.get("post_patch") if isinstance(manifest, dict) else None
        if isinstance(pp, dict) and pp.get("commands"):
            print("  Post-patch commands:")
            for cmd in pp.get("commands"):
                name = cmd.get("name") or " ".join(cmd.get("argv") or [])
                print(f"    - {name}")
        on_failure = manifest.get("on_failure") if isinstance(manifest, dict) else None
        if isinstance(on_failure, dict) and on_failure.get("commands"):
            print("  Failure-only commands (run only after execution failure):")
            for cmd in on_failure.get("commands"):
                name = cmd.get("name") or " ".join(cmd.get("argv") or [])
                print(f"    - {name}")
        profiles = preflight.get("validation_profiles") if isinstance(preflight, dict) else None
        if isinstance(profiles, list) and profiles:
            print("  Trusted validation profiles:")
            for profile in profiles:
                if isinstance(profile, dict):
                    print(f"    - {profile.get('name')} (local trusted command)")
        git = manifest.get("git") if isinstance(manifest, dict) else None
        if isinstance(git, dict) and git:
            print(f"  Git policy: add={git.get('add','off')} commit={git.get('commit','off')} push={git.get('push','off')}")
        print(f"{verb} RESULT: READY_TO_APPLY — project unchanged")
        return _finish_result(result, status="PASS", rc=0, stage="validate", diagnosis={"kind": "ready_to_apply", "message": "project unchanged", "affected_paths": []}, partial={"detected": False, "changed_paths": [], "evidence": "read_only_validate"})
    except PatchSchemaError as exc:
        classification = _diagnostic_class(exc.kind)
        report = getattr(exc, "report", None)
        if isinstance(report, dict):
            print(f"{verb}: preflight diagnostics for {source.name}")
            for check in report.get("checks") or []:
                if not isinstance(check, dict) or check.get("kind") != "file":
                    continue
                print(f"  - [{check.get('status','-')}] {check.get('path','-')}")
                if check.get("expected_sha256") is not None:
                    print(f"      sha256 expected={check.get('expected_sha256')} actual={check.get('actual_sha256','-')}")
                anchors = check.get("anchors")
                if isinstance(anchors, list):
                    missing = [a for a in anchors if isinstance(a, dict) and a.get("status") != "PASS"]
                    print(f"      anchors pass={len(anchors)-len(missing)} missing={len(missing)}")
        failure = _preflight_failure_payload(exc)
        result["preflight"] = failure
        diagnosis = {"kind": exc.kind, "message": str(exc), "affected_paths": list(failure.get("affected_paths") or [])}
        _print_issues(getattr(exc, "issues", None))
        print(f"{verb} RESULT: {classification} — project unchanged | {exc.kind}: {exc}", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=2, stage="preflight", diagnosis=diagnosis, partial={"detected": False, "changed_paths": [], "evidence": "read_only_validate"})
    except Exception as exc:
        print(f"{verb} RESULT: TOOL_ERROR — project unchanged | {type(exc).__name__}: {exc}", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=2, stage="validate", diagnosis={"kind": "tool_error", "message": f"{type(exc).__name__}: {exc}", "affected_paths": []}, partial={"detected": False, "changed_paths": [], "evidence": "read_only_validate"})
    finally:
        if temp_dir is not None:
            temp_dir.cleanup()
        if input_temp is not None:
            input_temp.cleanup()


def _preview_patch(root: Path, source: Path) -> int:
    """Read-only preflight plus deterministic OPS diff preview on a private mirror."""
    result = _base_result(source)
    temp_dir = None
    input_temp = None
    mirror_temp = None
    try:
        input_temp, execution_source, input_sha = _snapshot_patch_input(source)
        result["patch_sha256"] = input_sha
        temp_dir, _extracted, manifest, kind, payload, ops_data, preflight = _prepare_package(root, execution_source)
        result["preflight"] = preflight
        result["manifest_patch"] = manifest.get("patch") if isinstance(manifest, dict) else None
        _print_preflight_report(source, manifest, kind, preflight, inspect_only=True)
        if kind != "ops" or not isinstance(ops_data, dict):
            print("PREVIEW DIFF: unavailable for arbitrary Python payload; preflight is read-only and project is unchanged")
            print("PREVIEW RESULT: READY_TO_APPLY — project unchanged")
            return _finish_result(result, status="PASS", rc=0, stage="preview", diagnosis={"kind":"ready_to_apply","message":"project unchanged; deterministic diff unavailable for Python payload","affected_paths":[]}, partial={"detected":False,"changed_paths":[],"evidence":"read_only_preview"})
        mirror_temp = tempfile.TemporaryDirectory(prefix="ptv-ops-preview-")
        mirror = Path(mirror_temp.name)
        targets = [x for x in preflight.get("target_paths") or [] if isinstance(x, str)]
        before: dict[str, bytes | None] = {}
        for rel in targets:
            src = root.joinpath(*PurePosixPath(rel).parts)
            dst = mirror.joinpath(*PurePosixPath(rel).parts)
            if src.exists():
                if path_is_link_or_reparse(src) or not src.is_file():
                    raise PatchSchemaError(f"preview target is not a regular non-symlink file: {rel}", kind="rollback_path_unsafe", path=rel)
                raw = src.read_bytes()
                before[rel] = raw
                dst.parent.mkdir(parents=True, exist_ok=True)
                dst.write_bytes(raw)
            else:
                before[rel] = None
        execution = manifest.get("execution") if isinstance(manifest.get("execution"), dict) else {}
        timeout = int(execution.get("timeout_seconds", 900))
        worker = Path(__file__).resolve().parent / "python_patch_ops_worker.py"
        result_path = mirror / ".ptv-preview-result.json"
        outcome = _run_managed_process([
            sys.executable, str(worker), "--project-root", str(mirror), "--ops-json", str(payload),
            "--patch-name", str((manifest.get("patch") or {}).get("id") or source.stem), "--result", str(result_path),
        ], cwd=mirror, timeout=timeout)
        rc = outcome.effective_rc
        if outcome.lingering_descendants:
            raise PatchSchemaError("OPS preview worker left descendant processes; descendants were terminated", kind="tool_error")
        if outcome.timed_out:
            raise PatchSchemaError(f"OPS preview worker exceeded timeout_seconds={timeout}", kind="patch_operation_timeout")
        if rc:
            raise PatchSchemaError(f"OPS preview worker failed rc={rc}", kind="patch_operation_failed")
        print("PREVIEW DIFF (private mirror):")
        changed = 0
        for rel in targets:
            dst = mirror.joinpath(*PurePosixPath(rel).parts)
            after = dst.read_bytes() if dst.is_file() and not dst.is_symlink() else None
            old = before.get(rel)
            if old == after:
                continue
            changed += 1
            print(f"--- {rel}")
            if old is None:
                print(f"+++ {rel} (new file)")
            elif after is None:
                print(f"+++ {rel} (deleted)")
            if (old is None or len(old) <= 512*1024) and (after is None or len(after) <= 512*1024):
                try:
                    old_lines = [] if old is None else old.decode("utf-8").splitlines(keepends=True)
                    new_lines = [] if after is None else after.decode("utf-8").splitlines(keepends=True)
                    for line in difflib.unified_diff(old_lines, new_lines, fromfile=f"a/{rel}", tofile=f"b/{rel}", n=3):
                        sys.stdout.write(line)
                    if new_lines and not new_lines[-1].endswith("\n"):
                        sys.stdout.write("\n")
                    continue
                except UnicodeDecodeError:
                    pass
            print("  [binary/large file changed; unified text diff omitted]")
        if changed == 0:
            print("  [no target byte changes predicted]")
        print(f"PREVIEW RESULT: READY_TO_APPLY — project unchanged | predicted_changed={changed}")
        return _finish_result(result, status="PASS", rc=0, stage="preview", diagnosis={"kind":"ready_to_apply","message":f"project unchanged; predicted_changed={changed}","affected_paths":[]}, partial={"detected":False,"changed_paths":[],"evidence":"private_mirror_preview"})
    except PatchSchemaError as exc:
        print(f"PREVIEW RESULT: {_diagnostic_class(exc.kind)} — project unchanged | {exc.kind}: {exc}", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=2, stage="preview", diagnosis={"kind":exc.kind,"message":str(exc),"affected_paths":[exc.path] if exc.path else []}, partial={"detected":False,"changed_paths":[],"evidence":"read_only_preview"})
    except Exception as exc:
        print(f"PREVIEW RESULT: TOOL_ERROR — project unchanged | {type(exc).__name__}: {exc}", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=2, stage="preview", diagnosis={"kind":"tool_error","message":f"{type(exc).__name__}: {exc}","affected_paths":[]}, partial={"detected":False,"changed_paths":[],"evidence":"read_only_preview"})
    finally:
        if mirror_temp is not None: mirror_temp.cleanup()
        if temp_dir is not None: temp_dir.cleanup()
        if input_temp is not None: input_temp.cleanup()


def _execute_patch(root: Path, source: Path, *, no_validation: bool = False) -> int:
    result = _base_result(source)
    if path_is_link_or_reparse(source) or not source.is_file():
        print(f"ERROR: PATCH input is not a regular non-symlink file: {source}", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=2, stage="input", diagnosis={"kind": "package_invalid", "message": "input is not a regular non-symlink file", "affected_paths": []})

    global _ACTIVE_TERMINATION_SIGNAL
    _ACTIVE_TERMINATION_SIGNAL = None
    input_temp: tempfile.TemporaryDirectory[str] | None = None
    execution_source: Path | None = None
    input_sha: str | None = None
    temp_dir: tempfile.TemporaryDirectory[str] | None = None
    rollback_temp: tempfile.TemporaryDirectory[str] | None = None
    rollback_snapshot: dict[str, object] | None = None
    mutation_lock = None
    manifest: dict = {}
    preflight: dict[str, object] = {}
    target_paths: list[str] = []
    before_fp: str | None = None
    before_dirty: dict[str, str] = {}
    before_targets: dict[str, dict[str, object]] = {}
    try:
        try:
            input_temp, execution_source, input_sha = _snapshot_patch_input(source)
            result["patch_sha256"] = input_sha
            # Selection/package snapshotting may run concurrently, but preflight +
            # source mutation is serialized per project to prevent lost updates.
            mutation_lock = _acquire_project_mutation_lock(root)
            temp_dir, _extracted, manifest, kind, payload, ops_data, preflight = _prepare_package(root, execution_source, skip_validation=no_validation)
            result["preflight"] = preflight
            result["manifest_patch"] = manifest.get("patch") if isinstance(manifest, dict) else None
            result["recovery"] = manifest.get("recovery") if isinstance(manifest, dict) else None
            if preflight.get("legacy_archive") or preflight.get("legacy_standalone"):
                result["package_format"] = preflight.get("package_format")
                result["legacy_v4_compatibility"] = True
                result["project_scope_verified"] = False
                print(f"PACKAGE_FORMAT: {preflight.get('package_format')}")
                print("LEGACY_V4_COMPATIBILITY: TRUE")
                print("PROJECT_SCOPE_VERIFIED: FALSE")
                if preflight.get("legacy_scripts"):
                    print("LEGACY_V4_SCRIPTS: " + ", ".join(preflight.get("legacy_scripts") or []))
            _print_preflight_report(source, manifest, kind, preflight)
        except PatchSchemaError as exc:
            preflight_failure = _preflight_failure_payload(exc)
            affected_paths = list(preflight_failure.get("affected_paths") or [])
            diagnosis = {"kind": exc.kind, "message": str(exc), "affected_paths": affected_paths}
            if getattr(exc, "issues", None):
                diagnosis["issues"] = exc.issues
            result["preflight"] = preflight_failure
            print(f"PREFLIGHT FAIL — project unchanged | {_diagnostic_class(exc.kind)} | {exc.kind}: {exc}", file=sys.stderr)
            _print_issues(getattr(exc, "issues", None))
            return _finish_result(result, status="FAIL", rc=2, stage="preflight", diagnosis=diagnosis, partial={"detected": False, "changed_paths": [], "evidence": "preflight_failed_before_payload"})
        except Exception as exc:
            diagnosis = {"kind": "package_invalid", "message": f"{type(exc).__name__}: {exc}", "affected_paths": []}
            result["preflight"] = {"status": "FAIL", "kind": "package_invalid", "message": diagnosis["message"]}
            print(f"PREFLIGHT FAIL — project unchanged | {diagnosis['message']}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=2, stage="preflight", diagnosis=diagnosis, partial={"detected": False, "changed_paths": [], "evidence": "preflight_failed_before_payload"})

        target_paths = list(preflight.get("target_paths") or [])
        before_fp = _git_worktree_fingerprint(root)
        before_dirty = _dirty_paths(root)
        before_targets = _snapshot_declared_paths(root, target_paths)
        rollback_cfg = preflight.get("rollback") if isinstance(preflight, dict) else None
        if isinstance(rollback_cfg, dict):
            try:
                rollback_temp, rollback_snapshot = _prepare_rollback_snapshot(root, rollback_cfg)
                print(f"ROLLBACK SNAPSHOT: READY | targets={len(rollback_cfg.get('targets') or [])} | bytes={rollback_snapshot.get('total_bytes',0)}")
            except PatchSchemaError as exc:
                diagnosis = {"kind": exc.kind, "message": str(exc), "affected_paths": [exc.path] if exc.path else []}
                result["preflight"] = {
                    **(preflight if isinstance(preflight, dict) else {}),
                    "status": "FAIL", "kind": exc.kind, "message": str(exc),
                    "affected_paths": diagnosis["affected_paths"],
                }
                print(f"PREFLIGHT FAIL — project unchanged | {exc.kind}: {exc}", file=sys.stderr)
                return _finish_result(result, status="FAIL", rc=2, stage="preflight", diagnosis=diagnosis, partial={"detected": False, "changed_paths": [], "evidence": "rollback_snapshot_failed_before_payload"})

        timeout = 900
        if isinstance(manifest.get("execution"), dict):
            timeout = int(manifest["execution"].get("timeout_seconds", timeout))
        print(f"PATCH: {source.name}")
        print("Execution: IN-PLACE (SANDBOX/worktree disabled)")
        result["stage"] = "payload"
        if kind == "command_only":
            print("PATCH PAYLOAD: COMMAND_ONLY_PACKAGE (no source payload)")
            rc = 0
            payload_diag = None
        elif kind == "python":
            outcome = _execute_python(payload, root, timeout)
            rc = outcome.effective_rc
            payload_diag = None
            if outcome.lingering_descendants:
                payload_diag = {"kind": "patch_payload_lingering_descendants", "message": "Python PATCH left descendant processes after leader exit; descendants were terminated", "affected_paths": []}
                print("ERROR: Python PATCH left descendant processes after leader exit; descendants were terminated", file=sys.stderr)
            elif outcome.timed_out:
                payload_diag = {"kind": "patch_payload_timeout", "message": f"Python PATCH execution exceeded timeout_seconds={timeout}", "affected_paths": []}
                print(f"ERROR: Python PATCH timeout after {timeout}s", file=sys.stderr)
        else:
            if ops_data is None:
                ops_data = _read_json(payload, "PATCH_TOOL_OPS.json")
            patch_name = str((manifest.get("patch") or {}).get("id") or ops_data.get("patch_name") or source.stem)
            worker = Path(__file__).resolve().parent / "python_patch_ops_worker.py"
            worker_result = Path(temp_dir.name if temp_dir is not None else tempfile.gettempdir()) / f"ptv-ops-result-{os.getpid()}-{time.time_ns()}.json"
            outcome = _run_managed_process([
                sys.executable, str(worker), "--project-root", str(root),
                "--ops-json", str(payload), "--patch-name", patch_name, "--result", str(worker_result),
            ], cwd=root, timeout=timeout)
            rc = outcome.effective_rc
            payload_diag = None
            if outcome.lingering_descendants:
                payload_diag = {"kind": "patch_operation_lingering_descendants", "message": "OPS worker left descendant processes after leader exit; descendants were terminated", "affected_paths": []}
                print("ERROR: PATCH OPS worker left descendant processes; descendants were terminated", file=sys.stderr)
            worker_data: dict[str, object] = {}
            if worker_result.is_file():
                try:
                    loaded = json.loads(worker_result.read_text(encoding="utf-8"))
                    if isinstance(loaded, dict):
                        worker_data = loaded
                except Exception:
                    worker_data = {}
                try: worker_result.unlink()
                except OSError: pass
            if outcome.lingering_descendants:
                pass  # payload_diag already records the containment failure above.
            elif rc == 0:
                stats = worker_data.get("stats") if isinstance(worker_data.get("stats"), dict) else {}
                print(f"PATCH OPS: patched={stats.get('patched',0)} created={stats.get('created',0)} unchanged={stats.get('unchanged',0)}")
            elif outcome.timed_out:
                payload_diag = {"kind": "patch_operation_timeout", "message": f"OPS execution exceeded timeout_seconds={timeout}", "affected_paths": []}
                print(f"ERROR: PATCH OPS timeout after {timeout}s", file=sys.stderr)
            elif worker_data.get("kind") == "patch_failure":
                rel = str(worker_data.get("path") or "")
                msg = str(worker_data.get("message") or "OPS patch failure")
                synthetic = PatchFailure(rel, msg, expected=worker_data.get("expected") if isinstance(worker_data.get("expected"),str) else None, strategy=worker_data.get("strategy") if isinstance(worker_data.get("strategy"),str) else None)
                payload_diag = {"kind": _failure_kind_from_patch_failure(synthetic), "message": msg, "affected_paths": [rel] if rel else []}
                print(f"ERROR: {rel + ': ' if rel else ''}{msg}", file=sys.stderr)
            else:
                payload_diag = {"kind": str(worker_data.get("kind") or "patch_operation_failed"), "message": str(worker_data.get("message") or f"OPS worker failed rc={rc}"), "affected_paths": []}
                print(f"ERROR: {payload_diag['message']}", file=sys.stderr)
        if rc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            diagnosis = payload_diag or {"kind": "patch_payload_failed", "message": f"payload returned rc={rc}", "affected_paths": list(partial.get("changed_paths") or [])}
            if partial.get("detected") is True:
                print("!!! PARTIAL MODIFICATION DETECTED: project changed before PATCH failed !!!", file=sys.stderr)
                for rel in partial.get("changed_paths") or []:
                    print(f"  changed: {rel}", file=sys.stderr)
            elif partial.get("detected") is None:
                print("[PTV WARNING] PATCH failed but partial-modification state is unknown outside Git because no targets were declared.", file=sys.stderr)
            rollback_result = _maybe_rollback(
                root, preflight=preflight, rollback_temp=rollback_temp, rollback_snapshot=rollback_snapshot,
                before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets,
                target_paths=target_paths, trigger="payload_failure",
            )
            if rollback_result is not None:
                result["rollback"] = rollback_result
                diagnosis["rollback_status"] = rollback_result.get("status")
                if rollback_result.get("status") == "PASS":
                    partial = rollback_result.get("remaining_project_delta") if isinstance(rollback_result.get("remaining_project_delta"), dict) else partial
            partial = _apply_on_failure_commands(
                root, manifest, result, before_fp=before_fp, before_dirty=before_dirty,
                before_targets=before_targets, target_paths=target_paths, current_partial=partial,
            )
            print(f"RUN SUMMARY: FAIL | {source.name} rc={rc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=rc, stage="payload", diagnosis=diagnosis, partial=partial)

        payload_delta = _partial_state(
            root, before_fp=before_fp, before_dirty=before_dirty,
            before_targets=before_targets, target_paths=target_paths,
        )
        changed = False if kind == "command_only" else payload_delta.get("detected") is not False
        result["stage"] = "post_patch"
        post_report = _run_post_patch(root, manifest, changed=changed)
        if post_report is not None:
            result["post_patch"] = post_report
        rc = int((post_report or {}).get("rc") or 0)
        if rc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            post_rows = post_report.get("commands") if isinstance(post_report, dict) else None
            failed_row = None
            if isinstance(post_rows, list):
                failed_row = next((row for row in reversed(post_rows) if isinstance(row, dict) and row.get("status") != "PASS"), None)
            if isinstance(failed_row, dict) and failed_row.get("timed_out") is True:
                post_kind = "post_patch_timeout"
                post_message = f"post_patch command timed out (rc={rc})"
            elif isinstance(failed_row, dict) and failed_row.get("lingering_descendants") is True:
                post_kind = "post_patch_lingering_descendants"
                post_message = f"post_patch command left descendant processes (rc={rc})"
            else:
                post_kind = "post_patch_failed"
                post_message = f"post_patch returned rc={rc}"
            diagnosis = {"kind": post_kind, "message": post_message, "affected_paths": list(partial.get("changed_paths") or [])}
            if partial.get("detected") is True:
                print("!!! PARTIAL MODIFICATION DETECTED: patch payload changed project before validation failed !!!", file=sys.stderr)
            rollback_result = _maybe_rollback(
                root, preflight=preflight, rollback_temp=rollback_temp, rollback_snapshot=rollback_snapshot,
                before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets,
                target_paths=target_paths, trigger="post_patch_failure",
            )
            if rollback_result is not None:
                result["rollback"] = rollback_result
                diagnosis["rollback_status"] = rollback_result.get("status")
                if rollback_result.get("status") == "PASS":
                    partial = rollback_result.get("remaining_project_delta") if isinstance(rollback_result.get("remaining_project_delta"), dict) else partial
            partial = _apply_on_failure_commands(
                root, manifest, result, before_fp=before_fp, before_dirty=before_dirty,
                before_targets=before_targets, target_paths=target_paths, current_partial=partial,
            )
            print(f"RUN SUMMARY: FAIL | post_patch rc={rc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=rc, stage="post_patch", diagnosis=diagnosis, partial=partial)

        result["stage"] = "validation"
        if no_validation:
            preflight["_resolved_validation_profiles"] = []
            preflight["_resolved_validation_rerun_policy"] = {"max_commands": 0, "on_timeout": False}
            result["validation_selection"] = {
                "status": "DISABLED_BY_CLI", "mode": "off",
                "changed_paths": [], "requested_profiles": [],
                "auto_profiles": [], "final_profiles": [], "matched_rules": [],
            }
        else:
            try:
                from python_patch_project_state import resolve_effective_validation_profiles
                validation_delta = _partial_state(
                    root, before_fp=before_fp, before_dirty=before_dirty,
                    before_targets=before_targets, target_paths=target_paths,
                )
                validation_profiles, selection_report = resolve_effective_validation_profiles(
                    root, manifest, list(validation_delta.get("changed_paths") or []), disabled=False
                )
                preflight["_resolved_validation_profiles"] = validation_profiles
                rerun_policy = selection_report.get("diagnostic_rerun") if isinstance(selection_report, dict) else None
                preflight["_resolved_validation_rerun_policy"] = rerun_policy if isinstance(rerun_policy, dict) else {"max_commands": 1, "on_timeout": False}
                result["validation_selection"] = selection_report
            except Exception as exc:
                partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
                diagnosis = {
                    "kind": getattr(exc, "kind", "validation_selection_invalid"),
                    "message": f"validation selection failed after payload: {type(exc).__name__}: {exc}",
                    "affected_paths": list(partial.get("changed_paths") or []),
                }
                rollback_result = _maybe_rollback(
                    root, preflight=preflight, rollback_temp=rollback_temp, rollback_snapshot=rollback_snapshot,
                    before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets,
                    target_paths=target_paths, trigger="post_patch_failure",
                )
                if rollback_result is not None:
                    result["rollback"] = rollback_result
                    diagnosis["rollback_status"] = rollback_result.get("status")
                    if rollback_result.get("status") == "PASS" and isinstance(rollback_result.get("remaining_project_delta"), dict):
                        partial = rollback_result["remaining_project_delta"]
                partial = _apply_on_failure_commands(
                    root, manifest, result, before_fp=before_fp, before_dirty=before_dirty,
                    before_targets=before_targets, target_paths=target_paths, current_partial=partial,
                )
                print(f"RUN SUMMARY: FAIL | validation selection: {exc}", file=sys.stderr)
                return _finish_result(result, status="FAIL", rc=2, stage="validation", diagnosis=diagnosis, partial=partial)
        validation_report = _run_validation_profiles(root, preflight)
        if validation_report is not None:
            result["validation"] = validation_report
        rc = int((validation_report or {}).get("rc") or 0)
        if rc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            validation_rows = validation_report.get("profiles") if isinstance(validation_report, dict) else None
            failed_profile = None
            if isinstance(validation_rows, list):
                failed_profile = next((row for row in reversed(validation_rows) if isinstance(row, dict) and row.get("status") != "PASS"), None)
            if isinstance(failed_profile, dict) and failed_profile.get("timed_out") is True:
                validation_kind = "validation_profile_timeout"
                validation_message = f"trusted validation profile timed out (rc={rc})"
            elif isinstance(failed_profile, dict) and failed_profile.get("lingering_descendants") is True:
                validation_kind = "validation_profile_lingering_descendants"
                validation_message = f"trusted validation profile left descendant processes (rc={rc})"
            else:
                validation_kind = "validation_profile_failed"
                validation_message = f"trusted validation profile returned rc={rc}"
            diagnosis = {"kind": validation_kind, "message": validation_message, "affected_paths": list(partial.get("changed_paths") or [])}
            rollback_result = _maybe_rollback(
                root, preflight=preflight, rollback_temp=rollback_temp, rollback_snapshot=rollback_snapshot,
                before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets,
                target_paths=target_paths, trigger="post_patch_failure",
            )
            if rollback_result is not None:
                result["rollback"] = rollback_result
                diagnosis["rollback_status"] = rollback_result.get("status")
                if rollback_result.get("status") == "PASS" and isinstance(rollback_result.get("remaining_project_delta"), dict):
                    partial = rollback_result["remaining_project_delta"]
            partial = _apply_on_failure_commands(
                root, manifest, result, before_fp=before_fp, before_dirty=before_dirty,
                before_targets=before_targets, target_paths=target_paths, current_partial=partial,
            )
            print(f"RUN SUMMARY: FAIL | validation profile rc={rc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=rc, stage="validation", diagnosis=diagnosis, partial=partial)

        after_dirty = _dirty_paths(root)
        result["stage"] = "git"
        rc = _run_git_policy(root, manifest, before_dirty, after_dirty)
        if rc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            diagnosis = {"kind": "git_policy_failed", "message": f"git policy returned rc={rc}", "affected_paths": list(partial.get("changed_paths") or [])}
            if partial.get("detected") is True:
                print("!!! PARTIAL MODIFICATION DETECTED: project changed before Git policy failed !!!", file=sys.stderr)
            partial = _apply_on_failure_commands(
                root, manifest, result, before_fp=before_fp, before_dirty=before_dirty,
                before_targets=before_targets, target_paths=target_paths, current_partial=partial,
            )
            print(f"RUN SUMMARY: FAIL | git policy rc={rc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=rc, stage="git", diagnosis=diagnosis, partial=partial)

        result["stage"] = "archive"
        try:
            if execution_source is None or input_sha is None:
                raise RuntimeError("executed PATCH snapshot is unavailable")
            archived, queue_lifecycle = _archive_success(root, source, execution_source, input_sha)
            result["queue_lifecycle"] = queue_lifecycle
        except Exception as exc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            diagnosis = {"kind": "archive_failed", "message": str(exc), "affected_paths": list(partial.get("changed_paths") or [])}
            print(f"ERROR: PATCH succeeded but queue archive failed: {exc}", file=sys.stderr)
            partial = _apply_on_failure_commands(
                root, manifest, result, before_fp=before_fp, before_dirty=before_dirty,
                before_targets=before_targets, target_paths=target_paths, current_partial=partial,
            )
            return _finish_result(result, status="FAIL", rc=3, stage="archive", diagnosis=diagnosis, partial=partial)
        print(f"[INFO] PATCH ARCHIVED: {archived.relative_to(root).as_posix()}")
        if queue_lifecycle in {"replacement_restored"} or str(queue_lifecycle).startswith("replacement_preserved:"):
            print(
                f"[PTV v{VERSION} WARNING] queue input changed while PATCH was running; "
                "the exact executed package was archived and the replacement was kept for a later run.",
                file=sys.stderr,
            )
        print(f"RUN SUMMARY: PASS | {source.name}")
        project_delta = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
        result["project_delta"] = project_delta
        return _finish_result(
            result, status="PASS", rc=0, stage="complete",
            diagnosis={"kind": "none", "message": "PATCH completed", "affected_paths": []},
            partial={"detected": False, "changed_paths": [], "evidence": "not_applicable_on_success"},
        )
    except KeyboardInterrupt:
        signum = _ACTIVE_TERMINATION_SIGNAL or signal.SIGINT
        rc = 143 if signum == signal.SIGTERM else 130
        stage = str(result.get("stage") or "unknown")
        label = "SIGTERM" if signum == signal.SIGTERM else "Ctrl+C"
        print(f"INTERRUPTED by {label}", file=sys.stderr)
        diagnosis: dict[str, object] = {"kind": "interrupted", "message": label, "affected_paths": []}
        partial: dict[str, object] = {"detected": False, "changed_paths": [], "evidence": "interrupted_before_payload"}
        if stage in {"payload", "post_patch", "validation", "git", "archive"} and (target_paths or before_fp is not None):
            partial = _partial_state(
                root, before_fp=before_fp, before_dirty=before_dirty,
                before_targets=before_targets, target_paths=target_paths,
            )
            diagnosis["affected_paths"] = list(partial.get("changed_paths") or [])
        trigger = "payload_failure" if stage == "payload" else ("post_patch_failure" if stage in {"post_patch", "validation"} else None)
        if trigger is not None:
            rollback_result = _maybe_rollback(
                root, preflight=preflight, rollback_temp=rollback_temp, rollback_snapshot=rollback_snapshot,
                before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets,
                target_paths=target_paths, trigger=trigger,
            )
            if rollback_result is not None:
                result["rollback"] = rollback_result
                diagnosis["rollback_status"] = rollback_result.get("status")
                if rollback_result.get("status") == "PASS" and isinstance(rollback_result.get("remaining_project_delta"), dict):
                    partial = rollback_result["remaining_project_delta"]
        return _finish_result(result, status="FAIL", rc=rc, stage=stage, diagnosis=diagnosis, partial=partial)
    except Exception as exc:
        stage = str(result.get("stage") or "unknown")
        print(f"ERROR: PATCH execution failed: {type(exc).__name__}: {exc}", file=sys.stderr)
        partial: dict[str, object] = {"detected": None, "changed_paths": [], "evidence": "internal_error_state_unknown"}
        execution_started = stage in {"payload", "post_patch", "validation", "git", "archive"}
        if execution_started and (target_paths or before_fp is not None):
            try:
                partial = _partial_state(
                    root, before_fp=before_fp, before_dirty=before_dirty,
                    before_targets=before_targets, target_paths=target_paths,
                )
            except Exception:
                partial = {"detected": None, "changed_paths": [], "evidence": "internal_error_partial_recompute_failed"}
        diagnosis = {
            "kind": "internal_error",
            "message": f"{type(exc).__name__}: {exc}",
            "affected_paths": list(partial.get("changed_paths") or []),
        }
        # An unexpected exception after source execution began is still a PATCH
        # failure. Preserve the same recovery semantics as an ordinary payload /
        # post-validation failure, then run failure-only commands. Preflight and
        # package errors deliberately never reach this branch with execution_started.
        trigger = "payload_failure" if stage == "payload" else ("post_patch_failure" if stage in {"post_patch", "validation"} else None)
        if trigger is not None:
            try:
                rollback_result = _maybe_rollback(
                    root, preflight=preflight, rollback_temp=rollback_temp, rollback_snapshot=rollback_snapshot,
                    before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets,
                    target_paths=target_paths, trigger=trigger,
                )
                if rollback_result is not None:
                    result["rollback"] = rollback_result
                    diagnosis["rollback_status"] = rollback_result.get("status")
                    if rollback_result.get("status") == "PASS" and isinstance(rollback_result.get("remaining_project_delta"), dict):
                        partial = rollback_result["remaining_project_delta"]
            except Exception as rollback_exc:
                result["rollback"] = {"status": "FAIL", "error": f"{type(rollback_exc).__name__}: {rollback_exc}"}
                diagnosis["rollback_status"] = "FAIL"
                partial = {"detected": None, "changed_paths": list(partial.get("changed_paths") or []), "evidence": "internal_error_rollback_failed_state_unknown"}
        if execution_started:
            try:
                partial = _apply_on_failure_commands(
                    root, manifest, result, before_fp=before_fp, before_dirty=before_dirty,
                    before_targets=before_targets, target_paths=target_paths, current_partial=partial,
                )
            except KeyboardInterrupt:
                raise
            except Exception as failure_exc:
                result["on_failure"] = {"status": "FAIL", "rc": 2, "error": f"{type(failure_exc).__name__}: {failure_exc}"}
                partial = {"detected": None, "changed_paths": list(partial.get("changed_paths") or []), "evidence": "internal_error_on_failure_failed_state_unknown"}
        return _finish_result(result, status="FAIL", rc=2, stage=stage, diagnosis=diagnosis, partial=partial)
    finally:
        _release_project_mutation_lock(mutation_lock)
        if rollback_temp is not None:
            rollback_temp.cleanup()
        if temp_dir is not None:
            temp_dir.cleanup()
        if input_temp is not None:
            input_temp.cleanup()

def _paths(root: Path) -> int:
    print(f"Project root : {root}")
    print(f"Patch folder : {root / 'patchs'}")
    print(f"Patched files: {root / 'patchs' / 'patched'}")
    print(f"Tools folder : {root / 'tools'}")
    print(f"Patch helper : {Path(__file__).resolve().parent / 'python_patch_utils.py'}")
    return 0


def main(argv: list[str] | None = None) -> int:
    for managed_signal in (getattr(signal, "SIGTERM", None), getattr(signal, "SIGBREAK", None)):
        if managed_signal is None:
            continue
        try:
            signal.signal(managed_signal, _sigterm_as_interrupt)
        except (ValueError, OSError):
            pass
    args = list(sys.argv[1:] if argv is None else argv)
    if not args:
        print("ERROR: normal interactive use is ./tools/run_python_patches.sh with no arguments", file=sys.stderr)
        return 2
    if args[0] in {"version", "--version"}:
        print(VERSION); return 0
    if args[0] == "paths":
        root = Path.cwd().resolve(); return _paths(root)
    if args[0] == "health-search":
        from python_patch_health import run_search_health
        return run_search_health(Path.cwd().resolve(), compact=False)
    if args[0] in {"help", "--help", "-h"}:
        print("Python Patch Tool self-contained core. Normal use: ./tools/run_python_patches.sh")
        print("Interactive selector supports inspect/dry-run with key i. Direct validator: validate --patch <package>. Read-only diff preview: preview --patch <package>.")
        print("Search discovery self-test: ./tools/run_python_patches.sh health-search")
        return 0

    inspect_mode = False
    validate_mode = False
    preview_mode = False
    if args and args[0] in {"inspect", "validate", "preview"}:
        inspect_mode = args[0] == "inspect"
        validate_mode = args[0] == "validate"
        preview_mode = args[0] == "preview"
        args = args[1:]

    ap = argparse.ArgumentParser(add_help=False)
    ap.add_argument("--patch")
    ap.add_argument("--transaction", default="off")
    ap.add_argument("--no-validation", action="store_true")
    ns, unknown = ap.parse_known_args(args)
    if unknown:
        print(f"ERROR: unsupported self-contained runner argument(s): {' '.join(unknown)}", file=sys.stderr)
        return 2
    if ns.transaction != "off":
        print("ERROR: SANDBOX/worktree transaction modes are permanently unsupported; use --transaction off", file=sys.stderr)
        return 2
    if not ns.patch:
        print("ERROR: --patch is required for PATCH execution/inspect/validate/preview", file=sys.stderr)
        return 2
    root = Path.cwd().resolve()
    raw = Path(ns.patch)
    source = raw if raw.is_absolute() else root / raw
    if inspect_mode:
        return _inspect_patch(root, source, verb="INSPECT")
    if validate_mode:
        return _inspect_patch(root, source, verb="VALIDATE")
    if preview_mode:
        return _preview_patch(root, source)
    return _execute_patch(root, source, no_validation=bool(ns.no_validation))


if __name__ == "__main__":
    raise SystemExit(main())
