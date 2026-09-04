#!/usr/bin/env python3
from pathlib import Path
import os, shutil, subprocess, sys
root=Path(__file__).resolve().parent
names=[
 'self_test_version_consistency_v6_17_10.py',
 'self_test_windows_launchers_v6_17_10.py',
 'self_test_windows_runtime_v6_17_10.py',
 'self_test_windows_native_lane_v6_17_10.py',
 'self_test_diagnostics_v6_17_10.py',
 'self_test_inplace_routing_v6_17_10.py',
 'self_test_queue_discovery_v6_17_10.py',
 'self_test_local_duplicate_v6_17_10.py',
 'self_test_result_clarity_v6_17_10.py',
 'self_test_batch_reporting_v6_17_10.py',
 'self_test_batch_engine_v6_17_10.py',
 'self_test_preflight_continuation_v6_17_10.py',
 'self_test_recovery_menu_v6_17_10.py',
 'self_test_selection_contract_v6_17_10.py',
 'self_test_collect_exclusivity_v6_17_10.py',
 'self_test_fail_fast_v6_17_10.py',
 'self_test_collect_pack_v6_17_10.py',
 'self_test_collect_progress_v6_17_10.py',
 'self_test_ai_collect_contract_v6_17_10.py',
 'self_test_docs_v6_17_10.py',
 'self_test_self_contained_v6_17_10.py',
 'self_test_patch_preflight_v6_17_10.py',
 'self_test_safe_rollback_v6_17_10.py',
 'self_test_patch_recovery_v6_17_10.py',
 'self_test_fail_handoff_sources_v6_17_10.py',
 'self_test_inspect_quality_v6_17_10.py',
 'self_test_tool_health_v6_17_10.py',
 'self_test_robustness_v6_17_10.py',
 'self_test_audit_fixes_v6_17_10.py',
 'self_test_integrity_v6_17_10.py',
 'self_test_planning_features_v6_17_10.py',
 'self_test_contract_consistency_v6_17_10.py',
 'self_test_recovery_integrity_v6_17_10.py',
 'self_test_execution_audit_v6_17_10.py',
 'self_test_package_checksums_v6_17_10.py',
]
env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
timeout_bin=shutil.which('timeout')
for name in names:
    print(f'RUNNING: {name}',flush=True)
    test=[sys.executable,'-S',str(root/name)]
    if timeout_bin:
        cmd=[timeout_bin,'--kill-after=5s','180s',*test]
        rc=subprocess.run(cmd,cwd=root,env=env).returncode
        if rc==124:
            print(f'FAIL: self-test timeout (180s): {name}',file=sys.stderr,flush=True); raise SystemExit(124)
    else:
        try: rc=subprocess.run(test,cwd=root,env=env,timeout=180).returncode
        except subprocess.TimeoutExpired:
            print(f'FAIL: self-test timeout (180s): {name}',file=sys.stderr,flush=True); raise SystemExit(124)
    if rc: raise SystemExit(rc)
print('PASS: Python Patch Tool v6.17.10 full self-contained regression suite')
