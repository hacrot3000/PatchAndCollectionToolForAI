#!/usr/bin/env python3
from pathlib import Path
root=Path(__file__).resolve().parent; tools=root.parent
version=(root/'VERSION').read_text(encoding='utf-8').strip(); assert version=='6.17.11',version
for rel in ['python_patch_queue_dispatcher.py','python_patch_collect_progress_v6_7.py','python_patch_collect_compat.py','python_patch_collect_regex_worker.py','python_patch_collect_schema.py','python_patch_runner.py','python_patch_utils.py','python_patch_package_schema.py','python_patch_health.py','python_patch_batch.py','python_patch_project_state.py']:
    text=(root/rel).read_text(encoding='utf-8'); assert 'VERSION = "6.17.11"' in text,(rel,version)
launcher=(tools/'run_python_patches.sh').read_text(encoding='utf-8'); assert 'v6.17.11' in launcher
ps=(tools/'run_python_patches.ps1').read_text(encoding='utf-8'); assert 'v6.17.11' in ps
bat=(tools/'run_python_patches.bat').read_text(encoding='utf-8'); assert 'v6.17.11' in bat
master=(root/'self_test_python_patch_tool_v6_17_11.py').read_text(encoding='utf-8')
for name in [
 'self_test_windows_launchers_v6_17_11.py',
 'self_test_windows_native_lane_v6_17_11.py',
 'self_test_collect_progress_v6_17_11.py','self_test_local_duplicate_v6_17_11.py','self_test_result_clarity_v6_17_11.py','self_test_collect_exclusivity_v6_17_11.py',
 'self_test_batch_reporting_v6_17_11.py',
 'self_test_batch_engine_v6_17_11.py',
 'self_test_preflight_continuation_v6_17_11.py',
 'self_test_recovery_menu_v6_17_11.py',
 'self_test_history_live_status_v6_17_11.py',
 'self_test_recovery_integrity_v6_17_11.py',
 'self_test_execution_audit_v6_17_11.py',
 'self_test_collect_pack_v6_17_11.py','self_test_self_contained_v6_17_11.py','self_test_docs_v6_17_11.py',
 'self_test_patch_preflight_v6_17_11.py','self_test_safe_rollback_v6_17_11.py','self_test_patch_recovery_v6_17_11.py','self_test_fail_handoff_sources_v6_17_11.py','self_test_inspect_quality_v6_17_11.py','self_test_tool_health_v6_17_11.py','self_test_robustness_v6_17_11.py','self_test_audit_fixes_v6_17_11.py','self_test_integrity_v6_17_11.py','self_test_planning_features_v6_17_11.py']:
    assert name in master,name
dispatcher=(root/'python_patch_queue_dispatcher.py').read_text(encoding='utf-8')
assert '_acquire_project_queue_lock' not in dispatcher and '.ptv_queue.lock' not in dispatcher
for root_doc in ['implementing.md','PYTHON_PATCH_TOOL_FEATURES_VI.md','HUONG_DAN_PYTHON_PATCH_TOOL.html']:
    assert (tools/root_doc).is_file(),root_doc
assert (root/'docs'/'PATCH_PACKAGE_SCHEMA.json').is_file()
assert (root/'docs'/'PATCH_PACKAGE_GUIDE.md').is_file()
# Historical version references are allowed in changelog/task text; runtime/version/schema markers above must be current.
print('PASS: v6.17.11 runtime/docs/version/master coverage synchronized')

assert (tools/'run_windows_native_tests.ps1').is_file()

assert (root/'python_patch_ops_worker.py').is_file()

assert (root/'python_patch_collect_regex_worker.py').is_file()
