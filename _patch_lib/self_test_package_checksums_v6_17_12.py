#!/usr/bin/env python3
from __future__ import annotations
import hashlib
from pathlib import Path
HERE=Path(__file__).resolve().parent; ROOT=HERE.parents[1]; MANIFEST=HERE/'SHA256SUMS'
assert MANIFEST.is_file(),MANIFEST
entries={}
for line in MANIFEST.read_text(encoding='utf-8').splitlines():
    if not line.strip(): continue
    digest,rel=line.split(None,1); rel=rel.strip().lstrip('*')
    assert len(digest)==64 and all(c in '0123456789abcdef' for c in digest),line
    assert rel not in entries,rel
    entries[rel]=digest
required={
 'tools/run_python_patches.sh','tools/run_python_patches.ps1','tools/run_python_patches.bat','tools/run_windows_native_tests.ps1','tools/implementing.md','tools/PYTHON_PATCH_TOOL_FEATURES_VI.md','tools/HUONG_DAN_PYTHON_PATCH_TOOL.html',
 'tools/_patch_lib/VERSION','tools/_patch_lib/PACKAGE_CONTENTS.txt','tools/_patch_lib/python_patch_queue_dispatcher.py','tools/_patch_lib/python_patch_batch.py',
 'tools/_patch_lib/python_patch_runner.py','tools/_patch_lib/python_patch_ops_worker.py','tools/_patch_lib/python_patch_utils.py','tools/_patch_lib/python_patch_readonly_collector.py',
 'tools/_patch_lib/python_patch_collect_progress_v6_7.py','tools/_patch_lib/python_patch_collect_compat.py','tools/_patch_lib/python_patch_collect_regex_worker.py','tools/_patch_lib/python_patch_collect_schema.py',
 'tools/_patch_lib/python_patch_package_schema.py','tools/_patch_lib/python_patch_project_state.py','tools/_patch_lib/python_patch_health.py',
 'tools/_patch_lib/docs/COLLECT_ACTION_SCHEMA.json','tools/_patch_lib/docs/PATCH_PACKAGE_SCHEMA.json','tools/_patch_lib/docs/PATCH_PACKAGE_CHECKLIST.json','tools/_patch_lib/docs/PATCH_PACKAGE_GUIDE.md',
 'tools/_patch_lib/docs/AI_USAGE_CONTRACT.md','tools/_patch_lib/docs/CODE_COLLECTION_GUIDE.md',
 'tools/_patch_lib/self_test_self_contained_v6_17_12.py','tools/_patch_lib/self_test_docs_v6_17_12.py','tools/_patch_lib/self_test_batch_engine_v6_17_12.py','tools/_patch_lib/self_test_windows_native_lane_v6_17_12.py',
 'tools/_patch_lib/self_test_safe_rollback_v6_17_12.py','tools/_patch_lib/self_test_planning_features_v6_17_12.py','tools/_patch_lib/self_test_tool_health_v6_17_12.py','tools/_patch_lib/self_test_audit_fixes_v6_17_12.py','tools/_patch_lib/self_test_integrity_v6_17_12.py','tools/_patch_lib/self_test_recovery_integrity_v6_17_12.py','tools/_patch_lib/self_test_execution_audit_v6_17_12.py',
 'tools/_patch_lib/self_test_history_live_status_v6_17_12.py',
}
assert required<=set(entries),sorted(required-set(entries))
launcher=ROOT/'tools/run_python_patches.sh'; assert launcher.stat().st_mode & 0o111,oct(launcher.stat().st_mode)
actual=set()
for path in (ROOT/'tools').rglob('*'):
    if not path.is_file() or path==MANIFEST: continue
    if '__pycache__' in path.parts or path.suffix=='.pyc': continue
    actual.add(path.relative_to(ROOT).as_posix())
assert set(entries)==actual,{'missing_from_manifest':sorted(actual-set(entries)),'stale_in_manifest':sorted(set(entries)-actual)}
for rel,wanted in sorted(entries.items()):
    path=ROOT/rel; assert path.is_file(),rel
    got=hashlib.sha256(path.read_bytes()).hexdigest(); assert got==wanted,(rel,wanted,got)
print('PASS: v6.17.12 self-contained package SHA256SUMS exact coverage and public launchers')
