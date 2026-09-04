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

required = {
    "tools/run_python_patches.sh",
    "tools/_patch_lib/VERSION",
    "tools/_patch_lib/PACKAGE_CONTENTS.txt",
    "tools/_patch_lib/python_patch_queue_dispatcher.py",
    "tools/_patch_lib/python_patch_collect_progress_v6_7.py",
    "tools/_patch_lib/docs/AI_USAGE_CONTRACT.md",
    "tools/_patch_lib/docs/CODE_COLLECTION_GUIDE.md",
    "tools/_patch_lib/docs/PORTABLE_USAGE.md",
    "tools/_patch_lib/docs/PYTHON_PATCH_TOOL_FEATURE_STATUS.md",
    "tools/_patch_lib/self_test_python_patch_tool_v6_10_1.py",
    "tools/_patch_lib/self_test_local_duplicate_v6_10_1.py",
    "tools/_patch_lib/self_test_collect_exclusivity_v6_10_1.py",
}
assert required <= set(entries), sorted(required - set(entries))

# The public entry point must stay directly executable after a clean unzip.
# v6.9.1 accidentally shipped the launcher as 0644, which can turn the normal
# ./tools/run_python_patches.sh workflow into Permission denied on a clean install.
launcher = ROOT / "tools/run_python_patches.sh"
assert launcher.stat().st_mode & 0o111, oct(launcher.stat().st_mode)

actual_files: set[str] = set()
for path in (ROOT / "tools").rglob("*"):
    if not path.is_file() or path == MANIFEST:
        continue
    if "__pycache__" in path.parts or path.suffix == ".pyc":
        continue
    actual_files.add(path.relative_to(ROOT).as_posix())

# In a clean extracted release, every shipped file must be checksummed.  After
# installation, exact private-core files and older self-tests legitimately sit
# next to this overlay; they are not part of this release and must not make the
# integrity self-test fail.
private_core_present = any(
    (HERE / name).is_file()
    for name in ("python_patch_runner.py", "python_patch_readonly_collector.py", "python_patch_utils.py")
)
if private_core_present:
    assert set(entries) <= actual_files, sorted(set(entries) - actual_files)
else:
    assert set(entries) == actual_files, {
        "missing_from_manifest": sorted(actual_files - set(entries)),
        "stale_in_manifest": sorted(set(entries) - actual_files),
    }

for rel, wanted in sorted(entries.items()):
    path = ROOT / rel
    assert path.is_file(), rel
    actual = hashlib.sha256(path.read_bytes()).hexdigest()
    assert actual == wanted, (rel, wanted, actual)

print("PASS: v6.10.1 package SHA256SUMS validates clean releases and installed overlays")
