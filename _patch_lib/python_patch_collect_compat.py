#!/usr/bin/env python3
from __future__ import annotations

import argparse
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import sys
import tempfile
import time
import zipfile

VERSION = "6.11.0"
REQUEST_RE = re.compile(r"^CODE_COLLECTION_REQUEST(?:_[A-Za-z0-9._-]+)?\.json$", re.I)
MAX_REQUEST_JSON_BYTES = 1024 * 1024


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def _safe_id(value: object) -> str:
    text = str(value or "collect-pack").strip()
    text = re.sub(r"[^A-Za-z0-9._-]+", "_", text).strip("._-")
    return text[:96] or "collect-pack"


def _load_request(request_zip: Path) -> tuple[dict, str]:
    if request_zip.is_symlink() or not request_zip.is_file():
        raise ValueError("request ZIP must be a regular non-symlink file")
    if request_zip.suffix.lower() != ".zip":
        raise ValueError("request must be a .zip package")
    with zipfile.ZipFile(request_zip) as zf:
        members = [n for n in zf.namelist() if not n.endswith("/")]
        request_members = [n for n in members if REQUEST_RE.match(Path(n).name)]
        if len(request_members) != 1:
            raise ValueError(f"request ZIP must contain exactly one CODE_COLLECTION_REQUEST*.json (found {len(request_members)})")
        info = zf.getinfo(request_members[0])
        if info.file_size > MAX_REQUEST_JSON_BYTES:
            raise ValueError(f"request JSON is too large ({info.file_size} bytes)")
        try:
            data = json.loads(zf.read(request_members[0]).decode("utf-8"))
        except Exception as exc:
            raise ValueError(f"invalid request JSON ({type(exc).__name__})") from exc
    if not isinstance(data, dict):
        raise ValueError("request JSON root must be an object")
    actions = data.get("actions")
    if not isinstance(actions, list) or not actions:
        raise ValueError("request actions must be a non-empty array")
    return data, request_members[0]


def _is_pack_only(data: dict) -> bool:
    actions = data.get("actions")
    return bool(actions) and all(isinstance(a, dict) and str(a.get("type", "")).lower() == "pack" for a in actions)


def _resolve_pack_files(root: Path, data: dict) -> list[tuple[str, Path]]:
    seen: set[str] = set()
    result: list[tuple[str, Path]] = []
    for action_index, action in enumerate(data.get("actions", []), 1):
        if not isinstance(action, dict) or str(action.get("type", "")).lower() != "pack":
            raise ValueError(f"action {action_index}: only pack actions are handled by the v{VERSION} compatibility collector")
        paths = action.get("paths")
        if not isinstance(paths, list) or not paths:
            raise ValueError(f"action {action_index}: pack.paths must be a non-empty array")
        for path_index, raw in enumerate(paths, 1):
            if not isinstance(raw, str) or not raw.strip():
                raise ValueError(f"action {action_index} path {path_index}: path must be a non-empty string")
            rel_text = raw.strip()
            if "\\" in rel_text:
                raise ValueError(f"action {action_index} path {path_index}: use project-relative POSIX paths with '/' separators")
            rel = PurePosixPath(rel_text)
            if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts):
                raise ValueError(f"action {action_index} path {path_index}: unsafe/non-relative path: {rel_text}")
            canonical = rel.as_posix()
            if canonical in seen:
                continue
            candidate = root.joinpath(*rel.parts)
            try:
                lst = candidate.lstat()
            except FileNotFoundError as exc:
                raise ValueError(f"missing pack source: {canonical}") from exc
            if stat.S_ISLNK(lst.st_mode):
                raise ValueError(f"pack source must not be a symlink: {canonical}")
            if not stat.S_ISREG(lst.st_mode):
                raise ValueError(f"pack source must be a regular file: {canonical}")
            try:
                resolved = candidate.resolve(strict=True)
                resolved.relative_to(root)
            except Exception as exc:
                raise ValueError(f"pack source escapes project root: {canonical}") from exc
            seen.add(canonical)
            result.append((canonical, candidate))
    if not result:
        raise ValueError("pack request resolved to zero files")
    return result


def _archive_request(root: Path, request_zip: Path) -> Path:
    queued = request_zip.resolve(strict=True)
    patchs = (root / "patchs").resolve(strict=False)
    try:
        queued.relative_to(patchs)
    except ValueError as exc:
        raise ValueError("request ZIP must be located under project patchs/") from exc
    if queued.parent != patchs:
        raise ValueError("request ZIP must be a direct file under project patchs/")
    archived_dir = root / "patchs" / "patched"
    archived_dir.mkdir(parents=True, exist_ok=True)
    destination = archived_dir / request_zip.name
    if destination.exists() or destination.is_symlink():
        if destination.is_symlink() or not destination.is_file():
            raise ValueError(f"archive destination is unsafe: patchs/patched/{request_zip.name}")
        if _sha256(destination) == _sha256(request_zip):
            request_zip.unlink()
            return destination
        raise ValueError(f"archive destination already exists with different content: patchs/patched/{request_zip.name}")
    os.replace(request_zip, destination)
    return destination


