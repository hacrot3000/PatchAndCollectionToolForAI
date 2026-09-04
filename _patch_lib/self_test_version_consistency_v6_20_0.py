#!/usr/bin/env python3
from pathlib import Path
root=Path(__file__).resolve().parent; tools=root.parent
version=(root/'VERSION').read_text(encoding='utf-8').strip(); assert version=='6.20.2',version
runtime_version_modules=['python_patch_queue_dispatcher.py','python_patch_collect_progress_v6_7.py','python_patch_collect_compat.py','python_patch_collect_regex_worker.py','python_patch_collect_schema.py','python_patch_git_safe.py','python_patch_manual_workflow.py','python_patch_decompile_compat.py','install_python_patch_tool_v6.py','python_patch_runner.py','python_patch_utils.py','python_patch_package_schema.py','python_patch_health.py','python_patch_batch.py','python_patch_project_state.py','python_patch_diagnostics_compat.py','python_patch_database_select.py','python_patch_cleartext_companion.py','python_patch_ai_sync.py','python_patch_upload_alias.py']
for rel in runtime_version_modules:
    text=(root/rel).read_text(encoding='utf-8')
    assert 'from python_patch_version import VERSION' in text,(rel,'missing shared version loader')
    assert 'VERSION = "6.20.2"' not in text,(rel,'release version must not be duplicated in runtime module')
assert (root/'python_patch_version.py').is_file()
assert (root/'python_patch_release_metadata.py').is_file()
launcher=(tools/'run_python_patches.sh').read_text(encoding='utf-8'); assert 'v6.20.2' in launcher
ps=(tools/'run_python_patches.ps1').read_text(encoding='utf-8'); assert 'v6.20.2' in ps
bat=(tools/'run_python_patches.bat').read_text(encoding='utf-8'); assert 'v6.20.2' in bat
master=(root/'self_test_python_patch_tool_v6_20_0.py').read_text(encoding='utf-8')
for name in [
 'self_test_windows_launchers_v6_20_0.py',
 'self_test_windows_native_lane_v6_20_0.py',
 'self_test_search_discovery_v6_20_0.py',
 'self_test_search_partial_timeout_v6_20_0.py',
 'self_test_find_discovery_v6_20_0.py',
 'self_test_database_select_v6_20_0.py',
 'self_test_cleartext_companion_v6_20_0.py',
 'self_test_ai_sync_v6_20_0.py',
 'self_test_historical_diagnostics_v6_20_0.py',
 'self_test_validation_selection_v6_20_0.py',
 'self_test_public_entry_v6_20_0.py',
 'self_test_capability_disposition_v6_20_0.py',
 'self_test_collect_historical_actions_v6_20_0.py',
 'self_test_portable_installer_v6_20_0.py',
 'self_test_upgrade_continuity_v6_20_0.py',
 'self_test_collect_progress_v6_20_0.py','self_test_local_duplicate_v6_20_0.py','self_test_result_clarity_v6_20_0.py','self_test_upload_action_highlight_v6_20_0.py','self_test_copyable_upload_path_v6_20_0.py','self_test_history_artifact_highlight_v6_20_0.py','self_test_collect_exclusivity_v6_20_0.py',
 'self_test_batch_reporting_v6_20_0.py',
 'self_test_batch_engine_v6_20_0.py',
 'self_test_preflight_continuation_v6_20_0.py',
 'self_test_recovery_menu_v6_20_0.py',
 'self_test_failed_queue_grouping_v6_20_0.py',
 'self_test_history_live_status_v6_20_0.py',
 'self_test_recovery_integrity_v6_20_0.py',
 'self_test_execution_audit_v6_20_0.py',
 'self_test_git_safe_v6_20_0.py','self_test_manual_execution_v6_20_0.py','self_test_existing_capability_hardening_v6_20_2.py',
 'self_test_collect_pack_v6_20_0.py','self_test_self_contained_v6_20_0.py','self_test_docs_v6_20_0.py',
 'self_test_patch_preflight_v6_20_0.py','self_test_safe_rollback_v6_20_0.py','self_test_patch_recovery_v6_20_0.py','self_test_fail_handoff_sources_v6_20_0.py','self_test_inspect_quality_v6_20_0.py','self_test_tool_health_v6_20_0.py','self_test_robustness_v6_20_0.py','self_test_audit_fixes_v6_20_0.py','self_test_integrity_v6_20_0.py','self_test_planning_features_v6_20_0.py']:
    assert name in master,name
