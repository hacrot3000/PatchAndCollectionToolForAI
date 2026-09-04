#!/usr/bin/env python3
from pathlib import Path
import os, shutil, subprocess, sys
root=Path(__file__).resolve().parent
names=[
 'self_test_version_consistency_v6_14_0.py',
 'self_test_inplace_routing_v6_14_0.py',
 'self_test_queue_discovery_v6_14_0.py',
 'self_test_local_duplicate_v6_14_0.py',
 'self_test_selection_contract_v6_14_0.py',
 'self_test_collect_exclusivity_v6_14_0.py',
 'self_test_fail_fast_v6_14_0.py',
 'self_test_collect_pack_v6_14_0.py',
 'self_test_collect_progress_v6_14_0.py',
 'self_test_ai_collect_contract_v6_14_0.py',
 'self_test_docs_v6_14_0.py',
 'self_test_self_contained_v6_14_0.py',
 'self_test_patch_preflight_v6_14_0.py',
 'self_test_safe_rollback_v6_14_0.py',
 'self_test_patch_recovery_v6_14_0.py',
 'self_test_inspect_quality_v6_14_0.py',
 'self_test_tool_health_v6_14_0.py',
 'self_test_package_checksums_v6_14_0.py',
]
env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
timeout_bin=shutil.which('timeout')
for name in names:
    print(f'RUNNING: {name}',flush=True)
    test=[sys.executable,'-S',str(root/name)]
    if timeout_bin:
        cmd=[timeout_bin,'--kill-after=5s','90s',*test]
        rc=subprocess.run(cmd,cwd=root,env=env).returncode
        if rc==124:
            print(f'FAIL: self-test timeout: {name}',file=sys.stderr,flush=True); raise SystemExit(124)
    else:
        try: rc=subprocess.run(test,cwd=root,env=env,timeout=90).returncode
        except subprocess.TimeoutExpired:
            print(f'FAIL: self-test timeout: {name}',file=sys.stderr,flush=True); raise SystemExit(124)
    if rc: raise SystemExit(rc)
print('PASS: Python Patch Tool v6.14.0 full self-contained regression suite')
