#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import select
import signal
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import shutil
from itertools import islice
from datetime import datetime, timezone
import unicodedata
import zipfile
from dataclasses import dataclass
from pathlib import Path

from python_patch_collect_schema import CollectSchemaError, validate_request_data
from python_patch_health import print_health
from python_patch_batch import (
    BatchPlanError, PatchMeta, load_patch_meta, topo_order, previous_failed_identity,
    validate_previous_failure_declaration, transaction_compatibility, snapshot_targets,
    restore_targets, snapshot_package_bytes, requeue_packages, capture_compare_snapshot,
    build_diff_artifact,
)

try:
    import termios
    import tty
except Exception:
    termios = tty = None

try:
    import msvcrt
except Exception:
    msvcrt = None


VERSION = "6.17.1"
MAX_COLLECT_REQUEST_JSON_BYTES = 1024 * 1024
MAX_PATCH_MARKER_BYTES = 1024 * 1024
MAX_PATCH_MARKER_FILES = 8
COLLECT_JSON_RE = re.compile(r"^CODE_COLLECTION_REQUEST(?:_[A-Za-z0-9._-]+)?\.json$", re.I)
PATCH_PY_RE = re.compile(r"^patch_.*\.py$", re.I)
PATCH_MARKERS = (b"python_patch_utils", b"run_patch", b"PATCH_NAME")
_ANSI_RE = re.compile(
    r"(?:\x1B\][^\x07]*(?:\x07|\x1B\\))"
    r"|(?:\x1B\[[0-?]*[ -/]*[@-~])"
    r"|(?:\x1B[@-_])"
)


@dataclass(frozen=True)
class QueueItem:
    name: str
    kind: str
    detail: str = ""


class QueueSafetyError(RuntimeError):
    pass


@dataclass(frozen=True)
class LocalDuplicate:
    item: QueueItem
    history_name: str
    sha256: str
    ignored_name: str = ""


@dataclass(frozen=True)
class SessionDuplicate:
    item: QueueItem
    canonical_name: str
    sha256: str
    removed: bool


_LAST_EXECUTION_DETAILS: list[dict[str, object]] = []
_ACTIVE_RUN_ID: str | None = None


class _PatchChildSignal(BaseException):
    def __init__(self, signum: int):
        super().__init__(signum)
        self.signum = int(signum)


def _raise_patch_child_signal(signum, _frame):
    raise _PatchChildSignal(int(signum))


def _forward_patch_signal(proc: subprocess.Popen, signum: int) -> None:
    if proc.poll() is not None:
        return
    try:
        if os.name != "nt":
            os.killpg(proc.pid, signum)
        else:
            proc.send_signal(signum)
    except (ProcessLookupError, OSError):
        pass

MAX_PATCH_CAPTURE_BYTES = 8 * 1024 * 1024
MAX_HANDOFF_SOURCE_FILE_BYTES = 2 * 1024 * 1024
MAX_HANDOFF_SOURCE_TOTAL_BYTES = 20 * 1024 * 1024
RUN_HISTORY_LIMIT = 30


def _utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def _safe_slug(value: str, limit: int = 80) -> str:
    text = re.sub(r"[^A-Za-z0-9._-]+", "_", str(value)).strip("._-")
    return text[:limit] or "run"


def _atomic_json(path: Path, data: dict[str, object]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix=".ptv-json-", suffix=".tmp", dir=path.parent)
    os.close(fd)
    temp = Path(name)
    try:
        temp.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        os.replace(temp, path)
    finally:
        try: temp.unlink()
        except FileNotFoundError: pass


def _reject_duplicate_json_pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError(f"duplicate JSON key: {key}")
        out[key] = value
    return out


def _load_json(path: Path) -> dict[str, object] | None:
    try:
        data = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
        return data if isinstance(data, dict) else None
    except Exception:
        return None


def _project_mutation_lock_key(root: Path) -> str:
    return hashlib.sha256(str(root.resolve()).encode("utf-8", errors="surrogateescape")).hexdigest()[:32]


def _project_mutation_lock_path(root: Path) -> Path:
    base = Path(tempfile.gettempdir()) / "python_patch_tool_locks" / _project_mutation_lock_key(root)
    base.mkdir(parents=True, exist_ok=True)
    return base / "mutation.lock"


def _acquire_batch_mutation_lock(root: Path):
    """Own the same mutation lock used by runner children for an atomic batch."""
    path = _project_mutation_lock_path(root)
    fh = path.open("a+b")
    try:
        fh.seek(0, os.SEEK_END)
        if fh.tell() == 0:
            fh.write(b"0")
            fh.flush()
        fh.seek(0)
        started = time.monotonic()
        if os.name == "nt":
            if msvcrt is None:
                raise RuntimeError("native Windows file locking is unavailable")
            msvcrt.locking(fh.fileno(), msvcrt.LK_LOCK, 1)
        else:
            import fcntl
            fcntl.flock(fh.fileno(), fcntl.LOCK_EX)
        token = os.urandom(24).hex()
        fh.seek(0)
        fh.truncate(0)
        fh.write(token.encode("ascii"))
        fh.flush()
        try: os.fsync(fh.fileno())
        except OSError: pass
        waited = time.monotonic() - started
        if waited >= 0.25:
            print(f"BATCH MUTATION LOCK: acquired after waiting {waited:.1f}s for another PATCH process")
        return fh, _project_mutation_lock_key(root), token
    except Exception:
        fh.close()
        raise


def _release_batch_mutation_lock(fh) -> None:
    if fh is None:
        return
    try:
        try:
            fh.seek(0); fh.truncate(0); fh.write(b"0"); fh.flush()
        except OSError:
            pass
        fh.seek(0)
        if os.name == "nt":
            if msvcrt is not None:
                msvcrt.locking(fh.fileno(), msvcrt.LK_UNLCK, 1)
        else:
            import fcntl
            fcntl.flock(fh.fileno(), fcntl.LOCK_UN)
    finally:
        fh.close()


def _restore_batch_mutation_env(previous: tuple[str | None, str | None]) -> None:
    old_key, old_token = previous
    for name, value in (("PTV_PARENT_MUTATION_LOCK_KEY", old_key), ("PTV_PARENT_MUTATION_LOCK_TOKEN", old_token)):
        if value is None:
            os.environ.pop(name, None)
        else:
            os.environ[name] = value


def _artifact_run_root(root: Path) -> Path:
    cur = root.resolve(strict=True)
    for part in ("artifacts", "patch_tool"):
        nxt = cur / part
        if nxt.exists() or nxt.is_symlink():
            st = nxt.lstat()
            attrs = getattr(st, "st_file_attributes", 0)
            reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
            if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
                raise QueueSafetyError(f"unsafe artifact directory component: {nxt}")
        else:
            try:
                nxt.mkdir(exist_ok=False)
            except FileExistsError:
                st = nxt.lstat()
                attrs = getattr(st, "st_file_attributes", 0)
                reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
                if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
                    raise ValueError(f"unsafe directory component created concurrently: {nxt}")
        cur = nxt
    return cur


def _batch_run_dir(root: Path, run_id: str) -> Path:
    return _artifact_run_root(root) / "runs" / _safe_slug(run_id, 96)


def _batch_item_log_path(root: Path, run_id: str | None, index: int, item: QueueItem) -> Path | None:
    if not run_id:
        return None
    return _batch_run_dir(root, run_id) / "items" / f"{index:03d}_{_safe_slug(item.name, 96)}.log"


def _load_previous_run(root: Path) -> dict[str, object] | None:
    return _load_json(_artifact_run_root(root) / "LAST_RUN.json")


def _resume_items(root: Path, previous: dict[str, object] | None) -> list[str]:
    if not previous:
        return []
    if previous.get("status") == "FAIL":
        raw = previous.get("not_executed")
    elif previous.get("status") in {"CANCELLED", "IDLE"}:
        raw = previous.get("previous_resume_items")
    else:
        return []
    if not isinstance(raw, list):
        return []
    out: list[str] = []
    for value in raw:
        if isinstance(value, str) and (root / "patchs" / value).is_file():
            out.append(value)
    return out


def _print_resume_hint(root: Path, previous: dict[str, object] | None) -> list[str]:
    remain = _resume_items(root, previous)
    if not remain:
        return []
    failed = previous.get("failed_item") or previous.get("previous_failed_item") or "unknown"
    print(f"PREVIOUS RUN: FAIL at {_safe_display(str(failed))} | {len(remain)} selected item(s) remain unexecuted")
    for name in remain[:10]:
        print(f"  - {_safe_display(name)}")
    if len(remain) > 10:
        print(f"  ... {len(remain)-10} more")
    print("No automatic selection was changed; choose explicitly if you want to resume.")
    return remain


def _pins_path(root: Path) -> Path:
    return _artifact_run_root(root) / "PINNED_RUNS.json"


def _load_pinned_runs(root: Path) -> set[str]:
    data = _load_json(_pins_path(root))
    values = data.get("run_ids") if isinstance(data, dict) else None
    return {str(x) for x in values if isinstance(x, str) and x} if isinstance(values, list) else set()


def _save_pinned_runs(root: Path, pins: set[str]) -> None:
    _atomic_json(_pins_path(root), {"format": "ptv-pinned-runs", "version": 1, "run_ids": sorted(pins)})


def _history_entries(root: Path) -> list[tuple[Path, dict[str, object]]]:
    history = _artifact_run_root(root) / "history"
    out: list[tuple[Path, dict[str, object]]] = []
    if history.is_dir():
        for path in sorted(history.glob("*.json"), reverse=True):
            data = _load_json(path)
            if isinstance(data, dict): out.append((path, data))
    return out


def _find_history_entry(root: Path, run_id: str) -> tuple[Path, dict[str, object]] | None:
    for path, data in _history_entries(root):
        if str(data.get("run_id")) == str(run_id): return path, data
    return None


def _cleanup_history(root: Path) -> dict[str, int]:
    pins = _load_pinned_runs(root)
    entries = list(reversed(_history_entries(root)))  # oldest -> newest
    unpinned = [(p, d) for p, d in entries if str(d.get("run_id")) not in pins]
    remove_count = max(0, len(entries) - RUN_HISTORY_LIMIT)
    removed = 0
    for path, data in unpinned:
        if removed >= remove_count: break
        run_id = str(data.get("run_id") or "")
        try: path.unlink(); removed += 1
        except OSError: pass
        run_dir = _batch_run_dir(root, run_id)
        if run_dir.is_dir() and not run_dir.is_symlink():
            try: shutil.rmtree(run_dir)
            except OSError: pass
    return {"removed": removed, "pinned": len(pins), "remaining": len(_history_entries(root))}


def _write_run_report(root: Path, report: dict[str, object]) -> None:
    out = _artifact_run_root(root)
    try:
        _atomic_json(out / "LAST_RUN.json", report)
        history = out / "history"; history.mkdir(parents=True, exist_ok=True)
        stamp = str(report.get("started_at", "run")).replace(":", "").replace("+", "_").replace("-", "").replace(".", "_")
        run_id = _safe_slug(str(report.get("run_id") or "run"), 64)
        _atomic_json(history / f"{stamp}_{run_id}.json", report)
        _cleanup_history(root)
    except Exception as exc:
        print(f"[PTV v{VERSION} WARNING] could not write LAST_RUN/history: {type(exc).__name__}: {exc}", file=sys.stderr)


def _run_patch_child(
    root: Path,
    cmd: list[str],
    item: QueueItem,
    *,
    full_log_path: Path | None = None,
) -> tuple[int, str, dict[str, object] | None]:
    runtime = _artifact_run_root(root) / "runtime"
    runtime.mkdir(parents=True, exist_ok=True)
    token = f"{int(time.time()*1000000)}_{os.getpid()}_{_safe_slug(item.name,48)}"
    result_path = runtime / f"{token}.json"
    env = dict(os.environ)
    env["PTV_PATCH_RESULT_FILE"] = str(result_path)
    env["PYTHONUNBUFFERED"] = "1"
    proc = subprocess.Popen(
        cmd, cwd=root, env=env,
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        text=True, encoding="utf-8", errors="replace", bufsize=1,
        start_new_session=(os.name != "nt"),
    )
    assert proc.stdout is not None
    chunks: list[str] = []
    used = 0
    truncated = False
    log_file = None
    log_error: str | None = None
    if full_log_path is not None:
        try:
            full_log_path.parent.mkdir(parents=True, exist_ok=True)
            log_file = full_log_path.open("w", encoding="utf-8", errors="replace", newline="")
        except Exception as exc:
            log_error = f"{type(exc).__name__}: {exc}"
    interrupted_sig: int | None = None
    old_sigterm = None
    if hasattr(signal, "SIGTERM"):
        try:
            old_sigterm = signal.getsignal(signal.SIGTERM)
            signal.signal(signal.SIGTERM, _raise_patch_child_signal)
        except Exception:
            old_sigterm = None

    def consume(text: str) -> None:
        nonlocal used, truncated
        if not text:
            return
        sys.stdout.write(text); sys.stdout.flush()
        if log_file is not None:
            try:
                log_file.write(text)
                log_file.flush()
            except Exception:
                pass
        raw = text.encode("utf-8", errors="replace")
        if used < MAX_PATCH_CAPTURE_BYTES:
            room = MAX_PATCH_CAPTURE_BYTES - used
            part = raw[:room]
            chunks.append(part.decode("utf-8", errors="replace"))
            used += len(part)
            if len(raw) > room:
                truncated = True
        else:
            truncated = True

    try:
        try:
            for line in iter(proc.stdout.readline, ""):
                if not line:
                    break
                consume(line)
            raw_rc = proc.wait()
        except KeyboardInterrupt:
            interrupted_sig = signal.SIGINT
            _forward_patch_signal(proc, signal.SIGINT)
            try:
                tail, _ = proc.communicate(timeout=30)
                consume(tail or "")
            except subprocess.TimeoutExpired:
                _forward_patch_signal(proc, signal.SIGKILL)
                tail, _ = proc.communicate(timeout=5)
                consume(tail or "")
            raw_rc = 130
        except _PatchChildSignal as exc:
            interrupted_sig = exc.signum
            _forward_patch_signal(proc, exc.signum)
            try:
                tail, _ = proc.communicate(timeout=30)
                consume(tail or "")
            except (subprocess.TimeoutExpired, KeyboardInterrupt, _PatchChildSignal):
                _forward_patch_signal(proc, signal.SIGKILL)
                try:
                    tail, _ = proc.communicate(timeout=5)
                    consume(tail or "")
                except Exception:
                    pass
            raw_rc = 128 + abs(interrupted_sig)
    finally:
        if old_sigterm is not None:
            try: signal.signal(signal.SIGTERM, old_sigterm)
            except Exception: pass
        try: proc.stdout.close()
        except Exception: pass
        if log_file is not None:
            try: log_file.close()
            except Exception: pass

    if truncated:
        chunks.append("\n[PTV: console capture truncated at 8 MiB]\n")
    result = _load_json(result_path)
    try: result_path.unlink()
    except OSError: pass
    if interrupted_sig is not None:
        rc = 128 + abs(interrupted_sig)
    else:
        rc = _normalize_subprocess_rc(raw_rc)
    if log_error and result is not None:
        result.setdefault("report_warnings", []).append(f"full detail log unavailable: {log_error}")
    return rc, "".join(chunks), result


def _safe_handoff_source(root: Path, rel: str) -> Path | None:
    try:
        if not isinstance(rel, str) or not rel or "\\" in rel:
            return None
        pure = Path(rel)
        if pure.is_absolute() or ".." in pure.parts:
            return None
        path = root / pure
        if path.is_symlink() or not path.is_file():
            return None
        resolved = path.resolve(strict=True)
        resolved.relative_to(root.resolve())
        if path.stat().st_size > MAX_HANDOFF_SOURCE_FILE_BYTES:
            return None
        return path
    except Exception:
        return None


def _enrich_patch_diagnosis(patch_result: dict[str, object] | None, console_log: str) -> dict[str, object] | None:
    if patch_result is None:
        return None
    diagnosis = patch_result.get("diagnosis")
    if not isinstance(diagnosis, dict):
        diagnosis = {"kind": "unknown", "message": "no structured diagnosis", "affected_paths": []}
        patch_result["diagnosis"] = diagnosis
    kind = str(diagnosis.get("kind") or "unknown")
    text = console_log.lower()
    if kind in {"patch_payload_failed", "unknown", "internal_error"}:
        if any(x in text for x in ("sha-256 mismatch", "sha256 mismatch", "checksum mismatch", "source drift", "baseline mismatch")):
            diagnosis["kind"] = "source_drift"
        elif any(x in text for x in ("expected block not found", "anchor missing", "anchor not found", "fuzzy block not found", "match is ambiguous")):
            diagnosis["kind"] = "anchor_mismatch"
        elif "syntaxerror" in text or "syntax error" in text:
            diagnosis["kind"] = "syntax_error"
        elif "traceback (most recent call last)" in text:
            diagnosis["kind"] = "python_exception"
    affected = [x for x in (diagnosis.get("affected_paths") or []) if isinstance(x, str)]
    # Backward-compatible python_patch_utils diagnostics print "ERROR: rel/path: message".
    for match in re.finditer(r"(?m)^ERROR:\s+([^:\r\n]+):\s+", console_log):
        rel = match.group(1).strip()
        if rel and not rel.startswith(("http", "/")) and ".." not in Path(rel).parts and rel not in affected:
            affected.append(rel)
    diagnosis["affected_paths"] = affected
    return patch_result


