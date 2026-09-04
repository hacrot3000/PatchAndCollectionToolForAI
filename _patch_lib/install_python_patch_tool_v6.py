#!/usr/bin/env python3
"""Optional controlled installer/migration helper for Python Patch Tool.

Normal installation remains direct extraction at the project root.  This helper
exists only for the historical controlled-upgrade capability: it can back up and
remove a fixed list of obsolete Patch-Tool-managed *loose* files and optionally
create a safe current config when none exists.  It never deletes arbitrary files
and never overwrites an existing project config.
"""
from __future__ import annotations

import argparse
import json
import os
import shutil
import tempfile
from datetime import datetime, timezone
from pathlib import Path

VERSION = "6.20.0"

# Exact historical loose paths only.  Do not broaden this list to globs or
# directories: projects may legitimately have unrelated tools/* files.
LEGACY_MANAGED_LOOSE = (
    "tools/python_patch_runner.py",
    "tools/python_patch_utils.py",
    "tools/python_patch_diagnostics.py",
    "tools/python_patch_transaction.py",
    "tools/python_patch_intelligence.py",
    "tools/python_patch_identity.py",
    "tools/python_patch_commands.py",
    "tools/python_patch_selector.py",
    "tools/python_patch_source_baseline.py",
    "tools/python_patch_decompile_extractor.py",
    "tools/python_patch_code_collector.py",
    "tools/self_test_python_patch_tool_v5.py",
)

DEFAULT_CONFIG = {
    "automation": {
        "zero_argument": {
            "selection": "prompt",
            "non_interactive_confirmed": False,
            "initial_selection": "none",
            "selector_ui": "auto",
        }
    },
    "batch": {"failure_policy": "continue_independent", "transaction_policy": "patch"},
}

class InstallerError(RuntimeError):
    pass


def _inside(root: Path, path: Path) -> bool:
    try:
        path.relative_to(root)
        return True
    except ValueError:
        return False


def _safe_root(raw: str) -> Path:
    root = Path(raw).expanduser().resolve(strict=True)
    if not root.is_dir() or root.is_symlink():
        raise InstallerError("project root must be a real directory, not a symlink")
    launcher = root / "tools" / "run_python_patches.sh"
    lib = root / "tools" / "_patch_lib"
    if launcher.is_symlink() or not launcher.is_file() or lib.is_symlink() or not lib.is_dir():
        raise InstallerError("current portable Patch Tool layout is not present under project root")
    return root


def _backup_root(root: Path) -> Path:
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    base = root / "artifacts" / "patch_tool" / "installer_backups" / f"v{VERSION}_{stamp}_{os.getpid()}"
    if base.exists() or base.is_symlink():
        raise InstallerError(f"backup destination already exists: {base}")
    return base


def _atomic_write_json(path: Path, data: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=path.parent)
    tmp = Path(name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as fh:
            json.dump(data, fh, ensure_ascii=False, indent=2, sort_keys=True)
            fh.write("\n")
            fh.flush()
            try:
                os.fsync(fh.fileno())
            except OSError:
                pass
        os.replace(tmp, path)
    finally:
        try:
            tmp.unlink()
        except FileNotFoundError:
            pass


def run(root: Path, *, dry_run: bool, create_config: bool) -> dict:
    candidates: list[tuple[str, Path]] = []
    for rel in LEGACY_MANAGED_LOOSE:
        path = root / rel
        # A symlink is not a managed regular file and must never be followed or
        # removed by this compatibility helper.
        if path.is_symlink():
            raise InstallerError(f"refusing legacy managed path because it is a symlink: {rel}")
        if path.exists():
            if not path.is_file():
                raise InstallerError(f"refusing legacy managed path because it is not a regular file: {rel}")
            resolved = path.resolve(strict=True)
            if not _inside(root, resolved):
                raise InstallerError(f"legacy managed path escapes project root: {rel}")
            candidates.append((rel, path))

    config = root / ".python_patch_tool.json"
    if config.is_symlink():
        raise InstallerError("refusing .python_patch_tool.json symlink")
    create_needed = create_config and not config.exists()
    if config.exists() and not config.is_file():
        raise InstallerError(".python_patch_tool.json exists but is not a regular file")

    backup_dir = _backup_root(root) if candidates else None
    result = {
        "tool_version": VERSION,
        "dry_run": dry_run,
        "legacy_files": [rel for rel, _ in candidates],
        "backup_dir": backup_dir.relative_to(root).as_posix() if backup_dir else None,
        "config_created": False,
        "config_preserved": config.exists(),
    }
    if dry_run:
        return result

    if backup_dir is not None:
        for rel, path in candidates:
            target = backup_dir / rel
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(path, target, follow_symlinks=False)
        # Delete only after every candidate was copied successfully.
        for _rel, path in candidates:
            path.unlink()

    if create_needed:
        _atomic_write_json(config, DEFAULT_CONFIG)
        result["config_created"] = True
        result["config_preserved"] = False
    return result


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description="Optional controlled Python Patch Tool migration helper")
    ap.add_argument("--project-root", default=".")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--create-config", action="store_true", help="create safe current config only when none exists")
    ns = ap.parse_args(argv)
    try:
        root = _safe_root(ns.project_root)
        result = run(root, dry_run=ns.dry_run, create_config=ns.create_config)
    except Exception as exc:
        print(f"[PTV v{VERSION} INSTALL ERROR] {type(exc).__name__}: {exc}")
        return 2
    print(f"Python Patch Tool v{VERSION} controlled migration")
    print(f"Project root : {root}")
    print(f"Mode         : {'DRY-RUN' if ns.dry_run else 'APPLY'}")
    print(f"Legacy files : {len(result['legacy_files'])}")
    for rel in result["legacy_files"]:
        print(f"  - {rel}")
    if result["backup_dir"]:
        print(f"Backup       : {result['backup_dir']}")
    if result["config_created"]:
        print("Config       : created .python_patch_tool.json with safe prompt defaults")
    elif result["config_preserved"]:
        print("Config       : preserved existing .python_patch_tool.json")
    elif ns.create_config and ns.dry_run:
        print("Config       : would create .python_patch_tool.json")
    print("RESULT       : PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
