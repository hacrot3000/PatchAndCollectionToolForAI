#!/usr/bin/env python3
"""Copy-friendly short upload aliases for Patch Tool artifacts.

The canonical artifact keeps its descriptive long filename for audit/history.
ACTION REQUIRED may additionally expose a short hard-link under
``artifacts/ptv_to_ai`` so terminal hard wrapping does not split the pathname
into separate physical lines.  Aliases are additive and never replace the
canonical artifact in metadata.
"""
from __future__ import annotations

import os
import stat
import uuid
from pathlib import Path

VERSION = "6.20.0"
_ALIAS_PARTS = ("artifacts", "ptv_to_ai")
_MAX_ALIAS_FILES = 64
_PREFIXES = {"FAIL_HANDOFF": "FH", "COLLECT": "CR", "AI_SYNC": "AS"}


def _safe_dir(root: Path) -> Path | None:
    root = Path(root).absolute()
    cur = root
    try:
        for part in _ALIAS_PARTS:
            cur = cur / part
            if cur.exists() or cur.is_symlink():
                st = os.lstat(cur)
                if stat.S_ISLNK(st.st_mode) or not stat.S_ISDIR(st.st_mode):
                    return None
            else:
                cur.mkdir(mode=0o700)
        return cur
    except OSError:
        return None


def _prefix_for(path: Path, kind: str | None) -> str:
    if kind:
        return _PREFIXES.get(str(kind).upper(), "UP")
    name = path.name.upper()
    if name.startswith("FAIL_HANDOFF_"):
        return "FH"
    if name.startswith("CODE_COLLECTION_RESULT_"):
        return "CR"
    if name.startswith("AI_TOOL_SYNC_RESULT_"):
        return "AS"
    return "UP"


def _prune(directory: Path) -> None:
    """Bound directory-entry growth; unlinking a hard-link never removes source bytes."""
    try:
        rows = []
        for entry in directory.iterdir():
            if entry.is_symlink() or not entry.is_file():
                continue
            if not entry.name.startswith(("FH_", "CR_", "AS_", "UP_")):
                continue
            try:
                rows.append((entry.stat().st_mtime_ns, entry))
            except OSError:
                pass
        rows.sort(reverse=True)
        for _, entry in rows[_MAX_ALIAS_FILES:]:
            try:
                entry.unlink()
            except OSError:
                pass
    except OSError:
        pass


def create_upload_aliases(
    root: Path,
    zip_path: Path | str,
    text_path: Path | str | None = None,
    *,
    kind: str | None = None,
) -> tuple[Path, Path | None, bool]:
    """Return short copy-friendly paths when safe hard-links can be created.

    ``used_alias`` is true only when the ZIP alias was created.  Failure is
    deliberately fail-open for presentation: callers simply print canonical
    paths and never lose an artifact because an optional alias was unavailable.
    """
    source_zip = Path(zip_path).absolute()
    source_text = Path(text_path).absolute() if text_path is not None else None
    directory = _safe_dir(Path(root))
    if directory is None:
        return source_zip, source_text, False
    try:
        if source_zip.is_symlink() or not source_zip.is_file():
            return source_zip, source_text, False
    except OSError:
        return source_zip, source_text, False

    prefix = _prefix_for(source_zip, kind)
    for _ in range(8):
        token = uuid.uuid4().hex[:8]
        alias_zip = directory / f"{prefix}_{token}{source_zip.suffix.lower() or '.zip'}"
        alias_text = directory / f"{prefix}_{token}{source_text.suffix.lower() or '.txt'}" if source_text else None
        try:
            os.link(source_zip, alias_zip)
            if source_text is not None:
                if source_text.is_symlink() or not source_text.is_file():
                    try: alias_zip.unlink()
                    except OSError: pass
                    return source_zip, source_text, False
                try:
                    os.link(source_text, alias_text)
                except OSError:
                    try: alias_zip.unlink()
                    except OSError: pass
                    continue
            _prune(directory)
            return alias_zip, alias_text, True
        except FileExistsError:
            continue
        except OSError:
            return source_zip, source_text, False
    return source_zip, source_text, False
