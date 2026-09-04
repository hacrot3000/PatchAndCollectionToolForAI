#!/usr/bin/env python3
from __future__ import annotations

import difflib
import hashlib
import json
import os
import shutil
import stat
import tarfile
import tempfile
import time
import zipfile
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path, PurePosixPath
from typing import Any

from python_patch_package_schema import PatchSchemaError, check_compatibility, validate_manifest, resolve_project_path, _ops_target_paths

VERSION = "6.17.4"
MAX_MANIFEST_BYTES = 2 * 1024 * 1024
MAX_DIFF_FILE_BYTES = 512 * 1024
MAX_SNAPSHOT_FILE_BYTES = 8 * 1024 * 1024
MAX_SNAPSHOT_TOTAL_BYTES = 256 * 1024 * 1024


class BatchPlanError(RuntimeError):
    def __init__(self, message: str, *, kind: str = "batch_plan_invalid"):
        super().__init__(message)
        self.kind = kind


@dataclass
class PatchMeta:
    name: str
    patch_id: str
    manifest: dict[str, Any]
    depends_on: list[str]
    on_dependency_failure: str
    previous_failure: dict[str, Any] | None
    targets: list[str]
    effective_targets: list[str]
    package_sha256: str | None = None


def _reject_duplicate_json_pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError(f"duplicate JSON key: {key}")
        out[key] = value
    return out


