#!/usr/bin/env python3
from __future__ import annotations
import hashlib
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
MANIFEST = HERE / "SHA256SUMS"

assert MANIFEST.is_file(), MANIFEST
entries: dict[str, str] = {}
for line in MANIFEST.read_text(encoding="utf-8").splitlines():
    if not line.strip():
        continue
    digest, rel = line.split(None, 1)
    rel = rel.strip().lstrip("*")
    assert len(digest) == 64 and all(c in "0123456789abcdef" for c in digest), line
    assert rel not in entries, rel
    entries[rel] = digest

expected: set[str] = set()
for path in (ROOT / "tools").rglob("*"):
    if not path.is_file() or path == MANIFEST:
        continue
    if "__pycache__" in path.parts or path.suffix == ".pyc":
        continue
    expected.add(path.relative_to(ROOT).as_posix())

assert set(entries) == expected, {
    "missing_from_manifest": sorted(expected - set(entries)),
    "stale_in_manifest": sorted(set(entries) - expected),
}
for rel, wanted in sorted(entries.items()):
    actual = hashlib.sha256((ROOT / rel).read_bytes()).hexdigest()
    assert actual == wanted, (rel, wanted, actual)

print("PASS: v6.7.9 package SHA256SUMS covers every shipped file and all hashes match")