def _write_pack_result(root: Path, request_data: dict, request_member: str, request_zip: Path) -> Path:
    files = _resolve_pack_files(root, request_data)
    out_dir = root / "artifacts" / "patch_tool_code_collections"
    out_dir.mkdir(parents=True, exist_ok=True)
    # Microseconds + PID avoid same-second overwrite when the operator runs
    # multiple COLLECT processes concurrently by choice.
    stamp = datetime.now().strftime("%Y%m%d_%H%M%S_%f")
    result_name = f"CODE_COLLECTION_RESULT_{_safe_id(request_data.get('id'))}_{stamp}_{os.getpid()}.zip"
    final = out_dir / result_name
    fd, temp_name = tempfile.mkstemp(prefix=".ptv-pack-", suffix=".zip", dir=out_dir)
    os.close(fd)
    temp = Path(temp_name)
    try:
        entries: list[dict[str, object]] = []
        with zipfile.ZipFile(temp, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6) as zf:
            for rel, src in files:
                before = src.stat()
                digest = hashlib.sha256()
                copied = 0
                arcname = f"files/{rel}"
                with src.open("rb") as source, zf.open(arcname, "w") as target:
                    for chunk in iter(lambda: source.read(1024 * 1024), b""):
                        target.write(chunk)
                        digest.update(chunk)
                        copied += len(chunk)
                after = src.stat()
                identity_before = (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns)
                identity_after = (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns)
                if identity_before != identity_after:
                    raise ValueError(f"pack source changed while being collected; retry: {rel}")
                entries.append({
                    "path": rel,
                    "archive_path": arcname,
                    "size": copied,
                    "sha256": digest.hexdigest(),
                })
            manifest = {
                "format": "python-patch-tool-code-collection",
                "format_version": 1,
                "tool_version": VERSION,
                "request_id": request_data.get("id"),
                "title": request_data.get("title"),
                "request_member": request_member,
                "action": "pack",
                "file_count": len(entries),
                "files": entries,
            }
            # Manifest is written after file streaming so hashes describe the
            # exact bytes in this archive, even if the live project is active.
            zf.writestr("COLLECTION_MANIFEST.json", json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
        with zipfile.ZipFile(temp) as zf:
            bad = zf.testzip()
            if bad is not None:
                raise ValueError(f"generated result ZIP failed CRC at {bad}")
        # final is effectively unique, but never overwrite an existing result
        # if the filesystem/process namespace somehow collides.
        if final.exists() or final.is_symlink():
            raise ValueError(f"result ZIP name collision: {final.name}")
        os.link(temp, final)
        temp.unlink()
        return final
    finally:
        try:
            temp.unlink()
        except FileNotFoundError:
            pass


def _delegate(root: Path, rest: list[str]) -> int:
    raw = os.environ.get("PTV_PRIVATE_COLLECTOR", "").strip()
    if not raw:
        print(f"ERROR: request is not pack-only and no private collector delegate is installed", file=sys.stderr)
        return 2
    delegate = Path(raw)
    if not delegate.is_file():
        print(f"ERROR: readonly collector delegate is missing: {delegate}", file=sys.stderr)
        return 2
    argv = [sys.executable, str(delegate), "--project-root", str(root), *rest]
    os.execv(sys.executable, argv)
    return 2


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=f"Python Patch Tool v{VERSION} COLLECT compatibility layer")
    ap.add_argument("--project-root", required=True)
    ap.add_argument("rest", nargs=argparse.REMAINDER)
    ns = ap.parse_args(argv)
    root = Path(ns.project_root).resolve()
    rest = list(ns.rest)
    if len(rest) != 2 or rest[0] != "request":
        return _delegate(root, rest)

    request_arg = Path(rest[1])
    request_zip = request_arg if request_arg.is_absolute() else root / request_arg
    try:
        request_data, request_member = _load_request(request_zip)
    except Exception as exc:
        print(f"ERROR: invalid COLLECT request: {exc}", file=sys.stderr)
        return 2

    if not _is_pack_only(request_data):
        return _delegate(root, rest)

    result: Path | None = None
    try:
        result = _write_pack_result(root, request_data, request_member, request_zip)
        archived = _archive_request(root, request_zip)
    except Exception as exc:
        # A COLLECT failure must not leave a result ZIP that looks uploadable.
        # If request archival fails after ZIP creation, remove that new result
        # and keep/report the failure rather than creating ambiguous evidence.
        if result is not None:
            try:
                result.unlink()
            except FileNotFoundError:
                pass
            except OSError:
                pass
        print(f"ERROR: pack collection failed: {exc}", file=sys.stderr)
        return 2

    print(f"PACK: collected {len(request_data.get('actions', []))} action(s)", flush=True)
    print(f"ZIP : {result}", flush=True)
    try:
        rel_archived = archived.relative_to(root).as_posix()
    except ValueError:
        rel_archived = str(archived)
    print(f"REQUEST : {rel_archived}", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
