#!/usr/bin/env python3
from __future__ import annotations

import difflib
import hashlib
import json
import os
import shutil
import stat
import tempfile
import zipfile
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path, PurePosixPath
from typing import Any

from python_patch_package_schema import PatchSchemaError, check_compatibility, validate_manifest, resolve_project_path

VERSION = "6.17.0"
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


def _safe_json_zip_member(path: Path, member: str) -> dict[str, Any]:
    if not path.is_file() or path.is_symlink():
        raise BatchPlanError(f"PATCH package is unavailable or unsafe: {path.name}", kind="package_invalid")
    try:
        with zipfile.ZipFile(path) as zf:
            info = zf.getinfo(member)
            if info.file_size > MAX_MANIFEST_BYTES:
                raise BatchPlanError(f"{member} is too large in {path.name}", kind="package_invalid")
            raw = zf.read(info)
    except (KeyError, zipfile.BadZipFile, OSError) as exc:
        raise BatchPlanError(f"cannot read {member} from {path.name}: {type(exc).__name__}: {exc}", kind="package_invalid") from exc
    try:
        data = json.loads(raw.decode("utf-8"))
    except Exception as exc:
        raise BatchPlanError(f"invalid {member} in {path.name}: {type(exc).__name__}: {exc}", kind="package_invalid") from exc
    if not isinstance(data, dict):
        raise BatchPlanError(f"{member} root must be an object in {path.name}", kind="package_invalid")
    return data


def load_patch_meta(root: Path, name: str) -> PatchMeta:
    manifest = _safe_json_zip_member(root / "patchs" / name, "PATCH_TOOL_MANIFEST.json")
    try:
        validate_manifest(manifest)
        check_compatibility(manifest, VERSION)
    except PatchSchemaError:
        # Preserve the established standalone legacy/self-contained PATCH route.
        # Legacy packages cannot declare v6.17 dependency/transaction metadata;
        # they receive an internal scheduling key only so ordinary one-item runs
        # are not broken by the new batch planner.  This is not a provenance ID.
        if "schema_version" not in manifest and "patch" not in manifest:
            return PatchMeta(name, f"legacy:{name}", manifest, [], "block", None, [])
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
    return PatchMeta(name, patch_id, manifest, depends_on, on_fail, prev, target_list)


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

def snapshot_targets(root: Path, targets: list[str], snapshot_root: Path) -> dict[str, Any]:
    entries: list[dict[str, Any]] = []
    total = 0
    for index, rel in enumerate(sorted(set(targets))):
        path = _safe_target_path(root, rel)
        parent = path.parent
        if path.is_file():
            size = path.stat().st_size
            if size > MAX_SNAPSHOT_FILE_BYTES:
                raise BatchPlanError(f"batch transaction target exceeds per-file snapshot limit: {rel}", kind="batch_transaction_invalid")
            total += size
            if total > MAX_SNAPSHOT_TOTAL_BYTES:
                raise BatchPlanError("batch transaction snapshot exceeds total byte limit", kind="batch_transaction_invalid")
            backup = snapshot_root / f"{index:05d}.bin"
            backup.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(path, backup)
            entries.append({"path": rel, "kind": "file", "backup": backup.name, "sha256": _sha256(backup), "mode": path.stat().st_mode & 0o777})
        else:
            entries.append({"path": rel, "kind": "missing"})
    manifest = {"format": "ptv-batch-transaction-snapshot", "version": 1, "entries": entries, "total_bytes": total}
    snapshot_root.mkdir(parents=True, exist_ok=True)
    (snapshot_root / "SNAPSHOT.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return manifest


def restore_targets(root: Path, snapshot_root: Path, manifest: dict[str, Any]) -> dict[str, Any]:
    restored: list[str] = []
    errors: list[str] = []
    for entry in manifest.get("entries", []):
        if not isinstance(entry, dict) or not isinstance(entry.get("path"), str):
            continue
        rel = entry["path"]
        try:
            path = _safe_target_path(root, rel)
            if entry.get("kind") == "missing":
                if path.exists():
                    path.unlink()
            else:
                backup = snapshot_root / str(entry.get("backup"))
                if not backup.is_file() or _sha256(backup) != entry.get("sha256"):
                    raise RuntimeError("snapshot backup missing or corrupted")
                fd, tmp_name = tempfile.mkstemp(prefix=".ptv-batch-restore-", dir=path.parent)
                tmp = Path(tmp_name)
                try:
                    with os.fdopen(fd, "wb") as dst, backup.open("rb") as src:
                        shutil.copyfileobj(src, dst, length=1024 * 1024)
                    if os.name != "nt":
                        os.chmod(tmp, int(entry.get("mode", 0o644)))
                    os.replace(tmp, path)
                finally:
                    try: tmp.unlink()
                    except FileNotFoundError: pass
            restored.append(rel)
        except Exception as exc:
            errors.append(f"{rel}: {type(exc).__name__}: {exc}")
    return {"status": "PASS" if not errors else "FAIL", "restored_paths": restored, "errors": errors}


def snapshot_package_bytes(root: Path, names: list[str], snapshot_root: Path) -> dict[str, str]:
    snapshot_root.mkdir(parents=True, exist_ok=True)
    out: dict[str, str] = {}
    for i, name in enumerate(names):
        src = root / "patchs" / name
        if not src.is_file() or src.is_symlink():
            continue
        dst = snapshot_root / f"{i:04d}_{Path(name).name}"
        shutil.copy2(src, dst)
        out[name] = dst.name
    return out


def requeue_packages(root: Path, package_snapshot_root: Path, package_map: dict[str, str]) -> dict[str, str]:
    out: dict[str, str] = {}
    queue = root / "patchs"
    queue.mkdir(parents=True, exist_ok=True)
    for original, stored in package_map.items():
        src = package_snapshot_root / stored
        if not src.is_file():
            continue
        target = queue / original
        if target.exists():
            try:
                if target.is_file() and not target.is_symlink() and _sha256(target) == _sha256(src):
                    out[original] = target.name
                    continue
            except OSError:
                pass
            # Preserve a concurrent replacement. Republish the executed bytes under a deterministic retry name.
            date = datetime.now().strftime("%Y-%m-%d")
            target = queue / f"RETRY-{date}-{original}"
            n = 2
            while target.exists():
                target = queue / f"RETRY-{date}-{n}-{original}"; n += 1
        shutil.copy2(src, target)
        out[original] = target.name
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