def _decode_json_object(raw: bytes, *, member: str, package_name: str) -> dict[str, Any]:
    try:
        data = json.loads(raw.decode("utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
    except Exception as exc:
        raise BatchPlanError(
            f"invalid {member} in {package_name}: {type(exc).__name__}: {exc}",
            kind="package_invalid",
        ) from exc
    if not isinstance(data, dict):
        raise BatchPlanError(f"{member} root must be an object in {package_name}", kind="package_invalid")
    return data


def _safe_json_archive_member(path: Path, member: str, *, required: bool = True, max_bytes: int = MAX_MANIFEST_BYTES) -> dict[str, Any] | None:
    """Read one root JSON member from ZIP/TAR without extraction.

    Missing root metadata is allowed only for explicitly supported legacy archive
    routing. Duplicate member names and non-regular TAR members fail closed.
    """
    if not path.is_file() or path.is_symlink():
        raise BatchPlanError(f"PATCH package is unavailable or unsafe: {path.name}", kind="package_invalid")
    low = path.name.lower()
    try:
        if low.endswith(".zip"):
            with zipfile.ZipFile(path) as zf:
                matches = [info for info in zf.infolist() if info.filename == member]
                if not matches:
                    if required:
                        raise BatchPlanError(f"cannot read {member} from {path.name}: member missing", kind="package_invalid")
                    return None
                if len(matches) != 1:
                    raise BatchPlanError(f"duplicate {member} in {path.name}", kind="package_invalid")
                info = matches[0]
                if info.is_dir() or info.file_size > max_bytes:
                    raise BatchPlanError(f"{member} is invalid or too large in {path.name}", kind="package_invalid")
                raw = zf.read(info)
        elif low.endswith((".tar.gz", ".tgz")):
            with tarfile.open(path, "r:*") as tf:
                matches = [m for m in tf.getmembers() if m.name == member]
                if not matches:
                    if required:
                        raise BatchPlanError(f"cannot read {member} from {path.name}: member missing", kind="package_invalid")
                    return None
                if len(matches) != 1:
                    raise BatchPlanError(f"duplicate {member} in {path.name}", kind="package_invalid")
                info = matches[0]
                if not info.isfile() or info.size > max_bytes:
                    raise BatchPlanError(f"{member} is invalid or too large in {path.name}", kind="package_invalid")
                fh = tf.extractfile(info)
                if fh is None:
                    raise BatchPlanError(f"cannot read {member} from {path.name}", kind="package_invalid")
                raw = fh.read(max_bytes + 1)
                if len(raw) > max_bytes:
                    raise BatchPlanError(f"{member} is too large in {path.name}", kind="package_invalid")
        else:
            raise BatchPlanError(f"unsupported PATCH package extension: {path.name}", kind="package_invalid")
    except BatchPlanError:
        raise
    except (zipfile.BadZipFile, tarfile.TarError, OSError, KeyError) as exc:
        raise BatchPlanError(
            f"cannot read {member} from {path.name}: {type(exc).__name__}: {exc}",
            kind="package_invalid",
        ) from exc
    return _decode_json_object(raw, member=member, package_name=path.name)


def load_patch_meta(root: Path, name: str) -> PatchMeta:
    package = root / "patchs" / name
    package_sha_before = stable_package_sha256(package)
    if package.suffix.lower() == ".py":
        package_sha_after = stable_package_sha256(package)
        if package_sha_before != package_sha_after:
            raise BatchPlanError(f"PATCH package changed while loading metadata: {name}", kind="package_input_changed")
        return PatchMeta(name, f"legacy:{name}", {}, [], "block", None, [], [], package_sha_before)
    manifest = _safe_json_archive_member(package, "PATCH_TOOL_MANIFEST.json", required=False)
    if manifest is None:
        # Legacy ZIP/TAR archives are still discovered intentionally. They do
        # not participate in dependency/atomic metadata and are scheduled as a
        # single legacy unit; the runner enforces the one-Python-entrypoint rule.
        package_sha_after = stable_package_sha256(package)
        if package_sha_before != package_sha_after:
            raise BatchPlanError(f"PATCH package changed while loading metadata: {name}", kind="package_input_changed")
        return PatchMeta(name, f"legacy:{name}", {}, [], "block", None, [], [], package_sha_before)
    try:
        validate_manifest(manifest)
        check_compatibility(manifest, VERSION)
    except PatchSchemaError:
        # Preserve the established standalone legacy/self-contained PATCH route.
        # Legacy packages cannot declare v6.17 dependency/transaction metadata;
        # they receive an internal scheduling key only so ordinary one-item runs
        # are not broken by the new batch planner. This is not a provenance ID.
        if "schema_version" not in manifest and "patch" not in manifest:
            package_sha_after = stable_package_sha256(package)
            if package_sha_before != package_sha_after:
                raise BatchPlanError(f"PATCH package changed while loading metadata: {name}", kind="package_input_changed")
            return PatchMeta(name, f"legacy:{name}", manifest, [], "block", None, [], [], package_sha_before)
        raise
    patch = manifest.get("patch") if isinstance(manifest.get("patch"), dict) else {}
    patch_id = str(patch.get("id") or "").strip()
    if not patch_id:
        raise BatchPlanError(f"manifest.patch.id is required: {name}")
    batch = manifest.get("batch") if isinstance(manifest.get("batch"), dict) else {}
    depends = batch.get("depends_on") if isinstance(batch.get("depends_on"), list) else []
    depends_on = [str(x).strip() for x in depends if isinstance(x, str) and str(x).strip()]
    on_fail = str(batch.get("on_dependency_failure") or "block")
    prev = batch.get("previous_failure") if isinstance(batch.get("previous_failure"), dict) else None
    targets = manifest.get("targets") if isinstance(manifest.get("targets"), list) else []
    target_list = [str(x) for x in targets if isinstance(x, str)]
    effective = set(target_list)
    pre = manifest.get("preflight") if isinstance(manifest.get("preflight"), dict) else {}
    for spec in pre.get("files", []) if isinstance(pre.get("files"), list) else []:
        if isinstance(spec, dict) and isinstance(spec.get("path"), str):
            effective.add(str(spec["path"]))
    recovery = manifest.get("recovery") if isinstance(manifest.get("recovery"), dict) else {}
    rollback = recovery.get("rollback") if isinstance(recovery.get("rollback"), dict) else {}
    for rel in rollback.get("targets", []) if isinstance(rollback.get("targets"), list) else []:
        if isinstance(rel, str):
            effective.add(rel)
    ops_data = _safe_json_archive_member(
        package, "PATCH_TOOL_OPS.json", required=False, max_bytes=MAX_MANIFEST_BYTES * 8
    )
    if isinstance(ops_data, dict):
        effective.update(_ops_target_paths(ops_data.get("ops")))
    package_sha_after = stable_package_sha256(package)
    if package_sha_before != package_sha_after:
        raise BatchPlanError(f"PATCH package changed while loading metadata: {name}", kind="package_input_changed")
    return PatchMeta(name, patch_id, manifest, depends_on, on_fail, prev, target_list, sorted(effective), package_sha_before)


def topo_order(metas: list[PatchMeta]) -> list[PatchMeta]:
    by_id: dict[str, PatchMeta] = {}
    order_index: dict[str, int] = {}
    for i, meta in enumerate(metas):
        if meta.patch_id in by_id:
            raise BatchPlanError(f"duplicate manifest.patch.id in selected batch: {meta.patch_id}", kind="dependency_ambiguous")
        by_id[meta.patch_id] = meta
        order_index[meta.patch_id] = i
    for meta in metas:
        missing = [dep for dep in meta.depends_on if dep not in by_id]
        if missing:
            raise BatchPlanError(
                f"{meta.name} depends on PATCH id(s) not selected: {', '.join(missing)}",
                kind="dependency_missing",
            )
    indeg = {pid: 0 for pid in by_id}
    outgoing: dict[str, list[str]] = {pid: [] for pid in by_id}
    for meta in metas:
        for dep in meta.depends_on:
            indeg[meta.patch_id] += 1
            outgoing[dep].append(meta.patch_id)
    ready = sorted((pid for pid, n in indeg.items() if n == 0), key=lambda x: order_index[x])
    out: list[PatchMeta] = []
    while ready:
        pid = ready.pop(0)
        out.append(by_id[pid])
        for child in sorted(outgoing[pid], key=lambda x: order_index[x]):
            indeg[child] -= 1
            if indeg[child] == 0:
                ready.append(child)
                ready.sort(key=lambda x: order_index[x])
    if len(out) != len(metas):
        cycle = [pid for pid, n in indeg.items() if n > 0]
        raise BatchPlanError(f"PATCH dependency cycle detected: {', '.join(cycle)}", kind="dependency_cycle")
    return out


def previous_failed_identity(previous: dict[str, Any] | None) -> tuple[str | None, str | None]:
    if not isinstance(previous, dict) or previous.get("status") != "FAIL":
        return None, None
    failed_name = previous.get("failed_item")
    if not isinstance(failed_name, str) or not failed_name:
        # continue-on-failure may have several failures; the last unresolved one is the immediate predecessor.
        rows = previous.get("results") if isinstance(previous.get("results"), list) else []
        for row in reversed(rows):
            if isinstance(row, dict) and row.get("status") == "FAIL" and isinstance(row.get("name"), str):
                failed_name = row.get("name")
                break
    failed_id = None
    rows = previous.get("results") if isinstance(previous.get("results"), list) else []
    for row in reversed(rows):
        if not isinstance(row, dict) or row.get("name") != failed_name:
            continue
        pr = row.get("patch_result") if isinstance(row.get("patch_result"), dict) else {}
        mp = pr.get("manifest_patch") if isinstance(pr.get("manifest_patch"), dict) else {}
        value = mp.get("id")
        if isinstance(value, str) and value:
            failed_id = value
            break
    return (str(failed_name) if isinstance(failed_name, str) else None, failed_id)


def validate_previous_failure_declaration(meta: PatchMeta, failed_name: str, failed_id: str | None) -> dict[str, Any]:
    prev = meta.previous_failure
    if not isinstance(prev, dict):
        raise BatchPlanError(
            f"{meta.name} follows unresolved failed PATCH {failed_name} but does not declare batch.previous_failure",
            kind="previous_failure_action_required",
        )
    action = str(prev.get("action") or "")
    if action not in {"delete", "retry_before", "run_after", "block"}:
        raise BatchPlanError(f"{meta.name} has unsupported previous_failure.action={action!r}", kind="previous_failure_action_invalid")
    reason = str(prev.get("reason") or "").strip()
    if not reason:
        raise BatchPlanError(f"{meta.name} previous_failure.reason is required", kind="previous_failure_action_invalid")
    declared_id = str(prev.get("patch_id") or "").strip()
    declared_file = str(prev.get("patch_file") or "").strip()
    if failed_id and declared_id and declared_id != failed_id:
        raise BatchPlanError(
            f"{meta.name} previous_failure.patch_id={declared_id!r} does not match failed PATCH id {failed_id!r}",
            kind="previous_failure_action_mismatch",
        )
    if declared_file and declared_file != failed_name:
        raise BatchPlanError(
            f"{meta.name} previous_failure.patch_file={declared_file!r} does not match failed PATCH file {failed_name!r}",
            kind="previous_failure_action_mismatch",
        )
    if not declared_id and not declared_file:
        raise BatchPlanError(
            f"{meta.name} previous_failure must identify the predecessor with patch_id and/or patch_file",
            kind="previous_failure_action_invalid",
        )
    return {"action": action, "reason": reason, "patch_id": declared_id or failed_id, "patch_file": declared_file or failed_name}


def transaction_compatibility(metas: list[PatchMeta], policy: str) -> list[str]:
    if policy != "batch":
        return []
    issues: list[str] = []
    for meta in metas:
        if not meta.targets:
            issues.append(f"{meta.name}: batch transaction requires manifest.targets")
        post = meta.manifest.get("post_patch") if isinstance(meta.manifest.get("post_patch"), dict) else {}
        if isinstance(post.get("commands"), list) and post.get("commands"):
            issues.append(f"{meta.name}: batch transaction rejects post_patch.commands because side effects are not target-bounded")
        git = meta.manifest.get("git") if isinstance(meta.manifest.get("git"), dict) else {}
        if (git.get("add") not in {None, False, "off"} or
                git.get("commit") not in {None, False, "off"} or git.get("push") not in {None, "off"}):
            issues.append(f"{meta.name}: batch transaction rejects Git add/commit/push side effects")
    return issues


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as src:
        for chunk in iter(lambda: src.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def stable_package_sha256(path: Path) -> str:
    """Hash one queue package without following a replacement/symlink leaf.

    The descriptor generation is checked before/after hashing so planning and
    execution can bind dependency/target metadata to the exact package bytes.
    """
    flags = os.O_RDONLY | getattr(os, "O_BINARY", 0) | getattr(os, "O_NOFOLLOW", 0)
    try:
        fd = os.open(path, flags)
    except OSError as exc:
        raise BatchPlanError(
            f"PATCH package is unavailable or unsafe: {path.name}: {type(exc).__name__}",
            kind="package_input_changed",
        ) from exc
    try:
        before = os.fstat(fd)
        attrs = getattr(before, "st_file_attributes", 0)
        reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if not stat.S_ISREG(before.st_mode) or reparse:
            raise BatchPlanError(f"PATCH package is not a regular file: {path.name}", kind="package_input_changed")
        h = hashlib.sha256(); copied = 0
        with os.fdopen(os.dup(fd), "rb") as src:
            for chunk in iter(lambda: src.read(1024 * 1024), b""):
                h.update(chunk); copied += len(chunk)
        after = os.fstat(fd)
        ident_before = (before.st_dev, before.st_ino, before.st_size, getattr(before, "st_mtime_ns", int(before.st_mtime * 1e9)))
        ident_after = (after.st_dev, after.st_ino, after.st_size, getattr(after, "st_mtime_ns", int(after.st_mtime * 1e9)))
        if ident_before != ident_after or copied != before.st_size:
            raise BatchPlanError(f"PATCH package changed while hashing: {path.name}", kind="package_input_changed")
        return h.hexdigest()
    finally:
        os.close(fd)



def _safe_target_path(root: Path, rel: str) -> Path:
    pure = PurePosixPath(rel)
    if pure.is_absolute() or any(x in {"", ".", ".."} for x in pure.parts):
        raise BatchPlanError(f"unsafe batch target: {rel}", kind="batch_transaction_invalid")
    root_real = root.resolve(strict=True)
    cur = root_real
    for part in pure.parts[:-1]:
        cur = cur / part
        try:
            st = cur.lstat()
        except FileNotFoundError as exc:
            raise BatchPlanError(f"batch transaction target parent must already exist: {rel}", kind="batch_transaction_invalid") from exc
        attrs = getattr(st, "st_file_attributes", 0)
        reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
            raise BatchPlanError(f"unsafe batch transaction target ancestor: {rel}", kind="batch_transaction_invalid")
        try: cur.resolve(strict=True).relative_to(root_real)
        except Exception as exc:
            raise BatchPlanError(f"batch transaction target escapes project root: {rel}", kind="batch_transaction_invalid") from exc
    leaf = cur / pure.name
    try:
        st = leaf.lstat()
    except FileNotFoundError:
        return leaf
    attrs = getattr(st, "st_file_attributes", 0)
    reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
    if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISREG(st.st_mode):
        raise BatchPlanError(f"batch transaction target must be a regular file or missing: {rel}", kind="batch_transaction_invalid")
    return leaf


def _open_batch_parent_fd(root: Path, rel: str) -> tuple[int | None, Path, str]:
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
            os.close(fd); fd = next_fd
        return fd, parent_path, leaf
    except Exception:
        try: os.close(fd)
        except OSError: pass
        raise


def _copy_fd_snapshot(fd: int, backup: Path, *, rel: str, total_before: int) -> tuple[int, str, int]:
    before = os.fstat(fd)
    if not stat.S_ISREG(before.st_mode):
        raise BatchPlanError(f"batch transaction target changed type while snapshotting: {rel}", kind="batch_transaction_snapshot_race")
    if before.st_size > MAX_SNAPSHOT_FILE_BYTES:
        raise BatchPlanError(f"batch transaction target exceeds per-file snapshot limit: {rel}", kind="batch_transaction_invalid")
    copied = 0
    digest = hashlib.sha256()
    with os.fdopen(os.dup(fd), "rb") as src, backup.open("wb") as dst:
        for chunk in iter(lambda: src.read(1024 * 1024), b""):
            copied += len(chunk)
            if copied > MAX_SNAPSHOT_FILE_BYTES or total_before + copied > MAX_SNAPSHOT_TOTAL_BYTES:
                raise BatchPlanError("batch transaction snapshot exceeds total byte limit", kind="batch_transaction_invalid")
            dst.write(chunk); digest.update(chunk)
        dst.flush()
        try: os.fsync(dst.fileno())
        except OSError: pass
    after = os.fstat(fd)
    ident_before = (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns)
    ident_after = (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns)
    if ident_before != ident_after or copied != before.st_size:
        raise BatchPlanError(f"batch transaction target changed while snapshotting: {rel}", kind="batch_transaction_snapshot_race")
    return copied, digest.hexdigest(), stat.S_IMODE(before.st_mode)


def _sha256_open_target(parent_fd: int | None, parent_path: Path, leaf: str) -> tuple[str, int]:
    flags = os.O_RDONLY | getattr(os, "O_BINARY", 0) | getattr(os, "O_NOFOLLOW", 0)
    fd = os.open(leaf, flags, dir_fd=parent_fd) if parent_fd is not None else os.open(parent_path / leaf, flags)
    try:
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode):
            raise RuntimeError("target is not a regular file")
        h = hashlib.sha256(); copied = 0
        with os.fdopen(os.dup(fd), "rb") as src:
            for chunk in iter(lambda: src.read(1024 * 1024), b""):
                h.update(chunk); copied += len(chunk)
        after = os.fstat(fd)
        if (st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns) != (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns) or copied != st.st_size:
            raise RuntimeError("target changed while verifying")
        return h.hexdigest(), copied
    finally:
        os.close(fd)


def snapshot_targets(root: Path, targets: list[str], snapshot_root: Path) -> dict[str, Any]:
    entries: list[dict[str, Any]] = []
    total = 0
    snapshot_root.mkdir(parents=True, exist_ok=True)
    for index, rel in enumerate(sorted(set(targets))):
        # Validate project containment, then pin the parent directory on POSIX.
        _safe_target_path(root, rel)
        parent_fd = None
        try:
            parent_fd, parent_path, leaf = _open_batch_parent_fd(root, rel)
            try:
                st = os.stat(leaf, dir_fd=parent_fd, follow_symlinks=False) if parent_fd is not None else (parent_path / leaf).lstat()
            except FileNotFoundError:
                entries.append({"path": rel, "kind": "missing"})
                continue
            attrs = getattr(st, "st_file_attributes", 0)
            reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
            if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISREG(st.st_mode):
                raise BatchPlanError(f"batch transaction target must remain a regular file or missing: {rel}", kind="batch_transaction_snapshot_race")
            flags = os.O_RDONLY | getattr(os, "O_BINARY", 0) | getattr(os, "O_NOFOLLOW", 0)
            fd = os.open(leaf, flags, dir_fd=parent_fd) if parent_fd is not None else os.open(parent_path / leaf, flags)
            backup = snapshot_root / f"{index:05d}.bin"
            try:
                copied, digest, mode = _copy_fd_snapshot(fd, backup, rel=rel, total_before=total)
            finally:
                os.close(fd)
            total += copied
            entries.append({"path": rel, "kind": "file", "backup": backup.name, "sha256": digest, "size": copied, "mode": mode})
        except BatchPlanError:
            raise
        except (OSError, ValueError) as exc:
            raise BatchPlanError(f"batch transaction target changed/unsafe while snapshotting: {rel}: {type(exc).__name__}", kind="batch_transaction_snapshot_race") from exc
        finally:
            if parent_fd is not None:
                try: os.close(parent_fd)
                except OSError: pass
    manifest = {"format": "ptv-batch-transaction-snapshot", "version": 2, "entries": entries, "total_bytes": total}
    fd, name = tempfile.mkstemp(prefix=".ptv-batch-snapshot-", suffix=".json", dir=snapshot_root)
    temp = Path(name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as out:
            out.write(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n"); out.flush()
            try: os.fsync(out.fileno())
            except OSError: pass
        os.replace(temp, snapshot_root / "SNAPSHOT.json")
    finally:
        try: temp.unlink()
        except FileNotFoundError: pass
    return manifest


def restore_targets(root: Path, snapshot_root: Path, manifest: dict[str, Any]) -> dict[str, Any]:
    restored: list[str] = []
    errors: list[str] = []
    for index, entry in enumerate(manifest.get("entries", [])):
        if not isinstance(entry, dict) or not isinstance(entry.get("path"), str):
            errors.append("invalid rollback entry")
            continue
        rel = entry["path"]
        parent_fd = None
        try:
            _safe_target_path(root, rel)
            parent_fd, parent_path, leaf = _open_batch_parent_fd(root, rel)
            if entry.get("kind") == "missing":
                try:
                    st = os.stat(leaf, dir_fd=parent_fd, follow_symlinks=False) if parent_fd is not None else (parent_path / leaf).lstat()
                except FileNotFoundError:
                    restored.append(rel); continue
                if not (stat.S_ISREG(st.st_mode) or stat.S_ISLNK(st.st_mode)):
                    raise RuntimeError("rollback refuses to remove directory/non-file created at target")
                if parent_fd is not None:
                    os.unlink(leaf, dir_fd=parent_fd)
                else:
                    (parent_path / leaf).unlink()
            else:
                backup = snapshot_root / str(entry.get("backup"))
                if not backup.is_file() or backup.is_symlink() or _sha256(backup) != entry.get("sha256"):
                    raise RuntimeError("snapshot backup missing or corrupted")
                if parent_fd is not None:
                    temp_name = f".ptv-batch-restore-{os.getpid()}-{time.time_ns()}-{index}"
                    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
                    out_fd = os.open(temp_name, flags, 0o600, dir_fd=parent_fd)
                    try:
                        with os.fdopen(os.dup(out_fd), "wb") as dst, backup.open("rb") as src:
                            shutil.copyfileobj(src, dst, length=1024 * 1024); dst.flush()
                            try: os.fsync(dst.fileno())
                            except OSError: pass
                        if os.name != "nt": os.fchmod(out_fd, int(entry.get("mode", 0o644)))
                    finally:
                        os.close(out_fd)
                    try:
                        os.replace(temp_name, leaf, src_dir_fd=parent_fd, dst_dir_fd=parent_fd)
                    finally:
                        try: os.unlink(temp_name, dir_fd=parent_fd)
                        except FileNotFoundError: pass
                else:
                    path = parent_path / leaf
                    # Windows/fallback: revalidate immediately before path-based replace.
                    _safe_target_path(root, rel)
                    fd, tmp_name = tempfile.mkstemp(prefix=".ptv-batch-restore-", dir=path.parent)
                    tmp = Path(tmp_name)
                    try:
                        with os.fdopen(fd, "wb") as dst, backup.open("rb") as src:
                            shutil.copyfileobj(src, dst, length=1024 * 1024); dst.flush()
                            try: os.fsync(dst.fileno())
                            except OSError: pass
                        if os.name != "nt": os.chmod(tmp, int(entry.get("mode", 0o644)))
                        _safe_target_path(root, rel)
                        os.replace(tmp, path)
                    finally:
                        try: tmp.unlink()
                        except FileNotFoundError: pass
            restored.append(rel)
        except Exception as exc:
            errors.append(f"{rel}: {type(exc).__name__}: {exc}")
        finally:
            if parent_fd is not None:
                try: os.close(parent_fd)
                except OSError: pass

    verification_errors: list[str] = []
    for entry in manifest.get("entries", []):
        if not isinstance(entry, dict) or not isinstance(entry.get("path"), str):
            continue
        rel = entry["path"]
        parent_fd = None
        try:
            _safe_target_path(root, rel)
            parent_fd, parent_path, leaf = _open_batch_parent_fd(root, rel)
            if entry.get("kind") == "missing":
                try:
                    os.stat(leaf, dir_fd=parent_fd, follow_symlinks=False) if parent_fd is not None else (parent_path / leaf).lstat()
                except FileNotFoundError:
                    continue
                verification_errors.append(f"{rel}: expected missing after rollback")
            else:
                digest, size = _sha256_open_target(parent_fd, parent_path, leaf)
                if digest != entry.get("sha256") or (entry.get("size") is not None and size != entry.get("size")):
                    verification_errors.append(f"{rel}: restored bytes do not match snapshot")
        except Exception as exc:
            verification_errors.append(f"{rel}: verify {type(exc).__name__}: {exc}")
        finally:
            if parent_fd is not None:
                try: os.close(parent_fd)
                except OSError: pass
    errors.extend(verification_errors)
    return {
        "status": "PASS" if not errors else "FAIL",
        "restored_paths": restored,
        "errors": errors,
        "verified": not verification_errors,
    }


def snapshot_package_bytes(
    root: Path,
    names: list[str],
    snapshot_root: Path,
    *,
    expected_sha256: dict[str, str] | None = None,
) -> dict[str, dict[str, object]]:
    """Freeze every selected PATCH package and bind it to planned bytes.

    Missing/unsafe packages are transaction-preflight failures, never silently
    omitted. The returned in-memory map carries the original SHA/size so replay
    later rejects a corrupted transaction snapshot instead of blessing it.
    """
    snapshot_root.mkdir(parents=True, exist_ok=True)
    out: dict[str, dict[str, object]] = {}
    expected_sha256 = expected_sha256 or {}
    for i, name in enumerate(names):
        src = root / "patchs" / name
        if not src.is_file() or src.is_symlink():
            raise BatchPlanError(f"selected PATCH package disappeared or became unsafe before snapshot: {name}", kind="batch_transaction_snapshot_race")
        flags = os.O_RDONLY | getattr(os, "O_BINARY", 0) | getattr(os, "O_NOFOLLOW", 0)
        fd = os.open(src, flags)
        try:
            before = os.fstat(fd)
            if not stat.S_ISREG(before.st_mode):
                raise BatchPlanError(f"batch package changed type before snapshot: {name}", kind="batch_transaction_snapshot_race")
            dst = snapshot_root / f"{i:04d}_{Path(name).name}"
            copied = 0; digest = hashlib.sha256()
            with os.fdopen(os.dup(fd), "rb") as in_fh, dst.open("xb") as out_fh:
                for chunk in iter(lambda: in_fh.read(1024 * 1024), b""):
                    copied += len(chunk); digest.update(chunk); out_fh.write(chunk)
                out_fh.flush()
                try: os.fsync(out_fh.fileno())
                except OSError: pass
            after = os.fstat(fd)
            if (before.st_dev, before.st_ino, before.st_size, getattr(before, "st_mtime_ns", int(before.st_mtime * 1e9))) != (after.st_dev, after.st_ino, after.st_size, getattr(after, "st_mtime_ns", int(after.st_mtime * 1e9))) or copied != before.st_size:
                try: dst.unlink()
                except OSError: pass
                raise BatchPlanError(f"batch package changed while snapshotting: {name}", kind="batch_transaction_snapshot_race")
            actual_sha = digest.hexdigest()
            planned_sha = expected_sha256.get(name)
            if planned_sha and actual_sha != planned_sha:
                try: dst.unlink()
                except OSError: pass
                raise BatchPlanError(f"PATCH package bytes changed after planning/preflight: {name}", kind="package_input_changed")
            if _sha256(dst) != actual_sha:
                raise BatchPlanError(f"batch package snapshot hash verification failed: {name}", kind="batch_transaction_snapshot_race")
            out[name] = {"stored": dst.name, "sha256": actual_sha, "size": copied}
        finally:
            os.close(fd)
    return out


def _publish_requeue_no_overwrite(src: Path, target: Path, expected_sha: str) -> str:
    """Publish exact replay bytes without ever replacing a concurrent file.

    Returns ``published``, ``same`` or ``exists``. Any target created by this
    helper is hash-verified before success is reported.
    """
    def classify_existing() -> str:
        try:
            if target.is_symlink() or not target.is_file():
                return "exists"
            return "same" if _sha256(target) == expected_sha else "exists"
        except OSError:
            return "exists"

    try:
        os.link(src, target, follow_symlinks=False)
    except FileExistsError:
        return classify_existing()
    except OSError:
        # Filesystems such as exFAT/FAT/network shares may not support links.
        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_BINARY", 0)
        fd = -1
        created = False
        try:
            fd = os.open(target, flags, 0o600)
            created = True
            digest = hashlib.sha256()
            with os.fdopen(fd, "wb") as out_fh, src.open("rb") as in_fh:
                fd = -1
                for chunk in iter(lambda: in_fh.read(1024 * 1024), b""):
                    out_fh.write(chunk); digest.update(chunk)
                out_fh.flush()
                try: os.fsync(out_fh.fileno())
                except OSError: pass
            if digest.hexdigest() != expected_sha or _sha256(target) != expected_sha:
                raise BatchPlanError("requeued package fallback copy hash verification failed", kind="batch_requeue_failed")
            return "published"
        except FileExistsError:
            return classify_existing()
        except Exception:
            if created:
                try: target.unlink()
                except OSError: pass
            raise
        finally:
            if fd >= 0:
                try: os.close(fd)
                except OSError: pass
    if _sha256(target) != expected_sha:
        try: target.unlink()
        except OSError: pass
        raise BatchPlanError("requeued package hardlink hash verification failed", kind="batch_requeue_failed")
    return "published"


def requeue_packages(root: Path, package_snapshot_root: Path, package_map: dict[str, object]) -> dict[str, str]:
    out: dict[str, str] = {}
    queue = root / "patchs"
    if queue.exists() or queue.is_symlink():
        if queue.is_symlink() or not queue.is_dir():
            raise BatchPlanError("batch requeue requires a real patchs/ directory", kind="batch_requeue_failed")
    else:
        queue.mkdir(parents=True, exist_ok=False)
    for original, raw_entry in package_map.items():
        if isinstance(raw_entry, dict):
            stored = raw_entry.get("stored")
            recorded_sha = raw_entry.get("sha256")
            recorded_size = raw_entry.get("size")
        else:
            # Compatibility for direct callers/tests using the pre-v6.17.4 map.
            stored = raw_entry
            recorded_sha = None
            recorded_size = None
        if not isinstance(stored, str) or not stored:
            raise BatchPlanError(f"invalid replay package snapshot metadata for {original}", kind="batch_requeue_failed")
        src = package_snapshot_root / stored
        if not src.is_file() or src.is_symlink():
            raise BatchPlanError(
                f"replay package snapshot is missing or unsafe: {stored}",
                kind="batch_requeue_failed",
            )
        expected_sha = str(recorded_sha) if isinstance(recorded_sha, str) and recorded_sha else _sha256(src)
        try:
            actual_size = src.stat().st_size
            actual_sha = _sha256(src)
        except OSError as exc:
            raise BatchPlanError(f"cannot verify replay package snapshot {stored}: {type(exc).__name__}", kind="batch_requeue_failed") from exc
        if actual_sha != expected_sha or (isinstance(recorded_size, int) and actual_size != recorded_size):
            raise BatchPlanError(f"replay package snapshot was modified/corrupted: {stored}", kind="batch_requeue_failed")
        date = datetime.now().strftime("%Y-%m-%d")
        candidate_index = 1
        while True:
            if candidate_index == 1:
                target = queue / original
            elif candidate_index == 2:
                target = queue / f"RETRY-{date}-{original}"
            else:
                target = queue / f"RETRY-{date}-{candidate_index-1}-{original}"
            status = _publish_requeue_no_overwrite(src, target, expected_sha)
            if status in {"published", "same"}:
                out[original] = target.name
                break
            candidate_index += 1
            if candidate_index > 10000:
                raise BatchPlanError(f"could not allocate replay package name for {original}", kind="batch_requeue_failed")
    return out


def capture_compare_snapshot(root: Path, targets: list[str], out_root: Path) -> dict[str, Any]:
    data: dict[str, Any] = {}
    for rel in sorted(set(targets)):
        path = root.joinpath(*PurePosixPath(rel).parts)
        if path.is_file() and not path.is_symlink():
            size = path.stat().st_size
            item: dict[str, Any] = {"kind": "file", "size": size, "sha256": _sha256(path)}
            if size <= MAX_DIFF_FILE_BYTES:
                try:
                    text = path.read_text(encoding="utf-8")
                except Exception:
                    text = None
                if text is not None:
                    rel_copy = Path(*PurePosixPath(rel).parts)
                    dst = out_root / rel_copy
                    dst.parent.mkdir(parents=True, exist_ok=True)
                    dst.write_text(text, encoding="utf-8")
            data[rel] = item
        elif path.exists():
            data[rel] = {"kind": "other"}
        else:
            data[rel] = {"kind": "missing"}
    return data


def build_diff_artifact(root: Path, before: dict[str, Any], after: dict[str, Any], before_dir: Path, after_dir: Path, diff_path: Path) -> dict[str, Any]:
    changed: list[dict[str, Any]] = []
    lines: list[str] = []
    for rel in sorted(set(before) | set(after)):
        b = before.get(rel, {"kind": "missing"}); a = after.get(rel, {"kind": "missing"})
        if b == a:
            continue
        row = {"path": rel, "before": b, "after": a}
        changed.append(row)
        lines.append(f"### {rel}\n")
        bp = before_dir.joinpath(*PurePosixPath(rel).parts)
        ap = after_dir.joinpath(*PurePosixPath(rel).parts)
        if bp.is_file() or ap.is_file():
            try:
                bt = bp.read_text(encoding="utf-8").splitlines(True) if bp.is_file() else []
                at = ap.read_text(encoding="utf-8").splitlines(True) if ap.is_file() else []
                lines.extend(difflib.unified_diff(bt, at, fromfile=f"before/{rel}", tofile=f"after/{rel}"))
            except Exception:
                lines.append("[binary/large/unavailable text diff]\n")
        else:
            lines.append(f"before={b}\nafter={a}\n")
        lines.append("\n")
    diff_path.parent.mkdir(parents=True, exist_ok=True)
    diff_path.write_text("".join(lines) if lines else "[no declared-target difference]\n", encoding="utf-8")
    return {"changed_paths": [x["path"] for x in changed], "changes": changed, "diff_path": diff_path.as_posix()}