def _create_recovery_collect_request(root: Path, item: QueueItem, patch_result: dict[str, object]) -> Path | None:
    diagnosis = patch_result.get("diagnosis")
    recovery = patch_result.get("recovery")
    if isinstance(recovery, dict) and recovery.get("collect_on_source_drift") is False:
        return None
    if not isinstance(diagnosis, dict) or diagnosis.get("kind") not in {"source_drift", "anchor_mismatch"}:
        return None
    paths = []
    for rel in diagnosis.get("affected_paths") or []:
        if isinstance(rel, str) and _safe_handoff_source(root, rel) is not None and rel not in paths:
            paths.append(rel)
    if not paths:
        return None
    patch_sha = str(patch_result.get("patch_sha256") or "")
    suffix = patch_sha[:12] if patch_sha else hashlib.sha256(item.name.encode()).hexdigest()[:12]
    name = f"CODE_COLLECTION_REQUEST_patch_recovery_{suffix}.zip"
    target = root / "patchs" / name
    request = {
        "id": f"patch-recovery-{suffix}",
        "title": f"Exact current source for failed PATCH {item.name}",
        "actions": [{"type": "pack", "paths": paths}],
    }
    inner = name[:-4] + ".json"

    def existing_same() -> bool:
        try:
            if target.is_symlink() or not target.is_file():
                return False
            with zipfile.ZipFile(target) as zf:
                existing = json.loads(zf.read(inner).decode("utf-8"))
            return existing == request
        except Exception:
            return False

    if target.exists() or target.is_symlink():
        return target if existing_same() else None

    fd, temp_name = tempfile.mkstemp(prefix=".ptv-recovery-", suffix=".zip", dir=root / "patchs")
    os.close(fd)
    temp = Path(temp_name)
    try:
        with zipfile.ZipFile(temp, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            zf.writestr(inner, json.dumps(request, ensure_ascii=False, indent=2) + "\n")
        with zipfile.ZipFile(temp) as zf:
            if zf.testzip() is not None:
                raise ValueError("generated recovery request failed CRC")
        try:
            # Atomic no-overwrite publish when hard links are supported.
            os.link(temp, target)
            return target
        except FileExistsError:
            return target if existing_same() else None
        except OSError:
            # exFAT/FAT/network filesystems may not support hard links. Preserve
            # no-overwrite semantics with O_EXCL and verify the generated ZIP.
            fd = None
            try:
                fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(fd, "wb") as out_fh, temp.open("rb") as in_fh:
                    fd = None
                    shutil.copyfileobj(in_fh, out_fh, length=1024 * 1024)
                    out_fh.flush()
                    try: os.fsync(out_fh.fileno())
                    except OSError: pass
                return target if existing_same() else None
            except FileExistsError:
                return target if existing_same() else None
            finally:
                if fd is not None:
                    try: os.close(fd)
                    except OSError: pass
    except Exception as exc:
        print(f"[PTV v{VERSION} WARNING] could not create recovery COLLECT request: {type(exc).__name__}: {exc}", file=sys.stderr)
        return None
    finally:
        try: temp.unlink()
        except FileNotFoundError: pass
        except OSError: pass

_SENSITIVE_BASENAMES = {".env", ".env.local", "id_rsa", "id_dsa", "id_ed25519", "credentials.json"}
_SENSITIVE_SUFFIXES = (".pem", ".key", ".p12", ".pfx", ".keystore", ".jks")
_SENSITIVE_TEXT_PATTERNS = (
    ("private_key", re.compile(r"-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----", re.I)),
    ("authorization_header", re.compile(r"\bAuthorization\s*:\s*(?:Bearer|Basic)\s+\S+", re.I)),
    ("credential_assignment", re.compile(r"\b(?:api[_-]?key|access[_-]?token|secret|password)\s*[:=]\s*[^\s,;]+", re.I)),
)


def _sensitive_handoff_warnings(console_log: str, sources: list[tuple[str, Path]]) -> list[str]:
    """Return warning labels only; never copy detected secret values into metadata."""
    warnings: list[str] = []
    for rel, src in sources:
        low_name = src.name.lower()
        if low_name in _SENSITIVE_BASENAMES or low_name.endswith(_SENSITIVE_SUFFIXES):
            warnings.append(f"sensitive filename included exactly: {rel}")
        try:
            if src.stat().st_size <= 4 * 1024 * 1024:
                text = src.read_bytes()[:1024 * 1024].decode("utf-8", errors="ignore")
                for label, pattern in _SENSITIVE_TEXT_PATTERNS:
                    if pattern.search(text):
                        warnings.append(f"possible {label} in exact source attachment: {rel}")
        except OSError:
            pass
    sample = console_log[:4 * 1024 * 1024]
    for label, pattern in _SENSITIVE_TEXT_PATTERNS:
        if pattern.search(sample):
            warnings.append(f"possible {label} in console.log")
    return list(dict.fromkeys(warnings))


def _create_fail_handoff(
    root: Path,
    item: QueueItem,
    rc: int,
    console_log: str,
    patch_result: dict[str, object] | None,
    recovery_request: Path | None,
) -> Path | None:
    if isinstance(patch_result, dict):
        recovery = patch_result.get("recovery")
        if isinstance(recovery, dict) and recovery.get("fail_handoff") is False:
            return None
    out = _artifact_run_root(root) / "fail_handoffs"
    out.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now().strftime("%Y%m%d_%H%M%S_%f")
    final = out / f"FAIL_HANDOFF_{_safe_slug(item.name,70)}_{stamp}.zip"
    temp = out / f".{final.name}.tmp"
    summary = {
        "format": "python-patch-tool-fail-handoff",
        "format_version": 1,
        "tool_version": VERSION,
        "patch": item.name,
        "rc": rc,
        "patch_result": patch_result,
        "recovery_collect_request": recovery_request.name if recovery_request else None,
        "patch_attachment": "not_checked",
    }
    patch_path = root / "patchs" / item.name
    expected_patch_sha = patch_result.get("patch_sha256") if isinstance(patch_result, dict) else None
    attach_patch = False
    if patch_path.is_file() and not patch_path.is_symlink() and isinstance(expected_patch_sha, str):
        try:
            attach_patch = _sha256_file(patch_path) == expected_patch_sha
        except OSError:
            attach_patch = False
    if attach_patch:
        summary["patch_attachment"] = "exact_executed_queue_bytes"
    elif patch_path.exists() or patch_path.is_symlink():
        summary["patch_attachment"] = "omitted_queue_input_changed_or_unsafe"
    else:
        summary["patch_attachment"] = "omitted_queue_input_missing"
    try:
        total = 0
        affected: list[str] = []
        if isinstance(patch_result, dict) and isinstance(patch_result.get("diagnosis"), dict):
            affected.extend(x for x in patch_result["diagnosis"].get("affected_paths", []) if isinstance(x, str))
        if isinstance(patch_result, dict) and isinstance(patch_result.get("partial_modification"), dict):
            affected.extend(x for x in patch_result["partial_modification"].get("changed_paths", []) if isinstance(x, str))
        source_attachments: list[tuple[str, Path]] = []
        for rel in dict.fromkeys(affected):
            src = _safe_handoff_source(root, rel)
            if src is None:
                continue
            size = src.stat().st_size
            if total + size > MAX_HANDOFF_SOURCE_TOTAL_BYTES:
                break
            source_attachments.append((rel, src))
            total += size
        sensitive_warnings = _sensitive_handoff_warnings(console_log, source_attachments)
        summary["sensitive_content_warnings"] = sensitive_warnings
        with zipfile.ZipFile(temp, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6) as zf:
            zf.writestr("FAIL_SUMMARY.json", json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
            zf.writestr("console.log", console_log)
            if sensitive_warnings:
                warning_text = (
                    "WARNING: This diagnostic bundle intentionally preserves exact source/log bytes.\n"
                    "Review before uploading if the destination is not trusted.\n\n- "
                    + "\n- ".join(sensitive_warnings) + "\n"
                )
                zf.writestr("SENSITIVE_CONTENT_WARNING.txt", warning_text)
            if attach_patch:
                zf.write(patch_path, f"patch/{item.name}")
            for doc in [root/"tools"/"implementing.md", root/"tools"/"PYTHON_PATCH_TOOL_FEATURES_VI.md", root/"tools"/"_patch_lib"/"VERSION", root/"tools"/"_patch_lib"/"docs"/"PATCH_PACKAGE_SCHEMA.json"]:
                if doc.is_file():
                    zf.write(doc, f"tool_context/{doc.name}")
            if recovery_request is not None and recovery_request.is_file():
                zf.write(recovery_request, f"recovery/{recovery_request.name}")
            for rel, src in source_attachments:
                zf.write(src, f"current_source/{rel}")
        os.replace(temp, final)
        with zipfile.ZipFile(final) as zf:
            if zf.testzip() is not None:
                raise ValueError("FAIL_HANDOFF ZIP CRC check failed")
        print("")
        print("=" * 72)
        print("!!! [PRIMARY - UPLOAD THIS FILE] PATCH FAIL HANDOFF !!!")
        print(">>> ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER <<<")
        print(str(final))
        print("=" * 72)
        return final
    except Exception as exc:
        print(f"[PTV v{VERSION} WARNING] could not create FAIL_HANDOFF: {type(exc).__name__}: {exc}", file=sys.stderr)
        for path in (temp, final):
            try: path.unlink()
            except OSError: pass
        return None


def _runner_command(root: Path, action: str, item: QueueItem) -> list[str]:
    runner = root / "tools" / "_patch_lib" / "python_patch_runner.py"
    cmd = [sys.executable, str(runner)]
    if action in {"inspect", "validate"}:
        cmd.append(action)
    cmd += ["--patch", f"patchs/{item.name}", "--transaction", "off"]
    return cmd


def _inspect_item(root: Path, item: QueueItem) -> int:
    if item.kind != "PATCH":
        print("INSPECT: chỉ áp dụng cho PATCH; COLLECT được preflight theo schema khi discovery.")
        return 2
    try:
        return _normalize_subprocess_rc(subprocess.run(_runner_command(root, "inspect", item), cwd=root).returncode)
    except KeyboardInterrupt:
        return 130


def _validate_item(root: Path, item: QueueItem) -> int:
    if item.kind != "PATCH":
        print("VALIDATE: chỉ áp dụng cho PATCH.")
        return 2
    try:
        return _normalize_subprocess_rc(subprocess.run(_runner_command(root, "validate", item), cwd=root).returncode)
    except KeyboardInterrupt:
        return 130


def _safe_display(value: str) -> str:
    value = _ANSI_RE.sub("", str(value))
    out: list[str] = []
    for ch in value:
        if ch in "\r\n\t":
            out.append(" ")
            continue
        if unicodedata.category(ch) == "Cc":
            continue
        out.append(ch)
    return "".join(out)


def natural_name_key(value: str):
    return tuple(int(x) if x.isdigit() else x for x in re.split(r"(\d+)", value.lower()))



def _zip_has_root_patch_manifest(path: Path) -> bool:
    if path.suffix.lower() != ".zip" or not path.is_file():
        return False
    try:
        with zipfile.ZipFile(path) as zf:
            return "PATCH_TOOL_MANIFEST.json" in {n for n in zf.namelist() if not n.endswith("/")}
    except Exception:
        return False


def _zip_has_root_collection_manifest(path: Path) -> bool:
    """Return True for the canonical readonly COLLECT result marker.

    A result archive is evidence, never executable queue input.  This marker is
    intentionally checked before PATCH routing so an ambiguous archive carrying
    both root manifests fails closed as a collection result instead of running
    collected evidence as a PATCH.
    """
    if path.suffix.lower() != ".zip" or not path.is_file():
        return False
    try:
        with zipfile.ZipFile(path) as zf:
            return "COLLECTION_MANIFEST.json" in {n for n in zf.namelist() if not n.endswith("/")}
    except Exception:
        return False



def _archive_nonrunnable_reason(names: list[str]) -> str | None:
    """Return a structural support/distribution reason for archive members.

    Supports both root archives and one-or-more-folder wrappers by matching
    marker paths relative to their common prefix.  Re-zipping a valid COLLECT
    result on macOS commonly injects ``__MACOSX``/``.DS_Store`` metadata outside
    that wrapper.  Those metadata entries must not make collected ``patch_*.py``
    evidence executable again.
    """
    normalized = [n.replace("\\", "/").strip("/") for n in names]

    def _archive_metadata(name: str) -> bool:
        parts = [part for part in name.split("/") if part]
        if not parts:
            return True
        if parts[0] == "__MACOSX":
            return True
        return parts[-1] in {".DS_Store", "Thumbs.db", "desktop.ini"}

    semantic = [n for n in normalized if not _archive_metadata(n)]

    # Tool distribution: launcher and _patch_lib under the same prefix.
    launcher_suffix = "tools/run_python_patches.sh"
    for name in normalized:
        if name == launcher_suffix:
            prefix = ""
        elif name.endswith("/" + launcher_suffix):
            prefix = name[: -len(launcher_suffix)]
        else:
            continue
        lib_prefix = prefix + "tools/_patch_lib/"
        if any(n.startswith(lib_prefix) for n in normalized):
            return "tool_distribution"

    # Readonly COLLECT result archive: the canonical COLLECTION_MANIFEST.json
    # lives at the archive root. Accept one wrapper directory as well, because
    # users may re-zip an extracted collection folder before placing it in
    # patchs/. Canonical collection-result identity is fail-closed and wins
    # over patch-looking evidence; a normal v5 PATCH without that result marker
    # still routes by its root PATCH_TOOL_MANIFEST.json.
    collection_manifest = "COLLECTION_MANIFEST.json"
    if collection_manifest in semantic:
        return "collection_result_archive"
    for name in semantic:
        if Path(name).name != collection_manifest:
            continue
        parent = str(Path(name).parent).replace("\\", "/")
        if parent in {"", "."}:
            return "collection_result_archive"
        prefix = parent.rstrip("/") + "/"
        if semantic and all(n == parent or n.startswith(prefix) for n in semantic):
            return "collection_result_archive"

    # Handoff: the canonical marker pair shares the same archive parent.
    handoff_parents = {
        str(Path(n).parent).replace("\\", "/")
        for n in normalized
        if Path(n).name == "HANDOFF_README.md"
    }
    state_parents = {
        str(Path(n).parent).replace("\\", "/")
        for n in normalized
        if Path(n).name == "CURRENT_STATE.md"
    }
    if handoff_parents & state_parents:
        return "handoff_archive"
    return None


def _zip_known_nonrunnable(path: Path) -> tuple[bool, str]:
    """Recognize support/distribution ZIPs before COLLECT routing.

    A HANDOFF may legitimately preserve the original CODE_COLLECTION_REQUEST
    JSON as evidence. That must never make the handoff itself runnable as a
    new readonly collection request. Root PATCH manifest precedence is handled
    by discover_queue() before this helper.
    """
    if path.suffix.lower() != ".zip" or not path.is_file():
        return False, ""
    try:
        with zipfile.ZipFile(path) as zf:
            names = [n for n in zf.namelist() if not n.endswith("/")]
        reason = _archive_nonrunnable_reason(names)
        return (reason is not None, reason or "")
    except Exception:
        # Invalid ZIP handling remains with the normal classifier below.
        return False, ""

def inspect_collect_zip(path: Path):
    if path.suffix.lower() != ".zip" or not path.is_file():
        return False, ""
    try:
        with zipfile.ZipFile(path) as zf:
            req = [
                n
                for n in zf.namelist()
                if not n.endswith("/") and COLLECT_JSON_RE.match(Path(n).name)
            ]
            if len(req) != 1:
                return False, f"request_json_count={len(req)}"
            info = zf.getinfo(req[0])
            if info.file_size > MAX_COLLECT_REQUEST_JSON_BYTES:
                return False, f"request_too_large={info.file_size}"
            try:
                raw = zf.read(req[0])
                data = json.loads(raw.decode("utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
            except Exception as exc:
                return False, f"invalid_request_json:{type(exc).__name__}"
            try:
                validated = validate_request_data(data)
            except CollectSchemaError as exc:
                return False, f"schema_error:{exc}"
            return True, f"id={validated.get('id') or 'collect'} actions={len(validated['actions'])}"
    except Exception as exc:
        return False, f"invalid_zip:{type(exc).__name__}"


def _has_patch_markers(data: bytes) -> bool:
    return any(marker in data for marker in PATCH_MARKERS)


def _zip_is_patch(path: Path):
    try:
        with zipfile.ZipFile(path) as zf:
            names = [n for n in zf.namelist() if not n.endswith("/")]
            # A canonical readonly collection result is evidence, never a
            # runnable PATCH. Fail closed even if collected content also leaves
            # a PATCH_TOOL_MANIFEST.json at archive root.
            if "COLLECTION_MANIFEST.json" in names:
                return False, "collection_result_archive"
            # v5+ standard package: manifest must be at package root. Requiring
            # the root prevents a HANDOFF that merely embeds a patch tree from
            # being mistaken for the patch itself.
            if "PATCH_TOOL_MANIFEST.json" in names:
                return True, "manifest"
            # Known distribution/support bundles are not runnable patches.
            # Resolve these signatures before scanning Python text for legacy
            # helper markers because support evidence may mention PATCH_NAME.
            nonrunnable_reason = _archive_nonrunnable_reason(names)
            if nonrunnable_reason:
                return False, nonrunnable_reason

            py_names = [n for n in names if n.lower().endswith(".py")]
            if any(PATCH_PY_RE.match(Path(n).name) for n in py_names):
                return True, "legacy_patch_script"
            # Original v4 fallback: patch-named archive with Python payload.
            if path.name.lower().startswith("patch_") and py_names:
                return True, "legacy_patch_archive"
            # Legacy helper-marker scripts, bounded so queue discovery cannot
            # turn into an unbounded archive scan.
            total = 0
            scanned = 0
            for name in py_names:
                if scanned >= MAX_PATCH_MARKER_FILES or total >= MAX_PATCH_MARKER_BYTES:
                    break
                try:
                    info = zf.getinfo(name)
                    if info.file_size > MAX_PATCH_MARKER_BYTES:
                        continue
                    room = MAX_PATCH_MARKER_BYTES - total
                    data = zf.read(name)[:room]
                except Exception:
                    continue
                total += len(data)
                scanned += 1
                if _has_patch_markers(data):
                    return True, "legacy_helper_marker"
            return False, "no_patch_signature"
    except Exception as exc:
        return False, f"invalid_zip:{type(exc).__name__}"


def _tar_is_patch(path: Path):
    try:
        with tarfile.open(path, "r:*") as tf:
            members = [m for m in tf.getmembers() if m.isfile()]
            names = [m.name for m in members]
            if "PATCH_TOOL_MANIFEST.json" in names:
                return True, "manifest"
            nonrunnable_reason = _archive_nonrunnable_reason(names)
            if nonrunnable_reason:
                return False, nonrunnable_reason
            py_members = [m for m in members if m.name.lower().endswith(".py")]
            if any(PATCH_PY_RE.match(Path(m.name).name) for m in py_members):
                return True, "legacy_patch_script"
            if path.name.lower().startswith("patch_") and py_members:
                return True, "legacy_patch_archive"
            total = 0
            scanned = 0
            for member in py_members:
                if scanned >= MAX_PATCH_MARKER_FILES or total >= MAX_PATCH_MARKER_BYTES:
                    break
                if member.size > MAX_PATCH_MARKER_BYTES:
                    continue
                try:
                    fh = tf.extractfile(member)
                    if fh is None:
                        continue
                    room = MAX_PATCH_MARKER_BYTES - total
                    data = fh.read(room)
                except Exception:
                    continue
                total += len(data)
                scanned += 1
                if _has_patch_markers(data):
                    return True, "legacy_helper_marker"
            return False, "no_patch_signature"
    except Exception as exc:
        return False, f"invalid_tar:{type(exc).__name__}"


def inspect_patch_candidate(path: Path):
    low = path.name.lower()
    if low.endswith(".zip"):
        return _zip_is_patch(path)
    if low.endswith((".tar.gz", ".tgz")):
        return _tar_is_patch(path)
    if low.endswith(".py"):
        if PATCH_PY_RE.match(path.name):
            return True, "legacy_patch_script"
        try:
            if path.stat().st_size > MAX_PATCH_MARKER_BYTES:
                return False, "standalone_python_too_large"
            data = path.read_bytes()
        except Exception as exc:
            return False, f"unreadable_python:{type(exc).__name__}"
        return (_has_patch_markers(data), "legacy_helper_marker" if _has_patch_markers(data) else "no_patch_signature")
    return False, "unsupported_extension"


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _remove_session_duplicate_safely(queue_dir: Path, duplicate_name: str, canonical_name: str, expected_sha: str) -> tuple[bool, str | None]:
    duplicate = queue_dir / duplicate_name
    canonical = queue_dir / canonical_name
    guard = queue_dir / f".ptv-duplicate-guard-{os.getpid()}-{time.time_ns()}-{duplicate_name}"
    try:
        os.replace(duplicate, guard)
    except FileNotFoundError:
        return False, "duplicate queue file disappeared before removal"
    except OSError as exc:
        return False, f"could not isolate duplicate queue file ({type(exc).__name__})"
    try:
        if guard.is_symlink() or not guard.is_file() or _sha256_file(guard) != expected_sha:
            raise RuntimeError("duplicate queue file changed after hashing")
        if canonical.is_symlink() or not canonical.is_file() or _sha256_file(canonical) != expected_sha:
            raise RuntimeError("canonical queue file changed before duplicate removal")
        guard.unlink()
        return True, None
    except Exception as exc:
        # Never delete a file whose identity/content changed while duplicate
        # filtering was in progress. Restore it to the queue when possible.
        try:
            if not duplicate.exists() and not duplicate.is_symlink():
                os.replace(guard, duplicate)
                return False, str(exc)
        except OSError:
            pass
        try:
            preserved = queue_dir / f"PTV_UNEXPECTED_DUPLICATE_REPLACEMENT_{time.time_ns()}_{duplicate_name}"
            os.replace(guard, preserved)
            return False, f"{exc}; preserved as patchs/{preserved.name}"
        except OSError as preserve_exc:
            return False, f"{exc}; additionally could not restore isolated file ({type(preserve_exc).__name__})"
    finally:
        if guard.exists() or guard.is_symlink():
            try:
                if not duplicate.exists() and not duplicate.is_symlink():
                    os.replace(guard, duplicate)
            except OSError:
                pass


def _split_session_duplicate_patches(root: Path, items: list[QueueItem]):
    """Collapse byte-identical PATCH files already present in the same queue.

    Fast path: stat/group by file size first, then SHA-256 only groups that can
    possibly contain a duplicate. The first PATCH in natural queue order is the
    canonical file. Later byte-identical files are removed from ``patchs/``
    immediately, as explicitly requested. COLLECT requests are never deduped.
    """
    queue_dir = root / "patchs"
    runnable: list[QueueItem] = []
    duplicates: list[SessionDuplicate] = []
    warnings: list[str] = []

    size_by_name: dict[str, int | None] = {}
    size_counts: dict[int, int] = {}
    for item in items:
        if item.kind != "PATCH":
            continue
        path = queue_dir / item.name
        try:
            if path.is_symlink() or not path.is_file():
                size_by_name[item.name] = None
                continue
            size = path.stat().st_size
            size_by_name[item.name] = size
            size_counts[size] = size_counts.get(size, 0) + 1
        except OSError as exc:
            size_by_name[item.name] = None
            warnings.append(
                f"session duplicate stat skipped for patchs/{item.name} ({type(exc).__name__})"
            )

    canonical_by_hash: dict[tuple[int, str], QueueItem] = {}
    for item in items:
        if item.kind != "PATCH":
            runnable.append(item)
            continue
        size = size_by_name.get(item.name)
        if size is None or size_counts.get(size, 0) < 2:
            runnable.append(item)
            continue
        path = queue_dir / item.name
        try:
            digest = _sha256_file(path)
        except OSError as exc:
            warnings.append(
                f"session duplicate hash skipped for patchs/{item.name} ({type(exc).__name__})"
            )
            runnable.append(item)
            continue
        key = (size, digest)
        canonical = canonical_by_hash.get(key)
        if canonical is None:
            canonical_by_hash[key] = item
            runnable.append(item)
            continue
        removed, remove_warning = _remove_session_duplicate_safely(
            queue_dir, item.name, canonical.name, digest
        )
        if remove_warning:
            warnings.append(
                f"duplicate PATCH removal aborted safely for patchs/{item.name}: {remove_warning}"
            )
        if removed:
            duplicates.append(SessionDuplicate(item, canonical.name, digest, True))
        else:
            # If safe removal could not be proven, leave the item runnable; do
            # not silently suppress a possibly changed package.
            runnable.append(item)

    return runnable, duplicates, warnings


def _print_session_duplicate_removals(duplicates: list[SessionDuplicate], *, stream=None) -> None:
    if not duplicates:
        return
    out = stream or sys.stdout
    print("DUPLICATE PATCHES COLLAPSED IN THIS SESSION:", file=out)
    for index, duplicate in enumerate(duplicates, 1):
        status = "REMOVED:DUPLICATE_SESSION" if duplicate.removed else "SKIPPED:DUPLICATE_SESSION"
        print(f"{index}. [{status}] {_safe_display(duplicate.item.name)}", file=out)
        print(f"   Same content as: patchs/{_safe_display(duplicate.canonical_name)}", file=out)


def _split_local_duplicate_patches(root: Path, items: list[QueueItem]):
    """Split runnable items from PATCHes already present in local PASS history.

    Duplicate history is deliberately local-only: direct regular files under
    ``<project>/patchs/patched``.  No project key, network state, shared cache,
    Git history, or machine-external database participates.  Exact content
    SHA-256 is the authority, so a renamed copy is still a duplicate while a
    same-named package with different bytes remains runnable.

    Exact local-history duplicates are excluded from execution.  The caller
    immediately moves each confirmed duplicate into ``patchs/ignore`` so the
    same skipped input is reported once and cannot be offered again on the
    next zero-argument run.
    """
    queue_dir = root / "patchs"
    history_dir = queue_dir / "patched"

    # LOCAL means physically inside this resolved project tree.  Never follow
    # a symlinked patchs/ or patchs/patched/ into another project/shared cache:
    # doing so would make a patch executed elsewhere suppress this project.
    try:
        if queue_dir.is_symlink():
            return list(items), [], [
                "local duplicate check disabled: patchs/ is a symlink"
            ]
        if not queue_dir.is_dir():
            return list(items), [], []
        if history_dir.is_symlink():
            return list(items), [], [
                "local duplicate check disabled: patchs/patched/ is a symlink"
            ]
        if not history_dir.is_dir():
            return list(items), [], []
        resolved_root = root.resolve(strict=True)
        resolved_queue = queue_dir.resolve(strict=True)
        resolved_history = history_dir.resolve(strict=True)
        if resolved_queue.parent != resolved_root or resolved_history.parent != resolved_queue:
            return list(items), [], [
                "local duplicate check disabled: history escapes project-local patchs/"
            ]
    except OSError as exc:
        return list(items), [], [
            f"local duplicate history unavailable: patchs/patched ({type(exc).__name__})"
        ]

    by_size: dict[int, list[Path]] = {}
    warnings: list[str] = []
    try:
        history_entries = list(history_dir.iterdir())
    except OSError as exc:
        return list(items), [], [
            f"local duplicate history unavailable: patchs/patched ({type(exc).__name__})"
        ]

    for path in history_entries:
        try:
            if path.is_symlink() or not path.is_file():
                continue
            size = path.stat().st_size
        except OSError:
            continue
        by_size.setdefault(size, []).append(path)
    for paths in by_size.values():
        paths.sort(key=lambda x: natural_name_key(x.name))

    history_hash_cache: dict[Path, str | None] = {}
    runnable: list[QueueItem] = []
    duplicates: list[LocalDuplicate] = []

    for item in items:
        if item.kind != "PATCH":
            runnable.append(item)
            continue
        queued = root / "patchs" / item.name
        try:
            if queued.is_symlink() or not queued.is_file():
                runnable.append(item)
                continue
            size = queued.stat().st_size
        except OSError as exc:
            warnings.append(
                f"local duplicate check skipped for patchs/{item.name} ({type(exc).__name__})"
            )
            runnable.append(item)
            continue

        candidates = list(by_size.get(size, ()))
        if not candidates:
            runnable.append(item)
            continue
        # Prefer a same-name historical file in diagnostics, then natural name.
        candidates.sort(key=lambda x: (x.name != item.name, natural_name_key(x.name)))
        try:
            queued_hash = _sha256_file(queued)
        except OSError as exc:
            warnings.append(
                f"local duplicate check skipped for patchs/{item.name} ({type(exc).__name__})"
            )
            runnable.append(item)
            continue

        match: Path | None = None
        for historical in candidates:
            if historical not in history_hash_cache:
                try:
                    history_hash_cache[historical] = _sha256_file(historical)
                except OSError:
                    history_hash_cache[historical] = None
            if history_hash_cache[historical] == queued_hash:
                match = historical
                break
        if match is None:
            runnable.append(item)
            continue
        duplicates.append(LocalDuplicate(item, match.name, queued_hash))

    return runnable, duplicates, warnings



def _restore_ignore_guard(queue_dir: Path, source: Path, guard: Path) -> str | None:
    if not guard.exists() and not guard.is_symlink():
        return None
    try:
        if not source.exists() and not source.is_symlink():
            os.replace(guard, source)
            return None
        preserved = queue_dir / f"PTV_UNEXPECTED_IGNORE_REPLACEMENT_{time.time_ns()}_{source.name}"
        os.replace(guard, preserved)
        return f"original duplicate preserved as patchs/{preserved.name} because patchs/{source.name} changed concurrently"
    except OSError as exc:
        return f"could not restore isolated duplicate patchs/{source.name} ({type(exc).__name__})"


def _reserve_ignore_target(ignore_dir: Path, original_name: str, digest: str) -> tuple[Path, bool]:
    """Return a collision-safe ignore target and whether identical content already exists."""
    date_prefix = datetime.now().strftime("%Y-%m-%d")
    for index in range(1, 10000):
        prefix = date_prefix if index == 1 else f"{date_prefix}-{index}"
        target = ignore_dir / f"{prefix}-{original_name}"
        try:
            if target.is_symlink():
                continue
            if target.is_file():
                try:
                    if _sha256_file(target) == digest:
                        return target, True
                except OSError:
                    pass
                continue
            fd = os.open(target, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
            os.close(fd)
            return target, False
        except FileExistsError:
            continue
    raise RuntimeError("could not reserve a unique patchs/ignore filename")


def _move_local_duplicates_to_ignore(root: Path, duplicates: list[LocalDuplicate]):
    """Atomically retire confirmed local-history duplicates from the runnable queue."""
    if not duplicates:
        return [], []
    queue_dir = root / "patchs"
    ignore_dir = queue_dir / "ignore"
    warnings: list[str] = []
    moved: list[LocalDuplicate] = []
    try:
        if queue_dir.is_symlink() or not queue_dir.is_dir():
            raise RuntimeError("patchs/ is not a real directory")
        if ignore_dir.exists() or ignore_dir.is_symlink():
            if ignore_dir.is_symlink() or not ignore_dir.is_dir():
                raise RuntimeError("patchs/ignore is a symlink/non-directory")
        else:
            ignore_dir.mkdir(parents=False, exist_ok=False)
        resolved_root = root.resolve(strict=True)
        resolved_queue = queue_dir.resolve(strict=True)
        resolved_ignore = ignore_dir.resolve(strict=True)
        if resolved_queue.parent != resolved_root or resolved_ignore.parent != resolved_queue:
            raise RuntimeError("patchs/ignore escapes project-local patchs/")
    except (OSError, RuntimeError) as exc:
        return list(duplicates), [f"could not initialize patchs/ignore: {exc}"]

    for duplicate in duplicates:
        source = queue_dir / duplicate.item.name
        guard = queue_dir / f".ptv-ignore-guard-{os.getpid()}-{time.time_ns()}-{duplicate.item.name}"
        target: Path | None = None
        identical_existing = False
        try:
            if source.is_symlink() or not source.is_file():
                raise RuntimeError("queue duplicate disappeared or became unsafe before ignore move")
            os.replace(source, guard)
            if guard.is_symlink() or not guard.is_file():
                raise RuntimeError("isolated duplicate is not a regular file")
            if _sha256_file(guard) != duplicate.sha256:
                raise RuntimeError("queue duplicate changed after duplicate detection")
            target, identical_existing = _reserve_ignore_target(ignore_dir, duplicate.item.name, duplicate.sha256)
            if identical_existing:
                guard.unlink()
            else:
                # target is our exclusive zero-byte reservation; replacing it cannot overwrite user content.
                os.replace(guard, target)
            moved.append(LocalDuplicate(duplicate.item, duplicate.history_name, duplicate.sha256, target.name))
        except Exception as exc:
            if target is not None and not identical_existing:
                try:
                    if target.is_file() and target.stat().st_size == 0:
                        target.unlink()
                except OSError:
                    pass
            restore_warning = _restore_ignore_guard(queue_dir, source, guard)
            detail = f"could not move skipped duplicate patchs/{duplicate.item.name} into patchs/ignore ({type(exc).__name__}: {exc})"
            if restore_warning:
                detail += f"; {restore_warning}"
            warnings.append(detail)
            moved.append(duplicate)
    return moved, warnings


def _print_local_duplicate_skips(duplicates: list[LocalDuplicate], *, stream=None) -> None:
    if not duplicates:
        return
    out = stream or sys.stdout
    print("PATCHES SKIPPED / NOT EXECUTED:", file=out)
    for index, duplicate in enumerate(duplicates, 1):
        print(
            f"{index}. [SKIPPED:DUPLICATE_LOCAL] {_safe_display(duplicate.item.name)}",
            file=out,
        )
        print(
            f"   Local match: patchs/patched/{_safe_display(duplicate.history_name)}",
            file=out,
        )
        if duplicate.ignored_name:
            print(
                f"   Moved to ignore: patchs/ignore/{_safe_display(duplicate.ignored_name)}",
                file=out,
            )
        else:
            print("   Ignore move: FAILED — file remains in patchs/", file=out)


def discover_queue(root: Path):
    directory = root / "patchs"
    if directory.exists() or directory.is_symlink():
        if directory.is_symlink() or not directory.is_dir():
            raise QueueSafetyError("project patchs/ must be a real directory, not a symlink/non-directory")
    else:
        directory.mkdir(parents=True, exist_ok=False)
    try:
        if directory.resolve(strict=True).parent != root.resolve(strict=True):
            raise QueueSafetyError("project patchs/ escapes project root")
    except OSError as exc:
        raise QueueSafetyError("project patchs/ cannot be resolved safely") from exc
    items: list[QueueItem] = []
    warnings: list[str] = []

    for path in directory.iterdir():
        try:
            if path.is_symlink():
                warnings.append(f"SKIPPED symlink queue entry: patchs/{path.name}")
                continue
            if not path.is_file():
                continue
        except OSError as exc:
            warnings.append(f"SKIPPED unreadable entry: patchs/{path.name} ({type(exc).__name__})")
            continue

        low = path.name.lower()
        if path.suffix.lower() == ".json" and COLLECT_JSON_RE.match(path.name):
            warnings.append(f"RAW JSON REJECTED: patchs/{path.name}")
            continue

        # Canonical result archives produced by readonly COLLECT are evidence,
        # not executable queue input. This fail-closed marker wins even in an
        # ambiguous ZIP that also carries a root PATCH manifest.
        if path.suffix.lower() == ".zip" and _zip_has_root_collection_manifest(path):
            warnings.append(
                f"SKIPPED non-patch candidate: patchs/{path.name} (collection_result_archive)"
            )
            continue

        # A root PATCH manifest is the strongest PATCH request signature after
        # excluding canonical collection results. A PATCH may legitimately
        # carry a collection REQUEST JSON as a nested resource; do not route
        # that request resource into readonly COLLECT.
        if path.suffix.lower() == ".zip" and _zip_has_root_patch_manifest(path):
            items.append(QueueItem(path.name, "PATCH", "manifest"))
            continue

        if path.suffix.lower() == ".zip":
            nonrunnable, reason = _zip_known_nonrunnable(path)
            if nonrunnable:
                warnings.append(
                    f"SKIPPED non-patch candidate: patchs/{path.name} ({reason})"
                )
                continue

        ok, detail = inspect_collect_zip(path)
        if ok:
            items.append(QueueItem(path.name, "COLLECT", detail))
            continue

        collect_invalid = (
            path.suffix.lower() == ".zip"
            and (
                low.startswith("code_collection_request")
                or detail == "invalid_request"
                or detail.startswith("invalid_request_json:")
                or detail.startswith("request_too_large=")
                or detail.startswith("schema_error:")
                or (
                    detail.startswith("request_json_count=")
                    and detail != "request_json_count=0"
                )
            )
        )
        if collect_invalid:
            items.append(QueueItem(path.name, "COLLECT INVALID", detail))
            continue

        supported = low.endswith((".zip", ".py", ".tar.gz", ".tgz"))
        if not supported:
            continue
        is_patch, patch_detail = inspect_patch_candidate(path)
        if is_patch:
            items.append(QueueItem(path.name, "PATCH", patch_detail))
        else:
            warnings.append(
                f"SKIPPED non-patch candidate: patchs/{path.name} ({patch_detail})"
            )

    items.sort(key=lambda x: natural_name_key(x.name))
    return items, warnings


def _read_key_posix(fd):
    raw = os.read(fd, 1)
    if raw in {b"\r", b"\n"}:
        return "ENTER"
    if raw == b" ":
        return "SPACE"
    if raw == b"\x1b":
        seq = b""
        for _ in range(2):
            ready, _, _ = select.select([fd], [], [], 0.035)
            if ready:
                seq += os.read(fd, 1)
        return {b"[A": "UP", b"[B": "DOWN"}.get(seq, "ESC")
    return raw.decode(errors="ignore").lower()


# Backward-compatible internal alias retained for existing selector tests/helpers.
_read_key = _read_key_posix


def _read_key_windows():
    if msvcrt is None:
        return ""
    ch = msvcrt.getwch()
    if ch in {"\x00", "\xe0"}:
        code = msvcrt.getwch()
        return {"H": "UP", "P": "DOWN"}.get(code, "")
    if ch in {"\r", "\n"}:
        return "ENTER"
    if ch == " ":
        return "SPACE"
    if ch == "\x1b":
        return "ESC"
    if ch == "\x03":
        return "\x03"
    return ch.lower()


def _enable_windows_vt() -> bool:
    if os.name != "nt":
        return False
    try:
        import ctypes
        kernel32 = ctypes.windll.kernel32
        handle = kernel32.GetStdHandle(-11)  # STD_OUTPUT_HANDLE
        mode = ctypes.c_uint()
        if handle in (0, -1) or not kernel32.GetConsoleMode(handle, ctypes.byref(mode)):
            return False
        ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004
        if not kernel32.SetConsoleMode(handle, mode.value | ENABLE_VIRTUAL_TERMINAL_PROCESSING):
            return False
        return True
    except Exception:
        return False



def _enable_windows_vt_stream(stream) -> bool:
    if os.name != "nt":
        return True
    try:
        import ctypes
        handle_id = -12 if stream is sys.stderr else -11
        kernel32 = ctypes.windll.kernel32
        handle = kernel32.GetStdHandle(handle_id)
        mode = ctypes.c_uint()
        if handle in (0, -1) or not kernel32.GetConsoleMode(handle, ctypes.byref(mode)):
            return False
        return bool(kernel32.SetConsoleMode(handle, mode.value | 0x0004))
    except Exception:
        return False


def _result_color_enabled(stream) -> bool:
    if os.environ.get("NO_COLOR") is not None:
        return False
    if not bool(getattr(stream, "isatty", lambda: False)()):
        return False
    return _enable_windows_vt_stream(stream)


def _print_patch_result_banner(status: str, patch_name: str | None, *, rc: int | None = None, stream=None) -> None:
    if not patch_name:
        return
    out = stream or (sys.stderr if status == "FAIL" else sys.stdout)
    # Handoff/recovery paths may have been written to stdout while FAIL summary
    # uses stderr. Flush both streams so redirected combined logs preserve the
    # intended order: paths first, high-visibility final banner last.
    try:
        sys.stdout.flush()
        sys.stderr.flush()
    except Exception:
        pass
    safe_name = _safe_display(patch_name)
    if status == "FAIL":
        title = "!!! PATCH FAILED !!!"
        detail = f"PATCH: {safe_name}" + (f" | rc={rc}" if rc is not None else "")
        if _result_color_enabled(out):
            style, reset = "\x1b[1;93;41m", "\x1b[0m"  # bold yellow on red background
            print(f"{style}{title}{reset}", file=out)
            print(f"{style}{detail}{reset}", file=out)
        else:
            print(title, file=out)
            print(detail, file=out)
        return
    title = "=== PATCH COMPLETED ==="
    detail = f"PATCH: {safe_name}"
    if _result_color_enabled(out):
        style, reset = "\x1b[1;96m", "\x1b[0m"  # bold bright cyan
        print(f"{style}{title}{reset}", file=out)
        print(f"{style}{detail}{reset}", file=out)
    else:
        print(title, file=out)
        print(detail, file=out)


def _last_patch_name(*, status: str | None = None) -> str | None:
    for detail in reversed(_LAST_EXECUTION_DETAILS):
        if detail.get("kind") != "PATCH":
            continue
        if status is not None and detail.get("status") != status:
            continue
        name = detail.get("name")
        if isinstance(name, str) and name:
            return name
    return None


def _report_rows(report: dict[str, object]) -> list[dict[str, object]]:
    selected = [x for x in report.get("selected", []) if isinstance(x, str)] if isinstance(report.get("selected"), list) else []
    raw_results = report.get("results") if isinstance(report.get("results"), list) else []
    by_name: dict[str, dict[str, object]] = {}
    for raw in raw_results:
        if isinstance(raw, dict) and isinstance(raw.get("name"), str):
            by_name.setdefault(str(raw["name"]), raw)
    not_executed = set(x for x in report.get("not_executed", []) if isinstance(x, str)) if isinstance(report.get("not_executed"), list) else set()
    rows: list[dict[str, object]] = []
    for name in selected:
        detail = by_name.get(name)
        if detail is None:
            status = "NOT_EXECUTED" if name in not_executed else "UNKNOWN"
            rows.append({"name": name, "kind": "PATCH", "status": status, "rc": None})
        else:
            rows.append(dict(detail))
    # Preserve diagnostic visibility for any result recorded outside selected
    # (for example a late duplicate synthesized by a test or future policy).
    known = set(selected)
    for raw in raw_results:
        if isinstance(raw, dict) and isinstance(raw.get("name"), str) and raw["name"] not in known:
            rows.append(dict(raw)); known.add(str(raw["name"]))
    return rows


def _row_diagnosis(row: dict[str, object]) -> str:
    patch_result = row.get("patch_result")
    if isinstance(patch_result, dict):
        diagnosis = patch_result.get("diagnosis")
        if isinstance(diagnosis, dict):
            kind = diagnosis.get("kind")
            if isinstance(kind, str) and kind and kind != "none":
                return kind
    diagnosis = row.get("diagnosis")
    if isinstance(diagnosis, dict) and isinstance(diagnosis.get("kind"), str):
        return str(diagnosis["kind"])
    return ""


def _row_changed_count(row: dict[str, object]) -> int | None:
    patch_result = row.get("patch_result")
    if not isinstance(patch_result, dict):
        return None
    delta = patch_result.get("project_delta")
    if isinstance(delta, dict) and isinstance(delta.get("changed_paths"), list):
        return len(delta["changed_paths"])
    partial = patch_result.get("partial_modification")
    if isinstance(partial, dict) and isinstance(partial.get("changed_paths"), list):
        return len(partial["changed_paths"])
    return None


def _row_summary(row: dict[str, object]) -> str:
    status = str(row.get("status") or "UNKNOWN")
    if status == "PASS":
        changed = _row_changed_count(row)
        parts = ["completed"]
        if row.get("batch_rolled_back") is True:
            parts.append("BATCH-ROLLED-BACK")
        if changed is not None:
            parts.append(f"changed={changed}")
        elapsed = row.get("elapsed_seconds")
        if isinstance(elapsed, (int, float)):
            parts.append(f"{elapsed:.2f}s")
        return " | ".join(parts)
    if status == "FAIL":
        parts = []
        diagnosis = _row_diagnosis(row)
        if diagnosis:
            parts.append(diagnosis)
        if row.get("rc") is not None:
            parts.append(f"rc={row['rc']}")
        if row.get("fail_handoff"):
            parts.append("handoff ready")
        elapsed = row.get("elapsed_seconds")
        if isinstance(elapsed, (int, float)):
            parts.append(f"{elapsed:.2f}s")
        return " | ".join(parts) or "failed"
    if status == "NOT_EXECUTED":
        diagnosis = row.get("diagnosis") if isinstance(row.get("diagnosis"), dict) else {}
        message = diagnosis.get("message") if isinstance(diagnosis, dict) else None
        return str(message or "not executed")
    if status == "BLOCKED":
        blocked = row.get("blocked_by") if isinstance(row.get("blocked_by"), list) else []
        return "blocked by dependency failure" + (f": {','.join(str(x) for x in blocked)}" if blocked else "")
    if status == "PREFLIGHT_FAIL":
        diagnosis = row.get("diagnosis") if isinstance(row.get("diagnosis"), dict) else {}
        return f"batch preflight failed: {diagnosis.get('kind','unknown')}"
    if status == "SKIPPED_DUPLICATE_LOCAL":
        return f"duplicate -> {row.get('ignore_path') or 'ignore'}"
    return status.lower()


def _batch_counts(rows: list[dict[str, object]]) -> dict[str, int]:
    counts = {"PASS": 0, "FAIL": 0, "BLOCKED": 0, "PREFLIGHT_FAIL": 0, "NOT_EXECUTED": 0, "SKIPPED": 0, "OTHER": 0}
    for row in rows:
        status = str(row.get("status") or "UNKNOWN")
        if status in {"PASS", "FAIL", "BLOCKED", "PREFLIGHT_FAIL", "NOT_EXECUTED"}:
            counts[status] += 1
        elif status.startswith("SKIPPED"):
            counts["SKIPPED"] += 1
        else:
            counts["OTHER"] += 1
    return counts


def _batch_summary_text(report: dict[str, object]) -> str:
    rows = _report_rows(report)
    counts = _batch_counts(rows)
    lines = [
        f"BATCH REPORT | run={report.get('run_id','unknown')} | status={report.get('status','UNKNOWN')}",
        (
            f"SELECTED={len(rows)} | PASS={counts['PASS']} | FAIL={counts['FAIL']} | "
            f"BLOCKED={counts['BLOCKED']} | PREFLIGHT_FAIL={counts['PREFLIGHT_FAIL']} | NOT_EXECUTED={counts['NOT_EXECUTED']} | SKIPPED={counts['SKIPPED']}"
        ),
        "",
    ]
    for i, row in enumerate(rows, 1):
        lines.append(f"{i:>3}. [{str(row.get('status') or 'UNKNOWN')}] {row.get('name','unknown')}")
        lines.append(f"     {_row_summary(row)}")
    return "\n".join(lines).rstrip() + "\n"


def _finalize_batch_artifacts(root: Path, report: dict[str, object]) -> None:
    run_id = str(report.get("run_id") or "run")
    run_dir = _batch_run_dir(root, run_id)
    try:
        run_dir.mkdir(parents=True, exist_ok=True)
        summary_path = run_dir / "SUMMARY.txt"
        aggregate_path = run_dir / "batch.log"
        summary_text = _batch_summary_text(report)
        summary_path.write_text(summary_text, encoding="utf-8")
        with aggregate_path.open("w", encoding="utf-8", errors="replace") as out:
            out.write(summary_text)
            for i, row in enumerate(_report_rows(report), 1):
                out.write("\n" + "=" * 78 + "\n")
                out.write(f"ITEM {i}: [{row.get('status','UNKNOWN')}] {row.get('name','unknown')}\n")
                out.write(f"SUMMARY: {_row_summary(row)}\n")
                out.write("=" * 78 + "\n")
                rel = row.get("log_path")
                log_path = root / str(rel) if isinstance(rel, str) else None
                if log_path is not None and log_path.is_file():
                    with log_path.open("r", encoding="utf-8", errors="replace") as src:
                        shutil.copyfileobj(src, out)
                else:
                    out.write("[no execution log: item was not executed or log capture was unavailable]\n")
        report["batch_report_dir"] = run_dir.relative_to(root).as_posix()
        report["batch_summary"] = summary_path.relative_to(root).as_posix()
        report["batch_log"] = aggregate_path.relative_to(root).as_posix()
    except Exception as exc:
        report.setdefault("report_warnings", []).append(f"batch report artifacts unavailable: {type(exc).__name__}: {exc}")


def _print_batch_overview(root: Path, report: dict[str, object], *, stream=None) -> None:
    out = stream or sys.stdout
    rows = _report_rows(report)
    counts = _batch_counts(rows)
    print("", file=out)
    batch_status = str(report.get("status") or "UNKNOWN")
    title = f"BATCH RESULT — {batch_status}"
    if batch_status == "FAIL" and _result_color_enabled(out):
        print(f"\x1b[1;93;41m{title}\x1b[0m", file=out)
    elif batch_status == "PASS" and _result_color_enabled(out):
        print(f"\x1b[1;96m{title}\x1b[0m", file=out)
    else:
        print(title, file=out)
    print(
        f"Selected={len(rows)} | PASS={counts['PASS']} | FAIL={counts['FAIL']} | "
        f"BLOCKED={counts['BLOCKED']} | PREFLIGHT FAIL={counts['PREFLIGHT_FAIL']} | NOT EXECUTED={counts['NOT_EXECUTED']} | SKIPPED={counts['SKIPPED']}",
        file=out,
    )
    print(f"Policy: failure={report.get('failure_policy','fail_fast')} | transaction={report.get('transaction_policy','patch')}", file=out)
    tx = report.get("batch_transaction") if isinstance(report.get("batch_transaction"), dict) else None
    if tx:
        print(f"Batch transaction: {tx.get('status','UNKNOWN')}", file=out)
    for i, row in enumerate(rows, 1):
        print(f"  {i:>2}. [{row.get('status','UNKNOWN')}] {_safe_display(str(row.get('name','unknown')))}", file=out)
        print(f"      {_safe_display(_row_summary(row))}", file=out)
    if report.get("batch_log"):
        print(f"Aggregate log: {report['batch_log']}", file=out)
    if report.get("batch_report_dir"):
        print(f"Detail logs : {report['batch_report_dir']}/items/", file=out)
    launcher = r"tools\run_python_patches.bat report" if os.name == "nt" else "./tools/run_python_patches.sh report"
    print(f"Reopen report: {launcher}", file=out)


def _show_file_paged(path: Path, title: str) -> None:
    print("\n" + "=" * 78)
    print(_safe_display(title))
    print(_safe_display(str(path)))
    print("=" * 78)
    if not path.is_file():
        print("[log file is unavailable]")
        return
    interactive = bool(getattr(sys.stdin, "isatty", lambda: False)() and getattr(sys.stdout, "isatty", lambda: False)())
    if not interactive:
        with path.open("r", encoding="utf-8", errors="replace") as src:
            shutil.copyfileobj(src, sys.stdout)
        return
    try:
        page_lines = max(8, _selector_term_size()[1] - 6)
    except Exception:
        page_lines = 24
    with path.open("r", encoding="utf-8", errors="replace") as src:
        while True:
            chunk = list(islice(src, page_lines))
            if not chunk:
                break
            sys.stdout.writelines(chunk); sys.stdout.flush()
            probe = src.readline()
            if not probe:
                break
            # Put the probe back by rendering it as the first line of the next
            # page via a tiny in-memory prefix rather than requiring seek on a
            # text cookie with platform-dependent semantics.
            answer = input("-- Enter: tiếp | q: quay lại menu -- ").strip().lower()
            if answer in {"q", "quit"}:
                break
            sys.stdout.write(probe)


def _source_compare_path(root: Path, row: dict[str, object]) -> Path | None:
    info = row.get("source_compare") if isinstance(row.get("source_compare"), dict) else None
    rel = info.get("diff_path") if isinstance(info, dict) else None
    if isinstance(rel, str):
        path = root / rel
        if path.is_file(): return path
    return None


def _show_source_compare(root: Path, row: dict[str, object]) -> None:
    info = row.get("source_compare") if isinstance(row.get("source_compare"), dict) else None
    if not isinstance(info, dict):
        print("Source compare: unavailable (PATCH declared no targets or item did not execute).")
        return
    changed = info.get("changed_paths") if isinstance(info.get("changed_paths"), list) else []
    print(f"SOURCE BEFORE/AFTER — changed declared targets: {len(changed)}")
    for rel in changed:
        print(f"  ~ {_safe_display(str(rel))}")
    path = _source_compare_path(root, row)
    if path is not None:
        _show_file_paged(path, f"SOURCE DIFF — {row.get('name','unknown')}")


def _create_report_support_bundle(root: Path, report: dict[str, object], row_index: int) -> Path | None:
    rows = _report_rows(report)
    if not (0 <= row_index < len(rows)):
        return None
    row = rows[row_index]
    run_id = _safe_slug(str(report.get("run_id") or "run"), 64)
    out_dir = _artifact_run_root(root) / "support"; out_dir.mkdir(parents=True, exist_ok=True)
    final = out_dir / f"PTV_SUPPORT_{run_id}_{row_index+1:03d}_{_safe_slug(str(row.get('name') or 'item'),60)}.zip"
    temp = final.with_suffix(".zip.tmp")
    try:
        with zipfile.ZipFile(temp, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            zf.writestr("REPORT_ITEM.json", json.dumps({"run": {k: report.get(k) for k in ("run_id","status","tool_version","failure_policy","transaction_policy","batch_transaction")}, "item": row}, ensure_ascii=False, indent=2) + "\n")
            summary = report.get("batch_summary")
            if isinstance(summary, str) and (root / summary).is_file(): zf.write(root / summary, "RUN_SUMMARY.txt")
            for key, arc in (("log_path","DETAIL.log"),("preflight_log_path","PREFLIGHT.log")):
                rel = row.get(key)
                if isinstance(rel, str) and (root / rel).is_file(): zf.write(root / rel, arc)
            diff = _source_compare_path(root, row)
            if diff is not None: zf.write(diff, "SOURCE.diff")
            handoff = row.get("fail_handoff")
            if isinstance(handoff, str) and (root / handoff).is_file(): zf.write(root / handoff, f"artifacts/{Path(handoff).name}")
            recovery = row.get("recovery_collect_request")
            if isinstance(recovery, str) and (root / "patchs" / recovery).is_file(): zf.write(root / "patchs" / recovery, f"artifacts/{Path(recovery).name}")
        os.replace(temp, final)
        with zipfile.ZipFile(final) as zf:
            if zf.testzip() is not None: raise RuntimeError("support bundle CRC failed")
        print(f"SUPPORT BUNDLE: {final}")
        return final
    except Exception as exc:
        print(f"[PTV v{VERSION} WARNING] support bundle failed: {type(exc).__name__}: {exc}", file=sys.stderr)
        try: temp.unlink()
        except OSError: pass
        return None


def _print_item_detail(root: Path, row: dict[str, object]) -> None:
    print("\nPATCH DETAIL")
    print(f"Name       : {_safe_display(str(row.get('name','unknown')))}")
    print(f"Status     : {row.get('status','UNKNOWN')}")
    if row.get("patch_id"): print(f"Patch ID   : {_safe_display(str(row.get('patch_id')))}")
    deps = row.get("depends_on") if isinstance(row.get("depends_on"), list) else []
    if deps: print(f"Depends on : {_safe_display(', '.join(str(x) for x in deps))}")
    if row.get("rc") is not None: print(f"Return code: {row.get('rc')}")
    if isinstance(row.get("elapsed_seconds"), (int, float)): print(f"Elapsed    : {float(row['elapsed_seconds']):.3f}s")
    diagnosis = _row_diagnosis(row)
    if diagnosis: print(f"Diagnosis  : {_safe_display(diagnosis)}")
    if row.get("batch_rolled_back") is True: print("Transaction: changes from this item were rolled back by batch policy")
    if row.get("requeued_as"): print(f"Replay pkg : patchs/{_safe_display(str(row['requeued_as']))}")
    if row.get("fail_handoff"): print(f"FAIL handoff: {_safe_display(str(row['fail_handoff']))}")
    if row.get("recovery_collect_request"): print(f"Recovery COLLECT: patchs/{_safe_display(str(row['recovery_collect_request']))}")
    info = row.get("source_compare") if isinstance(row.get("source_compare"), dict) else None
    if info is not None:
        changed = info.get("changed_paths") if isinstance(info.get("changed_paths"), list) else []
        print(f"Source diff : {len(changed)} declared target(s) changed")
        path = _source_compare_path(root, row)
        if path is not None: print(f"              {path.relative_to(root).as_posix()}")
    rel = row.get("log_path")
    if isinstance(rel, str): _show_file_paged(root / rel, f"DETAIL LOG — {row.get('name','unknown')}")
    else: print("Execution log: unavailable (item was not executed).")


def _print_row_subset(rows: list[dict[str, object]], title: str) -> None:
    print(f"\n{title}")
    if not rows:
        print("  [none]"); return
    for i, row in rows:
        print(f"  {i:>2}. [{row.get('status','UNKNOWN')}] {_safe_display(str(row.get('name','unknown')))}")
        print(f"      {_safe_display(_row_summary(row))}")


def _list_history(root: Path) -> int:
    entries = _history_entries(root); pins = _load_pinned_runs(root)
    if not entries:
        print("No Patch Tool run history is available."); return 0
    print("PATCH TOOL RUN HISTORY")
    for i, (_path, report) in enumerate(entries[:max(RUN_HISTORY_LIMIT, len(pins)+10)], 1):
        rows = _report_rows(report); counts = _batch_counts(rows); rid = str(report.get("run_id") or "unknown")
        mark = "PIN" if rid in pins else "   "
        print(f"{i:>2}. [{mark}] {rid} | {report.get('status','UNKNOWN')} | PASS={counts['PASS']} FAIL={counts['FAIL']} BLOCKED={counts['BLOCKED']} PREFLIGHT_FAIL={counts['PREFLIGHT_FAIL']}")
    print("Manage: report --pin/--unpin/--delete/--export <run_id> | report --cleanup")
    return 0


def _pin_history(root: Path, run_id: str, pin: bool) -> int:
    if _find_history_entry(root, run_id) is None:
        print(f"Run not found: {run_id}", file=sys.stderr); return 2
    pins = _load_pinned_runs(root)
    if pin: pins.add(run_id)
    else: pins.discard(run_id)
    _save_pinned_runs(root, pins)
    print(f"RUN {'PINNED' if pin else 'UNPINNED'}: {run_id}")
    return 0


def _delete_history(root: Path, run_id: str) -> int:
    found = _find_history_entry(root, run_id)
    if found is None:
        print(f"Run not found: {run_id}", file=sys.stderr); return 2
    path, _report = found
    try: path.unlink()
    except OSError as exc:
        print(f"Cannot delete history record: {exc}", file=sys.stderr); return 2
    run_dir = _batch_run_dir(root, run_id)
    if run_dir.is_dir() and not run_dir.is_symlink(): shutil.rmtree(run_dir, ignore_errors=True)
    pins = _load_pinned_runs(root); pins.discard(run_id); _save_pinned_runs(root, pins)
    last = _load_previous_run(root)
    if isinstance(last, dict) and str(last.get("run_id")) == run_id:
        try: (_artifact_run_root(root) / "LAST_RUN.json").unlink()
        except OSError: pass
    print(f"RUN DELETED: {run_id}")
    return 0


def _export_history(root: Path, run_id: str) -> int:
    found = _find_history_entry(root, run_id)
    if found is None:
        print(f"Run not found: {run_id}", file=sys.stderr); return 2
    history_path, report = found
    out_dir = _artifact_run_root(root) / "exports"; out_dir.mkdir(parents=True, exist_ok=True)
    final = out_dir / f"PTV_RUN_{_safe_slug(run_id,64)}.zip"; temp = final.with_suffix(".zip.tmp")
    run_dir = _batch_run_dir(root, run_id)
    with zipfile.ZipFile(temp, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.write(history_path, "RUN.json")
        if run_dir.is_dir():
            for path in sorted(run_dir.rglob("*")):
                if path.is_file() and not path.is_symlink(): zf.write(path, f"run/{path.relative_to(run_dir).as_posix()}")
    os.replace(temp, final)
    print(f"RUN EXPORT: {final}")
    return 0


def _batch_report_menu(root: Path, report: dict[str, object]) -> None:
    rows = _report_rows(report)
    while True:
        _print_batch_overview(root, report)
        print("Menu: N=detail | a=aggregate | p=PASS | x=problems | c=changed | d N=diff | s N=support ZIP | h=history | q=exit")
        try: raw = input("report> ").strip()
        except (EOFError, KeyboardInterrupt): print(""); return
        low = raw.lower()
        if low in {"", "q", "quit", "esc"}: return
        if low in {"a", "all", "aggregate"}:
            rel = report.get("batch_log")
            if isinstance(rel, str): _show_file_paged(root / rel, "AGGREGATE BATCH LOG")
            else: print("Aggregate log is unavailable.")
            continue
        if low == "p":
            _print_row_subset([(i,r) for i,r in enumerate(rows,1) if r.get("status")=="PASS"], "PASS ITEMS"); continue
        if low == "x":
            _print_row_subset([(i,r) for i,r in enumerate(rows,1) if r.get("status") in {"FAIL","BLOCKED","PREFLIGHT_FAIL"}], "PROBLEM ITEMS"); continue
        if low == "c":
            _print_row_subset([(i,r) for i,r in enumerate(rows,1) if isinstance(r.get("source_compare"),dict) and (r["source_compare"].get("changed_paths") or [])], "ITEMS WITH SOURCE CHANGES"); continue
        if low == "h": _list_history(root); continue
        m = re.fullmatch(r"([ds])\s+(\d+)", low)
        if m and 1 <= int(m.group(2)) <= len(rows):
            idx = int(m.group(2))-1
            if m.group(1) == "d": _show_source_compare(root, rows[idx])
            else: _create_report_support_bundle(root, report, idx)
            continue
        if low.isdigit() and 1 <= int(low) <= len(rows):
            _print_item_detail(root, rows[int(low)-1]); continue
        print("Lựa chọn không hợp lệ.")


def _load_report_by_run_id(root: Path, run_id: str | None) -> dict[str, object] | None:
    if not run_id:
        latest = _load_previous_run(root)
        if isinstance(latest, dict) and latest.get("selected"): return latest
        for _path, data in _history_entries(root):
            if data.get("selected"): return data
        return latest
    found = _find_history_entry(root, run_id)
    return found[1] if found else None


def _report_command(
    root: Path, run_id: str | None = None, *, list_runs: bool = False,
    pin_run: str | None = None, unpin_run: str | None = None,
    delete_run: str | None = None, export_run: str | None = None,
    cleanup: bool = False, support_item: int | None = None,
) -> int:
    if list_runs: return _list_history(root)
    if pin_run: return _pin_history(root, pin_run, True)
    if unpin_run: return _pin_history(root, unpin_run, False)
    if delete_run: return _delete_history(root, delete_run)
    if export_run: return _export_history(root, export_run)
    if cleanup:
        result = _cleanup_history(root); print(f"HISTORY CLEANUP: removed={result['removed']} pinned={result['pinned']} remaining={result['remaining']}"); return 0
    report = _load_report_by_run_id(root, run_id)
    if not report:
        print("No matching Patch Tool run report is available.", file=sys.stderr); return 2
    if support_item is not None:
        return 0 if _create_report_support_bundle(root, report, support_item-1) is not None else 2
    _print_batch_overview(root, report)
    if sys.stdin.isatty() and sys.stdout.isatty(): _batch_report_menu(root, report)
    return 0


def _selector_term_size() -> tuple[int, int]:
    """Return live terminal (columns, rows) for fullscreen selector redraw.

    A direct TTY query wins over COLUMNS/LINES because environment values can
    become stale after resize. Width protects against physical line wrapping;
    height protects against a frame scrolling beyond the visible screen, which
    would make cursor-up redraw accounting incorrect for long queues.
    """
    try:
        fd = sys.stdout.fileno()
        size = os.get_terminal_size(fd)
        if size.columns > 0 and size.lines > 0:
            return size.columns, size.lines
    except Exception:
        pass

    def _positive_env(name: str, fallback: int) -> int:
        raw = os.environ.get(name)
        if raw:
            try:
                value = int(raw)
                if value > 0:
                    return value
            except (TypeError, ValueError):
                pass
        return fallback

    return _positive_env("COLUMNS", 120), _positive_env("LINES", 24)


def _selector_term_width() -> int:
    return _selector_term_size()[0]


def _selector_term_height() -> int:
    return _selector_term_size()[1]


def _selector_viewport(count: int, cursor: int, capacity: int) -> tuple[int, int]:
    """Return a stable visible item slice that always contains cursor.

    Keeping the frame at most terminal-height minus one physical row prevents
    scrolling from invalidating the next ``CSI n F`` redraw.  The slice is
    centered when practical and remains stable at the beginning/end.
    """
    if count <= 0:
        return 0, 0
    capacity = max(1, min(int(capacity), count))
    cursor = min(max(int(cursor), 0), count - 1)
    start = cursor - capacity // 2
    start = max(0, min(start, count - capacity))
    return start, start + capacity


def _display_cell_width(text: str) -> int:
    width = 0
    for ch in _safe_display(text):
        if unicodedata.combining(ch):
            continue
        width += 2 if unicodedata.east_asian_width(ch) in {"W", "F"} else 1
    return width


def _clip_selector_line(text: str, terminal_width: int | None = None) -> str:
    """Clip one fullscreen row so cursor-up redraw accounting stays exact."""
    clean = _safe_display(text)
    cols = _selector_term_width() if terminal_width is None else int(terminal_width)
    limit = max(1, cols - 2)
    if _display_cell_width(clean) <= limit:
        return clean
    if limit == 1:
        return "…"
    out: list[str] = []
    used = 0
    target = limit - 1
    for ch in clean:
        w = 0 if unicodedata.combining(ch) else (2 if unicodedata.east_asian_width(ch) in {"W", "F"} else 1)
        if used + w > target:
            break
        out.append(ch)
        used += w
    return "".join(out) + "…"


def _selection_mark(index: int, selected: set[int], priorities: dict[int, int]) -> str:
    if index not in selected:
        return " "
    if index in priorities:
        return str(priorities[index])
    return "x"


def _ordered_selection(items: list[QueueItem], selected: set[int], priorities: dict[int, int]):
    """Return selected items in requested execution order.

    Explicit 0..9 priorities run first from low to high. Items selected with
    plain [x] have no explicit priority and run afterwards in the tool's
    existing natural queue order. Stable original indexes break ties so equal
    priorities never disturb the queue ordering already shown to the user.
    """
    indexes = sorted(
        selected,
        key=lambda i: (priorities.get(i, 10), i),
    )
    return [items[i] for i in indexes]


def _render(items, cursor, selected, priorities, msg, prev):
    terminal_width, terminal_height = _selector_term_size()

    # Keep one physical row free below the frame.  Writing a frame as tall as
    # the terminal can itself trigger a scroll on the final newline, after
    # which cursor-up no longer returns to the frame's true first row.
    frame_budget = max(1, terminal_height - 1)
    full_footer = [
        "",
        "Space: chọn/bỏ [x] | 0-9: gán ưu tiên | ↑/↓: di chuyển",
        "a: tất cả PATCH [x] | n: bỏ tất cả | d: xóa | i: inspect PATCH | v: validate | h: health",
        "Enter: xác nhận | q/Esc: hủy | Số nhỏ chạy trước; cùng số giữ thứ tự hiện tại",
        _safe_display(msg) if msg else "",
    ]
    compact_help = "Space/[0-9]/↑↓ | i inspect | v validate | h health | Enter chạy | q hủy"

    # Scale the fixed UI down before sacrificing the cursor row. Even an
    # extremely short terminal must show the current item and must never write
    # more physical rows than terminal_height-1.
    if frame_budget >= 10:
        header_rows = 2
        footer = full_footer
    elif frame_budget >= 5:
        header_rows = 1
        footer = [compact_help, _safe_display(msg) if msg else ""]
    elif frame_budget == 4:
        header_rows = 1
        footer = [_safe_display(msg) if msg else compact_help]
    elif frame_budget >= 2:
        header_rows = 1
        footer = []
    else:
        header_rows = 0
        footer = []

    item_capacity = max(1, frame_budget - header_rows - len(footer))
    start, end = _selector_viewport(len(items), cursor, item_capacity)

    current_tag = f"CON TRỎ {cursor + 1}/{len(items)}" if items else "CON TRỎ 0/0"
    # Put the cursor identity first. On narrow terminals horizontal clipping
    # preserves the left side, so the operator must never lose i/N merely
    # because the decorative Vietnamese title is longer than the viewport.
    if header_rows == 2:
        if len(items) > item_capacity:
            header = [f"{current_tag} | VIEW {start + 1}-{end}/{len(items)} | CHỌN CÔNG VIỆC SẼ CHẠY", ""]
        else:
            header = [f"{current_tag} | CHỌN CÔNG VIỆC SẼ CHẠY", ""]
    elif header_rows == 1:
        header = [f"{current_tag} | CHỌN CÔNG VIỆC SẼ CHẠY"]
    else:
        header = []

    lines: list[tuple[str, bool]] = [(line, False) for line in header]
    for i in range(start, end):
        item = items[i]
        detail = f"  [{_safe_display(item.detail)}]" if item.detail else ""
        lines.append((
            f"{'›' if i == cursor else ' '} "
            f"[{_selection_mark(i, selected, priorities)}] {i + 1:>3}. "
            f"[{_safe_display(item.kind)}] {_safe_display(item.name)}{detail}",
            i == cursor,
        ))
    lines.extend((line, False) for line in footer)

    # Clip horizontally BEFORE adding ANSI emphasis. The current row is shown
    # in bold reverse video so a long filename can never make the user lose the
    # cursor position after Space/priority/navigation redraws. Every logical
    # row remains exactly one physical row.
    lines = [(_clip_selector_line(line, terminal_width), current) for line, current in lines[:frame_budget]]
    frame_height = max(prev, len(lines))
    # If the terminal shrank vertically, never attempt to cursor-up more rows
    # than are now physically addressable. Clearing the visible budget is safer
    # than letting an old oversized frame corrupt the new viewport.
    cursor_up = min(prev, max(0, frame_budget))
    if cursor_up:
        sys.stdout.write(f"\x1b[{cursor_up}F")
    padded = lines + [("", False)] * (max(len(lines), min(frame_height, frame_budget)) - len(lines))
    use_emphasis = bool(getattr(sys.stdout, "isatty", lambda: False)())
    for line, current in padded:
        if current and use_emphasis:
            rendered_line = "\x1b[1;7m" + line + "\x1b[0m"
        else:
            rendered_line = line
        sys.stdout.write("\r\x1b[2K" + rendered_line + "\n")
    sys.stdout.flush()
    return len(padded)


def _readline_or_interrupt():
    try:
        return sys.stdin.readline(), False
    except KeyboardInterrupt:
        return "", True


def _parse_index_spec(spec: str, count: int):
    """Parse the documented line-selector grammar into zero-based indexes."""
    text = spec.strip().lower()
    if not text:
        return set()
    result: set[int] = set()
    for token in re.split(r"[\s,]+", text):
        if not token:
            continue
        if re.fullmatch(r"\d+", token):
            value = int(token)
            if not 1 <= value <= count:
                raise ValueError(f"index out of range: {value}")
            result.add(value - 1)
            continue
        match = re.fullmatch(r"(\d+)-(\d+)", token)
        if match:
            first, last = (int(match.group(1)), int(match.group(2)))
            if first > last:
                first, last = last, first
            if first < 1 or last > count:
                raise ValueError(f"range out of bounds: {token}")
            result.update(range(first - 1, last))
            continue
        raise ValueError(f"invalid selector token: {token}")
    return result


def _load_zero_argument_config(root: Path):
    """Read only the previously documented zero-argument selection settings.

    Invalid or stale config must never turn into implicit execution; callers
    receive safe prompt defaults plus warnings.
    """
    cfg = {
        "selection": "prompt",
        "non_interactive_confirmed": False,
        "initial_selection": "none",
        "selector_ui": "auto",
        "failure_policy": "fail_fast",
        "transaction_policy": "patch",
    }
    warnings: list[str] = []
    path = root / ".python_patch_tool.json"
    if not path.is_file():
        return cfg, warnings
    try:
        data = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
        node = data.get("automation", {}).get("zero_argument", {})
    except Exception as exc:
        warnings.append(f"invalid .python_patch_tool.json; using prompt defaults ({type(exc).__name__})")
        return cfg, warnings
    if not isinstance(node, dict):
        warnings.append("automation.zero_argument is not an object; using prompt defaults")
        return cfg, warnings

    selection = str(node.get("selection", cfg["selection"])).lower()
    if selection in {"prompt", "all", "first", "newest"}:
        cfg["selection"] = selection
    else:
        warnings.append(f"unsupported zero_argument.selection={selection!r}; using prompt")

    cfg["non_interactive_confirmed"] = node.get("non_interactive_confirmed") is True

    initial = str(node.get("initial_selection", cfg["initial_selection"])).lower()
    if initial in {"none", "all"}:
        cfg["initial_selection"] = initial
    else:
        warnings.append(f"unsupported initial_selection={initial!r}; using none")

    selector_ui = str(node.get("selector_ui", cfg["selector_ui"])).lower()
    if selector_ui in {"auto", "line"}:
        cfg["selector_ui"] = selector_ui
    else:
        warnings.append(f"unsupported selector_ui={selector_ui!r}; using auto")

    batch = data.get("batch", {}) if isinstance(data, dict) else {}
    if isinstance(batch, dict):
        failure_policy = str(batch.get("failure_policy", cfg["failure_policy"])).lower()
        if failure_policy in {"fail_fast", "continue_independent"}:
            cfg["failure_policy"] = failure_policy
        else:
            warnings.append(f"unsupported batch.failure_policy={failure_policy!r}; using fail_fast")
        transaction_policy = str(batch.get("transaction_policy", cfg["transaction_policy"])).lower()
        if transaction_policy in {"patch", "batch"}:
            cfg["transaction_policy"] = transaction_policy
        else:
            warnings.append(f"unsupported batch.transaction_policy={transaction_policy!r}; using patch")
    elif batch not in ({}, None):
        warnings.append("batch config is not an object; using safe defaults")
    return cfg, warnings


def _selection_contract_error(chosen: list[QueueItem]) -> str | None:
    """Enforce per-invocation COLLECT exclusivity without process locking.

    A CODE_COLLECTION_REQUEST is intentionally a standalone readonly job: at
    most one COLLECT-like item may be selected, and it may not be mixed with
    PATCH work. This protects request/result lifecycle while still allowing
    independent terminals/processes to run concurrently when the operator
    chooses to do so.
    """
    collect_like = [item for item in chosen if item.kind.startswith("COLLECT")]
    if len(collect_like) > 1:
        return "CODE_COLLECTION_REQUEST chỉ được chọn đúng 1 cái mỗi lần; không thể chạy nhiều COLLECT cùng lúc"
    if collect_like and len(chosen) > 1:
        return "CODE_COLLECTION_REQUEST phải chạy riêng; không thể chọn COLLECT cùng với PATCH"
    return None


def _selected_collect_index(items: list[QueueItem], selected: set[int]) -> int | None:
    for i in selected:
        if 0 <= i < len(items) and items[i].kind.startswith("COLLECT"):
            return i
    return None


def _initial_selected(items, initial_selection: str):
    if len(items) == 1:
        return {0}
    if initial_selection == "all":
        # "all" means all PATCHes. Multiple COLLECT requests are never
        # auto-selected together and COLLECT is never mixed with PATCH.
        return {i for i, item in enumerate(items) if item.kind == "PATCH"}
    return set()


def _delete_indexes(
    root: Path,
    items: list[QueueItem],
    selected: set[int],
    indexes: set[int],
    priorities: dict[int, int] | None = None,
):
    """Delete queue files while preserving selection/priority index mapping."""
    priorities = dict(priorities or {})
    deleted: list[str] = []
    failures: list[str] = []
    for i in sorted(indexes, reverse=True):
        victim = items[i]
        target = root / "patchs" / victim.name
        try:
            # Queue discovery rejects symlinks, and unlink never follows one.
            target.unlink()
        except FileNotFoundError:
            # An external process already removed it: treat it as no longer queued.
            pass
        except OSError as exc:
            failures.append(f"{victim.name}: {type(exc).__name__}")
            continue
        deleted.append(victim.name)
        items.pop(i)
        selected = {j if j < i else j - 1 for j in selected if j != i}
        priorities = {
            (j if j < i else j - 1): value
            for j, value in priorities.items()
            if j != i
        }
    if len(items) == 1:
        selected = {0}
        # Auto-selected sole item follows normal tool order unless the surviving
        # item already had an explicit priority.
    priorities = {j: value for j, value in priorities.items() if j in selected}
    return selected, priorities, list(reversed(deleted)), list(reversed(failures))


def _select_items_line(root: Path, items: list[QueueItem], initial_selection: str):
    selected = _initial_selected(items, initial_selection)
    while items:
        print("CHỌN CÔNG VIỆC SẼ CHẠY")
        for i, item in enumerate(items, 1):
            mark = "x" if i - 1 in selected else " "
            print(f"  [{mark}] {i}. [{_safe_display(item.kind)}] {_safe_display(item.name)}")
        print("Nhập: 1,3-5 | a=all PATCH | n=none | d <range>=xóa | i <số>=inspect PATCH | v <số>=validate PATCH | h=health | q=quit | Enter=xác nhận")
        raw_line, interrupted = _readline_or_interrupt()
        if interrupted:
            print("\nCancelled by Ctrl+C.")
            raise KeyboardInterrupt
        if raw_line == "":
            return None
        raw = raw_line.strip().lower()
        if raw in {"q", "quit"}:
            return None
        if raw in {"h", "health"}:
            rc = print_health(root, compact=False)
            print(f"TOOL HEALTH rc={rc}")
            continue
        if raw in {"a", "all"}:
            patches = [item for item in items if item.kind == "PATCH"]
            if patches:
                return patches
            print("Không có PATCH để chọn tất cả; COLLECT phải chọn từng request một.")
            continue
        if raw in {"n", "none"}:
            selected.clear()
            continue
        if raw.startswith("i "):
            try:
                indexes = _parse_index_spec(raw[2:].strip(), len(items))
            except ValueError as exc:
                print(f"Inspect không hợp lệ: {_safe_display(str(exc))}")
                continue
            if len(indexes) != 1:
                print("Inspect yêu cầu đúng một PATCH index.")
                continue
            index = next(iter(indexes))
            try: sys.stdout.flush()
            except Exception: pass
            rc = _inspect_item(root, items[index])
            print(f"INSPECT rc={rc}")
            continue
        if raw.startswith("v "):
            try:
                indexes = _parse_index_spec(raw[2:].strip(), len(items))
            except ValueError as exc:
                print(f"Validate không hợp lệ: {_safe_display(str(exc))}")
                continue
            if len(indexes) != 1:
                print("Validate yêu cầu đúng một PATCH index.")
                continue
            index = next(iter(indexes))
            rc = _validate_item(root, items[index])
            print(f"VALIDATE rc={rc}")
            continue
        if raw.startswith("d "):
            try:
                indexes = _parse_index_spec(raw[2:].strip(), len(items))
            except ValueError as exc:
                print(f"Lựa chọn xóa không hợp lệ: {_safe_display(str(exc))}")
                continue
            if not indexes:
                print("Chưa chọn item để xóa.")
                continue
            names = ", ".join(_safe_display(items[i].name) for i in sorted(indexes))
            sys.stdout.write(f"Xóa vĩnh viễn {names}? [y/N]: ")
            sys.stdout.flush()
            confirm, interrupted = _readline_or_interrupt()
            if interrupted:
                print("\nCancelled by Ctrl+C.")
                raise KeyboardInterrupt
            if confirm == "" or confirm.strip().lower() != "y":
                print("Xóa đã hủy.")
                continue
            selected, _priorities, deleted, failures = _delete_indexes(root, items, selected, indexes)
            for name in deleted:
                print(f"DELETED: patchs/{_safe_display(name)}")
            for detail in failures:
                print(f"DELETE FAILED: {_safe_display(detail)}", file=sys.stderr)
            continue
        if raw == "":
            if selected:
                chosen = [items[i] for i in sorted(selected)]
                error = _selection_contract_error(chosen)
                if error:
                    print(f"Lựa chọn không hợp lệ: {error}")
                    continue
                return chosen
            print("Chưa chọn item nào.")
            continue
        try:
            selected = _parse_index_spec(raw, len(items))
        except ValueError as exc:
            print(f"Lựa chọn không hợp lệ: {_safe_display(str(exc))}")
            continue
        if not selected:
            print("Chưa chọn item nào.")
            continue
        # A concrete number/range entry confirms the selection only when it
        # respects the COLLECT-exclusive contract.
        chosen = [items[i] for i in sorted(selected)]
        error = _selection_contract_error(chosen)
        if error:
            print(f"Lựa chọn không hợp lệ: {error}")
            continue
        return chosen
    return []


def select_items(root, items, *, initial_selection="none", selector_ui="auto"):
    if not items:
        return []
    # Full-screen controls support POSIX termios and native Windows msvcrt.
    use_posix_tty = (
        selector_ui != "line" and os.name != "nt" and termios is not None and tty is not None
        and sys.stdin.isatty() and sys.stdout.isatty()
    )
    use_windows_tty = (
        selector_ui != "line" and os.name == "nt" and msvcrt is not None
        and sys.stdin.isatty() and sys.stdout.isatty() and _enable_windows_vt()
    )
    if not (use_posix_tty or use_windows_tty):
        return _select_items_line(root, items, initial_selection)

    fd = sys.stdin.fileno()
    old = termios.tcgetattr(fd) if use_posix_tty else None
    if use_posix_tty:
        tty.setcbreak(fd)
    cursor = 0
    selected = _initial_selected(items, initial_selection)
    priorities: dict[int, int] = {}
    rendered = 0
    if len(items) == 1:
        msg = "Một item duy nhất đã chọn sẵn; Enter để chạy."
    elif selected:
        msg = "Các item đã được chọn theo cấu hình; Enter để chạy."
    else:
        msg = ""
    delete = False
    try:
        while items:
            rendered = _render(items, cursor, selected, priorities, msg, rendered)
            msg = ""
            key = _read_key(fd) if use_posix_tty else _read_key_windows()
            if key == "\x03":
                raise KeyboardInterrupt
            if key in {"q", "ESC"}:
                return None
            if delete:
                delete = False
                if key != "y":
                    msg = "Xóa đã hủy."
                    continue
                victim = items[cursor]
                selected, priorities, deleted, failures = _delete_indexes(
                    root, items, selected, {cursor}, priorities
                )
                if failures:
                    msg = f"Xóa thất bại: {failures[0]}"
                elif deleted and len(items) == 1:
                    msg = "Còn một item; đã chọn sẵn. Enter để chạy."
                elif deleted and not items:
                    msg = "Queue đã trống sau khi xóa."
                elif deleted:
                    msg = f"Đã xóa {deleted[0]}."
                cursor = min(cursor, max(0, len(items) - 1))
                continue
            if key == "UP":
                cursor = (cursor - 1) % len(items)
            elif key == "DOWN":
                cursor = (cursor + 1) % len(items)
            elif key == "SPACE":
                current = items[cursor]
                if current.kind.startswith("COLLECT"):
                    if selected == {cursor}:
                        selected.clear()
                    else:
                        selected = {cursor}
                    priorities.clear()
                    msg = "COLLECT chạy độc lập: chỉ request này được chọn." if selected else "Đã bỏ chọn COLLECT."
                else:
                    # Selecting PATCH while a COLLECT is selected switches the
                    # invocation back to PATCH mode instead of creating a mixed
                    # invalid selection.
                    collect_index = _selected_collect_index(items, selected)
                    if collect_index is not None:
                        selected.clear(); priorities.clear()
                    if cursor in priorities:
                        priorities.pop(cursor, None)
                        selected.add(cursor)
                    elif cursor in selected:
                        selected.remove(cursor)
                    else:
                        selected.add(cursor)
            elif key in {str(i) for i in range(10)}:
                current = items[cursor]
                if current.kind != "PATCH":
                    msg = "COLLECT không dùng priority 0-9; dùng Space để chọn riêng request này."
                else:
                    if _selected_collect_index(items, selected) is not None:
                        selected.clear(); priorities.clear()
                    selected.add(cursor)
                    priorities[cursor] = int(key)
                    msg = f"Ưu tiên {key}: {_safe_display(current.name)}"
            elif key == "a":
                selected = {i for i, item in enumerate(items) if item.kind == "PATCH"}
                priorities.clear()
                if not selected:
                    msg = "Không có PATCH để chọn tất cả; COLLECT phải chọn từng request một."
            elif key == "n":
                selected.clear()
                priorities.clear()
            elif key == "d":
                delete = True
                msg = f"Xóa {_safe_display(items[cursor].name)}? y để xác nhận"
            elif key == "i":
                # Temporarily restore canonical terminal mode while the inspect
                # subprocess prints its preflight report. Inspect never changes
                # selection and never executes the PATCH payload.
                if use_posix_tty:
                    termios.tcsetattr(fd, termios.TCSADRAIN, old)
                sys.stdout.write("\n--- PATCH INSPECT / DRY-RUN ---\n"); sys.stdout.flush()
                rc = _inspect_item(root, items[cursor])
                print("--- END INSPECT ---")
                if use_posix_tty:
                    tty.setcbreak(fd)
                rendered = 0
                msg = f"Inspect {'PASS' if rc == 0 else 'FAIL'} rc={rc}; selection unchanged."
            elif key == "v":
                if use_posix_tty:
                    termios.tcsetattr(fd, termios.TCSADRAIN, old)
                sys.stdout.write("\n--- PATCH VALIDATE ---\n"); sys.stdout.flush()
                rc = _validate_item(root, items[cursor])
                print("--- END VALIDATE ---")
                if use_posix_tty:
                    tty.setcbreak(fd)
                rendered = 0
                msg = f"Validate {'PASS' if rc == 0 else 'FAIL'} rc={rc}; selection unchanged."
            elif key == "h":
                # Health is a read-only self-audit of the installed tool. It
                # never changes queue selection or project source.
                if use_posix_tty:
                    termios.tcsetattr(fd, termios.TCSADRAIN, old)
                sys.stdout.write("\n--- TOOL HEALTH / SELF-AUDIT ---\n"); sys.stdout.flush()
                rc = print_health(root, compact=False)
                print("--- END TOOL HEALTH ---")
                if use_posix_tty:
                    tty.setcbreak(fd)
                rendered = 0
                msg = f"Tool health {'PASS' if rc == 0 else 'FAIL'} rc={rc}; selection unchanged."
            elif key == "ENTER":
                if selected:
                    chosen = _ordered_selection(items, selected, priorities)
                    error = _selection_contract_error(chosen)
                    if error:
                        msg = error
                        continue
                    return chosen
                msg = "Chưa chọn item nào."
    finally:
        if use_posix_tty:
            try:
                termios.tcsetattr(fd, termios.TCSADRAIN, old)
            except Exception:
                pass
        sys.stdout.write("\r\x1b[2K")
        sys.stdout.flush()
    return []


def _configured_auto_selection(root: Path, items: list[QueueItem], cfg: dict):
    mode = cfg.get("selection", "prompt")
    if mode == "prompt":
        return None
    if not cfg.get("non_interactive_confirmed", False):
        print(
            f"[PTV v{VERSION} WARNING] zero_argument.selection={mode!r} is not "
            "confirmed; falling back to prompt"
        )
        return None
    # The documented automation predates unified COLLECT routing. Do not make
    # a pre-existing PATCH automation setting start COLLECT jobs implicitly.
    if any(item.kind != "PATCH" for item in items):
        print(
            f"[PTV v{VERSION} WARNING] automatic selection is limited to a "
            "PATCH-only queue; mixed PATCH/COLLECT queue requires confirmation"
        )
        return None
    if mode == "all":
        return list(items)
    if mode == "first":
        return [items[0]]
    if mode == "newest":
        try:
            newest = max(items, key=lambda item: (root / "patchs" / item.name).stat().st_mtime_ns)
        except OSError as exc:
            print(
                f"[PTV v{VERSION} WARNING] newest selection failed ({type(exc).__name__}); "
                "falling back to prompt"
            )
            return None
        return [newest]
    return None


def _normalize_subprocess_rc(rc: int) -> int:
    """Map signal-style negative subprocess return codes to shell convention."""
    value = int(rc)
    return 128 + abs(value) if value < 0 else value


def _build_batch_plan(root: Path, chosen: list[QueueItem], available: list[QueueItem], previous: dict[str, object] | None):
    """Resolve explicit dependencies and the unresolved-predecessor action.

    This is intentionally filename/manifest-id based only; it does not perform
    target overlap/conflict analysis or add any new provenance identity layer.
    """
    if any(item.kind != "PATCH" for item in chosen):
        return list(chosen), {}, None
    work = list(chosen)
    by_name = {item.name: item for item in available if item.kind == "PATCH"}
    failed_name, failed_id = previous_failed_identity(previous if isinstance(previous, dict) else None)
    previous_action = None
    selected_names = {x.name for x in work}
    if failed_name and (root / "patchs" / failed_name).is_file() and failed_name not in selected_names and work:
        first_meta = load_patch_meta(root, work[0].name)
        previous_action = validate_previous_failure_declaration(first_meta, failed_name, failed_id)
        action = str(previous_action.get("action"))
        if action == "block":
            raise BatchPlanError(
                f"{work[0].name} explicitly blocks while failed predecessor {failed_name} is unresolved",
                kind="previous_failure_blocked",
            )
        if action in {"retry_before", "run_after"}:
            failed_item = by_name.get(failed_name) or QueueItem(failed_name, "PATCH")
            if not (root / "patchs" / failed_name).is_file():
                raise BatchPlanError(f"previous failed PATCH is unavailable for {action}: {failed_name}", kind="previous_failure_missing")
            if action == "retry_before":
                work.insert(0, failed_item)
            else:
                work.append(failed_item)

    metas = [load_patch_meta(root, item.name) for item in work]
    ordered_metas = topo_order(metas)
    item_by_name = {item.name: item for item in work}
    ordered = [item_by_name[m.name] for m in ordered_metas]

    # Explicit run_after is stronger than ordinary stable order. It is only
    # allowed when dependency declarations do not force the opposite order.
    if previous_action and previous_action.get("action") == "run_after" and failed_name:
        failed_meta = next((m for m in ordered_metas if m.name == failed_name), None)
        if failed_meta:
            for m in ordered_metas:
                if failed_meta.patch_id in m.depends_on:
                    raise BatchPlanError(
                        f"run_after conflicts with depends_on: {m.name} depends on {failed_meta.patch_id}",
                        kind="previous_failure_action_conflict",
                    )
            ordered = [x for x in ordered if x.name != failed_name] + [item_by_name[failed_name]]
            ordered_metas = [m for m in ordered_metas if m.name != failed_name] + [failed_meta]
    meta_map = {m.name: m for m in ordered_metas}
    return ordered, meta_map, previous_action


def _batch_preflight(root: Path, chosen: list[QueueItem], metas: dict[str, PatchMeta], *, run_id: str, transaction_policy: str):
    """Validate every selected package before the first source write.

    Dependent PATCHes may legitimately describe the post-dependency source.
    For those only, SOURCE_DRIFT is recorded as DEFERRED_AFTER_DEPENDENCY; the
    normal runner still performs the full source preflight immediately before
    execution. Schema/package/tool failures are never deferred.
    """
    run_dir = _batch_run_dir(root, run_id)
    out_dir = run_dir / "preflight"
    out_dir.mkdir(parents=True, exist_ok=True)
    rows: list[dict[str, object]] = []
    tx_issues = transaction_compatibility([metas[x.name] for x in chosen if x.name in metas], transaction_policy)
    if tx_issues:
        for issue in tx_issues:
            rows.append({"status": "FAIL", "classification": "BATCH_TRANSACTION_INVALID", "message": issue})
        return False, rows
    ok = True
    for index, item in enumerate(chosen, 1):
        if item.kind != "PATCH":
            continue
        meta = metas[item.name]
        log_path = out_dir / f"{index:03d}_{_safe_slug(item.name,96)}.log"
        result_path = out_dir / f"{index:03d}_{_safe_slug(item.name,96)}.result.json"
        env = os.environ.copy()
        env["PTV_PATCH_RESULT_FILE"] = str(result_path)
        env["PYTHONDONTWRITEBYTECODE"] = "1"
        try:
            cp = subprocess.run(
                _runner_command(root, "validate", item), cwd=root, text=True, env=env,
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, encoding="utf-8", errors="replace", timeout=120,
            )
            text = cp.stdout or ""
        except Exception as exc:
            cp = None
            text = f"TOOL_ERROR: {type(exc).__name__}: {exc}\n"
        log_path.write_text(text, encoding="utf-8", errors="replace")
        patch_result = None
        if result_path.is_file() and not result_path.is_symlink():
            try:
                value = json.loads(result_path.read_text(encoding="utf-8"))
                if isinstance(value, dict): patch_result = value
            except Exception:
                patch_result = None
        rc = 2 if cp is None else _normalize_subprocess_rc(cp.returncode)
        classification = "READY_TO_APPLY" if rc == 0 else "UNKNOWN"
        for candidate in ("PATCH_INVALID", "SOURCE_DRIFT", "TOOL_ERROR", "READY_TO_APPLY"):
            if candidate in text:
                classification = candidate
                break
        deferred = bool(rc != 0 and classification == "SOURCE_DRIFT" and meta.depends_on)
        status = "DEFERRED_AFTER_DEPENDENCY" if deferred else ("PASS" if rc == 0 else "FAIL")
        if status == "FAIL": ok = False
        if isinstance(patch_result, dict):
            # The planner already parsed the manifest, so preserve its PATCH/recovery
            # metadata even when read-only preflight fails before runner returns it.
            if not isinstance(patch_result.get("manifest_patch"), dict):
                patch_result["manifest_patch"] = meta.manifest.get("patch") if isinstance(meta.manifest.get("patch"), dict) else None
            if patch_result.get("recovery") is None and isinstance(meta.manifest.get("recovery"), dict):
                patch_result["recovery"] = meta.manifest.get("recovery")
        rows.append({
            "name": item.name, "patch_id": meta.patch_id, "status": status,
            "classification": classification, "rc": rc,
            "log_path": log_path.relative_to(root).as_posix(),
            "result_path": result_path.relative_to(root).as_posix() if result_path.is_file() else None,
            "patch_result": patch_result,
            "depends_on": list(meta.depends_on),
        })
    return ok, rows


def _safe_to_continue_after_failure(detail: dict[str, object]) -> tuple[bool, str]:
    if int(detail.get("rc") or 0) == 130:
        return False, "interrupted"
    result = detail.get("patch_result") if isinstance(detail.get("patch_result"), dict) else {}
    diagnosis = result.get("diagnosis") if isinstance(result.get("diagnosis"), dict) else {}
    kind = str(diagnosis.get("kind") or "")
    if kind in {"tool_error", "internal_error", "rollback_failed", "rollback_incomplete", "package_invalid", "rollback_snapshot_race"}:
        return False, kind or "critical_failure"
    rollback = result.get("rollback") if isinstance(result.get("rollback"), dict) else None
    if rollback is not None and rollback.get("status") == "PASS":
        return True, "per_patch_rollback_restored"
    partial = result.get("partial_modification") if isinstance(result.get("partial_modification"), dict) else {}
    if partial.get("detected") is False:
        return True, "no_partial_modification"
    return False, "unsafe_partial_or_unknown_state"


def _apply_previous_failure_action(root: Path, action: dict[str, object] | None) -> dict[str, object] | None:
    if not action or action.get("action") != "delete":
        return action
    name = str(action.get("patch_file") or "")
    if not name:
        return action
    src = root / "patchs" / name
    if not src.is_file() or src.is_symlink():
        action["result"] = "already_absent"
        return action
    ignore = root / "patchs" / "ignore"
    ignore.mkdir(parents=True, exist_ok=True)
    date = datetime.now().strftime("%Y-%m-%d")
    dst = ignore / f"{date}-{name}"
    n = 2
    while dst.exists():
        dst = ignore / f"{date}-{n}-{name}"; n += 1
    os.replace(src, dst)
    action["result"] = "moved_to_ignore"
    action["ignore_path"] = dst.relative_to(root).as_posix()
    print(f"PREVIOUS FAILED PATCH ACTION: DELETE -> {action['ignore_path']}")
    return action


def _declared_targets_for(meta: PatchMeta | None) -> list[str]:
    return list(meta.effective_targets) if meta is not None else []


def _capture_item_compare_before(root: Path, run_id: str, index: int, item: QueueItem, meta: PatchMeta | None):
    if item.kind != "PATCH" or meta is None or not meta.effective_targets:
        return None
    base = _batch_run_dir(root, run_id) / "source_compare" / f"{index:03d}_{_safe_slug(item.name,72)}"
    before_dir = base / "before"
    before = capture_compare_snapshot(root, meta.effective_targets, before_dir)
    return base, before_dir, before


def _capture_item_compare_after(root: Path, captured, meta: PatchMeta | None):
    if captured is None or meta is None:
        return None
    base, before_dir, before = captured
    after_dir = base / "after"
    after = capture_compare_snapshot(root, meta.effective_targets, after_dir)
    diff_path = base / "source.diff"
    info = build_diff_artifact(root, before, after, before_dir, after_dir, diff_path)
    try:
        info["diff_path"] = diff_path.relative_to(root).as_posix()
    except ValueError:
        pass
    return info


def _collect_archive_postcondition(root: Path, item: QueueItem) -> tuple[bool, str]:
    """Verify the established COLLECT PASS queue lifecycle.

    A successful readonly collection must move its request ZIP from patchs/ to
    patchs/patched/.  Reporting PASS while the request remains runnable causes
    accidental repeated collections on the next zero-argument run.
    """
    source = root / "patchs" / item.name
    archived = root / "patchs" / "patched" / item.name
    try:
        if archived.is_symlink() or not archived.is_file():
            return False, f"COLLECT rc=0 but archived request is missing: patchs/patched/{item.name}"
        archived_sha = _sha256_file(archived)
    except OSError as exc:
        return False, f"COLLECT archive verification failed: {type(exc).__name__}"
    if source.exists() or source.is_symlink():
        try:
            if source.is_symlink() or not source.is_file():
                return False, f"COLLECT rc=0 but unsafe request replacement remains queued: patchs/{item.name}"
            if _sha256_file(source) == archived_sha:
                return False, f"COLLECT rc=0 but executed request is still queued: patchs/{item.name}"
            return True, f"request filename was replaced during COLLECT and the replacement remains queued: patchs/{item.name}"
        except OSError as exc:
            return False, f"COLLECT replacement verification failed: {type(exc).__name__}"
    return True, ""


def execute_items(
    root: Path,
    chosen: list[QueueItem],
    *,
    failure_policy: str = "fail_fast",
    metas: dict[str, PatchMeta] | None = None,
):
    """Execute a validated batch with controlled continuation and dependency blocking."""
    global _LAST_EXECUTION_DETAILS
    _LAST_EXECUTION_DETAILS = []
    metas = metas or {}
    contract_error = _selection_contract_error(chosen)
    if contract_error:
        print(f"[PTV v{VERSION} ERROR] SELECTION: {_safe_display(contract_error)}", file=sys.stderr)
        return 2, [], list(chosen), [], []
    executed: list[tuple[str, int]] = []
    late_duplicates: list[LocalDuplicate] = []
    duplicate_warnings: list[str] = []
    patch_status_by_id: dict[str, str] = {}
    first_failure_rc = 0

    for index, item in enumerate(chosen):
        item_started_mono = time.monotonic()
        item_started_at = _utc_now()
        detail_log_path = _batch_item_log_path(root, _ACTIVE_RUN_ID, index + 1, item)
        meta = metas.get(item.name)

        if item.kind == "PATCH" and meta is not None and meta.depends_on:
            failed_deps = [dep for dep in meta.depends_on if patch_status_by_id.get(dep) not in {"PASS", "SKIPPED_DUPLICATE_LOCAL"}]
            if failed_deps and meta.on_dependency_failure != "run_anyway":
                detail = {
                    "name": item.name, "kind": item.kind, "status": "BLOCKED", "rc": None,
                    "started_at": item_started_at,
                    "elapsed_seconds": round(time.monotonic() - item_started_mono, 3),
                    "blocked_by": failed_deps,
                    "diagnosis": {"kind": "dependency_failed", "message": f"blocked by failed dependency: {', '.join(failed_deps)}"},
                }
                _LAST_EXECUTION_DETAILS.append(detail)
                patch_status_by_id[meta.patch_id] = "BLOCKED"
                print(f"[BLOCKED] {_safe_display(item.name)} | dependency failure: {_safe_display(', '.join(failed_deps))}")
                continue

        if item.kind == "PATCH":
            still_runnable, now_duplicates, now_warnings = _split_local_duplicate_patches(root, [item])
            for warning in now_warnings:
                if warning not in duplicate_warnings:
                    duplicate_warnings.append(warning)
            if now_duplicates:
                now_duplicates, ignore_warnings = _move_local_duplicates_to_ignore(root, now_duplicates)
                late_duplicates.extend(now_duplicates)
                duplicate_warnings.extend(x for x in ignore_warnings if x not in duplicate_warnings)
                _print_local_duplicate_skips(now_duplicates)
                detail = {"name": item.name, "kind": item.kind, "status": "SKIPPED_DUPLICATE_LOCAL", "rc": 0}
                if now_duplicates and now_duplicates[0].ignored_name:
                    detail["ignore_path"] = f"patchs/ignore/{now_duplicates[0].ignored_name}"
                _LAST_EXECUTION_DETAILS.append(detail)
                if meta is not None:
                    patch_status_by_id[meta.patch_id] = "SKIPPED_DUPLICATE_LOCAL"
                continue
            if not still_runnable:
                duplicate_warnings.append(f"late duplicate check returned no decision for patchs/{item.name}; executing normally")
            cmd = _runner_command(root, "execute", item)
        elif item.kind == "COLLECT":
            progress = root / "tools" / "_patch_lib" / "python_patch_collect_progress_v6_7.py"
            compat = root / "tools" / "_patch_lib" / "python_patch_collect_compat.py"
            cmd = [sys.executable, str(progress), "--project-root", str(root), "--collector", str(compat), "--", "request", f"patchs/{item.name}"]
        else:
            rc = 2
            executed.append((item.name, rc))
            _LAST_EXECUTION_DETAILS.append({"name": item.name, "kind": item.kind, "status": "FAIL", "rc": rc, "diagnosis": {"kind": "invalid_queue_item"}})
            return rc, executed, chosen[index + 1 :], late_duplicates, duplicate_warnings

        compare_before = _capture_item_compare_before(root, _ACTIVE_RUN_ID or "run", index + 1, item, meta)
        try:
            try:
                sys.stdout.flush(); sys.stderr.flush()
            except Exception:
                pass
            console_log = ""
            patch_result = None
            collect_result = None
            if item.kind == "PATCH":
                rc, console_log, patch_result = _run_patch_child(root, cmd, item, full_log_path=detail_log_path)
                patch_result = _enrich_patch_diagnosis(patch_result, console_log)
            else:
                runtime = _artifact_run_root(root) / "runtime"
                runtime.mkdir(parents=True, exist_ok=True)
                collect_result_path = runtime / f"collect_{int(time.time()*1000000)}_{os.getpid()}.json"
                env = dict(os.environ); env["PTV_COLLECT_RESULT_FILE"] = str(collect_result_path)
                rc = _normalize_subprocess_rc(subprocess.run(cmd, cwd=root, env=env).returncode)
                collect_result = _load_json(collect_result_path)
                try: collect_result_path.unlink()
                except OSError: pass
        except KeyboardInterrupt:
            rc = 130
            console_log = "INTERRUPTED by Ctrl+C\n" if item.kind == "PATCH" else ""
            patch_result = None
            print(f"[PTV v{VERSION}] INTERRUPTED by Ctrl+C", file=sys.stderr)

        compare_info = _capture_item_compare_after(root, compare_before, meta)
        if rc == 0 and item.kind == "COLLECT":
            ok, post_detail = _collect_archive_postcondition(root, item)
            if not ok:
                print(f"[PTV v{VERSION} ERROR] {_safe_display(post_detail)}", file=sys.stderr)
                rc = 3
            elif post_detail:
                print(f"[PTV v{VERSION} WARNING] {_safe_display(post_detail)}")

        detail: dict[str, object] = {
            "name": item.name, "kind": item.kind,
            "status": "PASS" if rc == 0 else "FAIL", "rc": rc,
            "started_at": item_started_at,
            "elapsed_seconds": round(time.monotonic() - item_started_mono, 3),
        }
        if meta is not None:
            detail["patch_id"] = meta.patch_id
            detail["depends_on"] = list(meta.depends_on)
        if detail_log_path is not None and detail_log_path.is_file():
            try: detail["log_path"] = detail_log_path.relative_to(root).as_posix()
            except ValueError: detail["log_path"] = str(detail_log_path)
        if patch_result is not None:
            detail["patch_result"] = patch_result
        if compare_info is not None:
            detail["source_compare"] = compare_info
        if item.kind == "COLLECT" and collect_result is not None:
            detail["collect_result"] = collect_result
        executed.append((item.name, rc))

        if rc and item.kind == "PATCH":
            recovery_request = _create_recovery_collect_request(root, item, patch_result or {})
            if recovery_request is not None:
                detail["recovery_collect_request"] = recovery_request.name
                print(f"[NEXT RUN - COLLECT REQUEST READY] patchs/{_safe_display(recovery_request.name)}")
            handoff = _create_fail_handoff(root, item, rc, console_log, patch_result, recovery_request)
            if handoff is not None:
                try: detail["fail_handoff"] = handoff.relative_to(root).as_posix()
                except ValueError: detail["fail_handoff"] = str(handoff)
        _LAST_EXECUTION_DETAILS.append(detail)
        if meta is not None:
            patch_status_by_id[meta.patch_id] = str(detail["status"])

        if rc:
            if not first_failure_rc:
                first_failure_rc = rc
            if failure_policy != "continue_independent" or item.kind != "PATCH":
                return first_failure_rc, executed, chosen[index + 1 :], late_duplicates, duplicate_warnings
            safe, reason = _safe_to_continue_after_failure(detail)
            detail["continue_decision"] = {"allowed": safe, "reason": reason}
            if not safe:
                print(f"[PTV v{VERSION} SAFETY STOP] continue-on-failure blocked: {_safe_display(reason)}", file=sys.stderr)
                return first_failure_rc, executed, chosen[index + 1 :], late_duplicates, duplicate_warnings
            print(f"[PTV v{VERSION}] CONTINUE AFTER FAILURE: {_safe_display(item.name)} | {_safe_display(reason)}")

    return first_failure_rc, executed, [], late_duplicates, duplicate_warnings


def _resume_groups(previous: dict[str, object] | None) -> dict[str, list[str]]:
    groups = {"replay": [], "failed": [], "remaining": []}
    if not isinstance(previous, dict) or previous.get("status") != "FAIL":
        return groups
    rows = _report_rows(previous)
    for row in rows:
        name = row.get("name")
        if not isinstance(name, str):
            continue
        status = str(row.get("status") or "")
        if row.get("batch_rolled_back") is True and status == "PASS":
            groups["replay"].append(name)
        elif status == "FAIL":
            groups["failed"].append(name)
        elif status in {"BLOCKED", "NOT_EXECUTED", "PREFLIGHT_FAIL"}:
            groups["remaining"].append(name)
    return groups


def _resume_selection(root: Path, items: list[QueueItem], previous: dict[str, object] | None, *, mode: str | None = None):
    groups = _resume_groups(previous)
    by_name = {x.name: x for x in items}
    def available(names): return [by_name[n] for n in names if n in by_name]
    all_unresolved = available(groups["replay"] + groups["failed"] + groups["remaining"])
    failed_only = available(groups["failed"])
    remaining_only = available(groups["remaining"])
    if not all_unresolved:
        return None
    if mode in {"all", "failed", "remaining"}:
        return {"all": all_unresolved, "failed": failed_only, "remaining": remaining_only}[mode]
    if not (sys.stdin.isatty() and sys.stdout.isatty()):
        return None
    print("\nSMART RESUME — previous batch is incomplete")
    print(f"Replay after batch rollback: {len(groups['replay'])} | Failed: {len(groups['failed'])} | Remaining/blocked: {len(groups['remaining'])}")
    print("  1. Retry/replay all unresolved items in original order")
    print("  2. Retry failed PATCHes only")
    print("  3. Run remaining/blocked items only (dependency/predecessor rules still apply)")
    print("  4. Ignore resume suggestion and open normal queue selector")
    try:
        answer = input("resume> ").strip().lower()
    except (EOFError, KeyboardInterrupt):
        return None
    if answer == "1": return all_unresolved
    if answer == "2": return failed_only
    if answer == "3": return remaining_only
    return None


def _run_queue(root: Path, *, failure_policy_override: str | None = None, transaction_policy_override: str | None = None, force_resume: bool = False, resume_mode: str | None = None):
    global _ACTIVE_RUN_ID, _LAST_EXECUTION_DETAILS
    started_mono = time.monotonic()
    started_at = _utc_now()
    run_id = f"{int(time.time()*1000000)}_{os.getpid()}"
    _ACTIVE_RUN_ID = run_id
    previous = _load_previous_run(root)
    resume_items = _print_resume_hint(root, previous)
    queue_safety_error: str | None = None
    try:
        items, warnings = discover_queue(root)
    except QueueSafetyError as exc:
        items, warnings = [], []
        queue_safety_error = str(exc)
    items, session_duplicates, session_duplicate_warnings = _split_session_duplicate_patches(root, items)
    items, local_duplicates, duplicate_warnings = _split_local_duplicate_patches(root, items)
    local_duplicates, ignore_warnings = _move_local_duplicates_to_ignore(root, local_duplicates)
    duplicate_warnings = [*duplicate_warnings, *ignore_warnings]
    printed_warnings = set()
    for warning in [*warnings, *session_duplicate_warnings, *duplicate_warnings]:
        if warning in printed_warnings: continue
        printed_warnings.add(warning)
        print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")
    if local_duplicates:
        _print_local_duplicate_skips(local_duplicates)

    cfg, config_warnings = _load_zero_argument_config(root)
    for warning in config_warnings:
        print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")
    failure_policy = failure_policy_override or str(cfg.get("failure_policy") or "fail_fast")
    transaction_policy = transaction_policy_override or str(cfg.get("transaction_policy") or "patch")
    if failure_policy not in {"fail_fast", "continue_independent"}: failure_policy = "fail_fast"
    if transaction_policy not in {"patch", "batch"}: transaction_policy = "patch"
    batch_preflight_rows: list[dict[str, object]] = []
    previous_action: dict[str, object] | None = None
    batch_transaction: dict[str, object] | None = None

    def finish_report(status: str, rc: int, *, chosen: list[QueueItem] | None = None, executed=None, remaining=None, failed_item=None):
        chosen = chosen or []; executed = executed or []; remaining = remaining or []
        report: dict[str, object] = {
            "format": "python-patch-tool-last-run", "format_version": 2, "tool_version": VERSION,
            "run_id": run_id, "started_at": started_at, "finished_at": _utc_now(),
            "elapsed_seconds": round(time.monotonic() - started_mono, 3), "status": status, "exit_code": int(rc),
            "selected": [item.name for item in chosen], "execution_order": [name for name, _ in executed],
            "results": list(_LAST_EXECUTION_DETAILS), "not_executed": [item.name for item in remaining],
            "failed_item": failed_item, "failure_policy": failure_policy, "transaction_policy": transaction_policy,
            "batch_preflight": list(batch_preflight_rows), "previous_failure_action": previous_action,
            "batch_transaction": batch_transaction,
            "session_duplicates_removed": [
                {"name": d.item.name, "canonical": d.canonical_name, "sha256": d.sha256, "removed": d.removed}
                for d in session_duplicates
            ],
            "local_history_skipped": [
                {"name": d.item.name, "history_name": d.history_name, "sha256": d.sha256,
                 "ignore_path": f"patchs/ignore/{d.ignored_name}" if d.ignored_name else None}
                for d in local_duplicates
            ],
            "previous_resume_items": resume_items,
            "previous_failed_item": (previous.get("failed_item") or previous.get("previous_failed_item")) if isinstance(previous, dict) else None,
        }
        _finalize_batch_artifacts(root, report)
        _write_run_report(root, report)
        if len(report.get("selected") or []) > 1:
            if sys.stdin.isatty() and sys.stdout.isatty(): _batch_report_menu(root, report)
            else: _print_batch_overview(root, report, stream=sys.stderr if status == "FAIL" else sys.stdout)
        return rc

    if queue_safety_error is not None:
        print(f"[PTV v{VERSION} ERROR] QUEUE SAFETY: {_safe_display(queue_safety_error)}", file=sys.stderr)
        return finish_report("FAIL", 2)
    if not items:
        if session_duplicates:
            print("AUTO STATUS: IDLE — no new runnable package remains after duplicate filtering."); _print_session_duplicate_removals(session_duplicates)
        elif local_duplicates:
            print("AUTO STATUS: IDLE — local duplicate PATCHes were moved to patchs/ignore; no runnable package remains.")
        else:
            print("AUTO STATUS: IDLE — no runnable patch/collect package is waiting in patchs/.")
        print_health(root, compact=True)
        return finish_report("IDLE", 0)

    chosen = None
    if force_resume or (isinstance(previous, dict) and previous.get("status") == "FAIL"):
        chosen = _resume_selection(root, items, previous, mode=resume_mode)
        if chosen:
            print(f"SMART RESUME SELECTED: {len(chosen)} item(s)")
    if chosen is None:
        chosen = _configured_auto_selection(root, items, cfg)
    if chosen is None:
        try:
            chosen = select_items(root, list(items), initial_selection=cfg.get("initial_selection", "none"), selector_ui=cfg.get("selector_ui", "auto"))
        except KeyboardInterrupt:
            print("\nCancelled by Ctrl+C."); return finish_report("CANCELLED", 130)
    if chosen is None:
        print("Cancelled."); _print_session_duplicate_removals(session_duplicates); return finish_report("CANCELLED", 0)
    if not chosen:
        print("AUTO STATUS: IDLE — queue is empty or no runnable item remains; nothing executed.")
        _print_session_duplicate_removals(session_duplicates); return finish_report("IDLE", 0)

    # Resolve dependency order and enforce unresolved-predecessor handling before source changes.
    try:
        chosen, metas, previous_action = _build_batch_plan(root, chosen, items, previous)
    except Exception as exc:
        kind = getattr(exc, "kind", "batch_plan_invalid")
        _LAST_EXECUTION_DETAILS = [{
            "name": x.name, "kind": x.kind, "status": "PREFLIGHT_FAIL" if i == 0 else "NOT_EXECUTED", "rc": 2 if i == 0 else None,
            "diagnosis": {"kind": kind, "message": str(exc)},
        } for i, x in enumerate(chosen)]
        print(f"BATCH PREFLIGHT FAIL — project unchanged | {kind}: {_safe_display(str(exc))}", file=sys.stderr)
        return finish_report("FAIL", 2, chosen=chosen, remaining=chosen, failed_item=chosen[0].name if chosen else None)

    print(f"BATCH POLICY: failure={failure_policy} | transaction={transaction_policy}")
    if len(chosen) > 1:
        print("BATCH ORDER:")
        for i, item in enumerate(chosen, 1):
            meta = metas.get(item.name)
            dep = f" | depends_on={','.join(meta.depends_on)}" if meta and meta.depends_on else ""
            print(f"  {i}. {_safe_display(item.name)}{dep}")

    # Whole-batch preflight is a multi-PATCH/dependency/transaction gate. A
    # standalone ordinary PATCH keeps the established runner preflight/result
    # contract, while successor-action handling still preflights before moving
    # an unresolved predecessor out of the queue.
    needs_batch_preflight = (len(chosen) > 1 or transaction_policy == "batch" or previous_action is not None or any(m.depends_on for m in metas.values()))
    if needs_batch_preflight:
        preflight_ok, batch_preflight_rows = _batch_preflight(root, chosen, metas, run_id=run_id, transaction_policy=transaction_policy)
    else:
        preflight_ok, batch_preflight_rows = True, []
    if not preflight_ok:
        _LAST_EXECUTION_DETAILS = []
        fail_names = {str(r.get("name")) for r in batch_preflight_rows if r.get("status") == "FAIL" and r.get("name")}
        not_executed_items: list[QueueItem] = []
        for item in chosen:
            row = next((r for r in batch_preflight_rows if r.get("name") == item.name), None)
            if item.name in fail_names:
                patch_result = row.get("patch_result") if isinstance(row, dict) and isinstance(row.get("patch_result"), dict) else None
                if patch_result is None:
                    kind = str(row.get("classification") if row else "batch_preflight_failed").lower()
                    patch_result = {
                        "format": "python-patch-tool-patch-result", "format_version": 1, "tool_version": VERSION,
                        "patch_file": item.name, "patch_sha256": _sha256_file(root / "patchs" / item.name) if (root / "patchs" / item.name).is_file() else None,
                        "status": "FAIL", "rc": 2, "stage": "preflight",
                        "diagnosis": {"kind": kind, "message": "batch preflight rejected PATCH", "affected_paths": []},
                        "partial_modification": {"detected": False, "changed_paths": [], "evidence": "read_only_batch_preflight"},
                    }
                diagnosis = patch_result.get("diagnosis") if isinstance(patch_result.get("diagnosis"), dict) else {"kind": str(row.get("classification") if row else "batch_preflight_failed"), "message": "batch preflight rejected PATCH", "affected_paths": []}
                recovery_request = _create_recovery_collect_request(root, item, patch_result)
                log_text = ""
                if isinstance(row, dict) and isinstance(row.get("log_path"), str):
                    try: log_text = (root / str(row["log_path"])).read_text(encoding="utf-8", errors="replace")
                    except OSError: log_text = ""
                fail_handoff = _create_fail_handoff(root, item, 2, log_text, patch_result, recovery_request)
                detail = {
                    "name": item.name, "kind": item.kind, "status": "PREFLIGHT_FAIL", "rc": 2,
                    "diagnosis": diagnosis, "patch_result": patch_result,
                    "preflight_log_path": row.get("log_path") if row else None,
                    "recovery_collect_request": recovery_request.relative_to(root).as_posix() if recovery_request is not None else None,
                    "fail_handoff": fail_handoff.relative_to(root).as_posix() if fail_handoff is not None else None,
                }
                _LAST_EXECUTION_DETAILS.append(detail)
                if isinstance(row, dict):
                    row["recovery_collect_request"] = detail["recovery_collect_request"]
                    row["fail_handoff"] = detail["fail_handoff"]
            else:
                not_executed_items.append(item)
                _LAST_EXECUTION_DETAILS.append({"name": item.name, "kind": item.kind, "status": "NOT_EXECUTED", "rc": None,
                    "diagnosis": {"kind": "batch_preflight_failed_elsewhere", "message": "no payload executed because whole-batch preflight failed"},
                    "preflight_log_path": row.get("log_path") if row else None})
        print("BATCH PREFLIGHT: FAIL — no selected PATCH modified source", file=sys.stderr)
        for row in batch_preflight_rows:
            if row.get("status") == "FAIL": print(f"  - {_safe_display(str(row.get('name') or 'batch'))}: {_safe_display(str(row.get('classification')))}", file=sys.stderr)
        failed = next((x.name for x in chosen if x.name in fail_names), chosen[0].name if chosen else None)
        return finish_report("FAIL", 2, chosen=chosen, remaining=not_executed_items, failed_item=failed)
    if needs_batch_preflight:
        print("BATCH PREFLIGHT: PASS — all packages validated before first source write")
    deferred = [r for r in batch_preflight_rows if r.get("status") == "DEFERRED_AFTER_DEPENDENCY"]
    if deferred:
        print(f"  Deferred source checks after declared dependencies: {len(deferred)} (runner revalidates immediately before execution)")

    previous_action = _apply_previous_failure_action(root, previous_action)

    transaction_snapshot_root = None
    package_snapshot_root = None
    transaction_manifest = None
    package_map = None
    batch_mutation_lock = None
    batch_mutation_env_previous = (
        os.environ.get("PTV_PARENT_MUTATION_LOCK_KEY"),
        os.environ.get("PTV_PARENT_MUTATION_LOCK_TOKEN"),
    )
    if transaction_policy == "batch":
        try:
            batch_mutation_lock, batch_lock_key, batch_lock_token = _acquire_batch_mutation_lock(root)
            os.environ["PTV_PARENT_MUTATION_LOCK_KEY"] = batch_lock_key
            os.environ["PTV_PARENT_MUTATION_LOCK_TOKEN"] = batch_lock_token
            tx_root = _batch_run_dir(root, run_id) / "transaction"
            transaction_snapshot_root = tx_root / "source"
            package_snapshot_root = tx_root / "packages"
            all_targets = [rel for item in chosen for rel in _declared_targets_for(metas.get(item.name))]
            transaction_manifest = snapshot_targets(root, all_targets, transaction_snapshot_root)
            package_map = snapshot_package_bytes(root, [x.name for x in chosen if x.kind == "PATCH"], package_snapshot_root)
            batch_transaction = {"policy": "batch", "status": "READY", "targets": len(set(all_targets)), "packages": len(package_map)}
            print(f"BATCH TRANSACTION SNAPSHOT: READY | targets={len(set(all_targets))} | packages={len(package_map)}")
        except Exception as exc:
            kind = getattr(exc, "kind", "batch_transaction_snapshot_failed")
            _LAST_EXECUTION_DETAILS = [{"name": x.name, "kind": x.kind, "status": "NOT_EXECUTED", "rc": None,
                "diagnosis": {"kind": kind, "message": str(exc)}} for x in chosen]
            batch_transaction = {"policy": "batch", "status": "FAIL", "error": str(exc)}
            print(f"BATCH TRANSACTION PREFLIGHT FAIL — project unchanged | {_safe_display(str(exc))}", file=sys.stderr)
            _restore_batch_mutation_env(batch_mutation_env_previous)
            _release_batch_mutation_lock(batch_mutation_lock)
            batch_mutation_lock = None
            return finish_report("FAIL", 2, chosen=chosen, remaining=chosen)

    try:
        rc, executed, remaining, late_duplicates, late_duplicate_warnings = execute_items(root, chosen, failure_policy=failure_policy, metas=metas)
        local_duplicates = [*local_duplicates, *late_duplicates]
        for warning in late_duplicate_warnings:
            if warning in printed_warnings: continue
            printed_warnings.add(warning); print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}", file=sys.stderr if rc else sys.stdout)

        if transaction_policy == "batch" and transaction_snapshot_root is not None and transaction_manifest is not None:
            if rc:
                restored = restore_targets(root, transaction_snapshot_root, transaction_manifest)
                requeued = requeue_packages(root, package_snapshot_root, package_map or {}) if package_snapshot_root is not None else {}
                batch_transaction = {"policy": "batch", "status": "ROLLED_BACK" if restored.get("status") == "PASS" else "ROLLBACK_FAILED",
                    "restored_paths": restored.get("restored_paths", []), "errors": restored.get("errors", []), "requeued_packages": requeued}
                rollback_ok = restored.get("status") == "PASS"
                for detail in _LAST_EXECUTION_DETAILS:
                    if detail.get("status") in {"PASS", "FAIL"}:
                        detail["batch_rollback_attempted"] = True
                        detail["batch_rollback_status"] = "PASS" if rollback_ok else "FAIL"
                        detail["batch_rolled_back"] = rollback_ok
                        if detail.get("name") in requeued: detail["requeued_as"] = requeued[detail.get("name")]
                if not rollback_ok:
                    batch_transaction["original_rc"] = rc
                    rc = 70
                    print("!!! BATCH ROLLBACK FAILED — manual recovery required !!!", file=sys.stderr)
                else:
                    print(f"BATCH ROLLBACK: PASS | restored={len(restored.get('restored_paths') or [])} | replay packages requeued={len(requeued)}")
            else:
                batch_transaction = {"policy": "batch", "status": "COMMITTED", "targets": len(transaction_manifest.get("entries") or [])}
                print("BATCH TRANSACTION: COMMITTED")

    finally:
        if batch_mutation_lock is not None:
            _restore_batch_mutation_env(batch_mutation_env_previous)
            _release_batch_mutation_lock(batch_mutation_lock)
            batch_mutation_lock = None

    if rc:
        failed_rows = [x for x in _LAST_EXECUTION_DETAILS if x.get("status") == "FAIL"]
        failed_name = str(failed_rows[-1].get("name")) if failed_rows else (executed[-1][0] if executed else None)
        fail_count = len(failed_rows)
        print(f"SUMMARY: FAIL | failed={fail_count} | policy={failure_policy} | last={_safe_display(str(failed_name or 'unknown'))} rc={rc}", file=sys.stderr)
        if remaining:
            print(f"NOT EXECUTED: {len(remaining)} selected item(s)", file=sys.stderr)
            for item in remaining: print(f"  - {_safe_display(item.name)}", file=sys.stderr)
        if session_duplicates: _print_session_duplicate_removals(session_duplicates, stream=sys.stderr)
        finish_report("FAIL", rc, chosen=chosen, executed=executed, remaining=remaining, failed_item=failed_name)
        _print_patch_result_banner("FAIL", _last_patch_name(status="FAIL"), rc=rc, stream=sys.stderr)
        return rc

    if session_duplicates: _print_session_duplicate_removals(session_duplicates)
    completed_count = len([x for x in _LAST_EXECUTION_DETAILS if x.get("status") == "PASS"])
    session_duplicate_count = len(session_duplicates); local_duplicate_count = len(local_duplicates)
    local_duplicate_moved = sum(1 for d in local_duplicates if d.ignored_name)
    local_duplicate_move_failed = local_duplicate_count - local_duplicate_moved
    duplicate_count = session_duplicate_count + local_duplicate_count
    if duplicate_count:
        suffix = f" | {local_duplicate_move_failed} ignore move failure(s)" if local_duplicate_move_failed else ""
        print(f"SUMMARY: PASS | {completed_count} item(s) completed | {session_duplicate_count} duplicate file(s) collapsed in-session | {local_duplicate_moved} local duplicate(s) moved to ignore{suffix}")
    else:
        print(f"SUMMARY: PASS | {completed_count} item(s) completed")
    finish_report("PASS", 0, chosen=chosen, executed=executed)
    _print_patch_result_banner("PASS", _last_patch_name(status="PASS"))
    return 0


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--project-root", required=True)
    ap.add_argument("command", nargs="?", choices=["run", "resume", "report"], default="run")
    ap.add_argument("--failure-policy", choices=["fail_fast", "continue_independent"])
    ap.add_argument("--transaction-policy", choices=["patch", "batch"])
    ap.add_argument("--resume-mode", choices=["all", "failed", "remaining"])
    ap.add_argument("--run-id")
    ap.add_argument("--list", action="store_true", dest="list_runs")
    ap.add_argument("--pin")
    ap.add_argument("--unpin")
    ap.add_argument("--delete")
    ap.add_argument("--export")
    ap.add_argument("--cleanup", action="store_true")
    ap.add_argument("--support-item", type=int)
    ns = ap.parse_args(argv)
    root = Path(ns.project_root).resolve()
    if ns.command == "report":
        return _report_command(
            root, ns.run_id, list_runs=ns.list_runs, pin_run=ns.pin, unpin_run=ns.unpin,
            delete_run=ns.delete, export_run=ns.export, cleanup=ns.cleanup, support_item=ns.support_item,
        )
    # Deliberately NO process-wide/project-wide queue lock. Selection isolation
    # is per invocation only; operators may run other Patch Tool processes in
    # separate terminals when they intentionally choose to do so.
    return _run_queue(
        root, failure_policy_override=ns.failure_policy, transaction_policy_override=ns.transaction_policy,
        force_resume=(ns.command == "resume"), resume_mode=ns.resume_mode,
    )


if __name__ == "__main__":
    raise SystemExit(main())