dispatcher=(root/'python_patch_queue_dispatcher.py').read_text(encoding='utf-8')
assert '_acquire_project_queue_lock' not in dispatcher and '.ptv_queue.lock' not in dispatcher
for root_doc in ['implementing.md','PYTHON_PATCH_TOOL_FEATURES_VI.md','HUONG_DAN_PYTHON_PATCH_TOOL.html']:
    assert (tools/root_doc).is_file(),root_doc
assert (root/'docs'/'PATCH_PACKAGE_SCHEMA.json').is_file()
assert (root/'docs'/'PATCH_PACKAGE_GUIDE.md').is_file()
assert (root/'docs'/'GIT_SAFE_OPERATIONS.md').is_file()
assert (root/'docs'/'MANUAL_EXECUTION_WORKFLOW.md').is_file()
# Historical version references are allowed in changelog/task text; runtime/version/schema markers above must be current.
print('PASS: v6.20.2 runtime/docs/version/master coverage synchronized')

assert (tools/'run_windows_native_tests.ps1').is_file()

assert (root/'python_patch_ops_worker.py').is_file()

assert (root/'python_patch_collect_regex_worker.py').is_file()

wn=(tools/'run_windows_native_tests.ps1').read_text(encoding='utf-8'); assert 'self_test_windows_runtime_v6_20_0.py' in wn and 'self_test_windows_runtime_v6_18_2.py' not in wn

assert (root/'install_python_patch_tool_v5.py').is_file()
assert 'install_python_patch_tool_v6 import main' in (root/'install_python_patch_tool_v5.py').read_text(encoding='utf-8')
for rel in ['NO_SILENT_REMOVAL_POLICY.md','CAPABILITY_LEDGER.md','HISTORICAL_FEATURE_BASELINE_V5_15.md','HISTORICAL_FEATURE_STATUS_V5_15.json','CURRENT_CAPABILITY_DISPOSITION.json','LAYOUT_AND_MIGRATION.md','OUTPUT_FILES_GUIDE.md']:
    assert (root/'docs'/rel).is_file(),rel

assert (root/'docs'/'DATABASE_SELECT_ACTIVE_BUILDER.md').is_file()
assert (tools/'db_profiles.example.json').is_file()
schema_text=(root/'docs'/'COLLECT_ACTION_SCHEMA.json').read_text(encoding='utf-8')
assert 'database_select' in schema_text and 'raw_sql_allowed' in schema_text
master=(root/'self_test_python_patch_tool_v6_20_0.py').read_text(encoding='utf-8')
assert 'self_test_database_select_v6_20_0.py' in master

assert (root/'python_patch_cleartext_companion.py').is_file()
assert (root/'self_test_cleartext_companion_v6_20_0.py').is_file()

assert (root/'python_patch_ai_sync.py').is_file()
assert (root/'python_patch_upload_alias.py').is_file()
assert (root/'self_test_copyable_upload_path_v6_20_0.py').is_file()
assert (root/'self_test_ai_sync_v6_20_0.py').is_file()
assert 'self_test_ai_sync_v6_20_0.py' in (root/'self_test_python_patch_tool_v6_20_0.py').read_text(encoding='utf-8')

assert (root/'docs'/'AI_TOOL_SYNC_CONTRACT.md').is_file()
