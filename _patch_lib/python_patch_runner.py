#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import os
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
from datetime import datetime, timezone

from python_patch_utils import PatchFailure, diagnose_ops, finish_failure, run_ops
from python_patch_package_schema import PatchSchemaError, path_is_link_or_reparse, run_preflight, sha256_file

VERSION = "6.16.0"
_ACTIVE_TERMINATION_SIGNAL: int | None = None


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
    text = name.replace("\\", "/")
    rel = PurePosixPath(text)
    if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts):
        raise ValueError(f"unsafe archive member: {name}")
    return rel


def _safe_extract_zip(path: Path, dest: Path) -> None:
    with zipfile.ZipFile(path) as zf:
        for info in zf.infolist():
            if info.is_dir():
                continue
            rel = _safe_archive_name(info.filename)
            mode = (info.external_attr >> 16) & 0xFFFF
            if stat.S_ISLNK(mode):
                raise ValueError(f"archive symlink is not allowed: {info.filename}")
            target = dest.joinpath(*rel.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            with zf.open(info) as src, target.open("wb") as out:
                shutil.copyfileobj(src, out)


def _safe_extract_tar(path: Path, dest: Path) -> None:
    with tarfile.open(path, "r:*") as tf:
        for member in tf.getmembers():
            if member.isdir():
                continue
            rel = _safe_archive_name(member.name)
            if not member.isfile():
                raise ValueError(f"archive non-regular member is not allowed: {member.name}")
            src = tf.extractfile(member)
            if src is None:
                raise ValueError(f"cannot read archive member: {member.name}")
            target = dest.joinpath(*rel.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            with src, target.open("wb") as out:
                shutil.copyfileobj(src, out)


def _read_json(path: Path, label: str) -> dict:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        raise ValueError(f"invalid {label}: {type(exc).__name__}: {exc}") from exc
    if not isinstance(data, dict):
        raise ValueError(f"{label} root must be an object")
    return data


def _find_payload(extracted: Path) -> tuple[dict, str, Path]:
    manifest_path = extracted / "PATCH_TOOL_MANIFEST.json"
    manifest = _read_json(manifest_path, "PATCH_TOOL_MANIFEST.json") if manifest_path.is_file() else {}
    ops = extracted / "PATCH_TOOL_OPS.json"
    py_files: list[Path] = []
    for path in extracted.rglob("*.py"):
        rel = path.relative_to(extracted)
        if not path.is_file() or "__MACOSX" in rel.parts or "resources" in rel.parts:
            continue
        py_files.append(path)
    if ops.is_file() and py_files:
        raise ValueError("package contains both PATCH_TOOL_OPS.json and Python patch entrypoint")
    if ops.is_file():
        return manifest, "ops", ops
    if len(py_files) != 1:
        raise ValueError(f"package must contain exactly one Python patch entrypoint when OPS is absent (found {len(py_files)})")
    return manifest, "python", py_files[0]


def _git_bytes(root: Path, args: list[str], *, timeout: int = 30) -> bytes:
    proc = subprocess.run(["git", *args], cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout)
    if proc.returncode != 0:
        return b""
    return proc.stdout


def _git_worktree_fingerprint(root: Path) -> str | None:
    if not (root / ".git").exists():
        return None
    h = hashlib.sha256()
    h.update(_git_bytes(root, ["diff", "--binary", "HEAD", "--", "."], timeout=60))
    raw = _git_bytes(root, ["ls-files", "--others", "--exclude-standard", "-z"], timeout=30)
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
    proc = subprocess.run(
        ["git", "status", "--porcelain=v1", "-z", "--untracked-files=all"],
        cwd=root, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=30,
    )
    if proc.returncode != 0:
        return {}
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
        if status[0] in {"R", "C"} and i < len(parts):
            name = parts[i].decode("utf-8", errors="surrogateescape"); i += 1
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
        subprocess.run(argv, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5, check=False)
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
        _signal_subprocess_group(proc, initial_signal)
        deadline = time.monotonic() + max(0.1, grace_seconds)
        while time.monotonic() < deadline:
            if proc.poll() is not None:
                break
            time.sleep(0.05)
        # taskkill /T closes descendants as well as the leader. /F is the final
        # containment barrier before rollback or returning timeout/Ctrl+C.
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


def _run_managed_process(argv: list[str], *, cwd: Path, env: dict[str, str] | None = None, timeout: int) -> int:
    """Run payload/validation in an isolated process group.

    Timeout/interruption terminates descendants before control returns to the
    rollback path. This prevents an orphaned child from modifying the project
    after the tool has already reported FAIL or restored a snapshot.
    """
    kwargs: dict[str, object] = {"cwd": cwd, "env": env}
    if os.name != "nt":
        kwargs["start_new_session"] = True
    elif hasattr(subprocess, "CREATE_NEW_PROCESS_GROUP"):
        kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    proc = subprocess.Popen(argv, **kwargs)
    try:
        return proc.wait(timeout=max(1, int(timeout)))
    except subprocess.TimeoutExpired:
        term = signal.SIGTERM if hasattr(signal, "SIGTERM") else signal.SIGINT
        _quiesce_managed_group(proc, term, grace_seconds=1.0)
        return 124
    except KeyboardInterrupt:
        signum = _ACTIVE_TERMINATION_SIGNAL or signal.SIGINT
        _quiesce_managed_group(proc, signum, grace_seconds=1.0)
        raise


def _run_argv(root: Path, cmd: dict) -> int:
    argv = cmd.get("argv")
    if not isinstance(argv, list) or not argv or not all(isinstance(x, str) and x for x in argv):
        raise ValueError("post_patch command argv must be a non-empty string array")
    cwd_raw = cmd.get("cwd", ".")
    if not isinstance(cwd_raw, str) or not cwd_raw:
        raise ValueError("post_patch cwd must be a string")
    rel = PurePosixPath(cwd_raw)
    if rel.is_absolute() or any(p == ".." for p in rel.parts):
        raise ValueError(f"unsafe post_patch cwd: {cwd_raw}")
    cwd = root if cwd_raw == "." else root.joinpath(*rel.parts)
    if not cwd.is_dir():
        raise ValueError(f"post_patch cwd not found: {cwd_raw}")
    timeout = int(cmd.get("timeout_seconds", 300))
    name = cmd.get("name") or " ".join(argv)
    print(f"POST PATCH: {name}", flush=True)
    rc = _run_managed_process(argv, cwd=cwd, timeout=max(1, timeout))
    if rc == 124:
        print(f"ERROR: post_patch command timeout after {timeout}s", file=sys.stderr)
    return rc if rc >= 0 else 128 + abs(rc)


def _run_post_patch(root: Path, manifest: dict, *, changed: bool) -> int:
    pp = manifest.get("post_patch")
    if not isinstance(pp, dict):
        return 0
    commands = pp.get("commands")
    if not isinstance(commands, list) or not commands:
        return 0
    if not changed and not bool(pp.get("run_when_no_changes", False)):
        print("POST PATCH: skipped because payload produced no detected project delta")
        return 0
    for cmd in commands:
        if not isinstance(cmd, dict):
            print("ERROR: invalid post_patch command object", file=sys.stderr)
            return 2
        try:
            rc = _run_argv(root, cmd)
        except Exception as exc:
            print(f"ERROR: post_patch command invalid: {exc}", file=sys.stderr)
            return 2
        if rc:
            return rc
    return 0


def _run_git_policy(root: Path, manifest: dict, before_dirty: dict[str, str], after_dirty: dict[str, str]) -> int:
    policy = manifest.get("git")
    if not isinstance(policy, dict) or not (root / ".git").exists():
        return 0
    touched = _touched_paths(before_dirty, after_dirty)
    fail_on_error = bool(policy.get("fail_on_error", True))
    try:
        if policy.get("add") not in {None, "off", False} and touched:
            proc = subprocess.run(["git", "add", "--", *touched], cwd=root)
            if proc.returncode:
                raise RuntimeError(f"git add failed rc={proc.returncode}")
        commit_mode = policy.get("commit")
        if commit_mode == "auto" and touched:
            message = policy.get("commit_message")
            if not isinstance(message, str) or not message.strip():
                raise RuntimeError("git.commit=auto requires commit_message")
            # --only confines the commit to paths touched by this patch run.
            proc = subprocess.run(["git", "commit", "-m", message, "--only", "--", *touched], cwd=root)
            if proc.returncode not in {0, 1}:
                raise RuntimeError(f"git commit failed rc={proc.returncode}")
        if policy.get("push") == "auto":
            proc = subprocess.run(["git", "push"], cwd=root)
            if proc.returncode:
                raise RuntimeError(f"git push failed rc={proc.returncode}")
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1 if fail_on_error else 0
    return 0


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


def _execute_python(script: Path, root: Path, timeout: int) -> int:
    env = os.environ.copy()
    lib = str(Path(__file__).resolve().parent)
    env["PYTHONPATH"] = lib + (os.pathsep + env["PYTHONPATH"] if env.get("PYTHONPATH") else "")
    rc = _run_managed_process([sys.executable, str(script)], cwd=root, env=env, timeout=max(1, timeout))
    return rc if rc >= 0 else 128 + abs(rc)


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
    changed = sorted(set(_touched_paths(before_dirty, after_dirty)) | set(_snapshot_changes(before_targets, after_targets)))
    if before_fp is not None and after_fp is not None:
        detected: bool | None = before_fp != after_fp
    elif target_paths:
        detected = bool(_snapshot_changes(before_targets, after_targets))
    else:
        detected = None
    return {
        "detected": detected,
        "changed_paths": changed,
        "evidence": "git_worktree+declared_targets" if before_fp is not None else ("declared_targets" if target_paths else "insufficient_non_git_target_declaration"),
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


def _prepare_package(root: Path, source: Path):
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
    if not manifest_path.is_file():
        raise PatchSchemaError("archive PATCH requires root PATCH_TOOL_MANIFEST.json", kind="manifest_missing")
    manifest, kind, payload = _find_payload(extracted)
    ops_data = _read_json(payload, "PATCH_TOOL_OPS.json") if kind == "ops" else None
    preflight = run_preflight(root, manifest, extracted=extracted, kind=kind, payload=payload, ops_data=ops_data)
    if kind == "ops" and isinstance(ops_data, dict):
        ops_report = diagnose_ops(root, ops_data)
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


def _diagnostic_class(kind: str) -> str:
    kind = str(kind or "unknown")
    if kind in {"source_drift", "anchor_mismatch"}:
        return "SOURCE_DRIFT"
    if kind in {
        "schema_invalid", "manifest_missing", "package_invalid", "ops_invalid",
        "resource_missing", "tool_version_incompatible", "rollback_contract_invalid",
        "rollback_parent_missing", "rollback_path_unsafe", "command_missing",
        "worktree_requirement", "worktree_dirty", "patch_operation_failed",
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
    if path_is_link_or_reparse(source) or not source.is_file():
        print(f"{verb} RESULT: PATCH_INVALID — project unchanged | input is not a regular non-symlink file: {source}", file=sys.stderr)
        return 2
    temp_dir = None
    input_temp = None
    try:
        input_temp, execution_source, _input_sha = _snapshot_patch_input(source)
        temp_dir, _extracted, manifest, kind, _payload, ops_data, preflight = _prepare_package(root, execution_source)
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
        git = manifest.get("git") if isinstance(manifest, dict) else None
        if isinstance(git, dict) and git:
            print(f"  Git policy: add={git.get('add','off')} commit={git.get('commit','off')} push={git.get('push','off')}")
        print(f"{verb} RESULT: READY_TO_APPLY — project unchanged")
        return 0
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
        _print_issues(getattr(exc, "issues", None))
        print(f"{verb} RESULT: {classification} — project unchanged | {exc.kind}: {exc}", file=sys.stderr)
        return 2
    except Exception as exc:
        print(f"{verb} RESULT: TOOL_ERROR — project unchanged | {type(exc).__name__}: {exc}", file=sys.stderr)
        return 2
    finally:
        if temp_dir is not None:
            temp_dir.cleanup()
        if input_temp is not None:
            input_temp.cleanup()


def _execute_patch(root: Path, source: Path) -> int:
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
            temp_dir, _extracted, manifest, kind, payload, ops_data, preflight = _prepare_package(root, execution_source)
            result["preflight"] = preflight
            result["manifest_patch"] = manifest.get("patch") if isinstance(manifest, dict) else None
            result["recovery"] = manifest.get("recovery") if isinstance(manifest, dict) else None
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
        if kind == "python":
            rc = _execute_python(payload, root, timeout)
            payload_diag = None
        else:
            try:
                if ops_data is None:
                    ops_data = _read_json(payload, "PATCH_TOOL_OPS.json")
                patch_name = str((manifest.get("patch") or {}).get("id") or ops_data.get("patch_name") or source.stem)
                state = run_ops(root, ops_data, patch_name=patch_name)
                print(f"PATCH OPS: patched={state.stats.patched} created={state.stats.created} unchanged={state.stats.unchanged}")
                rc = 0
                payload_diag = None
            except PatchFailure as exc:
                rc = finish_failure(exc)
                payload_diag = {"kind": _failure_kind_from_patch_failure(exc), "message": str(exc), "affected_paths": [exc.rel_path] if exc.rel_path else []}
            except Exception as exc:
                rc = finish_failure(exc)
                payload_diag = {"kind": "patch_operation_failed", "message": f"{type(exc).__name__}: {exc}", "affected_paths": []}
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
            print(f"RUN SUMMARY: FAIL | {source.name} rc={rc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=rc, stage="payload", diagnosis=diagnosis, partial=partial)

        after_payload_fp = _git_worktree_fingerprint(root)
        changed = before_fp is None or after_payload_fp is None or before_fp != after_payload_fp
        result["stage"] = "post_patch"
        rc = _run_post_patch(root, manifest, changed=changed)
        if rc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            diagnosis = {"kind": "post_patch_failed", "message": f"post_patch returned rc={rc}", "affected_paths": list(partial.get("changed_paths") or [])}
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
            print(f"RUN SUMMARY: FAIL | post_patch rc={rc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=rc, stage="post_patch", diagnosis=diagnosis, partial=partial)

        after_dirty = _dirty_paths(root)
        result["stage"] = "git"
        rc = _run_git_policy(root, manifest, before_dirty, after_dirty)
        if rc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            diagnosis = {"kind": "git_policy_failed", "message": f"git policy returned rc={rc}", "affected_paths": list(partial.get("changed_paths") or [])}
            if partial.get("detected") is True:
                print("!!! PARTIAL MODIFICATION DETECTED: project changed before Git policy failed !!!", file=sys.stderr)
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
        if stage in {"payload", "post_patch", "git", "archive"} and (target_paths or before_fp is not None):
            partial = _partial_state(
                root, before_fp=before_fp, before_dirty=before_dirty,
                before_targets=before_targets, target_paths=target_paths,
            )
            diagnosis["affected_paths"] = list(partial.get("changed_paths") or [])
        trigger = "payload_failure" if stage == "payload" else ("post_patch_failure" if stage == "post_patch" else None)
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
        print(f"ERROR: PATCH execution failed: {type(exc).__name__}: {exc}", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=2, stage=str(result.get("stage") or "unknown"), diagnosis={"kind": "internal_error", "message": f"{type(exc).__name__}: {exc}", "affected_paths": []})
    finally:
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
    if hasattr(signal, "SIGTERM"):
        signal.signal(signal.SIGTERM, _sigterm_as_interrupt)
    args = list(sys.argv[1:] if argv is None else argv)
    if not args:
        print("ERROR: normal interactive use is ./tools/run_python_patches.sh with no arguments", file=sys.stderr)
        return 2
    if args[0] in {"version", "--version"}:
        print(VERSION); return 0
    if args[0] == "paths":
        root = Path.cwd().resolve(); return _paths(root)
    if args[0] in {"help", "--help", "-h"}:
        print("Python Patch Tool self-contained core. Normal use: ./tools/run_python_patches.sh")
        print("Interactive selector supports inspect/dry-run with key i. Direct validator: validate --patch <package>.")
        return 0

    inspect_mode = False
    validate_mode = False
    if args and args[0] in {"inspect", "validate"}:
        inspect_mode = args[0] == "inspect"
        validate_mode = args[0] == "validate"
        args = args[1:]

    ap = argparse.ArgumentParser(add_help=False)
    ap.add_argument("--patch")
    ap.add_argument("--transaction", default="off")
    ns, unknown = ap.parse_known_args(args)
    if unknown:
        print(f"ERROR: unsupported self-contained runner argument(s): {' '.join(unknown)}", file=sys.stderr)
        return 2
    if ns.transaction != "off":
        print("ERROR: SANDBOX/worktree transaction modes are permanently unsupported; use --transaction off", file=sys.stderr)
        return 2
    if not ns.patch:
        print("ERROR: --patch is required for PATCH execution/inspect/validate", file=sys.stderr)
        return 2
    root = Path.cwd().resolve()
    raw = Path(ns.patch)
    source = raw if raw.is_absolute() else root / raw
    if inspect_mode:
        return _inspect_patch(root, source, verb="INSPECT")
    if validate_mode:
        return _inspect_patch(root, source, verb="VALIDATE")
    return _execute_patch(root, source)


if __name__ == "__main__":
    raise SystemExit(main())
