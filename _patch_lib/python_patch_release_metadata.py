#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
from pathlib import Path
import re

try:
    from python_patch_version import VERSION
except ImportError:
    VERSION = "unknown"

MANAGED_BEGIN = "--- BEGIN GENERATED MANAGED FILE INDEX ---"
MANAGED_END = "--- END GENERATED MANAGED FILE INDEX ---"
REPO_ONLY_REL = frozenset({"README.md", "self-install-and-update.sh"})
REPO_ONLY_TOP_LEVEL = frozenset({".github", ".ptv_work"})


def _is_cache(path: Path) -> bool:
    return "__pycache__" in path.parts or path.suffix == ".pyc"


def _is_repo_metadata(path: Path, tool_dir: Path) -> bool:
    try:
        rel_parts = path.relative_to(tool_dir).parts
    except ValueError:
        return True
    return ".git" in rel_parts or bool(rel_parts and rel_parts[0] in REPO_ONLY_TOP_LEVEL)


def collect_managed_files(tool_dir: Path) -> list[Path]:
    tool_dir = tool_dir.resolve(strict=True)
    out: list[Path] = []
    for path in tool_dir.rglob("*"):
        rel = path.relative_to(tool_dir).as_posix()
        if rel in REPO_ONLY_REL or _is_cache(path) or _is_repo_metadata(path, tool_dir):
            continue
        if rel == "_patch_lib/SHA256SUMS":
            continue
        try:
            if path.is_symlink() or not path.is_file():
                continue
        except OSError:
            continue
        out.append(path)
    return sorted(out, key=lambda p: p.relative_to(tool_dir).as_posix())


def managed_relpaths(tool_dir: Path) -> list[str]:
    tool_dir = tool_dir.resolve(strict=True)
    return ["tools/" + p.relative_to(tool_dir).as_posix() for p in collect_managed_files(tool_dir)]


def refresh_package_contents(tool_dir: Path) -> None:
    tool_dir = tool_dir.resolve(strict=True)
    lib = tool_dir / "_patch_lib"
    path = lib / "PACKAGE_CONTENTS.txt"
    text = path.read_text(encoding="utf-8") if path.exists() else ""
    if MANAGED_BEGIN in text:
        text = text.split(MANAGED_BEGIN, 1)[0].rstrip() + "\n"
    if text:
        lines = text.splitlines()
        if lines and lines[0].startswith("Python Patch Tool v"):
            lines[0] = f"Python Patch Tool v{VERSION} portable package"
        text = "\n".join(lines).rstrip() + "\n"
    else:
        text = f"Python Patch Tool v{VERSION} portable package\n"
    # Keep old install examples current without disturbing historical prose.
    text = re.sub(r"python_patch_tool_v\d+\.\d+\.\d+\.zip", f"python_patch_tool_v{VERSION}.zip", text)
    entries = managed_relpaths(tool_dir)
    block = ["", MANAGED_BEGIN, f"# Generated from the managed tools tree for v{VERSION}.", *entries, MANAGED_END, ""]
    path.write_text(text.rstrip() + "\n" + "\n".join(block), encoding="utf-8")


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def refresh_sha256(tool_dir: Path) -> None:
    tool_dir = tool_dir.resolve(strict=True)
    manifest = tool_dir / "_patch_lib" / "SHA256SUMS"
    lines = []
    for path in collect_managed_files(tool_dir):
        rel = "tools/" + path.relative_to(tool_dir).as_posix()
        lines.append(f"{_sha256(path)}  {rel}")
    manifest.write_text("\n".join(lines) + "\n", encoding="utf-8")


def refresh_all(tool_dir: Path) -> None:
    tool_dir = tool_dir.resolve(strict=True)
    refresh_package_contents(tool_dir)
    # PACKAGE_CONTENTS changed above and is itself checksummed.
    refresh_sha256(tool_dir)


def check_all(tool_dir: Path) -> tuple[bool, list[str]]:
    tool_dir = tool_dir.resolve(strict=True)
    expected = set(managed_relpaths(tool_dir))
    manifest = tool_dir / "_patch_lib" / "SHA256SUMS"
    errors: list[str] = []
    seen: dict[str, str] = {}
    try:
        for line in manifest.read_text(encoding="utf-8").splitlines():
            if not line.strip():
                continue
            digest, rel = line.split(None, 1)
            rel = rel.strip().lstrip("*")
            if rel in seen:
                errors.append(f"duplicate checksum path: {rel}")
            seen[rel] = digest
    except Exception as exc:
        return False, [f"cannot read SHA256SUMS: {type(exc).__name__}: {exc}"]
    if set(seen) != expected:
        errors.append(f"checksum coverage mismatch missing={sorted(expected-set(seen))} stale={sorted(set(seen)-expected)}")
    for path in collect_managed_files(tool_dir):
        rel = "tools/" + path.relative_to(tool_dir).as_posix()
        wanted = seen.get(rel)
        if wanted and _sha256(path) != wanted:
            errors.append(f"checksum mismatch: {rel}")
    return not errors, errors


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description="Refresh/check deterministic Patch Tool package metadata")
    ap.add_argument("--tool-dir", default=str(Path(__file__).resolve().parent.parent))
    ap.add_argument("--check", action="store_true")
    ns = ap.parse_args(argv)
    tool_dir = Path(ns.tool_dir)
    if ns.check:
        ok, errors = check_all(tool_dir)
        if ok:
            print(f"PASS: v{VERSION} release metadata exact coverage")
            return 0
        for e in errors:
            print(f"FAIL: {e}")
        return 2
    refresh_all(tool_dir)
    print(f"REFRESHED: v{VERSION} PACKAGE_CONTENTS.txt + SHA256SUMS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
