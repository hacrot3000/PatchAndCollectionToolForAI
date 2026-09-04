#!/usr/bin/env python3
from pathlib import Path
import os, shutil, subprocess, sys
root=Path(__file__).resolve().parent
names=[
 'self_test_version_consistency_v6_20_0.py',
 'self_test_windows_launchers_v6_20_0.py',
 'self_test_windows_runtime_v6_20_0.py',
 'self_test_windows_native_lane_v6_20_0.py',
 'self_test_diagnostics_v6_20_0.py',
 'self_test_historical_diagnostics_v6_20_0.py',
 'self_test_validation_selection_v6_20_0.py',
 'self_test_public_entry_v6_20_0.py',
 'self_test_capability_disposition_v6_20_0.py',
 'self_test_inplace_routing_v6_20_0.py',
 'self_test_queue_discovery_v6_20_0.py',
 'self_test_local_duplicate_v6_20_0.py',
 'self_test_result_clarity_v6_20_0.py',
 'self_test_upload_action_highlight_v6_20_0.py',
 'self_test_copyable_upload_path_v6_20_0.py',
 'self_test_history_artifact_highlight_v6_20_0.py',
 'self_test_batch_reporting_v6_20_0.py',
 'self_test_batch_engine_v6_20_0.py',
 'self_test_preflight_continuation_v6_20_0.py',
 'self_test_recovery_menu_v6_20_0.py',
 'self_test_history_live_status_v6_20_0.py',
 'self_test_selection_contract_v6_20_0.py',
 'self_test_failed_queue_grouping_v6_20_0.py',
 'self_test_failed_queue_persistence_v6_20_0.py',
 'self_test_collect_exclusivity_v6_20_0.py',
 'self_test_fail_fast_v6_20_0.py',
 'self_test_collect_pack_v6_20_0.py',
 'self_test_collect_progress_v6_20_0.py',
 'self_test_ai_collect_contract_v6_20_0.py',
 'self_test_database_select_v6_20_0.py',
 'self_test_cleartext_companion_v6_20_0.py',
 'self_test_ai_sync_v6_20_0.py',
 'self_test_search_discovery_v6_20_0.py',
 'self_test_search_partial_timeout_v6_20_0.py',
 'self_test_find_discovery_v6_20_0.py',
 'self_test_collect_historical_actions_v6_20_0.py',
 'self_test_git_safe_v6_20_0.py',
 'self_test_manual_execution_v6_20_0.py',
 'self_test_existing_capability_hardening_v6_20_2.py',
 'self_test_historical_compatibility_v6_20_0.py',
 'self_test_portable_installer_v6_20_0.py',
 'self_test_capability_ledger_v6_20_0.py',
 'self_test_upgrade_continuity_v6_20_0.py',
 'self_test_docs_v6_20_0.py',
 'self_test_self_contained_v6_20_0.py',
 'self_test_patch_preflight_v6_20_0.py',
 'self_test_provenance_signature_v6_21_0.py',
 'self_test_safe_rollback_v6_20_0.py',
 'self_test_patch_recovery_v6_20_0.py',
 'self_test_fail_handoff_sources_v6_20_0.py',
 'self_test_inspect_quality_v6_20_0.py',
 'self_test_tool_health_v6_20_0.py',
 'self_test_robustness_v6_20_0.py',
 'self_test_audit_fixes_v6_20_0.py',
 'self_test_integrity_v6_20_0.py',
 'self_test_planning_features_v6_20_0.py',
 'self_test_contract_consistency_v6_20_0.py',
 'self_test_recovery_integrity_v6_20_0.py',
 'self_test_execution_audit_v6_20_0.py',
 'self_test_package_checksums_v6_20_0.py',
]
env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
timeout_bin=shutil.which('timeout')
# Most semantic tests are intentionally bounded to 180s. Two cumulative
# integration suites contain many independent subprocess scenarios and can
# legitimately exceed that wall-clock budget on slower disks/CI hosts even
# when every managed command remains individually bounded. Give only those
# aggregate harnesses more time so a slow host does not create a false release
# regression; product/runtime command timeouts remain unchanged.
long_tests={'self_test_batch_engine_v6_20_0.py','self_test_contract_consistency_v6_20_0.py'}
for name in names:
    print(f'RUNNING: {name}',flush=True)
    test=[sys.executable,'-S',str(root/name)]
    test_timeout=420 if name in long_tests else 180
    if timeout_bin:
        cmd=[timeout_bin,'--kill-after=5s',f'{test_timeout}s',*test]
        rc=subprocess.run(cmd,cwd=root,env=env).returncode
        if rc==124:
            print(f'FAIL: self-test timeout ({test_timeout}s): {name}',file=sys.stderr,flush=True); raise SystemExit(124)
    else:
        try: rc=subprocess.run(test,cwd=root,env=env,timeout=test_timeout).returncode
        except subprocess.TimeoutExpired:
            print(f'FAIL: self-test timeout ({test_timeout}s): {name}',file=sys.stderr,flush=True); raise SystemExit(124)
    if rc: raise SystemExit(rc)
print('PASS: Python Patch Tool v6.20.2 full self-contained regression suite')
