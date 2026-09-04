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
import textwrap
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
from python_patch_project_state import (
    update_patch_ledger, ledger_id_reuse, disk_preflight, load_project_config,
)
from python_patch_batch import (
    BatchPlanError, PatchMeta, load_patch_meta, topo_order,
    validate_previous_failure_declaration, transaction_compatibility, snapshot_targets,
    restore_targets, snapshot_package_bytes, requeue_packages, capture_compare_snapshot,
    build_diff_artifact, stable_package_sha256, analyze_static_conflicts,
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


VERSION = "6.20.0"
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
_ACTIVE_LIVE_STATUS_PANEL = None


class _PatchChildSignal(BaseException):
    def __init__(self, signum: int):
        super().__init__(signum)
        self.signum = int(signum)


def _raise_patch_child_signal(signum, _frame):
    raise _PatchChildSignal(int(signum))


def _windows_taskkill_tree(proc: subprocess.Popen, *, force: bool) -> None:
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


def _forward_patch_signal(proc: subprocess.Popen, signum: int) -> None:
    if proc.poll() is not None:
        return
    try:
        if os.name != "nt":
            os.killpg(proc.pid, signum)
        else:
            if signum == getattr(signal, "SIGKILL", -999):
                _windows_taskkill_tree(proc, force=True)
                return
            ctrl_break = getattr(signal, "CTRL_BREAK_EVENT", None)
            if ctrl_break is not None and signum in {getattr(signal, "SIGINT", 2), getattr(signal, "SIGTERM", 15)}:
                try:
                    proc.send_signal(ctrl_break)
                    return
                except Exception:
                    pass
            _windows_taskkill_tree(proc, force=False)
    except (ProcessLookupError, OSError):
        pass

MAX_PATCH_CAPTURE_BYTES = 8 * 1024 * 1024
MAX_HANDOFF_SOURCE_FILE_BYTES = 32 * 1024 * 1024
MAX_HANDOFF_SOURCE_TOTAL_BYTES = 128 * 1024 * 1024
MAX_HANDOFF_SOURCE_FILES = 256
MAX_HANDOFF_SCAN_FILES = 25000
MAX_HANDOFF_REFERENCE_TEXT_BYTES = 1024 * 1024
MAX_HANDOFF_DETAIL_LOG_BYTES = 64 * 1024 * 1024
MAX_HANDOFF_LOG_EVIDENCE_BYTES = 32 * 1024 * 1024
_LOCAL_DB_PROFILE_REL_PATHS = {
    "tools/db_profiles.local.json",
    ".python_patch_tool/db_profiles.local.json",
}
_DB_PROFILE_ENV = "PTV_DB_PROFILES_FILE"
_HANDOFF_SCAN_SKIP_DIRS = {
    ".git", "node_modules", ".venv", "venv", "__pycache__", "build", "dist",
    "artifacts", "patchs", ".idea", ".vscode",
}
_HANDOFF_SOURCE_SUFFIXES = {
    ".py", ".pyi", ".c", ".h", ".cc", ".hh", ".cpp", ".hpp", ".cxx", ".hxx",
    ".m", ".mm", ".java", ".kt", ".kts", ".swift", ".go", ".rs", ".cs",
    ".js", ".jsx", ".ts", ".tsx", ".vue", ".svelte", ".php", ".rb", ".lua",
    ".sh", ".bash", ".zsh", ".fish", ".ps1", ".bat", ".cmd", ".sql", ".proto",
    ".html", ".htm", ".css", ".scss", ".sass", ".less", ".xml", ".json", ".json5",
    ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".properties", ".gradle",
}
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


def _safe_lock_directory(path: Path) -> Path:
    """Create/validate a real private lock directory without accepting links."""
    if path.exists() or path.is_symlink():
        st = path.lstat()
        attrs = getattr(st, "st_file_attributes", 0)
        reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
            raise QueueSafetyError(f"unsafe mutation lock directory: {path}")
    else:
        try:
            path.mkdir(mode=0o700, exist_ok=False)
        except FileExistsError:
            return _safe_lock_directory(path)
    return path


def _project_mutation_lock_path(root: Path) -> Path:
    top = _safe_lock_directory(Path(tempfile.gettempdir()) / "python_patch_tool_locks")
    base = _safe_lock_directory(top / _project_mutation_lock_key(root))
    return base / "mutation.lock"


def _open_mutation_lock_file(path: Path):
    if path.exists() or path.is_symlink():
        st = path.lstat()
        attrs = getattr(st, "st_file_attributes", 0)
        reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISREG(st.st_mode):
            raise QueueSafetyError(f"unsafe mutation lock file: {path}")
    flags = os.O_RDWR | os.O_CREAT | getattr(os, "O_BINARY", 0) | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
    try:
        fd = os.open(path, flags, 0o600)
    except OSError as exc:
        raise QueueSafetyError(f"cannot safely open mutation lock file: {path}: {type(exc).__name__}") from exc
    try:
        st = os.fstat(fd)
        attrs = getattr(st, "st_file_attributes", 0)
        reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if not stat.S_ISREG(st.st_mode) or reparse:
            raise QueueSafetyError(f"mutation lock descriptor is not a regular file: {path}")
        return os.fdopen(fd, "r+b")
    except Exception:
        os.close(fd)
        raise


def _acquire_batch_mutation_lock(root: Path):
    """Own the same mutation lock used by runner children for an atomic batch."""
    path = _project_mutation_lock_path(root)
    fh = _open_mutation_lock_file(path)
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


def _existing_artifact_subdir_readonly(root: Path, *parts: str) -> Path | None:
    """Return an existing artifact directory without creating any state.

    Read-only views such as HISTORY must not materialize artifacts/patch_tool on
    an otherwise untouched project.  The traversal keeps the same symlink /
    reparse / non-directory fail-closed policy as the mutating helpers.
    """
    cur = root.resolve(strict=True)
    for part in ("artifacts", "patch_tool", *parts):
        if not part or part in {".", ".."} or "/" in part or "\\" in part:
            raise QueueSafetyError(f"unsafe artifact subdirectory name: {part!r}")
        nxt = cur / part
        if not (nxt.exists() or nxt.is_symlink()):
            return None
        st = nxt.lstat()
        attrs = getattr(st, "st_file_attributes", 0)
        reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
            raise QueueSafetyError(f"unsafe artifact subdirectory component: {nxt}")
        cur = nxt
    return cur


def _artifact_subdir(root: Path, *parts: str) -> Path:
    cur = _artifact_run_root(root)
    for part in parts:
        if not part or part in {".", ".."} or "/" in part or "\\" in part:
            raise QueueSafetyError(f"unsafe artifact subdirectory name: {part!r}")
        nxt = cur / part
        if nxt.exists() or nxt.is_symlink():
            st = nxt.lstat()
            attrs = getattr(st, "st_file_attributes", 0)
            reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
            if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
                raise QueueSafetyError(f"unsafe artifact subdirectory component: {nxt}")
        else:
            try:
                nxt.mkdir(exist_ok=False)
            except FileExistsError:
                st = nxt.lstat()
                attrs = getattr(st, "st_file_attributes", 0)
                reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
                if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
                    raise QueueSafetyError(f"unsafe artifact subdirectory created concurrently: {nxt}")
        cur = nxt
    return cur


def _batch_run_dir(root: Path, run_id: str) -> Path:
    return _artifact_subdir(root, "runs", _safe_slug(run_id, 96))


def _batch_item_log_path(root: Path, run_id: str | None, index: int, item: QueueItem) -> Path | None:
    if not run_id:
        return None
    return _batch_run_dir(root, run_id) / "items" / f"{index:03d}_{_safe_slug(item.name, 96)}.log"


def _load_previous_run(root: Path) -> dict[str, object] | None:
    return _load_json(_artifact_run_root(root) / "LAST_RUN.json")


def _unresolved_registry_path(root: Path) -> Path:
    return _artifact_run_root(root) / "UNRESOLVED_FAILURES.json"


def _failure_identity(row: dict[str, object]) -> tuple[str, str, str, str]:
    """Stable unresolved-work identity for PATCH and COLLECT rows.

    PATCH keeps patch.id + immutable package SHA. COLLECT uses the immutable
    request SHA captured before the collector may archive the request. Older
    COLLECT history without SHA is filename-compatible only during migration.
    """
    kind = str(row.get("kind") or "PATCH").upper()
    result = row.get("patch_result") if isinstance(row.get("patch_result"), dict) else {}
    mp = result.get("manifest_patch") if isinstance(result.get("manifest_patch"), dict) else {}
    patch_id = row.get("patch_id") or mp.get("id") or ""
    if kind == "COLLECT":
        sha = row.get("request_sha256") or ""
        patch_id = ""
    else:
        sha = result.get("patch_sha256") or ""
    name = row.get("name") or ""
    return kind, str(patch_id), str(sha).lower(), str(name)


def _load_unresolved_registry(root: Path) -> dict[str, object]:
    data = _load_json(_unresolved_registry_path(root))
    if not isinstance(data, dict) or data.get("format") != "python-patch-tool-unresolved-failures":
        return {"format":"python-patch-tool-unresolved-failures","format_version":1,"tool_version":VERSION,"entries":[]}
    if not isinstance(data.get("entries"), list):
        data["entries"] = []
    return data


def _unresolved_failure_rows(root: Path) -> list[dict[str, object]]:
    data = _load_unresolved_registry(root)
    out: list[dict[str, object]] = []
    for entry in data.get("entries") or []:
        if not isinstance(entry, dict) or entry.get("resolved") is True:
            continue
        row = entry.get("row")
        if isinstance(row, dict):
            out.append(dict(row))
    return out


def _planning_previous(root: Path, previous: dict[str, object] | None) -> dict[str, object] | None:
    """Return the failure state the dependency planner must enforce.

    LAST_RUN is only the most recent invocation.  An older unresolved failure must
    remain a predecessor constraint even after an unrelated PASS/IDLE run replaces
    LAST_RUN; otherwise a successor could bypass batch.previous_failure by simply
    opening the normal queue.  Keep ordinary LAST_RUN failure semantics intact and
    synthesize the smallest compatible failure report only when the registry is the
    sole source of unresolved state.
    """
    if isinstance(previous, dict) and previous.get("status") == "FAIL":
        return previous
    rows = [row for row in _unresolved_failure_rows(root) if str(row.get("kind") or "PATCH").upper() == "PATCH"]
    if not rows:
        return previous
    row = dict(rows[-1])
    name = str(row.get("name") or "")
    return {
        "format": "python-patch-tool-planning-previous",
        "format_version": 1,
        "tool_version": VERSION,
        "status": "FAIL",
        "failed_item": name or None,
        "results": [row],
        "source": "UNRESOLVED_FAILURES.json",
    }


def _resolve_registry_rows(root: Path, rows: list[dict[str, object]], reason: str) -> None:
    if not rows:
        return
    data = _load_unresolved_registry(root)
    ids = {_failure_identity(row) for row in rows}
    changed = False
    now = _utc_now()
    for entry in data.get("entries") or []:
        if not isinstance(entry, dict) or entry.get("resolved") is True:
            continue
        row = entry.get("row") if isinstance(entry.get("row"), dict) else {}
        if _failure_identity(row) in ids:
            entry["resolved"] = True
            entry["resolved_at"] = now
            entry["resolution"] = reason
            changed = True
    if changed:
        data["tool_version"] = VERSION
        _atomic_json(_unresolved_registry_path(root), data)


def _resolve_registry_previous_action(root: Path, action: dict[str, object] | None) -> None:
    if not isinstance(action, dict) or action.get("action") != "delete" or action.get("result") not in {"moved_to_ignore", "already_absent"}:
        return
    patch_id = str(action.get("patch_id") or "")
    patch_file = str(action.get("patch_file") or "")
    expected_sha = str(action.get("patch_sha256") or "").lower()
    data = _load_unresolved_registry(root)
    rows: list[dict[str, object]] = []
    for entry in data.get("entries") or []:
        if not isinstance(entry, dict) or entry.get("resolved") is True:
            continue
        row = entry.get("row") if isinstance(entry.get("row"), dict) else {}
        kind, pid, sha, name = _failure_identity(row)
        if kind != "PATCH":
            continue
        if expected_sha:
            identity_match = sha == expected_sha and (
                (patch_id and pid == patch_id)
                or (not patch_id and patch_file and name == patch_file)
            )
        else:
            # Without package SHA, never resolve multiple entries by patch_id
            # alone.  Exact logical filename is the narrowest safe fallback.
            identity_match = bool(patch_file and name == patch_file and (not patch_id or pid == patch_id))
        if identity_match:
            rows.append(row)
    _resolve_registry_rows(root, rows, "superseded_by_previous_failure_delete")


def _update_unresolved_registry(root: Path, report: dict[str, object]) -> None:
    data = _load_unresolved_registry(root)
    entries = [x for x in (data.get("entries") or []) if isinstance(x, dict)]
    now = _utc_now()
    current_rows = [
        x for x in (report.get("results") or [])
        if isinstance(x, dict) and str(x.get("kind") or "PATCH").upper() in {"PATCH", "COLLECT"}
    ]

    # A successful run resolves an older failure only for the exact same
    # logical package bytes (SHA-bound identity), not patch.id reuse alone.
    for row in current_rows:
        if str(row.get("status") or "") != "PASS":
            continue
        kind, pid, sha, name = _failure_identity(row)
        for entry in entries:
            if entry.get("resolved") is True:
                continue
            old = entry.get("row") if isinstance(entry.get("row"), dict) else {}
            okind, opid, osha, oname = _failure_identity(old)
            # A later PASS resolves an unresolved failure only when it is the
            # exact same logical package bytes.  patch.id reuse with different
            # SHA is warning-worthy provenance, not proof that the old failure
            # has been repaired/superseded.
            match = bool(
                kind == okind and sha and osha and sha == osha
                and ((pid and opid == pid) or (not pid and name == oname))
            )
            if match:
                entry["resolved"] = True
                entry["resolved_at"] = now
                entry["resolution"] = f"PASS in run {report.get('run_id','')}"

    for row in current_rows:
        if str(row.get("status") or "") not in {"FAIL", "PREFLIGHT_FAIL", "INCOMPLETE"}:
            continue
        ident = _failure_identity(row)
        existing = None
        for entry in entries:
            old = entry.get("row") if isinstance(entry.get("row"), dict) else {}
            if entry.get("resolved") is not True and _failure_identity(old) == ident:
                existing = entry
                break
        frozen = dict(row)
        if existing is None:
            entries.append({
                "first_failed_at": now, "last_failed_at": now,
                "first_run_id": report.get("run_id"), "last_run_id": report.get("run_id"),
                "resolved": False, "row": frozen,
            })
        else:
            existing["last_failed_at"] = now
            existing["last_run_id"] = report.get("run_id")
            existing["row"] = frozen
    # Keep resolved history bounded while never dropping unresolved entries.
    unresolved = [x for x in entries if x.get("resolved") is not True]
    resolved = [x for x in entries if x.get("resolved") is True][-100:]
    data = {
        "format":"python-patch-tool-unresolved-failures", "format_version":1,
        "tool_version":VERSION, "updated_at":now, "entries": unresolved + resolved,
    }
    _atomic_json(_unresolved_registry_path(root), data)


def _merged_failed_recovery_rows(root: Path, previous: dict[str, object] | None) -> list[dict[str, object]]:
    unresolved_patches = [row for row in _unresolved_failure_rows(root) if str(row.get("kind") or "PATCH").upper() == "PATCH"]
    rows = [*_failed_recovery_rows(previous), *unresolved_patches]
    out: list[dict[str, object]] = []
    seen: set[tuple[str,str,str]] = set()
    for row in rows:
        ident = _failure_identity(row)
        if ident in seen:
            continue
        seen.add(ident)
        out.append(row)
    return out


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
    history = _existing_artifact_subdir_readonly(root, "history")
    out: list[tuple[Path, dict[str, object]]] = []
    if history is not None and history.is_dir():
        for path in sorted(history.glob("*.json"), reverse=True):
            data = _load_json(path)
            if isinstance(data, dict): out.append((path, data))
    return out


def _is_meaningful_run(report: dict[str, object] | None) -> bool:
    """True only for invocations that actually selected/executed PATCH/COLLECT work.

    IDLE probes are intentionally excluded from the user-facing history and
    Smart Resume semantics.  LAST_RUN may still be IDLE so automation can see
    the most recent invocation, while history remains useful to an operator.
    """
    if not isinstance(report, dict):
        return False
    selected = report.get("selected")
    if isinstance(selected, list) and any(isinstance(x, str) and x for x in selected):
        return True
    rows = report.get("results")
    return isinstance(rows, list) and any(isinstance(x, dict) and x.get("name") for x in rows)


def _visible_history_entries(root: Path) -> list[tuple[Path, dict[str, object]]]:
    return [(path, report) for path, report in _history_entries(root) if _is_meaningful_run(report)]


def _latest_meaningful_run(root: Path, latest: dict[str, object] | None = None) -> dict[str, object] | None:
    if _is_meaningful_run(latest):
        return latest
    entries = _visible_history_entries(root)
    return entries[0][1] if entries else None


def _automatic_resume_available(root: Path, items: list[QueueItem], previous: dict[str, object] | None) -> bool:
    """Return True only when the immediately recorded failed run has queue work to recover.

    Persistent unresolved failures remain planner constraints, but they must not
    force the interactive SMART RESUME menu in front of a new unrelated queue.
    Auto-resume is therefore intentionally narrower than the persistent registry:
    at least one replay/failed/remaining item from LAST_RUN must still exist in the
    current runnable queue.
    """
    if not isinstance(previous, dict) or previous.get("status") != "FAIL":
        return False
    by_name = {item.name for item in items}
    groups = _resume_groups(previous)
    names: list[str] = []
    for name in groups["replay"] + groups["failed"] + groups["remaining"]:
        if isinstance(name, str) and name and name not in names:
            names.append(name)
    return any(name in by_name for name in names)


def _queue_item_sha256(root: Path, item: QueueItem) -> str | None:
    path = root / "patchs" / item.name
    try:
        if path.is_symlink() or not path.is_file(): return None
        return _sha256_file(path).lower()
    except Exception:
        return None


def _failure_row_matches_queue_item(root: Path, row: dict[str, object], item: QueueItem) -> bool:
    row_kind = str(row.get("kind") or "PATCH").upper()
    if row_kind != str(item.kind or "PATCH").upper(): return False
    queue_name = _recovery_row_queue_name(row) or str(row.get("name") or "")
    if queue_name != item.name: return False
    _kind, _pid, expected_sha, _name = _failure_identity(row)
    if not expected_sha: return True
    actual_sha = _queue_item_sha256(root, item)
    return bool(actual_sha and actual_sha == expected_sha)


def _freeze_legacy_failure_row(root: Path, row: dict[str, object], item: QueueItem) -> dict[str, object]:
    frozen = dict(row); current_sha = _queue_item_sha256(root, item)
    if not current_sha: return frozen
    if str(item.kind or "PATCH").upper() == "COLLECT": frozen.setdefault("request_sha256", current_sha)
    else:
        result = dict(frozen.get("patch_result") or {}) if isinstance(frozen.get("patch_result"), dict) else {}
        result.setdefault("patch_sha256", current_sha); frozen["patch_result"] = result
    return frozen


def _reconcile_unresolved_registry_from_history(root: Path, items: list[QueueItem], previous: dict[str, object] | None) -> None:
    """Migrate queued failures created before persistent PATCH+COLLECT state."""
    if not items: return
    data = _load_unresolved_registry(root); entries=[x for x in (data.get("entries") or []) if isinstance(x,dict)]
    unresolved_ids={_failure_identity(x.get("row") if isinstance(x.get("row"),dict) else {}) for x in entries if x.get("resolved") is not True}
    reports=[]; seen_runs=set()
    if _is_meaningful_run(previous):
        reports.append(previous); rid=str(previous.get("run_id") or ""); seen_runs.add(rid) if rid else None
    for _path, report in _visible_history_entries(root):
        rid=str(report.get("run_id") or "")
        if rid and rid in seen_runs: continue
        reports.append(report); seen_runs.add(rid) if rid else None
    changed=False; now=_utc_now()
    for item in items:
        if any(isinstance(e.get("row"),dict) and e.get("resolved") is not True and _failure_row_matches_queue_item(root,e["row"],item) for e in entries): continue
        latest=None; latest_run=None
        for report in reports:
            for row in _report_rows(report):
                if isinstance(row,dict) and _failure_row_matches_queue_item(root,row,item): latest=row; latest_run=report.get("run_id"); break
            if latest is not None: break
        if latest is None: continue
        status=str(latest.get("status") or "")
        recovery=status in {"FAIL","PREFLIGHT_FAIL","INCOMPLETE"} or (status=="PASS" and latest.get("batch_rolled_back") is True)
        if not recovery: continue
        frozen=_freeze_legacy_failure_row(root,latest,item); ident=_failure_identity(frozen)
        if ident in unresolved_ids: continue
        entries.append({"first_failed_at":now,"last_failed_at":now,"first_run_id":latest_run,"last_run_id":latest_run,"resolved":False,"migration":"history_reconcile_v6_20_0","row":frozen})
        unresolved_ids.add(ident); changed=True
    if changed:
        _atomic_json(_unresolved_registry_path(root),{"format":"python-patch-tool-unresolved-failures","format_version":1,"tool_version":VERSION,"updated_at":now,"entries":entries})


def _persistent_failed_queue_rows(root: Path, items: list[QueueItem], previous: dict[str, object] | None) -> list[dict[str, object]]:
    _reconcile_unresolved_registry_from_history(root,items,previous)
    rows=list(_unresolved_failure_rows(root))
    if isinstance(previous,dict):
        for row in _report_rows(previous):
            if not isinstance(row,dict): continue
            status=str(row.get("status") or "")
            if status not in {"FAIL","PREFLIGHT_FAIL","INCOMPLETE"} and not (status=="PASS" and row.get("batch_rolled_back") is True): continue
            rows.append(dict(row))
    out=[]; seen=set()
    for row in rows:
        ident=_failure_identity(row)
        if ident in seen: continue
        seen.add(ident); out.append(row)
    return out


def _last_failed_queue_names(root: Path, items: list[QueueItem], previous: dict[str, object] | None) -> set[str]:
    """Return queued PATCH/COLLECT names with persistent unresolved state."""
    by_name={item.name:item for item in items}; failed=set()
    for row in _persistent_failed_queue_rows(root,items,previous):
        queue_name=_recovery_row_queue_name(row) or str(row.get("name") or "")
        item=by_name.get(queue_name)
        if item is not None and _failure_row_matches_queue_item(root,row,item): failed.add(queue_name)
    return failed


def _group_selector_items(items: list[QueueItem], failed_names: set[str] | None) -> list[QueueItem]:
    """Stable two-group presentation order: new first, previous-failed second."""
    names = set(failed_names or ())
    if not names:
        return list(items)
    return [item for item in items if item.name not in names] + [item for item in items if item.name in names]


def _find_history_entry(root: Path, run_id: str) -> tuple[Path, dict[str, object]] | None:
    for path, data in _history_entries(root):
        if str(data.get("run_id")) == str(run_id): return path, data
    return None


def _cleanup_history(root: Path) -> dict[str, int]:
    pins = _load_pinned_runs(root)
    entries = list(reversed(_history_entries(root)))  # oldest -> newest
    removed = 0

    def remove_entry(path: Path, data: dict[str, object]) -> bool:
        nonlocal removed
        run_id = str(data.get("run_id") or "")
        try:
            path.unlink()
            removed += 1
        except OSError:
            return False
        run_dir = _batch_run_dir(root, run_id)
        if run_dir.is_dir() and not run_dir.is_symlink():
            try: shutil.rmtree(run_dir)
            except OSError: pass
        return True

    # Historical IDLE probes are not operator history.  Remove every unpinned
    # one first so they cannot consume the 30-run meaningful-history budget.
    for path, data in list(entries):
        rid = str(data.get("run_id") or "")
        if rid not in pins and not _is_meaningful_run(data):
            remove_entry(path, data)

    remaining_entries = list(reversed(_history_entries(root)))
    meaningful = [(p, d) for p, d in remaining_entries if _is_meaningful_run(d)]
    remove_count = max(0, len(meaningful) - RUN_HISTORY_LIMIT)
    for path, data in meaningful:
        if remove_count <= 0:
            break
        if str(data.get("run_id") or "") in pins:
            continue
        if remove_entry(path, data):
            remove_count -= 1
    return {"removed": removed, "pinned": len(pins), "remaining": len(_visible_history_entries(root))}



def _write_run_report(root: Path, report: dict[str, object]) -> None:
    out = _artifact_run_root(root)
    try:
        _atomic_json(out / "LAST_RUN.json", report)
        # IDLE is an invocation state, not useful PATCH/COLLECT history.  Keep
        # LAST_RUN authoritative but do not create another user-history entry.
        if _is_meaningful_run(report):
            history = _artifact_subdir(root, "history")
            stamp = str(report.get("started_at", "run")).replace(":", "").replace("+", "_").replace("-", "").replace(".", "_")
            run_id = _safe_slug(str(report.get("run_id") or "run"), 64)
            _atomic_json(history / f"{stamp}_{run_id}.json", report)
            _cleanup_history(root)
    except Exception as exc:
        print(f"[PTV v{VERSION} WARNING] could not write LAST_RUN/history: {type(exc).__name__}: {exc}", file=sys.stderr)



class _LivePatchStatus:
    """Best-effort fixed PATCH status header for interactive ANSI terminals.

    The child output is already proxied through the dispatcher, so a terminal
    scroll region can keep a compact status list fixed while log lines scroll
    below it.  The panel is deliberately conservative: redirected output,
    dumb terminals, tiny terminals, COLLECT-only runs and explicit opt-out use
    the historical plain console path unchanged.
    """

    def __init__(self, items: list[QueueItem]):
        self.items = list(items)
        self.statuses = {item.name: "WAITING" for item in self.items}
        self.active = False
        self.cols = 0
        self.rows = 0
        self.header_height = 0
        self.max_status_rows = 0
        self._warned_resize = False

    @staticmethod
    def _supported(items: list[QueueItem]) -> bool:
        if not items or any(item.kind != "PATCH" for item in items):
            return False
        if os.environ.get("PTV_DISABLE_LIVE_STATUS", "").strip().lower() in {"1", "true", "yes", "on"}:
            return False
        if os.environ.get("TERM", "").strip().lower() == "dumb":
            return False
        if not (getattr(sys.stdout, "isatty", lambda: False)() and getattr(sys.stdin, "isatty", lambda: False)()):
            return False
        if os.name == "nt":
            try:
                if not _enable_windows_vt_stream(sys.stdout):
                    return False
            except Exception:
                return False
        return True

    @classmethod
    def start_for(cls, items: list[QueueItem]):
        panel = cls(items)
        if not panel._supported(items):
            return panel
        try:
            size = shutil.get_terminal_size(fallback=(120, 40))
            panel.cols, panel.rows = int(size.columns), int(size.lines)
            if panel.cols < 40 or panel.rows < 8:
                return panel
            # Leave at least five rows for live console output.  Very large
            # batches use a sliding status window rather than consuming the
            # whole terminal with the fixed panel.
            panel.max_status_rows = max(1, min(10, panel.rows - 6))
            visible_rows = min(len(panel.items), panel.max_status_rows)
            if len(panel.items) > visible_rows:
                # One fixed row is used for the "... N more" summary.
                visible_rows = max(1, visible_rows - 1) + 1
            panel.header_height = visible_rows + 1  # separator row
            if panel.header_height >= panel.rows - 3:
                return panel
            out = sys.stdout
            out.write("\x1b[r\x1b[2J\x1b[H")
            panel.active = True
            global _ACTIVE_LIVE_STATUS_PANEL
            _ACTIVE_LIVE_STATUS_PANEL = panel
            panel._render(initial=True)
            out.write(f"\x1b[{panel.header_height + 1};{panel.rows}r")
            out.write(f"\x1b[{panel.header_height + 1};1H")
            out.flush()
        except Exception:
            panel.active = False
        return panel

    def _current_index(self) -> int:
        for index, item in enumerate(self.items):
            if self.statuses.get(item.name) == "RUNNING":
                return index
        for index, item in enumerate(self.items):
            if self.statuses.get(item.name) == "WAITING":
                return index
        return max(0, len(self.items) - 1)

    def _visible_indexes(self) -> tuple[list[int], int]:
        count = len(self.items)
        capacity = max(1, self.max_status_rows)
        if count <= capacity:
            return list(range(count)), 0
        item_capacity = max(1, capacity - 1)
        current = self._current_index()
        start = current - item_capacity // 2
        start = max(0, min(start, count - item_capacity))
        indexes = list(range(start, start + item_capacity))
        return indexes, count - len(indexes)

    @staticmethod
    def _status_style(status: str) -> tuple[str, str]:
        status = status.upper()
        if status == "PASS":
            return "\x1b[1;32m", "\x1b[0m"
        if status in {"FAIL", "FAILED", "PREFLIGHT_FAIL", "INTERRUPTED"}:
            return "\x1b[1;31m", "\x1b[0m"
        if status == "RUNNING":
            return "\x1b[1;36m", "\x1b[0m"
        if status in {"BLOCKED", "NOT_EXECUTED"}:
            return "\x1b[1;33m", "\x1b[0m"
        if status.startswith("SKIPPED"):
            return "\x1b[35m", "\x1b[0m"
        return "\x1b[2m", "\x1b[0m"

    def _plain_status_line(self, item: QueueItem, status: str) -> str:
        label = {
            "FAIL": "FAILED",
            "PREFLIGHT_FAIL": "PREFLIGHT FAILED",
            "NOT_EXECUTED": "NOT EXECUTED",
            "SKIPPED_DUPLICATE_LOCAL": "SKIPPED",
        }.get(status.upper(), status.upper())
        # Put the name first as requested, but reserve enough width for the
        # state so long OTA/NFC filenames cannot push RUNNING/PASS/FAIL away.
        suffix = f"  {label}"
        max_name_cells = max(8, self.cols - _display_cell_width(suffix) - 2)
        name = _safe_display(item.name)
        if _display_cell_width(name) > max_name_cells:
            # Reuse the selector's cell-aware clipping by temporarily giving it
            # a synthetic terminal width that leaves room for the suffix.
            name = _clip_selector_line(name, max_name_cells + 2)
        return f"{name}{suffix}"

    def _render(self, *, initial: bool = False) -> None:
        if not self.active:
            return
        try:
            current_size = shutil.get_terminal_size(fallback=(self.cols, self.rows))
            if int(current_size.columns) != self.cols or int(current_size.lines) != self.rows:
                self._disable_after_resize()
                return
            out = sys.stdout
            if not initial:
                out.write("\x1b7")
                out.write("\x1b[1;1H")
            indexes, omitted = self._visible_indexes()
            lines: list[tuple[str, str]] = []
            for index in indexes:
                item = self.items[index]
                status = self.statuses.get(item.name, "WAITING")
                lines.append((self._plain_status_line(item, status), status))
            if omitted:
                lines.append((f"… {omitted} PATCH khác (status vẫn được theo dõi)", "WAITING"))
            # Keep the fixed height deterministic even while the sliding window
            # changes around the currently running PATCH.
            expected_status_rows = self.header_height - 1
            lines = lines[:expected_status_rows]
            while len(lines) < expected_status_rows:
                lines.append(("", "WAITING"))
            for row_no, (plain, status) in enumerate(lines, 1):
                clipped = _clip_selector_line(plain, self.cols)
                style, reset = self._status_style(status)
                out.write(f"\x1b[{row_no};1H\x1b[2K{style}{clipped}{reset}")
            separator = "─" * max(1, self.cols - 1)
            out.write(f"\x1b[{self.header_height};1H\x1b[2K{separator}")
            if not initial:
                out.write("\x1b8")
            out.flush()
        except Exception:
            self.close()

    def _disable_after_resize(self) -> None:
        if not self.active:
            return
        self.close()
        if not self._warned_resize:
            self._warned_resize = True
            print(f"[PTV v{VERSION} WARNING] live PATCH status header disabled after terminal resize; using normal console output.")

    def set_status(self, name: str, status: str) -> None:
        if name in self.statuses:
            self.statuses[name] = str(status).upper()
        self._render()

    def mark_not_executed(self, items: list[QueueItem]) -> None:
        for item in items:
            if item.name in self.statuses and self.statuses[item.name] == "WAITING":
                self.statuses[item.name] = "NOT_EXECUTED"
        self._render()

    @staticmethod
    def _sanitize_log_text(text: str) -> str:
        clean = _ANSI_RE.sub("", text)
        out: list[str] = []
        for ch in clean:
            if ch in "\n\r\t":
                out.append(ch)
            elif unicodedata.category(ch) != "Cc":
                out.append(ch)
        return "".join(out)

    def write_log(self, text: str) -> None:
        if not text:
            return
        if not self.active:
            sys.stdout.write(text); sys.stdout.flush()
            return
        try:
            sys.stdout.write(self._sanitize_log_text(text)); sys.stdout.flush()
        except Exception:
            self.close()
            sys.stdout.write(text); sys.stdout.flush()

    def close(self) -> None:
        global _ACTIVE_LIVE_STATUS_PANEL
        if _ACTIVE_LIVE_STATUS_PANEL is self:
            _ACTIVE_LIVE_STATUS_PANEL = None
        if not self.active:
            return
        self.active = False
        try:
            out = sys.stdout
            out.write("\x1b[r")
            # Continue subsequent summaries below the live viewport instead of
            # overwriting the final fixed status rows.
            out.write(f"\x1b[{self.rows};1H\x1b[2K")
            out.write("\n")
            out.flush()
        except Exception:
            pass


def _run_patch_child(
    root: Path,
    cmd: list[str],
    item: QueueItem,
    *,
    full_log_path: Path | None = None,
    expected_patch_sha256: str | None = None,
    expected_targets: list[str] | None = None,
    live_status: _LivePatchStatus | None = None,
) -> tuple[int, str, dict[str, object] | None]:
    runtime = _artifact_subdir(root, "runtime")
    token = f"{int(time.time()*1000000)}_{os.getpid()}_{_safe_slug(item.name,48)}"
    result_path = runtime / f"{token}.json"
    env = dict(os.environ)
    env["PTV_PATCH_RESULT_FILE"] = str(result_path)
    env["PYTHONUNBUFFERED"] = "1"
    spawn_patch_sha: str | None = None
    spawn_patch_path = root / "patchs" / item.name
    try:
        spawn_patch_sha = stable_package_sha256(spawn_patch_path)
    except Exception:
        spawn_patch_sha = None
    if expected_patch_sha256 and spawn_patch_sha != expected_patch_sha256:
        message = f"PATCH package bytes changed after planning/preflight: {item.name}"
        result = {
            "format": "python-patch-tool-patch-result", "format_version": 1, "tool_version": VERSION,
            "patch_file": item.name, "patch_sha256": expected_patch_sha256, "status": "FAIL", "rc": 2,
            "stage": "package_identity",
            "preflight": {"target_paths": list(expected_targets or [])},
            "diagnosis": {"kind": "package_input_changed", "message": message, "affected_paths": list(expected_targets or [])},
            "partial_modification": {"detected": False, "changed_paths": [], "evidence": "package_identity_checked_before_child_spawn"},
        }
        text = f"[PTV v{VERSION} SAFETY STOP] {message}\n"
        sys.stderr.write(text); sys.stderr.flush()
        if full_log_path is not None:
            try:
                full_log_path.parent.mkdir(parents=True, exist_ok=True)
                full_log_path.write_text(text, encoding="utf-8")
            except Exception:
                pass
        return 2, text, result
    child_kwargs: dict[str, object] = {
        "cwd": root, "env": env, "stdout": subprocess.PIPE, "stderr": subprocess.STDOUT,
        "text": True, "encoding": "utf-8", "errors": "replace", "bufsize": 1,
    }
    if os.name != "nt":
        child_kwargs["start_new_session"] = True
    elif hasattr(subprocess, "CREATE_NEW_PROCESS_GROUP"):
        child_kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    proc = subprocess.Popen(cmd, **child_kwargs)
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
        if live_status is not None:
            live_status.write_log(text)
        else:
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
    if result is None and raw_rc != 0:
        # A child can die before it writes the structured result (signal, Python
        # startup/import failure, abrupt process exit). Preserve enough identity
        # for the mandatory FAIL_HANDOFF source collector to inspect the exact
        # unchanged queue package when possible.
        result = {
            "format": "python-patch-tool-patch-result",
            "format_version": 1,
            "tool_version": VERSION,
            "patch_file": item.name,
            "patch_sha256": spawn_patch_sha,
            "status": "FAIL",
            "rc": _normalize_subprocess_rc(raw_rc),
            "stage": "child_process",
            "preflight": None,
            "diagnosis": {
                "kind": "child_result_missing",
                "message": "PATCH child exited without structured result",
                "affected_paths": [],
            },
            "partial_modification": {
                "detected": None,
                "changed_paths": [],
                "evidence": "child_result_missing_state_unknown",
            },
        }
    if interrupted_sig is not None:
        rc = 128 + abs(interrupted_sig)
    else:
        rc = _normalize_subprocess_rc(raw_rc)
    if log_error and result is not None:
        result.setdefault("report_warnings", []).append(f"full detail log unavailable: {log_error}")
    return rc, "".join(chunks), result


def _path_is_link_or_reparse(path: Path) -> bool:
    try:
        st = path.lstat()
    except OSError:
        return True
    attrs = getattr(st, "st_file_attributes", 0)
    reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
    return stat.S_ISLNK(st.st_mode) or reparse


def _is_local_db_profile_path(root: Path, rel: str) -> bool:
    try:
        root_real = root.resolve(strict=True)
        pure = Path(rel)
        if pure.is_absolute() or ".." in pure.parts or not pure.parts:
            return False
        candidate = (root_real / pure).resolve(strict=False)
        normalized = candidate.relative_to(root_real).as_posix()
    except Exception:
        return False
    if normalized in _LOCAL_DB_PROFILE_REL_PATHS:
        return True
    raw = os.environ.get(_DB_PROFILE_ENV)
    if raw:
        try:
            override = Path(raw).expanduser()
            if not override.is_absolute(): override = root_real / override
            return override.resolve(strict=False) == candidate
        except Exception:
            return False
    return False


def _safe_handoff_source(root: Path, rel: str) -> Path | None:
    """Return a bounded real project file with no symlink/reparse ancestor.

    FAIL_HANDOFF is a diagnostic integrity boundary: source bytes should come
    from the named project path, not from a redirecting ancestor that may be
    swapped independently while the bundle is being assembled.
    """
    try:
        if not isinstance(rel, str) or not rel or "\\" in rel:
            return None
        if _is_local_db_profile_path(root, rel):
            return None
        pure = Path(rel)
        if pure.is_absolute() or ".." in pure.parts or not pure.parts:
            return None
        root_real = root.resolve(strict=True)
        cur = root_real
        for part in pure.parts[:-1]:
            cur = cur / part
            if _path_is_link_or_reparse(cur) or not cur.is_dir():
                return None
        path = root_real / pure
        if _path_is_link_or_reparse(path) or not path.is_file():
            return None
        resolved = path.resolve(strict=True)
        resolved.relative_to(root_real)
        if path.stat().st_size > MAX_HANDOFF_SOURCE_FILE_BYTES:
            return None
        return path
    except Exception:
        return None


def _normalize_handoff_candidate(root: Path, raw: str) -> str | None:
    """Normalize a log/metadata path into a safe project-relative POSIX path."""
    if not isinstance(raw, str):
        return None
    text = raw.strip().strip('"\'`()[]{}<>,;')
    if not text:
        return None
    text = re.sub(r":\d+(?::\d+)?$", "", text)
    try:
        native = Path(text)
        if native.is_absolute():
            resolved = native.resolve(strict=False)
            rel = resolved.relative_to(root.resolve())
            candidate = rel.as_posix()
            return candidate if _safe_handoff_source(root, candidate) is not None else None
    except Exception:
        pass
    text = text.replace("\\", "/")
    root_posix = root.resolve().as_posix().rstrip("/")
    if text.startswith(root_posix + "/"):
        text = text[len(root_posix) + 1 :]
    if re.match(r"^[A-Za-z]:/", text):
        return None
    pure = Path(text)
    if pure.is_absolute() or ".." in pure.parts or not pure.parts:
        return None
    rel = pure.as_posix()
    while rel.startswith("./"):
        rel = rel[2:]
    if not rel:
        return None
    return rel if _safe_handoff_source(root, rel) is not None else None


def _handoff_structured_path_evidence(
    root: Path,
    item: QueueItem,
    patch_result: dict[str, object] | None,
) -> list[tuple[str, str]]:
    """Return target/source paths proven related by structured PATCH evidence."""
    evidence: list[tuple[str, str]] = []
    seen: set[str] = set()

    def add(raw: object, reason: str) -> None:
        if not isinstance(raw, str):
            return
        rel = _normalize_handoff_candidate(root, raw)
        if rel is not None and rel not in seen:
            seen.add(rel)
            evidence.append((rel, reason))

    def add_many(values: object, reason: str) -> None:
        if isinstance(values, list):
            for value in values:
                add(value, reason)

    if isinstance(patch_result, dict):
        diagnosis = patch_result.get("diagnosis")
        if isinstance(diagnosis, dict):
            add_many(diagnosis.get("affected_paths"), "diagnosis.affected_paths")
            issues = diagnosis.get("issues")
            if isinstance(issues, list):
                for issue in issues:
                    if isinstance(issue, dict):
                        add(issue.get("path"), "diagnosis.issue.path")
        partial = patch_result.get("partial_modification")
        if isinstance(partial, dict):
            add_many(partial.get("changed_paths"), "partial_modification.changed_paths")
        preflight = patch_result.get("preflight")
        if isinstance(preflight, dict):
            add_many(preflight.get("target_paths"), "preflight.target_paths")
            add_many(preflight.get("affected_paths"), "preflight.affected_paths")
            for key in ("checks", "issues"):
                rows = preflight.get(key)
                if isinstance(rows, list):
                    for row in rows:
                        if isinstance(row, dict):
                            add(row.get("path"), f"preflight.{key}.path")
        rollback = patch_result.get("rollback")
        if isinstance(rollback, dict):
            add_many(rollback.get("restored_paths"), "rollback.restored_paths")
            remaining = rollback.get("remaining_project_delta")
            if isinstance(remaining, dict):
                add_many(remaining.get("changed_paths"), "rollback.remaining_project_delta")
        delta = patch_result.get("project_delta")
        if isinstance(delta, dict):
            add_many(delta.get("changed_paths"), "project_delta.changed_paths")

    patch_path = root / "patchs" / item.name
    expected_sha = patch_result.get("patch_sha256") if isinstance(patch_result, dict) else None
    exact_queue_bytes = False
    try:
        exact_queue_bytes = (
            patch_path.is_file() and not patch_path.is_symlink()
            and isinstance(expected_sha, str) and bool(expected_sha)
            and _sha256_file(patch_path) == expected_sha
        )
    except OSError:
        exact_queue_bytes = False
    if exact_queue_bytes:
        try:
            meta = load_patch_meta(root, item.name)
            for rel in meta.effective_targets:
                add(rel, "executed_patch.effective_target")
        except Exception:
            pass
    return evidence


def _handoff_console_path_evidence(root: Path, console_log: str) -> tuple[list[tuple[str, str]], set[str]]:
    """Extract source paths and unresolved basenames from failure output."""
    evidence: list[tuple[str, str]] = []
    seen: set[str] = set()
    unresolved_basenames: set[str] = set()
    suffix_alt = "|".join(sorted(re.escape(x.lstrip(".")) for x in _HANDOFF_SOURCE_SUFFIXES))
    patterns = [
        re.compile(r'''File\s+["']([^"']+)["']'''),
        re.compile(rf'''(?<![A-Za-z0-9_])((?:[A-Za-z]:[\\/])?[^\s"'<>|]+?\.(?:{suffix_alt}))(?::\d+(?::\d+)?)?''', re.I),
    ]
    for pattern in patterns:
        for match in pattern.finditer(console_log[:MAX_HANDOFF_LOG_EVIDENCE_BYTES]):
            raw = match.group(1)
            rel = _normalize_handoff_candidate(root, raw)
            if rel is not None:
                if rel not in seen:
                    seen.add(rel)
                    evidence.append((rel, "console_path"))
                continue
            cleaned = re.sub(r":\d+(?::\d+)?$", "", raw.strip().strip('"\'`()[]{}<>,;')).replace("\\", "/")
            if "/" not in cleaned and Path(cleaned).suffix.lower() in _HANDOFF_SOURCE_SUFFIXES:
                unresolved_basenames.add(cleaned)
    return evidence, unresolved_basenames


def _scan_handoff_basenames(root: Path, basenames: set[str]) -> tuple[list[tuple[str, str]], int, bool]:
    """Bounded repository scan used only when logs mention a source basename."""
    if not basenames:
        return [], 0, False
    found: list[tuple[str, str]] = []
    scanned = 0
    truncated = False
    wanted = set(basenames)
    for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
        dirnames[:] = [
            d for d in dirnames
            if d not in _HANDOFF_SCAN_SKIP_DIRS and not (Path(dirpath) / d).is_symlink()
        ]
        for name in filenames:
            scanned += 1
            if scanned > MAX_HANDOFF_SCAN_FILES:
                truncated = True
                return found, scanned - 1, truncated
            if name not in wanted:
                continue
            path = Path(dirpath) / name
            try:
                rel = path.relative_to(root).as_posix()
            except ValueError:
                continue
            if _safe_handoff_source(root, rel) is not None:
                found.append((rel, f"console_basename_scan:{name}"))
                if len(found) >= MAX_HANDOFF_SOURCE_FILES:
                    return found, scanned, True
    return found, scanned, truncated


def _related_source_references(root: Path, seeds: list[tuple[str, str]]) -> list[tuple[str, str]]:
    """Discover one-hop local code/config references and same-stem companions."""
    out: list[tuple[str, str]] = []
    seen: set[str] = {rel for rel, _ in seeds}
    suffix_alt = "|".join(sorted(re.escape(x.lstrip(".")) for x in _HANDOFF_SOURCE_SUFFIXES))
    quoted_ref = re.compile(rf'''["']([^"'\r\n]+?\.(?:{suffix_alt}))["']''', re.I)
    for rel, _reason in seeds[:MAX_HANDOFF_SOURCE_FILES]:
        src = _safe_handoff_source(root, rel)
        if src is None:
            continue
        for suffix in _HANDOFF_SOURCE_SUFFIXES:
            candidate = src.with_suffix(suffix)
            try:
                crel = candidate.relative_to(root).as_posix()
            except ValueError:
                continue
            if crel in seen or _safe_handoff_source(root, crel) is None:
                continue
            seen.add(crel)
            out.append((crel, f"same_stem_companion:{rel}"))
            if len(out) >= MAX_HANDOFF_SOURCE_FILES:
                return out
        try:
            with src.open("rb") as fh:
                sample = fh.read(MAX_HANDOFF_REFERENCE_TEXT_BYTES)
            text = sample.decode("utf-8", errors="ignore")
        except OSError:
            continue
        for match in quoted_ref.finditer(text):
            raw = match.group(1).replace("\\", "/")
            candidates = []
            if raw.startswith("/") or re.match(r"^[A-Za-z]:/", raw):
                candidates.append(raw)
            else:
                try:
                    candidates.append((src.parent / raw).relative_to(root).as_posix())
                except (ValueError, OSError):
                    pass
                root_rel = raw
                while root_rel.startswith("./"):
                    root_rel = root_rel[2:]
                candidates.append(root_rel)
            for candidate in candidates:
                crel = _normalize_handoff_candidate(root, candidate)
                if crel is None or crel in seen:
                    continue
                seen.add(crel)
                out.append((crel, f"one_hop_reference:{rel}"))
                if len(out) >= MAX_HANDOFF_SOURCE_FILES:
                    return out
                break
    return out



def _read_handoff_detail_log(path: Path | None, fallback: str) -> tuple[str, bytes | None, dict[str, object]]:
    """Read bounded first+last detail-log bytes for evidence and attachment.

    The regular console capture is intentionally capped at 8 MiB. A compiler
    or traceback path may appear later, so FAIL_HANDOFF independently samples
    the persisted per-item log. The bundle remains bounded even for runaway
    commands.
    """
    meta: dict[str, object] = {"available": False, "truncated": False, "bytes": 0}
    if path is None:
        return fallback, None, meta
    try:
        if _path_is_link_or_reparse(path) or not path.is_file():
            return fallback, None, meta
        size = path.stat().st_size
        meta["available"] = True
        meta["original_bytes"] = size
        with path.open("rb") as fh:
            if size <= MAX_HANDOFF_DETAIL_LOG_BYTES:
                raw = fh.read(MAX_HANDOFF_DETAIL_LOG_BYTES + 1)
                if len(raw) > MAX_HANDOFF_DETAIL_LOG_BYTES:
                    raw = raw[:MAX_HANDOFF_DETAIL_LOG_BYTES]
                    meta["truncated"] = True
            else:
                half = MAX_HANDOFF_DETAIL_LOG_BYTES // 2
                head = fh.read(half)
                fh.seek(max(0, size - half))
                tail = fh.read(half)
                marker = b"\n[PTV: middle of DETAIL.log omitted by 64 MiB FAIL_HANDOFF limit]\n"
                raw = head + marker + tail
                meta["truncated"] = True
        meta["bytes"] = len(raw)
        # Use at most the evidence budget, split across beginning/end so a late
        # compiler error is not hidden behind verbose build output.
        if len(raw) <= MAX_HANDOFF_LOG_EVIDENCE_BYTES:
            evidence_raw = raw
        else:
            half = MAX_HANDOFF_LOG_EVIDENCE_BYTES // 2
            evidence_raw = raw[:half] + b"\n[PTV: log evidence middle omitted]\n" + raw[-half:]
        evidence = evidence_raw.decode("utf-8", errors="replace")
        return evidence, raw, meta
    except OSError as exc:
        meta["error"] = f"{type(exc).__name__}: {exc}"
        return fallback, None, meta


def _snapshot_handoff_sources(
    root: Path,
    attachments: list[tuple[str, Path]],
    snapshot_root: Path,
) -> tuple[list[tuple[str, Path]], list[dict[str, object]]]:
    """Freeze discovered source bytes before creating the ZIP.

    Each source is copied independently from a no-follow descriptor with
    generation checks. A disappearing/replaced optional attachment is skipped
    without touching snapshots that were already frozen successfully.
    """
    frozen: list[tuple[str, Path]] = []
    skipped: list[dict[str, object]] = []
    frozen_total = 0
    for rel, _src_hint in attachments:
        dst: Path | None = None
        fd = -1
        if len(frozen) >= MAX_HANDOFF_SOURCE_FILES:
            skipped.append({"path": rel, "reason": "max_source_files_during_snapshot"})
            continue
        src = _safe_handoff_source(root, rel)
        if src is None:
            skipped.append({"path": rel, "reason": "source_unavailable_before_snapshot"})
            continue
        try:
            flags = os.O_RDONLY | getattr(os, "O_BINARY", 0) | getattr(os, "O_NOFOLLOW", 0)
            fd = os.open(src, flags)
            before = os.fstat(fd)
            attrs = getattr(before, "st_file_attributes", 0)
            reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
            if not stat.S_ISREG(before.st_mode) or reparse:
                raise ValueError("source is no longer a regular non-reparse file")
            if before.st_size > MAX_HANDOFF_SOURCE_FILE_BYTES:
                skipped.append({"path": rel, "reason": "source_over_per_file_limit"})
                continue
            if frozen_total + before.st_size > MAX_HANDOFF_SOURCE_TOTAL_BYTES:
                skipped.append({"path": rel, "reason": "max_total_source_bytes_during_snapshot"})
                continue
            dst = snapshot_root.joinpath(*Path(rel).parts)
            dst.parent.mkdir(parents=True, exist_ok=True)
            copied = 0
            with os.fdopen(os.dup(fd), "rb") as in_fh, dst.open("xb") as out_fh:
                while True:
                    chunk = in_fh.read(1024 * 1024)
                    if not chunk:
                        break
                    copied += len(chunk)
                    if copied > MAX_HANDOFF_SOURCE_FILE_BYTES:
                        raise ValueError("source exceeded per-file limit during snapshot")
                    if frozen_total + copied > MAX_HANDOFF_SOURCE_TOTAL_BYTES:
                        raise ValueError("source set exceeded total limit during snapshot")
                    out_fh.write(chunk)
                out_fh.flush()
                try: os.fsync(out_fh.fileno())
                except OSError: pass
            after = os.fstat(fd)
            same_generation = (
                before.st_dev == after.st_dev and before.st_ino == after.st_ino
                and before.st_size == after.st_size
                and getattr(before, "st_mtime_ns", int(before.st_mtime * 1e9))
                    == getattr(after, "st_mtime_ns", int(after.st_mtime * 1e9))
            )
            if not same_generation or copied != after.st_size:
                try: dst.unlink()
                except OSError: pass
                dst = None
                skipped.append({"path": rel, "reason": "source_changed_during_snapshot"})
                continue
            frozen.append((rel, dst))
            frozen_total += copied
        except Exception as exc:
            if dst is not None:
                try: dst.unlink()
                except OSError: pass
            skipped.append({"path": rel, "reason": "snapshot_failed", "error": type(exc).__name__})
        finally:
            if fd >= 0:
                try: os.close(fd)
                except OSError: pass
    return frozen, skipped


def _discover_fail_handoff_sources(
    root: Path,
    item: QueueItem,
    patch_result: dict[str, object] | None,
    console_log: str,
) -> tuple[list[tuple[str, Path]], dict[str, object]]:
    """Automatically discover and bound source attachments for every PATCH FAIL."""
    ordered: list[tuple[str, str]] = []
    reason_by_path: dict[str, list[str]] = {}

    def merge(rows: list[tuple[str, str]]) -> None:
        for rel, reason in rows:
            if rel not in reason_by_path:
                reason_by_path[rel] = []
                ordered.append((rel, reason))
            if reason not in reason_by_path[rel]:
                reason_by_path[rel].append(reason)

    structured = _handoff_structured_path_evidence(root, item, patch_result)
    merge(structured)
    console_rows, basenames = _handoff_console_path_evidence(root, console_log)
    merge(console_rows)
    scanned_rows, scanned_files, scan_truncated = _scan_handoff_basenames(root, basenames)
    merge(scanned_rows)
    merge(_related_source_references(root, ordered))

    attachments: list[tuple[str, Path]] = []
    included: list[dict[str, object]] = []
    skipped: list[dict[str, object]] = []
    total = 0
    for rel, _ in ordered:
        src = _safe_handoff_source(root, rel)
        if src is None:
            skipped.append({"path": rel, "reason": "unsafe_missing_or_over_per_file_limit"})
            continue
        try:
            size = src.stat().st_size
        except OSError:
            skipped.append({"path": rel, "reason": "stat_failed"})
            continue
        if len(attachments) >= MAX_HANDOFF_SOURCE_FILES:
            skipped.append({"path": rel, "reason": "max_source_files"})
            continue
        if total + size > MAX_HANDOFF_SOURCE_TOTAL_BYTES:
            skipped.append({"path": rel, "reason": "max_total_source_bytes", "size": size})
            continue
        attachments.append((rel, src))
        total += size
        included.append({"path": rel, "size": size, "reasons": reason_by_path.get(rel, [])})

    discovery = {
        "format": "python-patch-tool-fail-source-discovery",
        "format_version": 1,
        "mode": "automatic_on_every_patch_failure",
        "structured_seed_count": len(structured),
        "console_path_count": len(console_rows),
        "console_unresolved_basenames": sorted(basenames),
        "basename_scan_files_examined": scanned_files,
        "basename_scan_truncated": scan_truncated,
        "discovered_paths": len(ordered),
        "included_files": included,
        "skipped_files": skipped,
        "included_total_bytes": total,
        "limits": {
            "max_source_files": MAX_HANDOFF_SOURCE_FILES,
            "max_source_file_bytes": MAX_HANDOFF_SOURCE_FILE_BYTES,
            "max_source_total_bytes": MAX_HANDOFF_SOURCE_TOTAL_BYTES,
            "max_repository_scan_files": MAX_HANDOFF_SCAN_FILES,
            "max_reference_text_bytes_per_seed": MAX_HANDOFF_REFERENCE_TEXT_BYTES,
        },
    }
    return attachments, discovery


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


def _print_upload_action_block(path: Path | str, *, patch_failure: bool = False, companion_path: Path | str | None = None, root: Path | None = None, stream=None) -> None:
    """Print one high-visibility upload-required block without changing plain-text semantics.

    TTY/VT terminals receive a bright-yellow background for the PRIMARY label,
    ACTION REQUIRED instruction and exact artifact path.  The path remains
    complete/copyable and is additionally underlined.  NO_COLOR/non-TTY output
    stays byte-for-byte plain apart from the terminal-width-bounded rule rows.
    """
    out = stream if stream is not None else sys.stdout
    display_zip = Path(path).absolute()
    display_text = Path(companion_path).absolute() if companion_path is not None else None
    alias_used = False
    if root is not None:
        try:
            from python_patch_upload_alias import create_upload_aliases
            display_zip, display_text, alias_used = create_upload_aliases(
                root, display_zip, display_text, kind=("FAIL_HANDOFF" if patch_failure else "AI_SYNC")
            )
        except Exception:
            display_zip = Path(path).absolute()
            display_text = Path(companion_path).absolute() if companion_path is not None else None
    banner = (
        "!!! [PRIMARY - UPLOAD THIS FILE] PATCH FAIL HANDOFF !!!"
        if patch_failure else "!!! [PRIMARY - UPLOAD THIS FILE] !!!"
    )
    action = ">>> ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER <<<"
    try:
        cols = _selector_term_width()
    except Exception:
        cols = 120
    width = max(1, min(72, int(cols) - 2))
    rule = "=" * width
    print(rule, file=out)
    if _result_color_enabled(out):
        block_style, path_style, reset = "\x1b[1;30;103m", "\x1b[1;4;30;103m", "\x1b[0m"
        print(f"{block_style}{_clip_selector_line(banner, width + 2)}{reset}", file=out)
        print(f"{block_style}{_clip_selector_line(action, width + 2)}{reset}", file=out)
        # Keep the pathname on its own physical output row.  A short hard-link
        # alias is preferred when available so terminals/task renderers that
        # hard-wrap long rows do not split the copyable path into two lines.
        print(f"{block_style}ZIP (preferred) — copy path below:{reset}", file=out)
        print(f"{path_style}{_safe_display(display_zip)}{reset}", file=out)
        if display_text is not None:
            print(f"{block_style}Clear-text TXT — copy path below:{reset}", file=out)
            print(f"{path_style}{_safe_display(display_text)}{reset}", file=out)
    else:
        print(_clip_selector_line(banner, width + 2), file=out)
        print(_clip_selector_line(action, width + 2), file=out)
        print("ZIP (preferred) — copy path below:", file=out)
        print(_safe_display(display_zip), file=out)
        if display_text is not None:
            print("Clear-text TXT — copy path below:", file=out)
            print(_safe_display(display_text), file=out)
    print(rule, file=out)


def _create_fail_handoff(
    root: Path,
    item: QueueItem,
    rc: int,
    console_log: str,
    patch_result: dict[str, object] | None,
    recovery_request: Path | None,
    *,
    detail_log_path: Path | None = None,
) -> Path | None:
    if isinstance(patch_result, dict):
        recovery_cfg = patch_result.get("recovery")
        if isinstance(recovery_cfg, dict) and recovery_cfg.get("fail_handoff") is False:
            print(
                f"[PTV v{VERSION} WARNING] recovery.fail_handoff=false is deprecated/ignored; "
                "every PATCH failure now creates a FAIL_HANDOFF with automatic source collection.",
                file=sys.stderr,
            )
    out: Path | None = None
    final: Path | None = None
    temp: Path | None = None
    stamp = datetime.now().strftime("%Y%m%d_%H%M%S_%f")
    summary: dict[str, object] = {
        "format": "python-patch-tool-fail-handoff",
        "format_version": 2,
        "tool_version": VERSION,
        "patch": item.name,
        "rc": rc,
        "patch_result": patch_result,
        "recovery_collect_request": recovery_request.name if recovery_request else None,
        "patch_attachment": "not_checked",
        "attachment_warnings": [],
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

    snapshot_dir: Path | None = None
    try:
        try:
            out = _artifact_subdir(root, "fail_handoffs")
        except Exception as dir_exc:
            # A broken/symlinked optional subdirectory must not crash the PATCH
            # failure path. Fall back to the already-hardened artifact root.
            out = _artifact_run_root(root)
            summary.setdefault("attachment_warnings", []).append(
                f"fail_handoffs directory unavailable; used artifact-root fallback: {type(dir_exc).__name__}"
            )
            print(f"[PTV v{VERSION} WARNING] fail_handoffs directory unsafe/unavailable; using artifact-root fallback", file=sys.stderr)
        final = out / f"FAIL_HANDOFF_{_safe_slug(item.name,70)}_{stamp}.zip"
        fd, temp_name = tempfile.mkstemp(prefix=f".{final.name}.", suffix=".tmp", dir=out)
        os.close(fd)
        temp = Path(temp_name)
        evidence_log, detail_log_bytes, detail_log_meta = _read_handoff_detail_log(detail_log_path, console_log)
        summary["detail_log"] = detail_log_meta
        source_attachments, source_discovery = _discover_fail_handoff_sources(
            root, item, patch_result, evidence_log
        )

        snapshot_dir = Path(tempfile.mkdtemp(prefix=".ptv-handoff-sources-", dir=out))
        frozen_sources, snapshot_skips = _snapshot_handoff_sources(root, source_attachments, snapshot_dir)
        if snapshot_skips:
            source_discovery.setdefault("skipped_files", []).extend(snapshot_skips)
        frozen_paths = {rel for rel, _ in frozen_sources}
        included_rows = []
        included_total = 0
        for row in source_discovery.get("included_files") or []:
            if not isinstance(row, dict) or row.get("path") not in frozen_paths:
                continue
            rel = str(row["path"])
            snap = next(src for name, src in frozen_sources if name == rel)
            size = snap.stat().st_size
            item_row = dict(row)
            item_row["size"] = size
            item_row["sha256"] = _sha256_file(snap)
            item_row["snapshot"] = "stable_generation_copy"
            included_rows.append(item_row)
            included_total += size
        source_discovery["included_files"] = included_rows
        source_discovery["included_total_bytes"] = included_total
        source_discovery["snapshot_integrity"] = "generation_checked_per_file; failed files skipped without aborting handoff"

        sensitive_warnings = _sensitive_handoff_warnings(evidence_log, frozen_sources)
        summary["sensitive_content_warnings"] = sensitive_warnings

        from python_patch_ai_sync import decide_sync, patch_context_from_package
        patch_ai_context, patch_max_tested = patch_context_from_package(root / "patchs" / item.name)
        ai_sync_decision = decide_sync(
            root,
            ai_context=patch_ai_context,
            fallback_known_tool_version=patch_max_tested,
            channel="patch",
        )

        # v6.18.5 additive historical diagnostics compatibility.  Keep the
        # exact v6 evidence untouched and add a separately redacted/normalized
        # derivative layer for the v5 COMPLETE diagnostic capabilities.
        compat_diagnostics = None
        try:
            from python_patch_diagnostics_compat import build_compat_evidence
            diagnosis = patch_result.get("diagnosis") if isinstance(patch_result, dict) else None
            compat_diagnostics = build_compat_evidence(
                root,
                patch_name=item.name,
                exact_log=evidence_log,
                diagnosis=diagnosis if isinstance(diagnosis, dict) else None,
                detail_truncated=bool(detail_log_meta.get("truncated")),
                source_count=len(included_rows),
            )
            summary["compat_diagnostics"] = {
                "status": "AVAILABLE",
                "path": "compat_diagnostics/",
                "redacted_derivative": True,
                "exact_v6_evidence_preserved": True,
            }
        except Exception as diag_exc:
            summary["compat_diagnostics"] = {"status": "UNAVAILABLE", "error": type(diag_exc).__name__}
            summary.setdefault("attachment_warnings", []).append(
                f"compat diagnostics unavailable: {type(diag_exc).__name__}"
            )

        summary["source_discovery"] = {
            "mode": source_discovery.get("mode"),
            "discovered_paths": source_discovery.get("discovered_paths"),
            "included_files": len(included_rows),
            "included_total_bytes": included_total,
            "skipped_files": len(source_discovery.get("skipped_files") or []),
        }

        attachment_warnings: list[str] = [str(x) for x in (summary.get("attachment_warnings") or [])]
        with zipfile.ZipFile(temp, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6) as zf:
            # Required diagnostic core is written from immutable in-memory data.
            zf.writestr("console.log", console_log)
            if detail_log_bytes is not None:
                zf.writestr("DETAIL.log", detail_log_bytes)
            if sensitive_warnings:
                warning_text = (
                    "WARNING: This diagnostic bundle intentionally preserves exact source/log bytes.\n"
                    "Review before uploading if the destination is not trusted.\n"
                    "A redacted compatibility derivative is available under compat_diagnostics/ when generated.\n\n- "
                    + "\n- ".join(sensitive_warnings) + "\n"
                )
                zf.writestr("SENSITIVE_CONTENT_WARNING.txt", warning_text)

            if compat_diagnostics is not None:
                try:
                    from python_patch_diagnostics_compat import write_zip_evidence
                    write_zip_evidence(zf, compat_diagnostics)
                except Exception as diag_write_exc:
                    attachment_warnings.append(f"compat diagnostics write failed: {type(diag_write_exc).__name__}")

            # Every source is written from the stable snapshot, never directly
            # from a concurrently changing working tree.
            for rel, src in frozen_sources:
                try:
                    zf.write(src, f"current_source/{rel}")
                except Exception as exc:
                    attachment_warnings.append(f"source attachment write failed: {rel}: {type(exc).__name__}")

            # Optional context must never be allowed to destroy the mandatory
            # failure bundle if a file disappears during cleanup/concurrency.
            if attach_patch:
                try:
                    if patch_path.is_file() and not patch_path.is_symlink() and _sha256_file(patch_path) == expected_patch_sha:
                        zf.write(patch_path, f"patch/{item.name}")
                    else:
                        summary["patch_attachment"] = "omitted_queue_input_changed_during_handoff"
                except Exception as exc:
                    summary["patch_attachment"] = "omitted_queue_input_attachment_failed"
                    attachment_warnings.append(f"patch attachment failed: {type(exc).__name__}")
            docs = [
                root/"tools"/"implementing.md",
                root/"tools"/"PYTHON_PATCH_TOOL_FEATURES_VI.md",
                root/"tools"/"_patch_lib"/"VERSION",
                root/"tools"/"_patch_lib"/"docs"/"PATCH_PACKAGE_SCHEMA.json",
            ]
            for doc in docs:
                try:
                    if doc.is_file() and not doc.is_symlink():
                        zf.write(doc, f"tool_context/{doc.name}")
                except Exception as exc:
                    attachment_warnings.append(f"tool context attachment failed: {doc.name}: {type(exc).__name__}")
            if recovery_request is not None:
                try:
                    if recovery_request.is_file() and not recovery_request.is_symlink():
                        zf.write(recovery_request, f"recovery/{recovery_request.name}")
                except Exception as exc:
                    attachment_warnings.append(f"recovery request attachment failed: {type(exc).__name__}")

            # Human-run command evidence is part of the exact PATCH failure
            # context.  Embed it additively under manual_execution/ so an AI can
            # inspect the step instructions and console logs without losing the
            # normal FAIL_HANDOFF/source evidence.
            manual = patch_result.get("manual_execution") if isinstance(patch_result, dict) and isinstance(patch_result.get("manual_execution"), dict) else None
            if isinstance(manual, dict):
                summary["manual_execution"] = {
                    "status": manual.get("status"),
                    "result_zip": manual.get("result_zip"),
                    "result_text": manual.get("result_text"),
                    "work_dir": manual.get("work_dir"),
                }
                for key in ("result_zip", "result_text"):
                    raw = manual.get(key)
                    if not isinstance(raw, str) or not raw:
                        continue
                    try:
                        mp = root / raw
                        if mp.is_file() and not mp.is_symlink():
                            zf.write(mp, f"manual_execution/{mp.name}")
                    except Exception as exc:
                        attachment_warnings.append(f"manual execution {key} attachment failed: {type(exc).__name__}")
                raw_work = manual.get("work_dir")
                if isinstance(raw_work, str) and raw_work:
                    try:
                        work = (root / raw_work).resolve(strict=True)
                        work.relative_to(root.resolve(strict=True))
                        for mp in sorted(work.rglob("*")):
                            if not mp.is_file() or mp.is_symlink():
                                continue
                            rel = mp.relative_to(work).as_posix()
                            zf.write(mp, f"manual_execution/work/{rel}")
                    except Exception as exc:
                        attachment_warnings.append(f"manual execution work evidence attachment failed: {type(exc).__name__}")

            try:
                from python_patch_ai_sync import write_sync_bundle_to_zip
                summary["ai_tool_sync"] = write_sync_bundle_to_zip(zf, root, ai_sync_decision)
            except Exception as sync_exc:
                # AI sync is additive context; never destroy the mandatory failure
                # handoff if this derivative documentation channel malfunctions.
                summary["ai_tool_sync"] = {
                    "status": "UNAVAILABLE",
                    "error": type(sync_exc).__name__,
                    "tool_version": VERSION,
                }
                attachment_warnings.append(f"AI tool sync unavailable: {type(sync_exc).__name__}")

            summary["attachment_warnings"] = attachment_warnings
            # Write metadata after optional attachment attempts so it describes
            # what actually made it into this exact ZIP.
            zf.writestr("SOURCE_DISCOVERY.json", json.dumps(source_discovery, ensure_ascii=False, indent=2) + "\n")
            zf.writestr("FAIL_SUMMARY.json", json.dumps(summary, ensure_ascii=False, indent=2) + "\n")

        os.replace(temp, final)
        with zipfile.ZipFile(final) as zf:
            if zf.testzip() is not None:
                raise ValueError("FAIL_HANDOFF ZIP CRC check failed")
        companion = None
        try:
            from python_patch_cleartext_companion import create_zip_cleartext_companion
            companion = create_zip_cleartext_companion(final, artifact_kind="PATCH FAIL_HANDOFF")
        except Exception as companion_exc:
            # Never sacrifice the mandatory ZIP handoff if the derived text view
            # cannot be produced; surface the missing companion loudly instead.
            print(
                f"[PTV v{VERSION} WARNING] FAIL_HANDOFF clear-text companion unavailable: "
                f"{type(companion_exc).__name__}: {companion_exc}",
                file=sys.stderr,
            )
        print("")
        print(
            "FAIL HANDOFF SOURCES: "
            f"included={len(included_rows)} | "
            f"bytes={included_total} | "
            f"skipped={len(source_discovery.get('skipped_files') or [])}"
        )
        if detail_log_meta.get("truncated"):
            print("FAIL HANDOFF LOG: bounded DETAIL.log includes beginning + end of oversized log")
        if ai_sync_decision.attach:
            print(f"[PTV v{VERSION}] AI TOOL UPDATE INCLUDED — upload this FAIL_HANDOFF to AI before the next PATCH/COLLECT request")
        if companion is not None:
            try:
                from python_patch_ai_sync import mark_sync_delivered
                mark_sync_delivered(root, ai_sync_decision, artifact=final.relative_to(root).as_posix())
            except Exception:
                pass
        _print_upload_action_block(final, patch_failure=True, companion_path=companion, root=root)
        return final
    except Exception as exc:
        print(f"[PTV v{VERSION} WARNING] could not create FAIL_HANDOFF: {type(exc).__name__}: {exc}", file=sys.stderr)
        for path in (temp, final):
            if path is None:
                continue
            try: path.unlink()
            except OSError: pass
        return None
    finally:
        if snapshot_dir is not None:
            shutil.rmtree(snapshot_dir, ignore_errors=True)


def _run_foreground_child(
    root: Path, cmd: list[str], *, env: dict[str, str] | None = None, timeout: int | None = None, label: str = "child"
) -> int:
    """Run a foreground tool child with signal/tree containment.

    Unlike ``subprocess.run()``, this keeps the dispatcher in control of the
    whole child process group when Ctrl+C/SIGTERM arrives.  ``timeout=None``
    is intentional for COLLECT: collection is resource-bounded by its request
    contract and may legitimately be long-running, but it must still terminate
    cleanly when the user stops it.
    """
    kwargs: dict[str, object] = {"cwd": root, "env": env}
    if os.name != "nt":
        kwargs["start_new_session"] = True
    elif hasattr(subprocess, "CREATE_NEW_PROCESS_GROUP"):
        kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    proc = subprocess.Popen(cmd, **kwargs)
    old_sigterm = None
    if hasattr(signal, "SIGTERM"):
        try:
            old_sigterm = signal.getsignal(signal.SIGTERM)
            signal.signal(signal.SIGTERM, _raise_patch_child_signal)
        except Exception:
            old_sigterm = None
    started = time.monotonic()
    try:
        while True:
            try:
                raw = proc.wait(timeout=0.2)
                return _normalize_subprocess_rc(raw)
            except subprocess.TimeoutExpired:
                if timeout is not None and time.monotonic() - started >= max(1, int(timeout)):
                    term = getattr(signal, "SIGTERM", signal.SIGINT)
                    _forward_patch_signal(proc, term)
                    try:
                        proc.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        _forward_patch_signal(proc, getattr(signal, "SIGKILL", term))
                        try: proc.wait(timeout=5)
                        except subprocess.TimeoutExpired: pass
                    print(f"[PTV v{VERSION} TIMEOUT] {label} exceeded {int(timeout)}s; process tree contained", file=sys.stderr)
                    return 124
    except KeyboardInterrupt:
        _forward_patch_signal(proc, signal.SIGINT)
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            _forward_patch_signal(proc, getattr(signal, "SIGKILL", signal.SIGTERM))
            try: proc.wait(timeout=5)
            except subprocess.TimeoutExpired: pass
        raise
    except _PatchChildSignal as exc:
        _forward_patch_signal(proc, exc.signum)
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            _forward_patch_signal(proc, getattr(signal, "SIGKILL", exc.signum))
            try: proc.wait(timeout=5)
            except subprocess.TimeoutExpired: pass
        return 128 + abs(int(exc.signum))
    finally:
        if old_sigterm is not None:
            try: signal.signal(signal.SIGTERM, old_sigterm)
            except Exception: pass


def _runner_command(root: Path, action: str, item: QueueItem, *, no_validation: bool = False) -> list[str]:
    runner = root / "tools" / "_patch_lib" / "python_patch_runner.py"
    cmd = [sys.executable, str(runner)]
    if action in {"inspect", "validate", "preview"}:
        cmd.append(action)
    cmd += ["--patch", f"patchs/{item.name}", "--transaction", "off"]
    if no_validation and action == "execute":
        cmd.append("--no-validation")
    return cmd


def _run_runner_captured(
    root: Path, cmd: list[str], *, env: dict[str, str] | None = None, timeout: int = 120
) -> tuple[int, str, bool]:
    """Run a read-only runner child with timeout-aware tree containment.

    `subprocess.run(..., timeout=...)` force-kills only the direct child.  The
    runner can itself own a managed OPS/helper process group, so on timeout we
    first deliver a termination signal to the runner and let its cleanup path
    quiesce descendants before using a hard-kill fallback.
    """
    kwargs: dict[str, object] = {
        "cwd": root, "env": env, "stdout": subprocess.PIPE, "stderr": subprocess.STDOUT,
        "text": True, "encoding": "utf-8", "errors": "replace",
    }
    if os.name != "nt":
        kwargs["start_new_session"] = True
    elif hasattr(subprocess, "CREATE_NEW_PROCESS_GROUP"):
        kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    proc = subprocess.Popen(cmd, **kwargs)
    try:
        text, _ = proc.communicate(timeout=max(1, int(timeout)))
        return _normalize_subprocess_rc(proc.returncode or 0), text or "", False
    except subprocess.TimeoutExpired as exc:
        partial = exc.output if isinstance(exc.output, str) else ""
        term = getattr(signal, "SIGTERM", signal.SIGINT)
        _forward_patch_signal(proc, term)
        try:
            tail, _ = proc.communicate(timeout=10)
        except subprocess.TimeoutExpired:
            _forward_patch_signal(proc, getattr(signal, "SIGKILL", term))
            try:
                tail, _ = proc.communicate(timeout=5)
            except Exception:
                tail = ""
        text = (partial or "") + (tail or "")
        text += f"\n[PTV v{VERSION} TIMEOUT] runner exceeded {int(timeout)}s; process tree contained\n"
        return 124, text, True
    except KeyboardInterrupt:
        _forward_patch_signal(proc, signal.SIGINT)
        try:
            proc.communicate(timeout=10)
        except Exception:
            _forward_patch_signal(proc, getattr(signal, "SIGKILL", signal.SIGTERM))
        raise


def _inspect_item(root: Path, item: QueueItem) -> int:
    if item.kind != "PATCH":
        print("INSPECT: chỉ áp dụng cho PATCH; COLLECT được preflight theo schema khi discovery.")
        return 2
    try:
        return _run_foreground_child(root, _runner_command(root, "inspect", item), timeout=1830, label="PATCH inspect")
    except KeyboardInterrupt:
        return 130


def _preview_item(root: Path, item: QueueItem) -> int:
    if item.kind != "PATCH":
        print("PREVIEW: chỉ áp dụng cho PATCH.")
        return 2
    try:
        return _run_foreground_child(root, _runner_command(root, "preview", item), timeout=1830, label="PATCH preview")
    except KeyboardInterrupt:
        return 130


def _validate_item(root: Path, item: QueueItem) -> int:
    if item.kind != "PATCH":
        print("VALIDATE: chỉ áp dụng cho PATCH.")
        return 2
    try:
        return _run_foreground_child(root, _runner_command(root, "validate", item), timeout=1830, label="PATCH validate")
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


def _split_session_duplicate_patches(root: Path, items: list[QueueItem], *, history_replay_sha: dict[str, str] | None = None):
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

        item_is_replay = (history_replay_sha or {}).get(item.name, "").lower() == digest.lower()
        canonical_is_replay = (history_replay_sha or {}).get(canonical.name, "").lower() == digest.lower()
        if item_is_replay and not canonical_is_replay:
            # Preserve the exact rollback replay identity even when an ordinary
            # renamed duplicate sorts before it. Remove the ordinary duplicate
            # and promote the protected replay to canonical for this session.
            removed, remove_warning = _remove_session_duplicate_safely(
                queue_dir, canonical.name, item.name, digest
            )
            if remove_warning:
                warnings.append(
                    f"duplicate PATCH removal aborted safely for patchs/{canonical.name}: {remove_warning}"
                )
            canonical_by_hash[key] = item
            runnable = [x for x in runnable if x.name != canonical.name]
            runnable.append(item)
            if removed:
                duplicates.append(SessionDuplicate(canonical, item.name, digest, True))
            else:
                # Keep both runnable when safe removal of the ordinary duplicate
                # cannot be proven; never sacrifice the protected replay.
                runnable.append(canonical)
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


def _split_local_duplicate_patches(root: Path, items: list[QueueItem], *, history_replay_sha: dict[str, str] | None = None):
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
        # A successful PATCH that was subsequently rolled back by an atomic
        # batch is intentionally requeued for replay. Its exact bytes also
        # exist in patchs/patched/, so ordinary duplicate suppression would
        # otherwise delete the recovery package before SMART RESUME can use it.
        # Bypass history only for the exact queue filename+SHA recorded by the
        # previous rollback report; all ordinary duplicates remain suppressed.
        replay_sha = (history_replay_sha or {}).get(item.name)
        if replay_sha is not None and queued_hash.lower() == replay_sha.lower():
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


def _last_problem_patch_name() -> str | None:
    for detail in reversed(_LAST_EXECUTION_DETAILS):
        if detail.get("kind") != "PATCH":
            continue
        if detail.get("status") not in {"FAIL", "PREFLIGHT_FAIL"}:
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
        diagnosis = row.get("diagnosis") if isinstance(row.get("diagnosis"), dict) else {}
        kind = str(diagnosis.get("kind") or "")
        label = "blocked by related target failure" if kind == "related_target_failed" else "blocked by dependency failure"
        return label + (f": {','.join(str(x) for x in blocked)}" if blocked else "")
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


def _display_abs_path(root: Path, raw: object, *, base: str | None = None) -> tuple[str, bool] | None:
    """Return an absolute display path without requiring the artifact to exist."""
    if not isinstance(raw, str) or not raw.strip():
        return None
    try:
        value = Path(raw).expanduser()
        path = value if value.is_absolute() else (root / base / value if base else root / value)
        path = path.absolute()
        try:
            exists = path.is_file() and not path.is_symlink()
        except OSError:
            exists = False
        return str(path), exists
    except Exception:
        return None


_AI_UPLOAD_HISTORY_LABELS = frozenset({
    "COLLECT result",
    "COLLECT ZIP",
    "FAIL handoff",
    "FAIL handoff TXT",
    "COLLECT text",
    "Recovery COLLECT",
    "AI sync result",
    "AI sync TXT",
    "Manual result",
    "Manual result TXT",
})


def _print_history_artifact_line(
    label: str,
    path: str,
    exists: bool,
    *,
    stream=None,
    indent: str = "      ",
    label_width: int = 16,
) -> None:
    """Render report/history artifacts without changing plain-text semantics.

    Artifacts that normally need to be uploaded back to AI are highlighted with
    the same bright-yellow background used by the primary COLLECT/FAIL_HANDOFF
    upload block.  Missing required artifacts use the existing high-visibility
    yellow-on-red failure style.  NO_COLOR/non-TTY output remains plain and
    exact paths are never clipped.
    """
    out = stream or sys.stdout
    safe_path = _safe_display(path)
    suffix = "" if exists else " [missing]"
    prefix = f"{indent}{label:<{label_width}}: "
    if label in _AI_UPLOAD_HISTORY_LABELS and _result_color_enabled(out):
        reset = "\x1b[0m"
        if exists:
            label_style = "\x1b[1;30;103m"
            path_style = "\x1b[1;4;30;103m"
        else:
            label_style = "\x1b[1;93;41m"
            path_style = "\x1b[1;4;93;41m"
        print(f"{label_style}{prefix}{reset}{path_style}{safe_path}{suffix}{reset}", file=out)
        return
    print(f"{prefix}{safe_path}{suffix}", file=out)


def _report_status_style(status: str) -> tuple[str, str]:
    """Return a concise report/history status style; plain output is unchanged."""
    value = str(status or "UNKNOWN").upper()
    reset = "\x1b[0m"
    if value in {"FAIL", "FAILED", "PREFLIGHT_FAIL", "INTERRUPTED"}:
        return "\x1b[1;93;41m", reset
    if value in {"INCOMPLETE", "BLOCKED", "NOT_EXECUTED"}:
        return "\x1b[1;30;103m", reset
    if value == "PASS":
        return "\x1b[1;96m", reset
    if value.startswith("SKIPPED"):
        return "\x1b[35m", reset
    return "", reset


def _important_row_artifacts(root: Path, row: dict[str, object]) -> list[tuple[str, str, bool]]:
    found: list[tuple[str, str, bool]] = []
    seen: set[str] = set()
    def add(label: str, raw: object, *, base: str | None = None) -> None:
        item = _display_abs_path(root, raw, base=base)
        if item is None:
            return
        path, exists = item
        key = os.path.normcase(path)
        if key in seen:
            return
        seen.add(key)
        found.append((label, path, exists))

    collect = row.get("collect_result") if isinstance(row.get("collect_result"), dict) else None
    if collect is not None:
        add("COLLECT result", collect.get("result_zip"))
        add("COLLECT text", collect.get("result_text"))
        add("Request archive", collect.get("request_archive"))
    patch_result = row.get("patch_result") if isinstance(row.get("patch_result"), dict) else None
    manual = patch_result.get("manual_execution") if isinstance(patch_result, dict) and isinstance(patch_result.get("manual_execution"), dict) else None
    if manual is not None:
        add("Manual result", manual.get("result_zip"))
        add("Manual result TXT", manual.get("result_text"))
    add("FAIL handoff", row.get("fail_handoff"))
    add("FAIL handoff TXT", row.get("fail_handoff_text"))
    add("AI sync result", row.get("ai_sync_result"))
    add("AI sync TXT", row.get("ai_sync_result_text"))
    recovery = row.get("recovery_collect_request")
    if isinstance(recovery, str) and recovery:
        rp = Path(recovery)
        base = None if rp.is_absolute() or "/" in recovery.replace("\\", "/") else "patchs"
        add("Recovery COLLECT", recovery, base=base)
    requeued = row.get("requeued_as")
    if isinstance(requeued, str) and requeued:
        add("Replay PATCH", requeued, base="patchs")
    name = row.get("name")
    if row.get("status") == "PASS" and isinstance(name, str) and name:
        add("Archived package", name, base="patchs/patched")
    if str(row.get("status") or "") in {"FAIL", "PREFLIGHT_FAIL"}:
        add("Detail log", row.get("log_path"))
        add("Preflight log", row.get("preflight_log_path"))
    return found


def _print_important_artifacts(root: Path, rows: list[dict[str, object]], *, stream=None) -> None:
    out = stream or sys.stdout
    groups = []
    for index, row in enumerate(rows, 1):
        artifacts = _important_row_artifacts(root, row)
        if artifacts:
            groups.append((index, str(row.get("name") or "unknown"), artifacts))
    if not groups:
        return
    if _result_color_enabled(out):
        print("\x1b[1;93mImportant files:\x1b[0m", file=out)
    else:
        print("Important files:", file=out)
    for index, name, artifacts in groups:
        print(f"  {index:>2}. {_safe_display(name)}", file=out)
        for label, path, exists in artifacts:
            _print_history_artifact_line(label, path, exists, stream=out)


def _print_batch_overview(root: Path, report: dict[str, object], *, stream=None) -> None:
    out = stream or sys.stdout
    rows = _report_rows(report)
    counts = _batch_counts(rows)
    print("", file=out)
    batch_status = str(report.get("status") or "UNKNOWN")
    title = f"BATCH RESULT — {batch_status}"
    if _result_color_enabled(out):
        style, reset = _report_status_style(batch_status)
        print(f"{style}{title}{reset}" if style else title, file=out)
    else:
        print(title, file=out)
    print(
        f"Selected={len(rows)} | PASS={counts['PASS']} | FAIL={counts['FAIL']} | "
        f"BLOCKED={counts['BLOCKED']} | PREFLIGHT FAIL={counts['PREFLIGHT_FAIL']} | NOT EXECUTED={counts['NOT_EXECUTED']} | SKIPPED={counts['SKIPPED']}",
        file=out,
    )
    print(f"Policy: failure={report.get('failure_policy','continue_independent')} | transaction={report.get('transaction_policy','patch')}", file=out)
    tx = report.get("batch_transaction") if isinstance(report.get("batch_transaction"), dict) else None
    if tx:
        print(f"Batch transaction: {tx.get('status','UNKNOWN')}", file=out)
    for i, row in enumerate(rows, 1):
        status = str(row.get("status") or "UNKNOWN").upper()
        prefix = f"  {i:>2}. "
        status_text = f"[{status}]"
        name = _safe_display(str(row.get("name", "unknown")))
        if _result_color_enabled(out):
            style, reset = _report_status_style(status)
            rendered_status = f"{style}{status_text}{reset}" if style else status_text
            print(f"{prefix}{rendered_status} {name}", file=out)
        else:
            print(f"{prefix}{status_text} {name}", file=out)
        print(f"      {_safe_display(_row_summary(row))}", file=out)
    if report.get("batch_log"):
        absolute = _display_abs_path(root, report.get("batch_log"))
        print(f"Aggregate log: {_safe_display(absolute[0] if absolute else str(report['batch_log']))}", file=out)
    if report.get("batch_report_dir"):
        try:
            detail_dir = (root / str(report["batch_report_dir"]) / "items").absolute()
            print(f"Detail logs : {_safe_display(str(detail_dir))}{os.sep}", file=out)
        except Exception:
            print(f"Detail logs : {report['batch_report_dir']}/items/", file=out)
    _print_important_artifacts(root, rows, stream=out)
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
    out_dir = _artifact_subdir(root, "support")
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
    archived_name = row.get("name")
    if row.get("status") == "PASS" and isinstance(archived_name, str):
        archived_path = root / "patchs" / "patched" / archived_name
        try:
            if archived_path.is_file() and not archived_path.is_symlink():
                print(f"Archived pkg: {_safe_display(str(archived_path.absolute()))}")
        except OSError:
            pass
    if row.get("fail_handoff"):
        shown = _display_abs_path(root, row.get("fail_handoff"))
        handoff_path = shown[0] if shown else str(row["fail_handoff"])
        _print_history_artifact_line("FAIL handoff", handoff_path, bool(shown and shown[1]), indent="", label_width=16)
    if row.get("fail_handoff_text"):
        shown = _display_abs_path(root, row.get("fail_handoff_text"))
        handoff_text_path = shown[0] if shown else str(row["fail_handoff_text"])
        _print_history_artifact_line("FAIL handoff TXT", handoff_text_path, bool(shown and shown[1]), indent="", label_width=16)
    if row.get("recovery_collect_request"):
        raw_recovery = str(row.get("recovery_collect_request"))
        base = None if Path(raw_recovery).is_absolute() or "/" in raw_recovery.replace("\\", "/") else "patchs"
        shown = _display_abs_path(root, raw_recovery, base=base)
        recovery_path = shown[0] if shown else raw_recovery
        _print_history_artifact_line("Recovery COLLECT", recovery_path, bool(shown and shown[1]), indent="", label_width=16)
    collect = row.get("collect_result") if isinstance(row.get("collect_result"), dict) else None
    if collect is not None:
        if collect.get("result_zip"):
            shown = _display_abs_path(root, collect.get("result_zip"))
            collect_path = shown[0] if shown else str(collect.get("result_zip"))
            _print_history_artifact_line("COLLECT ZIP", collect_path, bool(shown and shown[1]), indent="", label_width=12)
        if collect.get("result_text"):
            shown = _display_abs_path(root, collect.get("result_text"))
            collect_text_path = shown[0] if shown else str(collect.get("result_text"))
            _print_history_artifact_line("COLLECT text", collect_text_path, bool(shown and shown[1]), indent="", label_width=12)
        if collect.get("request_archive"):
            shown = _display_abs_path(root, collect.get("request_archive"))
            print(f"Request ZIP : {_safe_display(shown[0] if shown else str(collect.get('request_archive')))}")
        quality = collect.get("quality") if isinstance(collect.get("quality"), dict) else None
        if quality is not None:
            parts = []
            for key in ("files", "reports", "missing", "truncated_reports"):
                if key in quality:
                    parts.append(f"{key}={quality.get(key)}")
            if parts:
                print(f"COLLECT quality: {' | '.join(parts)}")
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


def _history_primary_name(report: dict[str, object]) -> str:
    names: list[str] = []
    selected = report.get("selected")
    if isinstance(selected, list):
        names.extend(str(x) for x in selected if isinstance(x, str) and x)
    if not names:
        for row in _report_rows(report):
            name = row.get("name")
            if isinstance(name, str) and name and name not in names:
                names.append(name)
    if not names:
        return "[không có PATCH/COLLECT]"
    if len(names) == 1:
        return names[0]
    return f"{names[0]} +{len(names)-1} mục"


def _list_history(root: Path) -> int:
    entries = _visible_history_entries(root); pins = _load_pinned_runs(root)
    if not entries:
        print("Chưa có lịch sử PATCH/COLLECT có công việc thực sự."); return 0
    print("PATCH TOOL RUN HISTORY — IDLE runs are hidden")
    for i, (_path, report) in enumerate(entries[:max(RUN_HISTORY_LIMIT, len(pins)+10)], 1):
        rid = str(report.get("run_id") or "unknown")
        mark = "PIN" if rid in pins else "   "
        print(f"{i:>2}. [{mark}] {_history_row_text(report, pinned=rid in pins)}")
        print(f"     run_id: {rid}")
    print("Manage: report --pin/--unpin/--delete/--export <run_id> | report --cleanup")
    return 0



def _history_default_index(entries: list[tuple[Path, dict[str, object]]]) -> int:
    """Prefer the newest meaningful PASS run, then any meaningful run.

    Zero-argument IDLE checks are intentionally not the default history row:
    the operator asked to review the last real PATCH/COLLECT result, not the
    empty probe that led them into history.
    """
    for index, (_path, report) in enumerate(entries):
        if str(report.get("status") or "").upper() == "PASS" and bool(report.get("selected")):
            return index
    for index, (_path, report) in enumerate(entries):
        if bool(report.get("selected")):
            return index
    return 0


def _history_display_time(raw: object) -> str:
    text = str(raw or "").strip()
    if not text:
        return "unknown-time"
    try:
        parsed = datetime.fromisoformat(text.replace("Z", "+00:00"))
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed.astimezone().strftime("%Y-%m-%d %H:%M:%S")
    except Exception:
        return text.replace("T", " ")[:19]


def _history_row_text(report: dict[str, object], *, pinned: bool = False) -> str:
    status = str(report.get("status") or "UNKNOWN").upper()
    when = _history_display_time(report.get("started_at"))
    name = _history_primary_name(report)
    pin = " | PIN" if pinned else ""
    # Operator-facing order: package name first, then LOCAL execution time, then state.
    return f"{_safe_display(name)} | {when} | {status}{pin}"



def _render_history_selector(
    entries: list[tuple[Path, dict[str, object]]], cursor: int, pins: set[str], prev: int, msg: str = ""
) -> int:
    terminal_width, terminal_height = _selector_term_size()
    frame_budget = max(1, terminal_height - 1)
    footer = [
        "↑/↓: di chuyển | Enter: mở kết quả | q/Esc: quay lại",
        _safe_display(msg) if msg else "",
    ] if frame_budget >= 5 else ([] if frame_budget <= 2 else ["↑/↓ | Enter mở | q quay lại"])
    header = ["LỊCH SỬ CHẠY PATCH TOOL — PATCH/COLLECT thực tế; mặc định: lần PASS gần nhất"] if frame_budget >= 2 else []
    item_capacity = max(1, frame_budget - len(header) - len(footer))
    start, end = _selector_viewport(len(entries), cursor, item_capacity)
    lines: list[tuple[str, bool]] = [(x, False) for x in header]
    for i in range(start, end):
        _path, report = entries[i]
        rid = str(report.get("run_id") or "")
        lines.append((
            f"{'›' if i == cursor else ' '} {i + 1:>3}. {_history_row_text(report, pinned=rid in pins)}",
            i == cursor,
        ))
    lines.extend((x, False) for x in footer)
    lines = [(_clip_selector_line(line, terminal_width), current) for line, current in lines[:frame_budget]]
    frame_height = max(prev, len(lines))
    cursor_up = min(prev, max(0, frame_budget))
    if cursor_up:
        sys.stdout.write(f"\x1b[{cursor_up}F")
    padded = lines + [("", False)] * (max(len(lines), min(frame_height, frame_budget)) - len(lines))
    use_emphasis = bool(getattr(sys.stdout, "isatty", lambda: False)())
    for line, current in padded:
        rendered_line = "\x1b[1;7m" + line + "\x1b[0m" if current and use_emphasis else line
        sys.stdout.write("\r\x1b[2K" + rendered_line + "\n")
    sys.stdout.flush()
    return len(padded)


def _zero_work_history_landing(root: Path) -> int:
    """Best-effort read-only HISTORY landing for an invocation with no work.

    Artifact safety remains fail-closed for actual report/recovery operations.
    Merely having no runnable package must not become a failed run because an
    optional historical artifact tree is unavailable/unsafe; emit a warning
    and leave project state untouched instead.
    """
    try:
        return _history_browser(root)
    except QueueSafetyError as exc:
        print(f"[PTV v{VERSION} WARNING] HISTORY unavailable: {_safe_display(str(exc))}", file=sys.stderr)
        return 0


def _history_browser(root: Path) -> int:
    """Interactive history browser backed by the same persisted run reports.

    Enter opens the normal report menu, so detail logs, source diffs,
    FAIL_HANDOFF/recovery paths and support ZIP creation behave exactly like
    the just-finished run report.
    """
    entries = _visible_history_entries(root)
    if not entries:
        print("Chưa có lịch sử PATCH/COLLECT có công việc thực sự.")
        return 0
    default_index = _history_default_index(entries)
    pins = _load_pinned_runs(root)
    use_posix_tty = (
        os.name != "nt" and termios is not None and tty is not None
        and sys.stdin.isatty() and sys.stdout.isatty()
    )
    use_windows_tty = (
        os.name == "nt" and msvcrt is not None
        and sys.stdin.isatty() and sys.stdout.isatty() and _enable_windows_vt()
    )
    if not (use_posix_tty or use_windows_tty):
        print("PATCH TOOL RUN HISTORY")
        for i, (_path, report) in enumerate(entries, 1):
            mark = "*" if i - 1 == default_index else " "
            print(f"{mark} {i:>2}. {_history_row_text(report, pinned=str(report.get('run_id') or '') in pins)}")
        # Captured IDE/task runners are non-interactive.  Showing HISTORY must
        # never block on input from a pipe that may stay open indefinitely.
        if not (sys.stdin.isatty() and sys.stdout.isatty()):
            return 0
        while True:
            try:
                raw = input(f"History [Enter={default_index + 1}, number=open, q=back]: ").strip().lower()
            except (EOFError, KeyboardInterrupt):
                print("")
                return 0
            if raw in {"q", "quit", "esc"}:
                return 0
            index = default_index if raw == "" else (int(raw) - 1 if raw.isdigit() else -1)
            if not 0 <= index < len(entries):
                print("Lựa chọn lịch sử không hợp lệ.")
                continue
            _batch_report_menu(root, entries[index][1])
            return 0

    fd = sys.stdin.fileno()
    old = termios.tcgetattr(fd) if use_posix_tty else None
    if use_posix_tty:
        tty.setcbreak(fd)
    cursor = default_index
    rendered = 0
    msg = ""
    try:
        while True:
            rendered = _render_history_selector(entries, cursor, pins, rendered, msg)
            msg = ""
            key = _read_key(fd) if use_posix_tty else _read_key_windows()
            if key == "\x03":
                raise KeyboardInterrupt
            if key in {"q", "ESC"}:
                return 0
            if key == "UP":
                cursor = (cursor - 1) % len(entries)
                continue
            if key == "DOWN":
                cursor = (cursor + 1) % len(entries)
                continue
            if key == "ENTER":
                if use_posix_tty:
                    termios.tcsetattr(fd, termios.TCSADRAIN, old)
                sys.stdout.write("\r\x1b[2K\n"); sys.stdout.flush()
                _batch_report_menu(root, entries[cursor][1])
                if use_posix_tty:
                    tty.setcbreak(fd)
                rendered = 0
                entries = _history_entries(root)
                if not entries:
                    print("No Patch Tool run history is available.")
                    return 0
                cursor = min(cursor, len(entries) - 1)
                pins = _load_pinned_runs(root)
    finally:
        if use_posix_tty:
            try:
                termios.tcsetattr(fd, termios.TCSADRAIN, old)
            except Exception:
                pass
        sys.stdout.write("\r\x1b[2K")
        sys.stdout.flush()


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
    out_dir = _artifact_subdir(root, "exports")
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
        if rows:
            print(f"Menu: 1..{len(rows)}=detail | a=aggregate | p=PASS | x=problems | c=changed | d N=diff | s N=support ZIP | h=history | q=exit")
        else:
            print("Menu: a=aggregate | h=history | q=exit")
        try: raw = input("report> ").strip()
        except (EOFError, KeyboardInterrupt): print(""); return
        low = raw.lower()
        if low in {"", "q", "quit", "esc"}: return
        if low in {"a", "all", "aggregate"}:
            rel = report.get("batch_log")
            if isinstance(rel, str): _show_file_paged(root / rel, "AGGREGATE BATCH LOG")
            else: print("Aggregate log is unavailable.")
            continue
        if low == "h": _list_history(root); continue
        if not rows:
            if low in {"n", "p", "x", "c"} or re.fullmatch(r"[ds]\s+\d+", low) or low.isdigit():
                print("Run này không có PATCH/COLLECT item để xem detail/diff/support.")
            else:
                print("Lựa chọn không hợp lệ.")
            continue
        if low == "n":
            print(f"Detail dùng số thứ tự 1..{len(rows)} (ví dụ: 1), không phải chữ N.")
            continue
        if low == "p":
            _print_row_subset([(i,r) for i,r in enumerate(rows,1) if r.get("status")=="PASS"], "PASS ITEMS"); continue
        if low == "x":
            _print_row_subset([(i,r) for i,r in enumerate(rows,1) if r.get("status") in {"FAIL","BLOCKED","PREFLIGHT_FAIL"}], "PROBLEM ITEMS"); continue
        if low == "c":
            _print_row_subset([(i,r) for i,r in enumerate(rows,1) if isinstance(r.get("source_compare"),dict) and (r["source_compare"].get("changed_paths") or [])], "ITEMS WITH SOURCE CHANGES"); continue
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


def _render(items, cursor, selected, priorities, msg, prev, *, show_history: bool = False, failed_group_names: set[str] | None = None):
    terminal_width, terminal_height = _selector_term_size()

    # Keep one physical row free below the frame.  Writing a frame as tall as
    # the terminal can itself trigger a scroll on the final newline, after
    # which cursor-up no longer returns to the frame's true first row.
    frame_budget = max(1, terminal_height - 1)
    full_footer = [
        "",
        "Space: chọn/bỏ [x] | 0-9: gán ưu tiên | ↑/↓: di chuyển",
        "a: tất cả PATCH [x] | n: bỏ tất cả | /: tìm/lọc | d: xóa | i: inspect | p: preview | v: validate | h: health",
        "Enter: xác nhận/mở HISTORY | q/Esc: hủy | Số nhỏ chạy trước; cùng số giữ thứ tự hiện tại",
        _safe_display(msg) if msg else "",
    ]
    compact_help = "Space/[0-9]/↑↓ | / lọc | i/p/v | Enter chạy/mở HISTORY | q hủy"

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

    failed_group_names = set(failed_group_names or ())
    # Group headers are presentation-only physical rows.  They are deliberately
    # not selectable and therefore do not change cursor numbering or operations.
    # On pathological tiny terminals omit them rather than hiding the current item.
    group_header_budget = 2 if failed_group_names and frame_budget >= 6 else 0
    item_capacity = max(1, frame_budget - header_rows - len(footer) - group_header_budget)
    total_rows = len(items) + (1 if show_history else 0)
    start, end = _selector_viewport(total_rows, cursor, item_capacity)

    current_tag = f"CON TRỎ {cursor + 1}/{total_rows}" if total_rows else "CON TRỎ 0/0"
    # Put the cursor identity first. On narrow terminals horizontal clipping
    # preserves the left side, so the operator must never lose i/N merely
    # because the decorative Vietnamese title is longer than the viewport.
    if header_rows == 2:
        if total_rows > item_capacity:
            header = [f"{current_tag} | VIEW {start + 1}-{end}/{total_rows} | CHỌN CÔNG VIỆC SẼ CHẠY", ""]
        else:
            header = [f"{current_tag} | CHỌN CÔNG VIỆC SẼ CHẠY", ""]
    elif header_rows == 1:
        header = [f"{current_tag} | CHỌN CÔNG VIỆC SẼ CHẠY"]
    else:
        header = []

    lines: list[tuple[str, bool]] = [(line, False) for line in header]
    last_visible_group: str | None = None
    for i in range(start, end):
        if i < len(items):
            item = items[i]
            group = "failed" if item.name in failed_group_names else "new"
            if group_header_budget and group != last_visible_group:
                label = "   Failed patch/collect (unresolved):" if group == "failed" else "   New patch/collect:"
                lines.append((label, False))
                last_visible_group = group
            detail = f"  [{_safe_display(item.detail)}]" if item.detail else ""
            lines.append((
                f"{'›' if i == cursor else ' '} "
                f"[{_selection_mark(i, selected, priorities)}] {i + 1:>3}. "
                f"[{_safe_display(item.kind)}] {_safe_display(item.name)}{detail}",
                i == cursor,
            ))
        else:
            lines.append((
                f"{'›' if i == cursor else ' '}     H. [HISTORY] Xem lại lịch sử chạy gần đây",
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
        "failure_policy": "continue_independent",
        "transaction_policy": "patch",
    }
    warnings: list[str] = []
    path = root / ".python_patch_tool.json"
    if not path.exists() and not path.is_symlink():
        return cfg, warnings
    try:
        # Use the same bounded/non-symlink/duplicate-key parser as project
        # identity and trusted validation profiles.  One config file must have
        # one validity contract across selector, batch policy and runner paths.
        data = load_project_config(root)
        node = data.get("automation", {}).get("zero_argument", {})
    except Exception as exc:
        warnings.append(f"invalid .python_patch_tool.json; using prompt defaults ({type(exc).__name__}: {exc})")
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
            warnings.append(f"unsupported batch.failure_policy={failure_policy!r}; using continue_independent")
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
        registry_rows = [row for row in _unresolved_failure_rows(root) if _failure_row_matches_queue_item(root, row, victim)]
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
        _resolve_registry_rows(root, registry_rows, "deleted_from_normal_queue")
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


def _selector_search_blob(root: Path, item: QueueItem, cache: dict[str,str]) -> str:
    cached = cache.get(item.name)
    if cached is not None:
        return cached
    parts = [item.name, item.kind, item.detail]
    if item.kind == "PATCH":
        try:
            meta = load_patch_meta(root, item.name)
            parts.extend([meta.patch_id, str((meta.manifest.get("patch") or {}).get("summary") or "")])
            parts.extend(meta.effective_targets)
        except Exception:
            pass
    blob = "\n".join(str(x) for x in parts).casefold()
    cache[item.name] = blob
    return blob


def _filter_selector_items(root: Path, items: list[QueueItem], query: str, cache: dict[str,str]) -> list[QueueItem]:
    q = query.strip().casefold()
    if not q:
        return list(items)
    return [item for item in items if q in _selector_search_blob(root,item,cache)]


def _select_items_line(root: Path, items: list[QueueItem], initial_selection: str, *, show_history: bool = False, failed_group_names: set[str] | None = None):
    all_items = list(items)
    search_cache: dict[str,str] = {}
    selected = _initial_selected(items, initial_selection)
    failed_group_names = set(failed_group_names or ())
    while items:
        print("CHỌN CÔNG VIỆC SẼ CHẠY")
        last_group: str | None = None
        for i, item in enumerate(items, 1):
            group = "failed" if item.name in failed_group_names else "new"
            if failed_group_names and group != last_group:
                print("   last failed patch/collect:" if group == "failed" else "   New patch/collect:")
                last_group = group
            mark = "x" if i - 1 in selected else " "
            print(f"  [{mark}] {i}. [{_safe_display(item.kind)}] {_safe_display(item.name)}")
        if show_history:
            print("      H. [HISTORY] Xem lại lịch sử chạy gần đây")
        print("Nhập: 1,3-5 | /text=lọc | /=bỏ lọc | a=all PATCH | n=none | d <range>=xóa | i <số>=inspect | p <số>=preview | v <số>=validate | h=health | r=history | q=quit | Enter=xác nhận")
        raw_line, interrupted = _readline_or_interrupt()
        if interrupted:
            print("\nCancelled by Ctrl+C.")
            raise KeyboardInterrupt
        if raw_line == "":
            return None
        raw = raw_line.strip().lower()
        if raw in {"q", "quit"}:
            return None
        if raw.startswith("/"):
            query = raw_line.strip()[1:]
            filtered = _filter_selector_items(root, all_items, query, search_cache)
            if not filtered:
                print(f"Không có item khớp bộ lọc: {_safe_display(query)}")
                continue
            items[:] = filtered
            selected = _initial_selected(items, "none")
            print(f"FILTER: {_safe_display(query) if query else '[cleared]'} | {len(items)}/{len(all_items)} item")
            continue
        if raw in {"h", "health"}:
            rc = print_health(root, compact=False)
            print(f"TOOL HEALTH rc={rc}")
            continue
        if show_history and raw in {"r", "history"}:
            _history_browser(root)
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
        if raw.startswith("p "):
            try:
                indexes = _parse_index_spec(raw[2:].strip(), len(items))
            except ValueError as exc:
                print(f"Preview không hợp lệ: {_safe_display(str(exc))}")
                continue
            if len(indexes) != 1:
                print("Preview yêu cầu đúng một PATCH index.")
                continue
            index = next(iter(indexes))
            rc = _preview_item(root, items[index])
            print(f"PREVIEW rc={rc}")
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
            if deleted:
                all_items[:] = [x for x in all_items if x.name not in set(deleted)]
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


def select_items(root, items, *, initial_selection="none", selector_ui="auto", show_history: bool = False, failed_group_names: set[str] | None = None):
    if not items:
        if show_history and sys.stdin.isatty() and sys.stdout.isatty():
            _history_browser(root)
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
        return _select_items_line(root, items, initial_selection, show_history=show_history, failed_group_names=failed_group_names)

    all_items = list(items)
    search_cache: dict[str,str] = {}
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
        while items or show_history:
            total_rows = len(items) + (1 if show_history else 0)
            cursor = min(cursor, max(0, total_rows - 1))
            rendered = _render(items, cursor, selected, priorities, msg, rendered, show_history=show_history, failed_group_names=failed_group_names)
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
                if deleted:
                    all_items[:] = [x for x in all_items if x.name not in set(deleted)]
                cursor = min(cursor, max(0, len(items) - 1))
                continue
            if key == "UP":
                cursor = (cursor - 1) % total_rows
                continue
            elif key == "DOWN":
                cursor = (cursor + 1) % total_rows
                continue
            on_history = bool(show_history and cursor == len(items))
            if on_history and key == "ENTER":
                if use_posix_tty:
                    termios.tcsetattr(fd, termios.TCSADRAIN, old)
                sys.stdout.write("\r\x1b[2K\n"); sys.stdout.flush()
                _history_browser(root)
                if use_posix_tty:
                    tty.setcbreak(fd)
                rendered = 0
                cursor = 0 if items else len(items)
                msg = "Đã quay lại queue; HISTORY vẫn có thể mở lại bằng Enter."
                continue
            if on_history and (key == "SPACE" or key in {str(i) for i in range(10)} or key in {"d", "i", "p", "v"}):
                msg = "HISTORY: nhấn Enter để xem lịch sử; mục này không phải PATCH/COLLECT."
                continue
            if key == "SPACE":
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
            elif key == "/":
                if use_posix_tty:
                    termios.tcsetattr(fd, termios.TCSADRAIN, old)
                sys.stdout.write("\nBộ lọc (tên/id/summary/target; Enter trống để bỏ lọc): "); sys.stdout.flush()
                try:
                    query = sys.stdin.readline().rstrip("\r\n")
                except KeyboardInterrupt:
                    query = ""
                filtered = _filter_selector_items(root, all_items, query, search_cache)
                if use_posix_tty:
                    tty.setcbreak(fd)
                rendered = 0
                if filtered:
                    items[:] = filtered
                    selected = _initial_selected(items, "none")
                    priorities.clear(); cursor = 0
                    msg = f"FILTER: {query if query else '[cleared]'} | {len(items)}/{len(all_items)} item"
                else:
                    msg = f"Không có item khớp: {query}"
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
            elif key == "p":
                if use_posix_tty:
                    termios.tcsetattr(fd, termios.TCSADRAIN, old)
                sys.stdout.write("\n--- PATCH PREVIEW DIFF ---\n"); sys.stdout.flush()
                rc = _preview_item(root, items[cursor])
                print("--- END PREVIEW ---")
                if use_posix_tty:
                    tty.setcbreak(fd)
                rendered = 0
                msg = f"Preview {'PASS' if rc == 0 else 'FAIL'} rc={rc}; project unchanged."
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


def _resolve_explicit_selection(
    root: Path, items: list[QueueItem], *, patch_specs: list[str] | None = None,
    select_all: bool = False, select_spec: str | None = None,
) -> list[QueueItem] | None:
    """Resolve historical/non-interactive selector flags against the current runnable queue.

    This is deliberately semantic rather than launcher-only compatibility: the
    dispatcher itself must understand the selection so future launcher refactors
    cannot silently preserve a flag while dropping its behavior.
    """
    patch_specs = list(patch_specs or [])
    modes = int(bool(patch_specs)) + int(bool(select_all)) + int(select_spec is not None)
    if modes == 0:
        return None
    if modes > 1:
        raise ValueError("use only one explicit selection mode: repeated --patch, --all/-a, or --select")
    if select_all:
        chosen = [item for item in items if item.kind == "PATCH"]
        if not chosen and items:
            raise ValueError("--all selects PATCH packages only; current queue has no runnable PATCH")
        return chosen
    if select_spec is not None:
        text = str(select_spec).strip().lower()
        if text in {"a", "all", "*"}:
            chosen = list(items)
        else:
            indexes = _parse_index_spec(text, len(items))
            chosen = [item for i, item in enumerate(items) if i in indexes]
        err = _selection_contract_error(chosen)
        if err:
            raise ValueError(err)
        return chosen

    by_name = {item.name: item for item in items}
    chosen: list[QueueItem] = []
    seen: set[str] = set()
    for raw in patch_specs:
        spec = str(raw).strip()
        item = None
        if spec.isdigit():
            idx = int(spec)
            if 1 <= idx <= len(items):
                item = items[idx - 1]
        if item is None:
            normalized = spec.replace("\\", "/")
            if normalized.startswith("patchs/"):
                normalized = normalized.split("/", 1)[1]
            item = by_name.get(Path(normalized).name if "/" in normalized else normalized)
        if item is None:
            raise ValueError(f"explicit --patch target is not runnable in current queue: {spec}")
        if item.kind != "PATCH":
            raise ValueError(f"--patch accepts PATCH packages only: {item.name}")
        if item.name not in seen:
            chosen.append(item); seen.add(item.name)
    return chosen


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


def _failure_relation_targets(row: dict[str, object] | None) -> set[str]:
    """Best-effort declared/observed target set for unresolved-failure relation checks."""
    if not isinstance(row, dict):
        return set()
    out: set[str] = set()
    pr = row.get("patch_result") if isinstance(row.get("patch_result"), dict) else {}
    pre = pr.get("preflight") if isinstance(pr.get("preflight"), dict) else {}
    for value in pre.get("target_paths") if isinstance(pre.get("target_paths"), list) else []:
        if isinstance(value, str) and value:
            out.add(value)
    diag = pr.get("diagnosis") if isinstance(pr.get("diagnosis"), dict) else {}
    for value in diag.get("affected_paths") if isinstance(diag.get("affected_paths"), list) else []:
        if isinstance(value, str) and value:
            out.add(value)
    partial = pr.get("partial_modification") if isinstance(pr.get("partial_modification"), dict) else {}
    for value in partial.get("changed_paths") if isinstance(partial.get("changed_paths"), list) else []:
        if isinstance(value, str) and value:
            out.add(value)
    return out


def _build_batch_plan(root: Path, chosen: list[QueueItem], available: list[QueueItem], previous: dict[str, object] | None):
    """Resolve explicit dependencies and unresolved-predecessor actions.

    Cross-run failure constraints are relation-based, not position-based: an
    unrelated first PATCH must not be forced to handle a failure that only a
    later successor depends on/overlaps, and a related later successor must not
    be able to bypass ``batch.previous_failure`` merely because it is not first.

    The manifest currently has a singular ``batch.previous_failure`` contract.
    Therefore if one selected batch is related to more than one unresolved
    predecessor, fail closed and require the operator to resolve/retry those
    failures through SMART RESUME before running the successor batch.
    """
    if any(item.kind != "PATCH" for item in chosen):
        return list(chosen), {}, None
    work = list(chosen)
    by_name = {item.name: item for item in available if item.kind == "PATCH"}
    selected_names = {x.name for x in work}
    previous_action = None

    # Load selected metadata once for relation checks before any predecessor is
    # inserted into the work list.
    initial_meta = {item.name: load_patch_meta(root, item.name) for item in work}
    related_contexts: list[dict[str, object]] = []
    for failed_row in _merged_failed_recovery_rows(root, previous):
        _failed_kind, failed_id, _failed_sha, failed_name = _failure_identity(failed_row)
        if not failed_name:
            continue
        bound_failed = _bind_recovery_queue_row(root, failed_row)
        failed_queue_name = str(bound_failed.get("_recovery_queue_name")) if isinstance(bound_failed, dict) and bound_failed.get("_recovery_queue_name") else None
        failed_selected_name = failed_queue_name or failed_name
        # Selecting the exact failed/requeued package is an explicit retry and
        # does not require a successor declaration for that same predecessor.
        if failed_selected_name in selected_names:
            continue

        failed_targets = _failure_relation_targets(failed_row)
        # Prefer package metadata when the exact replay bytes are still queued;
        # this preserves target relation even for early preflight failures whose
        # structured result did not yet populate preflight.target_paths.
        if failed_queue_name and (root / "patchs" / failed_queue_name).is_file():
            try:
                failed_targets.update(load_patch_meta(root, failed_queue_name).effective_targets)
            except Exception:
                pass

        related: list[tuple[int, PatchMeta]] = []
        for idx, item in enumerate(work):
            meta = initial_meta[item.name]
            dependency_related = bool(failed_id and failed_id in meta.depends_on)
            target_related = bool(failed_targets and failed_targets.intersection(meta.effective_targets))
            declared_prev = meta.previous_failure if isinstance(meta.previous_failure, dict) else {}
            declared_id = str(declared_prev.get("patch_id") or "")
            declared_file = str(declared_prev.get("patch_file") or "")
            declaration_related = bool(
                (failed_id and declared_id and declared_id == failed_id)
                or (declared_file and declared_file == failed_name)
            )
            if dependency_related or target_related or declaration_related:
                related.append((idx, meta))
        if related:
            related_contexts.append({
                "failed_row": failed_row,
                "bound_failed": bound_failed,
                "failed_name": failed_name,
                "failed_id": failed_id or None,
                "failed_queue_name": failed_queue_name,
                "failed_targets": failed_targets,
                "related": related,
            })

    if len(related_contexts) > 1:
        names = ", ".join(str(x.get("failed_name") or "?") for x in related_contexts)
        raise BatchPlanError(
            "selected PATCHes are related to multiple unresolved failed PATCHes; "
            f"resolve/retry them with SMART RESUME first: {names}",
            kind="multiple_previous_failures_action_required",
        )

    failed_queue_name = None
    if related_contexts:
        ctx = related_contexts[0]
        failed_name = str(ctx["failed_name"])
        failed_id = str(ctx.get("failed_id") or "") or None
        failed_queue_name = str(ctx.get("failed_queue_name") or "") or None
        bound_failed = ctx.get("bound_failed") if isinstance(ctx.get("bound_failed"), dict) else None
        # The earliest related successor owns the singular previous_failure
        # decision for this predecessor.  Unrelated PATCHes before it remain
        # completely independent.
        related = ctx["related"]
        successor_meta = sorted(related, key=lambda x: x[0])[0][1]
        previous_action = validate_previous_failure_declaration(successor_meta, failed_name, failed_id)
        previous_action["successor_file"] = successor_meta.name
        previous_action["queue_file"] = failed_queue_name or failed_name
        expected_sha = _recovery_row_expected_sha(bound_failed or ctx["failed_row"])
        if expected_sha:
            previous_action["patch_sha256"] = expected_sha
        action = str(previous_action.get("action"))
        if action == "block":
            raise BatchPlanError(
                f"{successor_meta.name} explicitly blocks while failed predecessor {failed_name} is unresolved",
                kind="previous_failure_blocked",
            )
        if action in {"retry_before", "run_after"}:
            if not failed_queue_name:
                raise BatchPlanError(
                    f"previous failed PATCH is unavailable for {action}: {failed_name}",
                    kind="previous_failure_missing",
                )
            failed_item = by_name.get(failed_queue_name) or QueueItem(failed_queue_name, "PATCH")
            if not (root / "patchs" / failed_queue_name).is_file():
                raise BatchPlanError(
                    f"previous failed PATCH is unavailable for {action}: {failed_queue_name}",
                    kind="previous_failure_missing",
                )
            successor_index = next(i for i,x in enumerate(work) if x.name == successor_meta.name)
            if action == "retry_before":
                work.insert(successor_index, failed_item)
            else:
                work.insert(successor_index + 1, failed_item)

    metas = [load_patch_meta(root, item.name) for item in work]
    ordered_metas = topo_order(metas)
    item_by_name = {item.name: item for item in work}
    ordered = [item_by_name[m.name] for m in ordered_metas]

    # Explicit run_after is stronger than ordinary stable order. It is only
    # allowed when dependency declarations do not force the opposite order.
    if previous_action and previous_action.get("action") == "run_after" and failed_queue_name:
        failed_meta = next((m for m in ordered_metas if m.name == failed_queue_name), None)
        successor_file = str(previous_action.get("successor_file") or "")
        successor_meta = next((m for m in ordered_metas if m.name == successor_file), None)
        if failed_meta and successor_meta:
            if failed_meta.patch_id in successor_meta.depends_on:
                raise BatchPlanError(
                    f"run_after conflicts with depends_on: {successor_meta.name} depends on {failed_meta.patch_id}",
                    kind="previous_failure_action_conflict",
                )
            # Move the failed replay immediately after its declaring successor
            # while keeping all other stable-topological order intact.
            ordered = [x for x in ordered if x.name != failed_queue_name]
            pos = next((i for i,x in enumerate(ordered) if x.name == successor_file), len(ordered)-1)
            ordered.insert(pos + 1, item_by_name[failed_queue_name])
            ordered_metas = [m for m in ordered_metas if m.name != failed_queue_name]
            mpos = next((i for i,m in enumerate(ordered_metas) if m.name == successor_file), len(ordered_metas)-1)
            ordered_metas.insert(mpos + 1, failed_meta)
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
            validate_rc, text, validate_timed_out = _run_runner_captured(
                root, _runner_command(root, "validate", item), env=env, timeout=120
            )
            cp = object()  # sentinel: the runner started and returned a classified result
        except Exception as exc:
            cp = None
            validate_rc = 2
            validate_timed_out = False
            text = f"TOOL_ERROR: {type(exc).__name__}: {exc}\n"
        log_path.write_text(text, encoding="utf-8", errors="replace")
        patch_result = None
        if result_path.is_file() and not result_path.is_symlink():
            try:
                value = json.loads(result_path.read_text(encoding="utf-8"))
                if isinstance(value, dict): patch_result = value
            except Exception:
                patch_result = None
        rc = 2 if cp is None else int(validate_rc)
        classification = "READY_TO_APPLY" if rc == 0 else ("TOOL_ERROR" if validate_timed_out else "UNKNOWN")
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


def _materialize_batch_preflight_failure(
    root: Path,
    item: QueueItem,
    row: dict[str, object] | None,
) -> dict[str, object]:
    """Create the normal recovery artifacts for one read-only PATCH preflight failure.

    The failure is item-local: no payload has executed and the project is unchanged.
    Capturing the handoff here (before any independent PATCH writes source) preserves
    the exact diagnostic/source state that caused the preflight rejection.
    """
    row = row or {}
    patch_result = row.get("patch_result") if isinstance(row.get("patch_result"), dict) else None
    if patch_result is None:
        kind = str(row.get("classification") or "batch_preflight_failed").lower()
        patch_path = root / "patchs" / item.name
        patch_result = {
            "format": "python-patch-tool-patch-result", "format_version": 1, "tool_version": VERSION,
            "patch_file": item.name,
            "patch_sha256": _sha256_file(patch_path) if patch_path.is_file() and not patch_path.is_symlink() else None,
            "status": "FAIL", "rc": 2, "stage": "preflight",
            "diagnosis": {"kind": kind, "message": "batch preflight rejected PATCH", "affected_paths": []},
            "partial_modification": {"detected": False, "changed_paths": [], "evidence": "read_only_batch_preflight"},
        }
    diagnosis = patch_result.get("diagnosis") if isinstance(patch_result.get("diagnosis"), dict) else {
        "kind": str(row.get("classification") or "batch_preflight_failed"),
        "message": "batch preflight rejected PATCH",
        "affected_paths": [],
    }
    recovery_request = _create_recovery_collect_request(root, item, patch_result)
    log_text = ""
    detail_log_path = None
    if isinstance(row.get("log_path"), str):
        detail_log_path = root / str(row["log_path"])
        try:
            log_text = detail_log_path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            log_text = ""
    fail_handoff = _create_fail_handoff(
        root, item, 2, log_text, patch_result, recovery_request,
        detail_log_path=detail_log_path,
    )
    detail: dict[str, object] = {
        "name": item.name, "kind": item.kind, "status": "PREFLIGHT_FAIL", "rc": 2,
        "diagnosis": diagnosis, "patch_result": patch_result,
        "preflight_log_path": row.get("log_path"),
        "recovery_collect_request": recovery_request.relative_to(root).as_posix() if recovery_request is not None else None,
        "fail_handoff": fail_handoff.relative_to(root).as_posix() if fail_handoff is not None else None,
        "continue_decision": {"allowed": True, "reason": "read_only_preflight_failure_project_unchanged"},
    }
    row["recovery_collect_request"] = detail["recovery_collect_request"]
    row["fail_handoff"] = detail["fail_handoff"]
    return detail


def _safe_to_continue_after_failure(detail: dict[str, object]) -> tuple[bool, str]:
    """Return whether unrelated PATCHes may continue after this failure.

    v6.17.5 defaults to continuation for independent work. Only an operator
    interruption or an uncontained/unknown source mutation is a global stop.
    Dependency/target relationships are handled per successor in execute_items.
    """
    if int(detail.get("rc") or 0) == 130:
        return False, "interrupted"
    result = detail.get("patch_result") if isinstance(detail.get("patch_result"), dict) else {}
    diagnosis = result.get("diagnosis") if isinstance(result.get("diagnosis"), dict) else {}
    kind = str(diagnosis.get("kind") or "")
    rollback = result.get("rollback") if isinstance(result.get("rollback"), dict) else None
    partial = result.get("partial_modification") if isinstance(result.get("partial_modification"), dict) else {}
    on_failure = result.get("on_failure") if isinstance(result.get("on_failure"), dict) else None
    # Failure-only commands run after rollback and may themselves modify the
    # project.  Their final state therefore has precedence over an earlier
    # rollback PASS when deciding whether unrelated PATCHes may continue.
    if on_failure is not None:
        if str(on_failure.get("status") or "") != "PASS" or int(on_failure.get("rc") or 0) != 0:
            return False, "on_failure_commands_failed_or_incomplete"
        if partial.get("detected") is False:
            return True, "on_failure_commands_passed_project_unchanged"
        return False, "on_failure_commands_left_partial_or_unknown_state"
    if rollback is not None and rollback.get("status") == "PASS":
        return True, "per_patch_rollback_restored"
    if kind in {"rollback_failed", "rollback_incomplete", "rollback_snapshot_race"}:
        return False, kind
    if partial.get("detected") is False:
        return True, "failure_contained_project_unchanged"
    return False, "unsafe_partial_or_unknown_state"


def _apply_previous_failure_action(root: Path, action: dict[str, object] | None) -> dict[str, object] | None:
    if not action or action.get("action") != "delete":
        return action
    logical_name = str(action.get("patch_file") or "")
    queue_name = str(action.get("queue_file") or logical_name)
    if not queue_name or Path(queue_name).name != queue_name:
        action["result"] = "identity_invalid"
        return action
    src = root / "patchs" / queue_name
    if not src.is_file() or src.is_symlink():
        action["result"] = "already_absent"
        return action
    expected_sha = str(action.get("patch_sha256") or "").lower()
    try:
        current_sha = stable_package_sha256(src)
    except Exception as exc:
        action["result"] = "identity_check_failed"
        action["error"] = f"{type(exc).__name__}: {exc}"
        return action
    if expected_sha and current_sha.lower() != expected_sha:
        action["result"] = "identity_mismatch"
        action["error"] = "queue package no longer matches the failed predecessor SHA"
        return action
    candidate = LocalDuplicate(QueueItem(queue_name, "PATCH"), "previous-failure", current_sha)
    moved, warnings = _move_local_duplicates_to_ignore(root, [candidate])
    action["warnings"] = warnings
    moved_item = moved[0] if moved else candidate
    if moved_item.ignored_name:
        action["result"] = "moved_to_ignore"
        action["ignore_path"] = f"patchs/ignore/{moved_item.ignored_name}"
        print(f"PREVIOUS FAILED PATCH ACTION: DELETE -> {action['ignore_path']}")
    else:
        action["result"] = "move_failed"
        if warnings:
            action["error"] = warnings[0]
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
    failure_policy: str = "continue_independent",
    metas: dict[str, PatchMeta] | None = None,
    history_replay_sha: dict[str, str] | None = None,
    preflight_failure_details: dict[str, dict[str, object]] | None = None,
    no_validation: bool = False,
):
    """Execute a validated batch with controlled continuation and dependency blocking."""
    global _LAST_EXECUTION_DETAILS
    _LAST_EXECUTION_DETAILS = []
    metas = metas or {}
    preflight_failure_details = preflight_failure_details or {}
    contract_error = _selection_contract_error(chosen)
    if contract_error:
        print(f"[PTV v{VERSION} ERROR] SELECTION: {_safe_display(contract_error)}", file=sys.stderr)
        return 2, [], list(chosen), [], []
    executed: list[tuple[str, int]] = []
    late_duplicates: list[LocalDuplicate] = []
    duplicate_warnings: list[str] = []
    patch_status_by_id: dict[str, str] = {}
    failed_target_records: list[tuple[str, set[str]]] = []
    first_failure_rc = 0
    live_status = _LivePatchStatus.start_for(chosen)

    def _finish_execution(result):
        try:
            remaining_items = result[2] if isinstance(result, tuple) and len(result) >= 3 else []
            if live_status is not None and isinstance(remaining_items, list):
                live_status.mark_not_executed(remaining_items)
        finally:
            if live_status is not None:
                live_status.close()
        return result

    for index, item in enumerate(chosen):
        item_started_mono = time.monotonic()
        item_started_at = _utc_now()
        detail_log_path = _batch_item_log_path(root, _ACTIVE_RUN_ID, index + 1, item)
        meta = metas.get(item.name)

        preflight_detail = preflight_failure_details.get(item.name)
        if item.kind == "PATCH" and preflight_detail is not None:
            if live_status is not None:
                live_status.set_status(item.name, "PREFLIGHT_FAIL")
            detail = dict(preflight_detail)
            detail.setdefault("started_at", item_started_at)
            detail.setdefault("elapsed_seconds", round(time.monotonic() - item_started_mono, 3))
            _LAST_EXECUTION_DETAILS.append(detail)
            if meta is not None:
                patch_status_by_id[meta.patch_id] = "PREFLIGHT_FAIL"
                failed_target_records.append((meta.patch_id, set(meta.effective_targets)))
            if not first_failure_rc:
                first_failure_rc = int(detail.get("rc") or 2)
            diagnosis = detail.get("diagnosis") if isinstance(detail.get("diagnosis"), dict) else {}
            reason = str(diagnosis.get("kind") or "preflight_failed")
            print(f"[PREFLIGHT_FAIL] {_safe_display(item.name)} | {_safe_display(reason)}")
            if failure_policy != "continue_independent":
                return _finish_execution((first_failure_rc, executed, chosen[index + 1 :], late_duplicates, duplicate_warnings))
            print(f"[PTV v{VERSION}] CONTINUE AFTER PREFLIGHT FAILURE: {_safe_display(item.name)} | project unchanged")
            continue

        if item.kind == "PATCH" and meta is not None:
            failed_deps = [
                dep for dep in meta.depends_on
                if patch_status_by_id.get(dep) not in {"PASS", "SKIPPED_DUPLICATE_LOCAL"}
            ]
            targets = set(meta.effective_targets)
            related_target_failures = [
                predecessor for predecessor, failed_targets in failed_target_records
                if targets and failed_targets and targets.intersection(failed_targets)
            ]
            blockers = list(dict.fromkeys([*failed_deps, *related_target_failures]))
            if blockers:
                if meta.on_dependency_failure == "run_anyway":
                    print(
                        f"[PTV v{VERSION} WARNING] batch.on_dependency_failure=run_anyway is deprecated/ignored; "
                        "related PATCH failures are always BLOCKED by current safety policy",
                        file=sys.stderr,
                    )
                diagnosis_kind = "dependency_failed" if failed_deps else "related_target_failed"
                relation = "dependency failure" if failed_deps else "related target failure"
                detail = {
                    "name": item.name, "kind": item.kind, "status": "BLOCKED", "rc": None,
                    "started_at": item_started_at,
                    "elapsed_seconds": round(time.monotonic() - item_started_mono, 3),
                    "blocked_by": blockers,
                    "diagnosis": {"kind": diagnosis_kind, "message": f"blocked by {relation}: {', '.join(blockers)}"},
                }
                _LAST_EXECUTION_DETAILS.append(detail)
                if live_status is not None:
                    live_status.set_status(item.name, "BLOCKED")
                patch_status_by_id[meta.patch_id] = "BLOCKED"
                failed_target_records.append((meta.patch_id, targets))
                print(f"[BLOCKED] {_safe_display(item.name)} | {relation}: {_safe_display(', '.join(blockers))}")
                continue

        collect_request_sha256 = _queue_item_sha256(root, item) if item.kind == "COLLECT" else None

        if item.kind == "PATCH":
            still_runnable, now_duplicates, now_warnings = _split_local_duplicate_patches(root, [item], history_replay_sha=history_replay_sha)
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
                if live_status is not None:
                    live_status.set_status(item.name, "SKIPPED_DUPLICATE_LOCAL")
                if meta is not None:
                    patch_status_by_id[meta.patch_id] = "SKIPPED_DUPLICATE_LOCAL"
                continue
            if not still_runnable:
                duplicate_warnings.append(f"late duplicate check returned no decision for patchs/{item.name}; executing normally")
            cmd = _runner_command(root, "execute", item, no_validation=no_validation)
        elif item.kind == "COLLECT":
            progress = root / "tools" / "_patch_lib" / "python_patch_collect_progress_v6_7.py"
            compat = root / "tools" / "_patch_lib" / "python_patch_collect_compat.py"
            cmd = [sys.executable, str(progress), "--project-root", str(root), "--collector", str(compat), "--", "request", f"patchs/{item.name}"]
        else:
            rc = 2
            executed.append((item.name, rc))
            _LAST_EXECUTION_DETAILS.append({"name": item.name, "kind": item.kind, "status": "FAIL", "rc": rc, "diagnosis": {"kind": "invalid_queue_item"}})
            if live_status is not None:
                live_status.set_status(item.name, "FAIL")
            return _finish_execution((rc, executed, chosen[index + 1 :], late_duplicates, duplicate_warnings))

        if live_status is not None:
            live_status.set_status(item.name, "RUNNING")
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
                rc, console_log, patch_result = _run_patch_child(
                    root, cmd, item, full_log_path=detail_log_path,
                    expected_patch_sha256=(meta.package_sha256 if meta is not None else None),
                    expected_targets=(list(meta.effective_targets) if meta is not None else None),
                    live_status=live_status,
                )
                patch_result = _enrich_patch_diagnosis(patch_result, console_log)
            else:
                runtime = _artifact_subdir(root, "runtime")
                collect_result_path = runtime / f"collect_{int(time.time()*1000000)}_{os.getpid()}.json"
                env = dict(os.environ); env["PTV_COLLECT_RESULT_FILE"] = str(collect_result_path)
                rc = _run_foreground_child(root, cmd, env=env, timeout=None, label="COLLECT")
                collect_result = _load_json(collect_result_path)
                try: collect_result_path.unlink()
                except OSError: pass
        except KeyboardInterrupt:
            rc = 130
            console_log = "INTERRUPTED by Ctrl+C\n" if item.kind == "PATCH" else ""
            patch_result = None
            if live_status is not None:
                live_status.set_status(item.name, "INTERRUPTED")
            print(f"[PTV v{VERSION}] INTERRUPTED by Ctrl+C", file=sys.stderr)

        collect_incomplete = bool(item.kind == "COLLECT" and rc == 3 and isinstance(collect_result, dict) and collect_result.get("status") == "INCOMPLETE")
        if live_status is not None and rc != 130:
            live_status.set_status(item.name, "INCOMPLETE" if collect_incomplete else ("PASS" if rc == 0 else "FAIL"))
        compare_info = _capture_item_compare_after(root, compare_before, meta)
        if (rc == 0 or collect_incomplete) and item.kind == "COLLECT":
            ok, post_detail = _collect_archive_postcondition(root, item)
            if not ok:
                print(f"[PTV v{VERSION} ERROR] {_safe_display(post_detail)}", file=sys.stderr)
                rc = 3
            elif post_detail:
                print(f"[PTV v{VERSION} WARNING] {_safe_display(post_detail)}")

        detail: dict[str, object] = {
            "name": item.name, "kind": item.kind,
            "status": "INCOMPLETE" if collect_incomplete else ("PASS" if rc == 0 else "FAIL"), "rc": rc,
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
        if item.kind == "COLLECT":
            if collect_request_sha256: detail["request_sha256"] = collect_request_sha256
            if collect_result is not None: detail["collect_result"] = collect_result

        # A successful PATCH has no normal handoff ZIP. If the request was made
        # against an older/unknown AI tool context, publish a one-shot AI sync
        # result so the next AI turn learns the current schemas/contracts too.
        if rc == 0 and item.kind == "PATCH":
            try:
                from python_patch_ai_sync import decide_sync, create_standalone_sync_result
                manifest = meta.manifest if meta is not None and isinstance(meta.manifest, dict) else {}
                ai_context = manifest.get("ai_context") if isinstance(manifest.get("ai_context"), dict) else None
                compat = manifest.get("compatibility") if isinstance(manifest.get("compatibility"), dict) else {}
                max_tested = compat.get("max_tested_version") if isinstance(compat.get("max_tested_version"), str) else None
                sync_decision = decide_sync(
                    root,
                    ai_context=ai_context,
                    fallback_known_tool_version=max_tested,
                    channel="patch",
                )
                sync_pair = create_standalone_sync_result(root, decision=sync_decision, source_name=item.name)
                if sync_pair is not None:
                    sync_zip, sync_text = sync_pair
                    try: detail["ai_sync_result"] = sync_zip.relative_to(root).as_posix()
                    except ValueError: detail["ai_sync_result"] = str(sync_zip)
                    try: detail["ai_sync_result_text"] = sync_text.relative_to(root).as_posix()
                    except ValueError: detail["ai_sync_result_text"] = str(sync_text)
                    print(f"[PTV v{VERSION}] AI TOOL UPDATE REQUIRED: current client knowledge differs from request context")
                    _print_upload_action_block(sync_zip, patch_failure=False, companion_path=sync_text, root=root)
            except Exception as sync_exc:
                # AI synchronization is additive and must never downgrade a
                # successfully applied PATCH into a failure.
                print(f"[PTV v{VERSION} WARNING] AI tool sync result unavailable: {type(sync_exc).__name__}: {sync_exc}", file=sys.stderr)

        executed.append((item.name, rc))

        if rc and item.kind == "PATCH":
            recovery_request = _create_recovery_collect_request(root, item, patch_result or {})
            if recovery_request is not None:
                detail["recovery_collect_request"] = recovery_request.name
                print(f"[NEXT RUN - COLLECT REQUEST READY] patchs/{_safe_display(recovery_request.name)}")
            handoff = _create_fail_handoff(root, item, rc, console_log, patch_result, recovery_request, detail_log_path=detail_log_path)
            if handoff is not None:
                try: detail["fail_handoff"] = handoff.relative_to(root).as_posix()
                except ValueError: detail["fail_handoff"] = str(handoff)
                handoff_text = handoff.with_suffix(".txt")
                if handoff_text.is_file() and not handoff_text.is_symlink():
                    try: detail["fail_handoff_text"] = handoff_text.relative_to(root).as_posix()
                    except ValueError: detail["fail_handoff_text"] = str(handoff_text)
        _LAST_EXECUTION_DETAILS.append(detail)
        if meta is not None:
            patch_status_by_id[meta.patch_id] = str(detail["status"])
            if rc and item.kind == "PATCH":
                failed_target_records.append((meta.patch_id, set(meta.effective_targets)))

        if rc:
            if not first_failure_rc:
                first_failure_rc = rc
            if failure_policy != "continue_independent" or item.kind != "PATCH":
                return _finish_execution((first_failure_rc, executed, chosen[index + 1 :], late_duplicates, duplicate_warnings))
            safe, reason = _safe_to_continue_after_failure(detail)
            detail["continue_decision"] = {"allowed": safe, "reason": reason}
            if not safe:
                print(f"[PTV v{VERSION} SAFETY STOP] continue-on-failure blocked: {_safe_display(reason)}", file=sys.stderr)
                return _finish_execution((first_failure_rc, executed, chosen[index + 1 :], late_duplicates, duplicate_warnings))
            print(f"[PTV v{VERSION}] CONTINUE AFTER FAILURE: {_safe_display(item.name)} | {_safe_display(reason)}")

    return _finish_execution((first_failure_rc, executed, [], late_duplicates, duplicate_warnings))


def _recovery_row_queue_name(row: dict[str, object]) -> str | None:
    """Return the queue filename that represents the exact failed/replay package.

    Batch rollback may have to requeue the immutable snapshot under RETRY-* when
    the original filename was concurrently occupied. Recovery must follow that
    published identity instead of blindly reusing the historical filename.
    """
    replay = row.get("requeued_as")
    if isinstance(replay, str) and replay.strip():
        return replay.strip()
    name = row.get("name")
    return name.strip() if isinstance(name, str) and name.strip() else None


def _recovery_row_expected_sha(row: dict[str, object]) -> str | None:
    result = row.get("patch_result") if isinstance(row.get("patch_result"), dict) else None
    value = result.get("patch_sha256") if isinstance(result, dict) else None
    if isinstance(value, str) and re.fullmatch(r"[0-9a-fA-F]{64}", value):
        return value.lower()
    return None


def _bind_recovery_queue_row(root: Path, row: dict[str, object]) -> dict[str, object] | None:
    """Bind one previous-run row to the exact current queue package, if present.

    A same-name replacement must never be retried/deleted as though it were the
    failed package from the previous run. Reports from older versions may not
    carry a SHA; those retain filename-only compatibility.
    """
    queue_name = _recovery_row_queue_name(row)
    if not queue_name or Path(queue_name).name != queue_name:
        return None
    patch_root = root / "patchs"
    path = patch_root / queue_name
    try:
        patch_root_real = patch_root.resolve(strict=True)
        if path.is_symlink() or not path.is_file() or path.parent.resolve(strict=True) != patch_root_real:
            return None
        expected_sha = _recovery_row_expected_sha(row)
        if expected_sha is not None and stable_package_sha256(path) != expected_sha:
            return None
    except Exception:
        return None
    bound = dict(row)
    bound["_recovery_queue_name"] = queue_name
    return bound


def _previous_replay_identities(previous: dict[str, object] | None) -> dict[str, str]:
    """Exact queue filename -> SHA allowed to replay despite PASS history."""
    if not isinstance(previous, dict) or previous.get("status") != "FAIL":
        return {}
    out: dict[str, str] = {}
    rows = previous.get("results") if isinstance(previous.get("results"), list) else []
    for row in rows:
        if not isinstance(row, dict) or row.get("batch_rolled_back") is not True:
            continue
        if str(row.get("status") or "") != "PASS":
            continue
        queue_name = _recovery_row_queue_name(row)
        sha = _recovery_row_expected_sha(row)
        if queue_name and Path(queue_name).name == queue_name and sha:
            out[queue_name] = sha
    return out


def _unresolved_replay_identities(root: Path) -> dict[str, str]:
    """Protect exact replay packages belonging to any unresolved failed run.

    An unrelated PASS or an IDLE invocation may replace LAST_RUN, but must not
    make a rollback replay look like an ordinary local-history duplicate.
    """
    registry = _load_unresolved_registry(root)
    run_ids: list[str] = []
    for entry in registry.get("entries") or []:
        if not isinstance(entry, dict) or entry.get("resolved") is True:
            continue
        rid = entry.get("last_run_id") or entry.get("first_run_id")
        if isinstance(rid, str) and rid and rid not in run_ids:
            run_ids.append(rid)
    out: dict[str, str] = {}
    for rid in run_ids:
        found = _find_history_entry(root, rid)
        if found is not None:
            out.update(_previous_replay_identities(found[1]))
    return out


def _failed_recovery_rows(previous: dict[str, object] | None) -> list[dict[str, object]]:
    if not isinstance(previous, dict) or previous.get("status") != "FAIL":
        return []
    out: list[dict[str, object]] = []
    seen: set[str] = set()
    for row in _report_rows(previous):
        name = row.get("name")
        if not isinstance(name, str) or not name or name in seen:
            continue
        if str(row.get("kind") or "PATCH") != "PATCH":
            continue
        if str(row.get("status") or "") not in {"FAIL", "PREFLIGHT_FAIL"}:
            continue
        seen.add(name)
        out.append(row)
    return out


def _resume_groups(previous: dict[str, object] | None) -> dict[str, list[str]]:
    groups = {"replay": [], "failed": [], "remaining": []}
    if not isinstance(previous, dict) or previous.get("status") != "FAIL":
        return groups
    failed_names = {str(row.get("name")) for row in _failed_recovery_rows(previous)}
    for row in _report_rows(previous):
        name = row.get("name")
        if not isinstance(name, str):
            continue
        status = str(row.get("status") or "")
        queue_name = _recovery_row_queue_name(row) or name
        if row.get("batch_rolled_back") is True and status == "PASS":
            groups["replay"].append(queue_name)
        elif name in failed_names:
            groups["failed"].append(queue_name)
        elif status in {"BLOCKED", "NOT_EXECUTED"}:
            groups["remaining"].append(name)
    return groups


def _failure_row_diagnosis(row: dict[str, object]) -> str:
    result = row.get("patch_result") if isinstance(row.get("patch_result"), dict) else None
    diagnosis = result.get("diagnosis") if isinstance(result, dict) and isinstance(result.get("diagnosis"), dict) else None
    if diagnosis is None and isinstance(row.get("diagnosis"), dict):
        diagnosis = row.get("diagnosis")
    kind = str(diagnosis.get("kind") or "unknown") if isinstance(diagnosis, dict) else "unknown"
    return f"{row.get('status','FAIL')} | {kind}"


def _render_choice_frame(
    title: str,
    subtitle: str,
    options: list[dict[str, str]],
    cursor: int,
    previous_rows: int,
) -> int:
    """Render an arrow-key single-choice menu with the selected description below it."""
    terminal_width, terminal_height = _selector_term_size()
    frame_budget = max(1, terminal_height - 1)
    cursor = max(0, min(cursor, len(options) - 1))
    description = options[cursor].get("description", "") if options else ""
    desc_width = max(18, terminal_width - 6)
    desc_lines = textwrap.wrap(_safe_display(description), width=desc_width) or [""]
    desc_lines = desc_lines[:3]
    fixed = 2 + 1 + len(desc_lines) + 1
    item_capacity = max(1, frame_budget - fixed)
    start, end = _selector_viewport(len(options), cursor, item_capacity)
    lines: list[tuple[str, bool]] = [(title, False), (subtitle, False)]
    for i in range(start, end):
        option = options[i]
        lines.append((f"{'›' if i == cursor else ' '} {option['label']}", i == cursor))
    lines.append(("MÔ TẢ MỤC ĐANG CHỌN:", False))
    lines.extend((f"  {line}", False) for line in desc_lines)
    lines.append(("↑/↓: di chuyển | Enter: chọn | q/Esc: mở queue bình thường", False))
    lines = [(_clip_selector_line(text, terminal_width), current) for text, current in lines[:frame_budget]]
    frame_height = max(previous_rows, len(lines))
    cursor_up = min(previous_rows, max(0, frame_budget))
    if cursor_up:
        sys.stdout.write(f"\x1b[{cursor_up}F")
    padded = lines + [("", False)] * (max(len(lines), min(frame_height, frame_budget)) - len(lines))
    use_emphasis = bool(getattr(sys.stdout, "isatty", lambda: False)())
    for line, current in padded:
        rendered = f"\x1b[1;7m{line}\x1b[0m" if current and use_emphasis else line
        sys.stdout.write("\r\x1b[2K" + rendered + "\n")
    sys.stdout.flush()
    return len(padded)


def _interactive_choice_menu(title: str, subtitle: str, options: list[dict[str, str]]) -> str | None:
    if not options:
        return None
    use_posix_tty = (
        os.name != "nt" and termios is not None and tty is not None
        and sys.stdin.isatty() and sys.stdout.isatty()
    )
    use_windows_tty = (
        os.name == "nt" and msvcrt is not None
        and sys.stdin.isatty() and sys.stdout.isatty() and _enable_windows_vt()
    )
    if not (use_posix_tty or use_windows_tty):
        return None
    fd = sys.stdin.fileno()
    old = termios.tcgetattr(fd) if use_posix_tty else None
    if use_posix_tty:
        tty.setcbreak(fd)
    cursor = 0
    rendered = 0
    try:
        while True:
            rendered = _render_choice_frame(title, subtitle, options, cursor, rendered)
            key = _read_key(fd) if use_posix_tty else _read_key_windows()
            if key == "\x03":
                raise KeyboardInterrupt
            if key == "UP":
                cursor = (cursor - 1) % len(options)
            elif key == "DOWN":
                cursor = (cursor + 1) % len(options)
            elif key == "ENTER":
                return options[cursor]["key"]
            elif key in {"q", "ESC"}:
                return "normal"
    finally:
        if use_posix_tty:
            try:
                termios.tcsetattr(fd, termios.TCSADRAIN, old)
            except Exception:
                pass
        sys.stdout.write("\r\x1b[2K")
        sys.stdout.flush()


def _render_failed_patch_selector(
    rows: list[dict[str, object]], cursor: int, selected: set[int], purpose: str, previous_rows: int
) -> int:
    terminal_width, terminal_height = _selector_term_size()
    frame_budget = max(1, terminal_height - 1)
    current = rows[cursor] if rows else {}
    desc = f"{_failure_row_diagnosis(current)} | {_safe_display(str(current.get('name') or 'unknown'))}"
    footer = [
        "",
        f"Đang chọn: {len(selected)}/{len(rows)} | {purpose}",
        f"MÔ TẢ: {desc}",
        "Space: chọn/bỏ | a: tất cả | n: bỏ tất cả | ↑/↓: di chuyển | Enter: xác nhận | q/Esc: hủy",
    ]
    header = [f"CHỌN PATCH LỖI — {purpose}"]
    item_capacity = max(1, frame_budget - len(header) - len(footer))
    start, end = _selector_viewport(len(rows), cursor, item_capacity)
    lines: list[tuple[str, bool]] = [(x, False) for x in header]
    for i in range(start, end):
        row = rows[i]
        mark = "x" if i in selected else " "
        lines.append((
            f"{'›' if i == cursor else ' '} [{mark}] {i+1:>3}. [FAILED PATCH] {_safe_display(str(row.get('name') or 'unknown'))}",
            i == cursor,
        ))
    lines.extend((x, False) for x in footer)
    lines = [(_clip_selector_line(text, terminal_width), cur) for text, cur in lines[:frame_budget]]
    frame_height = max(previous_rows, len(lines))
    if previous_rows:
        sys.stdout.write(f"\x1b[{min(previous_rows, frame_budget)}F")
    padded = lines + [("", False)] * (max(len(lines), min(frame_height, frame_budget)) - len(lines))
    use_emphasis = bool(getattr(sys.stdout, "isatty", lambda: False)())
    for text, cur in padded:
        rendered = f"\x1b[1;7m{text}\x1b[0m" if cur and use_emphasis else text
        sys.stdout.write("\r\x1b[2K" + rendered + "\n")
    sys.stdout.flush()
    return len(padded)


def _select_failed_rows(rows: list[dict[str, object]], *, purpose: str) -> list[dict[str, object]] | None:
    if not rows:
        return []
    if len(rows) == 1:
        return list(rows)
    use_posix_tty = (
        os.name != "nt" and termios is not None and tty is not None
        and sys.stdin.isatty() and sys.stdout.isatty()
    )
    use_windows_tty = (
        os.name == "nt" and msvcrt is not None
        and sys.stdin.isatty() and sys.stdout.isatty() and _enable_windows_vt()
    )
    if not (use_posix_tty or use_windows_tty):
        return None
    fd = sys.stdin.fileno()
    old = termios.tcgetattr(fd) if use_posix_tty else None
    if use_posix_tty:
        tty.setcbreak(fd)
    cursor = 0
    selected: set[int] = set()
    rendered = 0
    try:
        while True:
            rendered = _render_failed_patch_selector(rows, cursor, selected, purpose, rendered)
            key = _read_key(fd) if use_posix_tty else _read_key_windows()
            if key == "\x03":
                raise KeyboardInterrupt
            if key == "UP":
                cursor = (cursor - 1) % len(rows)
            elif key == "DOWN":
                cursor = (cursor + 1) % len(rows)
            elif key == "SPACE":
                if cursor in selected:
                    selected.remove(cursor)
                else:
                    selected.add(cursor)
            elif key == "a":
                selected = set(range(len(rows)))
            elif key == "n":
                selected.clear()
            elif key == "ENTER":
                if selected:
                    return [rows[i] for i in sorted(selected)]
            elif key in {"q", "ESC"}:
                return None
    finally:
        if use_posix_tty:
            try:
                termios.tcsetattr(fd, termios.TCSADRAIN, old)
            except Exception:
                pass
        sys.stdout.write("\r\x1b[2K")
        sys.stdout.flush()


def _queued_failed_rows(root: Path, rows: list[dict[str, object]]) -> list[dict[str, object]]:
    out: list[dict[str, object]] = []
    for row in rows:
        bound = _bind_recovery_queue_row(root, row)
        if bound is not None:
            out.append(bound)
    return out


def _existing_recovery_request(root: Path, row: dict[str, object]) -> Path | None:
    raw = row.get("recovery_collect_request")
    if not isinstance(raw, str) or not raw:
        return None
    rel = raw.replace("\\", "/")
    path = root / rel if rel.startswith("patchs/") else root / "patchs" / rel
    try:
        if path.is_file() and not path.is_symlink() and path.parent.resolve(strict=True) == (root / "patchs").resolve(strict=True):
            ok, _detail = inspect_collect_zip(path)
            return path if ok else None
    except OSError:
        return None
    return None


def _failed_row_handoff_paths(root: Path, row: dict[str, object]) -> list[str]:
    raw = row.get("fail_handoff")
    if not isinstance(raw, str) or not raw:
        return []
    path = root / raw
    out: list[str] = []
    try:
        if path.is_symlink() or not path.is_file():
            return []
        with zipfile.ZipFile(path) as zf:
            data = json.loads(zf.read("SOURCE_DISCOVERY.json").decode("utf-8"))
        for item in data.get("included_files") or []:
            rel = item.get("path") if isinstance(item, dict) else None
            if isinstance(rel, str) and rel not in out and _safe_handoff_source(root, rel) is not None:
                out.append(rel)
    except Exception:
        return []
    return out


def _publish_failed_patch_collect_request(root: Path, row: dict[str, object]) -> Path | None:
    existing = _existing_recovery_request(root, row)
    if existing is not None:
        return existing
    name = str(row.get("name") or "")
    if not name:
        return None
    queue_name = str(row.get("_recovery_queue_name") or _recovery_row_queue_name(row) or name)
    item = QueueItem(queue_name, "PATCH")
    patch_result = row.get("patch_result") if isinstance(row.get("patch_result"), dict) else None
    if patch_result is None:
        patch_path = root / "patchs" / queue_name
        patch_sha = None
        try:
            if patch_path.is_file() and not patch_path.is_symlink():
                patch_sha = _sha256_file(patch_path)
        except OSError:
            pass
        patch_result = {
            "patch_file": name,
            "patch_sha256": patch_sha,
            "diagnosis": row.get("diagnosis") if isinstance(row.get("diagnosis"), dict) else {"kind": "previous_run_failure", "affected_paths": []},
            "partial_modification": {"detected": None, "changed_paths": []},
        }
    detail_path = None
    for key in ("log_path", "preflight_log_path"):
        rel = row.get(key)
        if isinstance(rel, str):
            candidate = root / rel
            try:
                if candidate.is_file() and not candidate.is_symlink():
                    detail_path = candidate
                    break
            except OSError:
                pass
    evidence, _raw, _meta = _read_handoff_detail_log(detail_path, "")
    attachments, _discovery = _discover_fail_handoff_sources(root, item, patch_result, evidence)
    paths: list[str] = [rel for rel, _src in attachments]
    for rel in _failed_row_handoff_paths(root, row):
        if rel not in paths:
            paths.append(rel)
    if not paths:
        try:
            meta = load_patch_meta(root, queue_name)
            for rel in meta.effective_targets:
                if _safe_handoff_source(root, rel) is not None and rel not in paths:
                    paths.append(rel)
        except Exception:
            pass
    if not paths:
        return None
    seed = f"{name}\n" + "\n".join(paths)
    digest = hashlib.sha256(seed.encode("utf-8", errors="replace")).hexdigest()[:12]
    request_name = f"CODE_COLLECTION_REQUEST_failed_patch_{_safe_slug(name,40)}_{digest}.zip"
    target = root / "patchs" / request_name
    inner = request_name[:-4] + ".json"
    request = {
        "id": f"failed-patch-{digest}",
        "title": f"Current source related to failed PATCH {name}",
        "actions": [{"type": "pack", "paths": paths}],
    }

    def same_existing() -> bool:
        try:
            if target.is_symlink() or not target.is_file():
                return False
            with zipfile.ZipFile(target) as zf:
                return json.loads(zf.read(inner).decode("utf-8")) == request
        except Exception:
            return False

    if target.exists() or target.is_symlink():
        return target if same_existing() else None
    fd, temp_name = tempfile.mkstemp(prefix=".ptv-failed-collect-", suffix=".zip", dir=root / "patchs")
    os.close(fd)
    temp = Path(temp_name)
    try:
        with zipfile.ZipFile(temp, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            zf.writestr(inner, json.dumps(request, ensure_ascii=False, indent=2) + "\n")
        with zipfile.ZipFile(temp) as zf:
            if zf.testzip() is not None:
                raise ValueError("generated failed-PATCH COLLECT request failed CRC")
        try:
            os.link(temp, target)
        except FileExistsError:
            return target if same_existing() else None
        except OSError:
            fd2 = None
            try:
                fd2 = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(fd2, "wb") as out_fh, temp.open("rb") as in_fh:
                    fd2 = None
                    shutil.copyfileobj(in_fh, out_fh, length=1024 * 1024)
                    out_fh.flush()
                    try:
                        os.fsync(out_fh.fileno())
                    except OSError:
                        pass
            finally:
                if fd2 is not None:
                    try:
                        os.close(fd2)
                    except OSError:
                        pass
        return target if same_existing() else None
    except Exception as exc:
        print(f"[PTV v{VERSION} WARNING] could not create failed-PATCH COLLECT request for {_safe_display(name)}: {type(exc).__name__}: {exc}", file=sys.stderr)
        return None
    finally:
        try:
            temp.unlink()
        except OSError:
            pass


def _retire_failed_rows(root: Path, rows: list[dict[str, object]]) -> tuple[list[dict[str, str]], list[str]]:
    candidates: list[LocalDuplicate] = []
    for row in rows:
        name = row.get("_recovery_queue_name") or _recovery_row_queue_name(row)
        if not isinstance(name, str):
            continue
        bound = _bind_recovery_queue_row(root, row)
        if bound is None:
            continue
        name = str(bound["_recovery_queue_name"])
        src = root / "patchs" / name
        try:
            if src.is_file() and not src.is_symlink():
                candidates.append(LocalDuplicate(QueueItem(name, "PATCH"), "failed-recovery", _sha256_file(src)))
        except OSError:
            continue
        req = _existing_recovery_request(root, row)
        if req is not None:
            try:
                candidates.append(LocalDuplicate(QueueItem(req.name, "COLLECT"), "failed-recovery", _sha256_file(req)))
            except OSError:
                pass
    moved, warnings = _move_local_duplicates_to_ignore(root, candidates)
    retired = [{"source": x.item.name, "ignore_name": x.ignored_name} for x in moved if x.ignored_name]
    return retired, warnings


def _execute_failed_patch_collects(root: Path, rows: list[dict[str, object]]):
    global _LAST_EXECUTION_DETAILS
    requests: list[QueueItem] = []
    setup_failures: list[dict[str, object]] = []
    for row in rows:
        req = _publish_failed_patch_collect_request(root, row)
        if req is None:
            setup_failures.append({
                "name": str(row.get("name") or "unknown"), "kind": "COLLECT", "status": "FAIL", "rc": 2,
                "diagnosis": {"kind": "failed_patch_collect_unavailable", "message": "no safe current source path could be prepared for COLLECT"},
            })
        else:
            requests.append(QueueItem(req.name, "COLLECT", f"failed PATCH: {row.get('name','unknown')}"))
    combined_details = list(setup_failures)
    executed: list[tuple[str, int]] = []
    first_rc = 2 if setup_failures else 0
    for request in requests:
        rc, part_executed, _remaining, _dups, _warnings = execute_items(root, [request], failure_policy="fail_fast")
        combined_details.extend(_LAST_EXECUTION_DETAILS)
        executed.extend(part_executed)
        if rc and not first_rc:
            first_rc = rc
    _LAST_EXECUTION_DETAILS = combined_details
    return first_rc, requests, executed


def _resume_selection(root: Path, items: list[QueueItem], previous: dict[str, object] | None, *, mode: str | None = None, show_history: bool = False):
    groups = _resume_groups(previous)
    by_name = {x.name: x for x in items}
    def available(names):
        return [by_name[n] for n in names if n in by_name]
    failed_rows = _merged_failed_recovery_rows(root, previous)
    queued_failed_rows = _queued_failed_rows(root, failed_rows)
    failed_queue_names = [str(row.get("_recovery_queue_name") or "") for row in queued_failed_rows]
    ordered_names: list[str] = []
    for name in groups["replay"] + failed_queue_names + groups["remaining"]:
        if name and name not in ordered_names:
            ordered_names.append(name)
    all_unresolved = available(ordered_names)
    failed_only = available(failed_queue_names)
    remaining_only = available(groups["remaining"])
    if mode in {"all", "failed", "remaining"}:
        chosen = {"all": all_unresolved, "failed": failed_only, "remaining": remaining_only}[mode]
        return {"action": "run", "items": chosen} if chosen else None
    if not (sys.stdin.isatty() and sys.stdout.isatty()):
        return None
    if not (all_unresolved or failed_rows):
        return None
    options = [
        {
            "key": "all",
            "label": "Retry/replay toàn bộ phần chưa hoàn tất",
            "description": "Chạy lại các PATCH đã rollback, PATCH bị lỗi và các item còn BLOCKED/NOT_EXECUTED theo thứ tự hợp lệ.",
        },
        {
            "key": "failed",
            "label": "Retry PATCH lỗi",
            "description": "Chỉ chạy lại PATCH đã FAIL/PREFLIGHT_FAIL. Nếu có nhiều PATCH lỗi, bạn có thể chọn nhiều bằng Space.",
        },
        {
            "key": "remaining",
            "label": "Chạy phần còn lại / đang bị BLOCKED",
            "description": "Không retry PATCH lỗi; chỉ xét các item chưa chạy. Dependency và predecessor rule vẫn được kiểm tra trước khi thực thi.",
        },
        {
            "key": "collect_failed",
            "label": "COLLECT source của PATCH lỗi",
            "description": "Tự xác định source hiện tại liên quan đến PATCH lỗi và chạy CODE_COLLECTION_REQUEST. Có thể chọn nhiều PATCH; COLLECT được chạy tuần tự từng request.",
        },
        {
            "key": "delete_failed",
            "label": "Xóa PATCH lỗi khỏi hàng đợi",
            "description": "Loại PATCH lỗi khỏi patchs/ bằng cách chuyển an toàn vào patchs/ignore. Nếu có nhiều PATCH lỗi, bạn có thể chọn nhiều PATCH cùng lúc.",
        },
        {
            "key": "history",
            "label": "Xem lại lịch sử chạy gần đây",
            "description": "Mở lịch sử run đã lưu và xem lại kết quả, detail/aggregate log, source diff, FAIL_HANDOFF, recovery COLLECT và support ZIP.",
        },
        {
            "key": "normal",
            "label": "Bỏ qua phục hồi và mở queue bình thường",
            "description": "Không thay đổi PATCH lỗi ở bước này; quay về màn hình chọn PATCH/COLLECT thông thường.",
        },
    ]
    action = _interactive_choice_menu(
        "SMART RESUME — LẦN CHẠY CÓ CÔNG VIỆC GẦN NHẤT CÓ PATCH LỖI",
        f"Rollback replay={len(groups['replay'])} | Failed={len(failed_rows)} | Remaining/blocked={len(groups['remaining'])}",
        options,
    )
    if action in {None, "normal"}:
        return None
    if action == "history":
        return {"action": "history"}
    if action == "all":
        return {"action": "run", "items": all_unresolved} if all_unresolved else None
    if action == "remaining":
        return {"action": "run", "items": remaining_only} if remaining_only else None
    if action == "failed":
        rows = _select_failed_rows(queued_failed_rows, purpose="RETRY") if len(queued_failed_rows) > 1 else queued_failed_rows
        if rows is None:
            return None
        names = {str(row.get("_recovery_queue_name") or _recovery_row_queue_name(row) or "") for row in rows}
        selected = [item for item in failed_only if item.name in names]
        return {"action": "run", "items": selected} if selected else None
    if action == "collect_failed":
        rows = _select_failed_rows(failed_rows, purpose="COLLECT SOURCE") if len(failed_rows) > 1 else failed_rows
        return {"action": "collect_failed", "rows": rows} if rows else None
    if action == "delete_failed":
        rows = _select_failed_rows(queued_failed_rows, purpose="XÓA KHỎI QUEUE") if len(queued_failed_rows) > 1 else queued_failed_rows
        return {"action": "delete_failed", "rows": rows} if rows else None
    return None

def _recipe_project_key(root: Path) -> str | None:
    # Recipe identity is a reproducibility boundary.  An invalid local config
    # must not silently degrade into an unbound recipe.
    cfg = load_project_config(root)
    project = cfg.get("project") if isinstance(cfg, dict) else None
    value = project.get("key") if isinstance(project, dict) else None
    return str(value) if isinstance(value, str) and value else None


def _write_batch_recipe(root: Path, path: Path, chosen: list[QueueItem], metas: dict[str, PatchMeta], *, failure_policy: str, transaction_policy: str) -> None:
    packages = []
    for item in chosen:
        meta = metas.get(item.name)
        if meta is None or not meta.package_sha256:
            continue
        packages.append({"name": item.name, "sha256": meta.package_sha256, "patch_id": meta.patch_id})
    data = {
        "format":"python-patch-tool-batch-recipe", "format_version":1, "tool_version":VERSION,
        "project_key": _recipe_project_key(root),
        "failure_policy": failure_policy, "transaction_policy": transaction_policy,
        "packages": packages,
    }
    if not path.is_absolute():
        path = root / path
    path.parent.mkdir(parents=True, exist_ok=True)
    _atomic_json(path, data)
    print(f"BATCH RECIPE: {path}")


def _recipe_identity_map(root: Path, raw_path: str | None) -> dict[str,str]:
    if raw_path is None:
        return {}
    path = Path(raw_path)
    if not path.is_absolute(): path = root / path
    try:
        if path.is_symlink() or not path.is_file(): return {}
        data = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
        rows = data.get("packages") if isinstance(data,dict) else None
        out: dict[str,str] = {}
        for row in rows or []:
            if not isinstance(row,dict): continue
            name=row.get("name"); sha=row.get("sha256")
            if isinstance(name,str) and Path(name).name==name and isinstance(sha,str) and re.fullmatch(r"[0-9a-fA-F]{64}",sha):
                out[name]=sha.lower()
        return out
    except Exception:
        return {}


def _load_batch_recipe(root: Path, path: Path, items: list[QueueItem]) -> tuple[list[QueueItem], str | None, str | None]:
    if not path.is_absolute():
        path = root / path
    if path.is_symlink() or not path.is_file():
        raise BatchPlanError(f"batch recipe is missing/unsafe: {path}", kind="recipe_invalid")
    try:
        data = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
    except Exception as exc:
        raise BatchPlanError(f"cannot parse batch recipe: {type(exc).__name__}: {exc}", kind="recipe_invalid") from exc
    if not isinstance(data, dict) or data.get("format") != "python-patch-tool-batch-recipe" or data.get("format_version") != 1:
        raise BatchPlanError("unsupported batch recipe format", kind="recipe_invalid")
    expected_key = data.get("project_key")
    actual_key = _recipe_project_key(root)
    if expected_key is not None and expected_key != actual_key:
        raise BatchPlanError(f"batch recipe project mismatch: recipe={expected_key!r} local={actual_key!r}", kind="project_mismatch")
    rows = data.get("packages")
    if not isinstance(rows, list) or not rows:
        raise BatchPlanError("batch recipe packages[] is empty", kind="recipe_invalid")
    by_name = {x.name:x for x in items if x.kind == "PATCH"}
    chosen: list[QueueItem] = []
    for row in rows:
        if not isinstance(row, dict):
            raise BatchPlanError("batch recipe package entry must be an object", kind="recipe_invalid")
        name = row.get("name"); sha = row.get("sha256"); recipe_patch_id = row.get("patch_id")
        if (
            not isinstance(name, str) or Path(name).name != name
            or not isinstance(sha, str) or not re.fullmatch(r"[0-9a-fA-F]{64}", sha)
            or not isinstance(recipe_patch_id, str) or not recipe_patch_id
        ):
            raise BatchPlanError("batch recipe contains invalid package identity", kind="recipe_invalid")
        item = by_name.get(name)
        if item is None:
            raise BatchPlanError(f"batch recipe package is not present in patchs/: {name}", kind="recipe_package_missing")
        actual = stable_package_sha256(root/"patchs"/name)
        if actual.lower() != sha.lower():
            raise BatchPlanError(f"batch recipe SHA mismatch for {name}: expected={sha.lower()} actual={actual}", kind="package_input_changed")
        try:
            actual_meta = load_patch_meta(root, item.name)
        except BatchPlanError:
            raise
        except Exception as exc:
            raise BatchPlanError(f"cannot read batch recipe package metadata for {name}: {type(exc).__name__}: {exc}", kind="recipe_invalid") from exc
        if actual_meta.patch_id != recipe_patch_id:
            raise BatchPlanError(
                f"batch recipe patch_id mismatch for {name}: expected={recipe_patch_id!r} actual={actual_meta.patch_id!r}",
                kind="recipe_invalid",
            )
        chosen.append(item)
    failure = data.get("failure_policy")
    transaction = data.get("transaction_policy")
    if failure not in {None,"fail_fast","continue_independent"} or transaction not in {None,"patch","batch"}:
        raise BatchPlanError("batch recipe contains invalid policy", kind="recipe_invalid")
    return chosen, failure, transaction


def _plan_queue(
    root: Path, *, export_recipe: str | None = None,
    failure_policy_override: str | None = None, transaction_policy_override: str | None = None,
) -> int:
    """Read-only batch plan over all queued PATCHes; COLLECT requests are listed but not planned."""
    previous = _load_previous_run(root)
    cfg, config_warnings = _load_zero_argument_config(root)
    failure_policy = failure_policy_override or str(cfg.get("failure_policy") or "continue_independent")
    transaction_policy = transaction_policy_override or str(cfg.get("transaction_policy") or "patch")
    for warning in config_warnings:
        print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")
    try:
        items, warnings = discover_queue(root)
    except QueueSafetyError as exc:
        print(f"PLAN FAIL — queue safety: {_safe_display(str(exc))}", file=sys.stderr)
        return 2
    for warning in warnings:
        print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")
    chosen = [x for x in items if x.kind == "PATCH"]
    if not chosen:
        print("PLAN: no PATCH package is waiting in patchs/.")
        return 0
    try:
        chosen, metas, previous_action = _build_batch_plan(root, chosen, items, _planning_previous(root, previous))
    except Exception as exc:
        print(f"PLAN FAIL — {getattr(exc,'kind','batch_plan_invalid')}: {_safe_display(str(exc))}", file=sys.stderr)
        return 2
    ordered_metas = [metas[x.name] for x in chosen if x.name in metas]
    tx_issues = transaction_compatibility(ordered_metas, transaction_policy)
    if tx_issues:
        print(f"BATCH PLAN FAIL — transaction_policy={transaction_policy} is incompatible:", file=sys.stderr)
        for issue in tx_issues:
            print(f"  - {_safe_display(issue)}", file=sys.stderr)
        return 2
    conflicts = analyze_static_conflicts(ordered_metas)
    print("BATCH PLAN — READ ONLY")
    print(f"BATCH POLICY: failure={failure_policy} | transaction={transaction_policy}")
    for i,item in enumerate(chosen,1):
        meta = metas[item.name]
        deps = ",".join(meta.depends_on) if meta.depends_on else "-"
        print(f"  {i}. {_safe_display(item.name)} | id={_safe_display(meta.patch_id)} | sha={str(meta.package_sha256)[:12]} | targets={len(meta.effective_targets)} | depends_on={deps}")
        old = ledger_id_reuse(root, meta.patch_id, str(meta.package_sha256 or "")) if meta.package_sha256 else []
        if old:
            print(f"     WARNING PATCH ID REUSE: {len(old)} prior SHA variant(s)")
    if previous_action:
        print(f"PREVIOUS FAILURE ACTION: {previous_action.get('action')} | {previous_action.get('reason','')}")
    if conflicts:
        print("STATIC CONFLICTS:")
        for row in conflicts:
            label = "ORDER-DEPENDENT" if row.get("relation") == "order_dependent_overlap" else "DEPENDENCY-ORDERED"
            print(f"  [{label}] {row.get('left')} <-> {row.get('right')} | {', '.join(row.get('overlap') or [])}")
    else:
        print("STATIC CONFLICTS: none from declared/effective targets")
    targets = sorted({rel for meta in ordered_metas for rel in meta.effective_targets})
    resources = disk_preflight(root,[root/"patchs"/x.name for x in chosen],targets)
    print(f"RESOURCE PREFLIGHT: {resources.get('status')} | project_free={resources.get('actual_project_free_bytes')} required={resources.get('required_project_free_bytes')} | temp_free={resources.get('actual_temp_free_bytes')} required={resources.get('required_temp_free_bytes')}")
    # Do not enter mirror preview when the resource gate already proves the plan
    # cannot be executed safely.  In particular, copying very large/sparse
    # targets into the preview mirror could itself consume the space the gate
    # is intended to protect.
    if resources.get("status") != "PASS":
        return 2
    preview_rc = 0
    for item in chosen:
        print(f"\n--- PREVIEW {item.name} ---")
        rc = _preview_item(root,item)
        if rc and not preview_rc: preview_rc = rc
    if export_recipe is not None:
        try:
            _write_batch_recipe(root, Path(export_recipe), chosen, metas, failure_policy=failure_policy, transaction_policy=transaction_policy)
        except Exception as exc:
            print(f"PLAN FAIL — recipe export: {type(exc).__name__}: {exc}",file=sys.stderr)
            return 2
    if resources.get("status") != "PASS":
        return 2
    return preview_rc


def _run_queue(
    root: Path, *, failure_policy_override: str | None = None, transaction_policy_override: str | None = None,
    force_resume: bool = False, resume_mode: str | None = None, recipe_path: str | None = None,
    zero_argument_invocation: bool = False, explicit_patch_specs: list[str] | None = None,
    select_all: bool = False, select_spec: str | None = None, no_validation: bool = False,
):
    global _ACTIVE_RUN_ID, _LAST_EXECUTION_DETAILS
    started_mono = time.monotonic()
    started_at = _utc_now()
    run_id = f"{int(time.time()*1000000)}_{os.getpid()}"
    _ACTIVE_RUN_ID = run_id
    queue_safety_error: str | None = None
    recipe_chosen: list[QueueItem] | None = None
    recipe_failure: str | None = None
    recipe_transaction: str | None = None
    recipe_error: Exception | None = None
    try:
        items, warnings = discover_queue(root)
    except QueueSafetyError as exc:
        items, warnings = [], []
        queue_safety_error = str(exc)

    # A zero-argument invocation with no runnable PATCH/COLLECT is not a run.
    # Exit before touching LAST_RUN/history/registry/run artifacts.  Discovery
    # warnings (for example a handoff ZIP with no patch signature) remain visible
    # so the operator still understands why the queue is empty.  Preserve the
    # v6.17.12 operator workflow: on an interactive terminal, an empty queue
    # opens the existing HISTORY browser after status/health.  Reviewing history
    # must not manufacture an IDLE run or overwrite LAST_RUN.
    if zero_argument_invocation and recipe_path is None and queue_safety_error is None and not items:
        for warning in warnings:
            print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")
        print("AUTO STATUS: IDLE — no runnable patch/collect package is waiting in patchs/.")
        print_health(root, compact=True)
        # HISTORY is the zero-work landing page even when a task runner is
        # non-TTY.  In non-interactive mode the browser prints the history list
        # and exits cleanly on EOF; in a terminal it remains interactive.
        _zero_work_history_landing(root)
        _ACTIVE_RUN_ID = None
        _LAST_EXECUTION_DETAILS = []
        return 0

    previous = _load_previous_run(root)
    # Automatic resume is about the immediately recorded real run.  Do not
    # fall back to arbitrary older history here; persistent unresolved failures
    # are enforced separately by the dependency planner.
    meaningful_previous = previous if _is_meaningful_run(previous) else None
    history_replay_sha = _unresolved_replay_identities(root)
    history_replay_sha.update(_previous_replay_identities(meaningful_previous))
    resume_items = _resume_items(root, meaningful_previous)
    if force_resume:
        _print_resume_hint(root, meaningful_previous)

    # Recipe identity is a pre-mutation contract. Parse/project-bind/SHA-check it
    # before duplicate-history cleanup can move any queue package to ignore.
    # A malformed/tampered recipe must fail with the queue byte-for-byte intact.
    if recipe_path is not None and queue_safety_error is None:
        try:
            if failure_policy_override is not None or transaction_policy_override is not None:
                raise BatchPlanError(
                    "batch recipe owns failure_policy/transaction_policy; CLI policy overrides are not allowed with --recipe (create a new recipe with plan overrides instead)",
                    kind="recipe_invalid",
                )
            recipe_chosen, recipe_failure, recipe_transaction = _load_batch_recipe(root, Path(recipe_path), items)
            for item in recipe_chosen:
                try:
                    history_replay_sha[item.name] = stable_package_sha256(root / "patchs" / item.name).lower()
                except Exception:
                    pass
        except Exception as exc:
            recipe_error = exc

    session_duplicates: list[SessionDuplicate] = []
    local_duplicates: list[LocalDuplicate] = []
    session_duplicate_warnings: list[str] = []
    duplicate_warnings: list[str] = []
    ignore_warnings: list[str] = []
    if recipe_error is None and recipe_chosen is None:
        items, session_duplicates, session_duplicate_warnings = _split_session_duplicate_patches(root, items, history_replay_sha=history_replay_sha)
        items, local_duplicates, duplicate_warnings = _split_local_duplicate_patches(root, items, history_replay_sha=history_replay_sha)
        local_duplicates, ignore_warnings = _move_local_duplicates_to_ignore(root, local_duplicates)
        duplicate_warnings = [*duplicate_warnings, *ignore_warnings]
    elif recipe_chosen is not None:
        # A recipe is an exact named/SHA-bound execution set.  Do not mutate
        # unrelated queue entries via ordinary duplicate cleanup, and do not
        # suppress recipe packages merely because identical bytes appear in
        # local history.
        session_duplicate_warnings.append("batch recipe active: duplicate-history cleanup is scoped out; only recipe packages will execute")
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
    failure_policy = failure_policy_override or str(cfg.get("failure_policy") or "continue_independent")
    transaction_policy = transaction_policy_override or str(cfg.get("transaction_policy") or "patch")
    if failure_policy not in {"fail_fast", "continue_independent"}: failure_policy = "continue_independent"
    if transaction_policy not in {"patch", "batch"}: transaction_policy = "patch"
    batch_preflight_rows: list[dict[str, object]] = []
    previous_action: dict[str, object] | None = None
    resume_action_report: dict[str, object] | None = None
    batch_transaction: dict[str, object] | None = None
    static_conflicts: list[dict[str, object]] = []
    resource_preflight_report: dict[str, object] | None = None
    # Historical v5.12 audit contract: runnable packages that the operator did
    # not select remain in patchs/ and are recorded explicitly.  Keep this as
    # report metadata only; it must never change execution or duplicate logic.
    user_not_selected_names: list[str] = []
    if recipe_path is not None and recipe_error is None:
        if failure_policy_override is None and recipe_failure:
            failure_policy = str(recipe_failure)
        if transaction_policy_override is None and recipe_transaction:
            transaction_policy = str(recipe_transaction)

    def finish_report(status: str, rc: int, *, chosen: list[QueueItem] | None = None, executed=None, remaining=None, failed_item=None):
        chosen = chosen or []; executed = executed or []; remaining = remaining or []
        report: dict[str, object] = {
            "format": "python-patch-tool-last-run", "format_version": 2, "tool_version": VERSION,
            "run_id": run_id, "started_at": started_at, "finished_at": _utc_now(),
            "elapsed_seconds": round(time.monotonic() - started_mono, 3), "status": status, "exit_code": int(rc),
            "selected": [item.name for item in chosen], "execution_order": [name for name, _ in executed],
            "user_not_selected": list(user_not_selected_names),
            "results": list(_LAST_EXECUTION_DETAILS), "not_executed": [item.name for item in remaining],
            "failed_item": failed_item, "failure_policy": failure_policy, "transaction_policy": transaction_policy,
            "batch_preflight": list(batch_preflight_rows), "previous_failure_action": previous_action,
            "resume_action": resume_action_report, "batch_transaction": batch_transaction,
            "static_conflicts": list(static_conflicts), "resource_preflight": resource_preflight_report,
            "session_duplicates_removed": [
                {"name": d.item.name, "canonical": d.canonical_name, "sha256": d.sha256, "removed": d.removed}
                for d in session_duplicates
            ],
            "local_history_skipped": [
                {"name": d.item.name, "history_name": d.history_name, "sha256": d.sha256,
                 "ignore_path": f"patchs/ignore/{d.ignored_name}" if d.ignored_name else None}
                for d in local_duplicates
            ],
            "previous_resume_items": resume_items if status != "IDLE" else [],
            "previous_failed_item": (meaningful_previous.get("failed_item") or meaningful_previous.get("previous_failed_item")) if isinstance(meaningful_previous, dict) else None,
        }
        _finalize_batch_artifacts(root, report)
        _write_run_report(root, report)
        try:
            _update_unresolved_registry(root, report)
        except Exception as exc:
            print(f"[PTV v{VERSION} WARNING] could not update unresolved-failure registry: {type(exc).__name__}: {exc}", file=sys.stderr)
        try:
            update_patch_ledger(root, report)
        except Exception as exc:
            print(f"[PTV v{VERSION} WARNING] could not update PATCH ledger: {type(exc).__name__}: {exc}", file=sys.stderr)
        if len(report.get("selected") or []) > 1:
            if sys.stdin.isatty() and sys.stdout.isatty(): _batch_report_menu(root, report)
            else: _print_batch_overview(root, report, stream=sys.stderr if status == "FAIL" else sys.stdout)
        return rc

    if recipe_error is not None:
        kind = getattr(recipe_error, "kind", "recipe_invalid")
        _LAST_EXECUTION_DETAILS = [{
            "name": Path(recipe_path).name if recipe_path else "BATCH_RECIPE.json",
            "kind": "RECIPE", "status": "PREFLIGHT_FAIL", "rc": 2,
            "diagnosis": {"kind": kind, "message": str(recipe_error)},
        }]
        print(f"BATCH RECIPE FAIL — project unchanged | {kind}: {_safe_display(str(recipe_error))}", file=sys.stderr)
        return finish_report("FAIL", 2, failed_item=Path(recipe_path).name if recipe_path else "BATCH_RECIPE.json")

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
        if zero_argument_invocation:
            if session_duplicates or local_duplicates:
                print("\nQUEUE CLEANUP SUMMARY — queue ban đầu có package nhưng tất cả đã bị duplicate/auto-filter.")
                if session_duplicates:
                    print(f"  Session duplicates removed : {len(session_duplicates)}")
                if local_duplicates:
                    print(f"  Local-history duplicates   : {len(local_duplicates)} (đã chuyển vào patchs/ignore khi có thể)")
            _zero_work_history_landing(root)
        # Zero-work invocations are not runs: do not create a run directory,
        # LAST_RUN/history entry, ledger update or unresolved-failure update.
        # Old history/registry remain intact for explicit review and planner
        # safety, but nothing from this invocation is persisted as a run.
        _ACTIVE_RUN_ID = None
        _LAST_EXECUTION_DETAILS = []
        return 0

    chosen = list(recipe_chosen) if recipe_chosen is not None else None
    explicit_selection_requested = bool(explicit_patch_specs) or bool(select_all) or select_spec is not None
    if chosen is None and explicit_selection_requested:
        try:
            chosen = _resolve_explicit_selection(
                root, items, patch_specs=explicit_patch_specs, select_all=select_all, select_spec=select_spec,
            )
            print("EXPLICIT SELECTION: " + ", ".join(item.name for item in chosen))
        except Exception as exc:
            _LAST_EXECUTION_DETAILS = [{
                "name": "CLI_SELECTION", "kind": "SELECTION", "status": "PREFLIGHT_FAIL", "rc": 2,
                "diagnosis": {"kind": "selection_invalid", "message": str(exc)},
            }]
            print(f"SELECTION FAIL — project unchanged | {_safe_display(str(exc))}", file=sys.stderr)
            return finish_report("FAIL", 2, failed_item="CLI_SELECTION")
    # v6.20.0: persistent failed grouping; recovery no longer hijacks the next ordinary zero-argument run.
    # Smart Resume remains available explicitly through the ``resume`` command;
    # ordinary queue selection shows previous failed/replay items as a second
    # visual group instead.  Planner safety for unresolved predecessors is
    # unchanged and still applies after selection.
    if chosen is None and not explicit_selection_requested and force_resume:
        while True:
            try:
                decision = _resume_selection(
                    root, items, meaningful_previous, mode=resume_mode, show_history=zero_argument_invocation,
                )
            except KeyboardInterrupt:
                print("\nCancelled by Ctrl+C.")
                return finish_report("CANCELLED", 130)
            if isinstance(decision, dict) and str(decision.get("action") or "") == "history":
                _history_browser(root)
                continue
            break
        if isinstance(decision, dict):
            action = str(decision.get("action") or "")
            if action == "run":
                chosen = list(decision.get("items") or [])
                if chosen:
                    print(f"SMART RESUME SELECTED: {len(chosen)} item(s)")
            elif action == "collect_failed":
                rows = list(decision.get("rows") or [])
                failed_names = [str(row.get("name") or "unknown") for row in rows]
                rc, requests, executed = _execute_failed_patch_collects(root, rows)
                resume_action_report = {"action": "collect_failed", "failed_patches": failed_names, "requests": [x.name for x in requests]}
                status = "PASS" if rc == 0 else "FAIL"
                failed_item = next((str(d.get("name")) for d in _LAST_EXECUTION_DETAILS if d.get("status") == "FAIL"), None)
                return finish_report(status, rc, chosen=requests, executed=executed, failed_item=failed_item)
            elif action == "delete_failed":
                rows = list(decision.get("rows") or [])
                retired, retire_warnings = _retire_failed_rows(root, rows)
                for warning in retire_warnings:
                    print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")
                resume_action_report = {"action": "delete_failed", "failed_patches": [str(row.get("name") or "unknown") for row in rows], "retired": retired}
                retired_names = {str(x.get("source")) for x in retired}
                resolved_rows = []
                for selected_row in rows:
                    qn = str(selected_row.get("_recovery_queue_name") or _recovery_row_queue_name(selected_row) or "")
                    if qn in retired_names:
                        resolved_rows.append(selected_row)
                _resolve_registry_rows(root, resolved_rows, "deleted_from_queue")
                for row in retired:
                    print(f"FAILED ITEM REMOVED FROM QUEUE: patchs/{_safe_display(str(row.get('source')))} -> patchs/ignore/{_safe_display(str(row.get('ignore_name')))}")
                items = [item for item in items if item.name not in retired_names]
                if not items:
                    print("AUTO STATUS: IDLE — selected failed PATCHes were removed; no runnable item remains.")
                    print_health(root, compact=True)
                    if zero_argument_invocation and sys.stdin.isatty() and sys.stdout.isatty():
                        _history_browser(root)
                    return finish_report("IDLE", 0)
    if chosen is None:
        chosen = _configured_auto_selection(root, items, cfg)
    if chosen is None:
        failed_group_names = _last_failed_queue_names(root, items, meaningful_previous)
        selector_items = _group_selector_items(items, failed_group_names)
        try:
            chosen = select_items(
                root, selector_items, initial_selection=cfg.get("initial_selection", "none"),
                selector_ui=cfg.get("selector_ui", "auto"), show_history=zero_argument_invocation,
                failed_group_names=failed_group_names,
            )
        except KeyboardInterrupt:
            print("\nCancelled by Ctrl+C."); return finish_report("CANCELLED", 130)
    if chosen is None:
        print("Cancelled."); _print_session_duplicate_removals(session_duplicates); return finish_report("CANCELLED", 0)
    if not chosen:
        print("AUTO STATUS: IDLE — queue is empty or no runnable item remains; nothing executed.")
        _print_session_duplicate_removals(session_duplicates); return finish_report("IDLE", 0)

    if recipe_chosen is None:
        chosen_names_for_audit = {item.name for item in chosen}
        user_not_selected_names[:] = [item.name for item in items if item.name not in chosen_names_for_audit]
        if user_not_selected_names:
            print(f"USER NOT SELECTED: {len(user_not_selected_names)} runnable package(s) preserved in patchs/")

    # Resolve dependency order and enforce unresolved-predecessor handling before source changes.
    try:
        chosen, metas, previous_action = _build_batch_plan(root, chosen, items, _planning_previous(root, previous))
    except Exception as exc:
        kind = getattr(exc, "kind", "batch_plan_invalid")
        _LAST_EXECUTION_DETAILS = [{
            "name": x.name, "kind": x.kind, "status": "PREFLIGHT_FAIL" if i == 0 else "NOT_EXECUTED", "rc": 2 if i == 0 else None,
            "diagnosis": {"kind": kind, "message": str(exc)},
        } for i, x in enumerate(chosen)]
        print(f"BATCH PREFLIGHT FAIL — project unchanged | {kind}: {_safe_display(str(exc))}", file=sys.stderr)
        return finish_report("FAIL", 2, chosen=chosen, remaining=chosen, failed_item=chosen[0].name if chosen else None)

    if metas:
        ordered_metas = [metas[item.name] for item in chosen if item.name in metas]
        static_conflicts = analyze_static_conflicts(ordered_metas)
        if static_conflicts:
            print("STATIC CONFLICT ANALYSIS:")
            for row in static_conflicts:
                label = "ORDER-DEPENDENT" if row.get("relation") == "order_dependent_overlap" else "DEPENDENCY-ORDERED"
                overlap = ",".join((row.get("overlap") or [])[:4])
                more = len(row.get("overlap") or []) - 4
                if more > 0: overlap += f",...(+{more})"
                print(f"  [{label}] {_safe_display(str(row.get('left')))} <-> {_safe_display(str(row.get('right')))} | {overlap}")
        for meta in ordered_metas:
            if meta.package_sha256:
                old_rows = ledger_id_reuse(root, meta.patch_id, meta.package_sha256)
                if old_rows:
                    old_shas = ", ".join(str(x.get("sha256",""))[:12] for x in old_rows[:3])
                    print(f"[PTV v{VERSION} WARNING] PATCH ID REUSE: {meta.patch_id} has prior different SHA(s): {old_shas}")
        all_targets = sorted({rel for meta in ordered_metas for rel in meta.effective_targets})
        resource_preflight_report = disk_preflight(root, [root/"patchs"/x.name for x in chosen], all_targets)
        print(
            "RESOURCE PREFLIGHT: " + str(resource_preflight_report.get("status"))
            + f" | project_free={resource_preflight_report.get('actual_project_free_bytes')} required={resource_preflight_report.get('required_project_free_bytes')}"
            + f" | temp_free={resource_preflight_report.get('actual_temp_free_bytes')} required={resource_preflight_report.get('required_temp_free_bytes')}"
        )
        if resource_preflight_report.get("status") != "PASS":
            _LAST_EXECUTION_DETAILS = [{
                "name": x.name, "kind": x.kind, "status": "PREFLIGHT_FAIL" if i == 0 else "NOT_EXECUTED",
                "rc": 2 if i == 0 else None,
                "diagnosis": {"kind":"insufficient_disk_space","message":"resource preflight found insufficient project/temp free space"},
            } for i, x in enumerate(chosen)]
            print("BATCH PREFLIGHT FAIL — project unchanged | insufficient_disk_space", file=sys.stderr)
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
    preflight_failure_details: dict[str, dict[str, object]] = {}
    if not preflight_ok:
        failed_rows = [r for r in batch_preflight_rows if r.get("status") == "FAIL"]
        global_preflight_failure = any(not r.get("name") for r in failed_rows)
        # Batch-transaction mode is intentionally all-or-nothing. Likewise,
        # explicit fail_fast preserves the old whole-batch preflight barrier.
        # Under the default patch transaction + continue_independent policy,
        # a read-only failure belongs only to that PATCH; independent items
        # continue while dependency/target-related successors are BLOCKED by
        # execute_items using the normal relationship state machine.
        must_abort_whole_batch = (
            global_preflight_failure
            or transaction_policy == "batch"
            or failure_policy != "continue_independent"
        )
        if must_abort_whole_batch:
            _LAST_EXECUTION_DETAILS = []
            fail_names = {str(r.get("name")) for r in failed_rows if r.get("name")}
            not_executed_items: list[QueueItem] = []
            for item in chosen:
                row = next((r for r in batch_preflight_rows if r.get("name") == item.name), None)
                if item.name in fail_names:
                    detail = _materialize_batch_preflight_failure(root, item, row)
                    _LAST_EXECUTION_DETAILS.append(detail)
                else:
                    not_executed_items.append(item)
                    _LAST_EXECUTION_DETAILS.append({
                        "name": item.name, "kind": item.kind, "status": "NOT_EXECUTED", "rc": None,
                        "diagnosis": {
                            "kind": "batch_preflight_failed_elsewhere",
                            "message": "no payload executed because whole-batch preflight failed",
                        },
                        "preflight_log_path": row.get("log_path") if row else None,
                    })
            print("BATCH PREFLIGHT: FAIL — no selected PATCH modified source", file=sys.stderr)
            for row in failed_rows:
                print(f"  - {_safe_display(str(row.get('name') or 'batch'))}: {_safe_display(str(row.get('classification')))}", file=sys.stderr)
            failed = next((x.name for x in chosen if x.name in fail_names), chosen[0].name if chosen else None)
            return finish_report("FAIL", 2, chosen=chosen, remaining=not_executed_items, failed_item=failed)

        # Capture every failing PATCH's recovery artifacts before any independent
        # PATCH writes source. This preserves the exact source/log evidence that
        # caused the read-only preflight rejection while still allowing the
        # default continuation policy to do its job.
        for item in chosen:
            row = next((r for r in failed_rows if r.get("name") == item.name), None)
            if row is not None:
                preflight_failure_details[item.name] = _materialize_batch_preflight_failure(root, item, row)
        print(
            f"BATCH PREFLIGHT: PARTIAL — rejected={len(preflight_failure_details)}; "
            "independent PATCHes will continue",
            file=sys.stderr,
        )
        for row in failed_rows:
            print(
                f"  - {_safe_display(str(row.get('name') or 'batch'))}: "
                f"{_safe_display(str(row.get('classification')))}",
                file=sys.stderr,
            )
    else:
        if needs_batch_preflight:
            print("BATCH PREFLIGHT: PASS — all packages validated before first source write")
    deferred = [r for r in batch_preflight_rows if r.get("status") == "DEFERRED_AFTER_DEPENDENCY"]
    if deferred:
        print(f"  Deferred source checks after declared dependencies: {len(deferred)} (runner revalidates immediately before execution)")

    previous_action = _apply_previous_failure_action(root, previous_action)
    _resolve_registry_previous_action(root, previous_action)
    if (
        isinstance(previous_action, dict)
        and previous_action.get("action") == "delete"
        and previous_action.get("result") not in {"moved_to_ignore", "already_absent"}
    ):
        reason = str(previous_action.get("error") or previous_action.get("result") or "previous failure delete failed")
        _LAST_EXECUTION_DETAILS = [{
            "name": str(previous_action.get("queue_file") or previous_action.get("patch_file") or "previous-failure"),
            "kind": "PATCH", "status": "PREFLIGHT_FAIL", "rc": 2,
            "diagnosis": {"kind": "previous_failure_identity_changed", "message": reason},
        }]
        print(f"PREVIOUS FAILED PATCH ACTION: DELETE FAILED — {_safe_display(reason)}", file=sys.stderr)
        return finish_report("FAIL", 2, chosen=chosen, remaining=chosen, failed_item=str(previous_action.get("queue_file") or previous_action.get("patch_file") or "previous-failure"))

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
            expected_package_sha = {
                x.name: metas[x.name].package_sha256
                for x in chosen if x.kind == "PATCH" and x.name in metas and metas[x.name].package_sha256
            }
            package_map = snapshot_package_bytes(
                root, [x.name for x in chosen if x.kind == "PATCH"], package_snapshot_root,
                expected_sha256=expected_package_sha,
            )
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
        rc, executed, remaining, late_duplicates, late_duplicate_warnings = execute_items(
            root, chosen, failure_policy=failure_policy, metas=metas, history_replay_sha=history_replay_sha,
            preflight_failure_details=preflight_failure_details,
            no_validation=no_validation,
        )
        local_duplicates = [*local_duplicates, *late_duplicates]
        for warning in late_duplicate_warnings:
            if warning in printed_warnings: continue
            printed_warnings.add(warning); print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}", file=sys.stderr if rc else sys.stdout)

        if transaction_policy == "batch" and transaction_snapshot_root is not None and transaction_manifest is not None:
            if rc:
                restored = restore_targets(root, transaction_snapshot_root, transaction_manifest)
                rollback_ok = restored.get("status") == "PASS"
                requeued: dict[str, str] = {}
                requeue_error: str | None = None
                if package_snapshot_root is not None:
                    try:
                        requeued = requeue_packages(root, package_snapshot_root, package_map or {})
                    except Exception as exc:
                        requeue_error = f"{type(exc).__name__}: {exc}"
                tx_status = "ROLLED_BACK" if rollback_ok and requeue_error is None else (
                    "ROLLBACK_FAILED" if not rollback_ok else "REQUEUE_FAILED"
                )
                batch_transaction = {
                    "policy": "batch", "status": tx_status,
                    "restored_paths": restored.get("restored_paths", []),
                    "errors": restored.get("errors", []),
                    "requeued_packages": requeued,
                }
                if requeue_error is not None:
                    batch_transaction["requeue_error"] = requeue_error
                    batch_transaction["package_snapshot_dir"] = package_snapshot_root.relative_to(root).as_posix() if package_snapshot_root.is_relative_to(root) else str(package_snapshot_root)
                for detail in _LAST_EXECUTION_DETAILS:
                    if detail.get("status") in {"PASS", "FAIL"}:
                        detail["batch_rollback_attempted"] = True
                        detail["batch_rollback_status"] = "PASS" if rollback_ok else "FAIL"
                        detail["batch_rolled_back"] = rollback_ok
                        detail["batch_requeue_status"] = "FAIL" if requeue_error is not None else "PASS"
                        if detail.get("name") in requeued: detail["requeued_as"] = requeued[detail.get("name")]
                if not rollback_ok:
                    batch_transaction["original_rc"] = rc
                    rc = 70
                    print("!!! BATCH ROLLBACK FAILED — manual recovery required !!!", file=sys.stderr)
                    if requeue_error is not None:
                        print(f"!!! BATCH REQUEUE ALSO FAILED — {_safe_display(requeue_error)} !!!", file=sys.stderr)
                elif requeue_error is not None:
                    batch_transaction["original_rc"] = rc
                    rc = 71
                    print("BATCH ROLLBACK: PASS — source restored", file=sys.stderr)
                    print(f"!!! BATCH REQUEUE FAILED — exact replay package remains in transaction snapshot | {_safe_display(requeue_error)} !!!", file=sys.stderr)
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
        failed_rows = [x for x in _LAST_EXECUTION_DETAILS if x.get("status") in {"FAIL", "PREFLIGHT_FAIL"}]
        incomplete_rows = [x for x in _LAST_EXECUTION_DETAILS if x.get("status") == "INCOMPLETE"]
        failed_name = str(failed_rows[-1].get("name")) if failed_rows else (executed[-1][0] if executed else None)
        fail_count = len(failed_rows)
        # COLLECT INCOMPLETE is a bounded-evidence result, not an execution
        # failure.  Preserve rc=3 so automation cannot mistake it for PASS,
        # while keeping the run/report status semantically distinct from FAIL.
        if rc == 3 and incomplete_rows and not failed_rows:
            incomplete_name = str(incomplete_rows[-1].get("name") or failed_name or "unknown")
            print(
                f"SUMMARY: INCOMPLETE | incomplete={len(incomplete_rows)} | failed=0 | "
                f"last={_safe_display(incomplete_name)} rc={rc}",
                file=sys.stderr,
            )
            if remaining:
                print(f"NOT EXECUTED: {len(remaining)} selected item(s)", file=sys.stderr)
                for item in remaining: print(f"  - {_safe_display(item.name)}", file=sys.stderr)
            if session_duplicates: _print_session_duplicate_removals(session_duplicates, stream=sys.stderr)
            finish_report("INCOMPLETE", rc, chosen=chosen, executed=executed, remaining=remaining)
            return rc
        print(f"SUMMARY: FAIL | failed={fail_count} | policy={failure_policy} | last={_safe_display(str(failed_name or 'unknown'))} rc={rc}", file=sys.stderr)
        if remaining:
            print(f"NOT EXECUTED: {len(remaining)} selected item(s)", file=sys.stderr)
            for item in remaining: print(f"  - {_safe_display(item.name)}", file=sys.stderr)
        if session_duplicates: _print_session_duplicate_removals(session_duplicates, stream=sys.stderr)
        finish_report("FAIL", rc, chosen=chosen, executed=executed, remaining=remaining, failed_item=failed_name)
        _print_patch_result_banner("FAIL", _last_problem_patch_name(), rc=rc, stream=sys.stderr)
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


def _is_zero_argument_dispatch(raw_argv: list[str]) -> bool:
    remaining: list[str] = []
    skip_next = False
    for arg in raw_argv:
        if skip_next:
            skip_next = False
            continue
        if arg == "--project-root":
            skip_next = True
            continue
        if arg.startswith("--project-root="):
            continue
        remaining.append(arg)
    return not remaining


def main(argv=None):
    raw_argv = list(sys.argv[1:] if argv is None else argv)
    zero_argument_invocation = _is_zero_argument_dispatch(raw_argv)
    ap = argparse.ArgumentParser()
    ap.add_argument("--project-root", required=True)
    ap.add_argument("command", nargs="?", choices=["run", "resume", "report", "plan"], default="run")
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
    ap.add_argument("--recipe")
    ap.add_argument("--export-recipe", nargs="?", const="BATCH_RECIPE.json")
    # Historical automation compatibility. These are real dispatcher semantics,
    # not launcher-only aliases, so behavioral regression tests can exercise them.
    ap.add_argument("--patch", action="append", dest="patch_specs")
    ap.add_argument("--all", "-a", action="store_true", dest="select_all")
    ap.add_argument("--select")
    ap.add_argument("-y", action="store_true", dest="assume_yes")
    ap.add_argument("--zip-failed", action="store_true")
    ap.add_argument("--keep-failed-zip", action="store_true")
    ap.add_argument("--move", action="store_true")
    ap.add_argument("--no-validation", action="store_true", help="Disable trusted validation profiles and delta auto-selection for this run")
    ns = ap.parse_args(raw_argv)
    root = Path(ns.project_root).resolve()
    try:
        if ns.command in {"run", "resume", "plan"}:
            try:
                load_project_config(root)
            except Exception as exc:
                print(
                    f"[PTV v{VERSION} ERROR] PROJECT CONFIG: {_safe_display(str(exc))}",
                    file=sys.stderr,
                )
                return 2
        if ns.command == "plan":
            return _plan_queue(
                root, export_recipe=ns.export_recipe,
                failure_policy_override=ns.failure_policy, transaction_policy_override=ns.transaction_policy,
            )
        if ns.command == "report":
            return _report_command(
                root, ns.run_id, list_runs=ns.list_runs, pin_run=ns.pin, unpin_run=ns.unpin,
                delete_run=ns.delete, export_run=ns.export, cleanup=ns.cleanup, support_item=ns.support_item,
            )
        # Deliberately NO process-wide/project-wide queue lock. Selection isolation
        # is per invocation only; operators may run other Patch Tool processes in
        # separate terminals when they intentionally choose to do so.
        if ns.zip_failed:
            os.environ["PTV_LEGACY_ZIP_FAILED"] = "1"
        if ns.keep_failed_zip:
            os.environ["PTV_LEGACY_KEEP_FAILED_ZIP"] = "1"
        if ns.assume_yes:
            os.environ["PTV_LEGACY_ASSUME_YES"] = "1"
        # --move is retained as a compatibility no-op: current queue semantics
        # already archive every successful PATCH into patchs/patched.
        return _run_queue(
            root, failure_policy_override=ns.failure_policy, transaction_policy_override=ns.transaction_policy,
            force_resume=(ns.command == "resume"), resume_mode=ns.resume_mode, recipe_path=ns.recipe,
            zero_argument_invocation=zero_argument_invocation, explicit_patch_specs=ns.patch_specs,
            select_all=ns.select_all, select_spec=ns.select, no_validation=ns.no_validation,
        )
    except QueueSafetyError as exc:
        # Fail closed without a Python traceback when the queue/artifact/lock
        # filesystem boundary itself is unsafe.  We intentionally do not try to
        # write LAST_RUN here because its artifact root may be the unsafe object.
        print(f"[PTV v{VERSION} ERROR] FILESYSTEM SAFETY: {_safe_display(str(exc))}", file=sys.stderr)
        return 2
    finally:
        panel = globals().get("_ACTIVE_LIVE_STATUS_PANEL")
        if panel is not None:
            try:
                panel.close()
            except Exception:
                pass


if __name__ == "__main__":
    raise SystemExit(main())
