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

actual_files: set[str] = set()
for path in (ROOT / "tools").rglob("*"):
    if not path.is_file() or path == MANIFEST:
        continue
    if "__pycache__" in path.parts or path.suffix == ".pyc":
        continue
    actual_files.add(path.relative_to(ROOT).as_posix())

# In a clean extraction of the release ZIP there is no private installed core,
# so enforce exact archive coverage and catch both omitted and stale manifest
# entries. After overlay installation into a real project, private core/helper
# files and older project-owned tools legitimately coexist under tools/. Those
# files are outside this release's ownership and must not make the package
# integrity self-test fail.
private_core_markers = [
    HERE / "python_patch_runner.py",
    HERE / "python_patch_readonly_collector.py",
    HERE / "python_patch_utils.py",
]
installed_overlay = any(path.exists() for path in private_core_markers)

if not installed_overlay:
    assert set(entries) == actual_files, {
        "missing_from_manifest": sorted(actual_files - set(entries)),
        "stale_in_manifest": sorted(set(entries) - actual_files),
    }
else:
    missing_managed = sorted(rel for rel in entries if not (ROOT / rel).is_file())
    assert not missing_managed, {"missing_managed_release_files": missing_managed}

for rel, wanted in sorted(entries.items()):
    path = ROOT / rel
    assert path.is_file(), rel
    actual = hashlib.sha256(path.read_bytes()).hexdigest()
    assert actual == wanted, (rel, wanted, actual)

mode = "clean-release strict coverage" if not installed_overlay else "installed-overlay managed-file verification"
print(f"PASS: v6.7.11 package SHA256SUMS ({mode})")
