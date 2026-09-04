#!/usr/bin/env python3
from __future__ import annotations
import ast, json, sys
from pathlib import Path

HERE=Path(__file__).resolve().parent
ROOT=HERE.parent.parent
sys.path.insert(0,str(HERE))
import python_patch_queue_dispatcher as q

assert q.VERSION=='6.18.1'

def defs(path: Path):
    tree=ast.parse(path.read_text(encoding='utf-8'))
    return {n.name for n in ast.walk(tree) if isinstance(n,(ast.FunctionDef,ast.AsyncFunctionDef,ast.ClassDef))}

qd=defs(HERE/'python_patch_queue_dispatcher.py')
for name in {
    'discover_queue','select_items','_history_browser','_resume_selection','_report_command',
    '_batch_report_menu','_create_report_support_bundle','_plan_queue','_build_batch_plan',
    '_split_local_duplicate_patches','_split_session_duplicate_patches','_move_local_duplicates_to_ignore',
    '_create_recovery_collect_request','_failed_recovery_rows','_merged_failed_recovery_rows',
    '_inspect_item','_preview_item','_validate_item','_acquire_batch_mutation_lock',
}:
    assert name in qd, f'missing established queue/report/recovery capability: {name}'

src='\n'.join(p.read_text(encoding='utf-8') for p in HERE.glob('python_patch_*.py'))
for needle in [
    'continue_independent',
    'H. [HISTORY] Xem lại lịch sử chạy gần đây',
    'QUEUE CLEANUP SUMMARY',
    'SMART RESUME',
    'UNRESOLVED_FAILURES.json',
    'PATCH_LEDGER.json',
    'FAIL_HANDOFF',
    'support ZIP',
    'PTV_DISABLE_LIVE_STATUS',
    'SEARCH_INCONSISTENCY',
]:
    assert needle in src or needle in (HERE/'python_patch_collect_compat.py').read_text(encoding='utf-8'), needle

patch=json.loads((HERE/'docs'/'PATCH_PACKAGE_SCHEMA.json').read_text(encoding='utf-8'))
manifest=patch['manifest']
legacy_manifest_fields={
    'schema_version','project','patch','compatibility','execution','batch','targets','preflight',
    'resources','validation','post_patch','on_failure','git','recovery'
}
assert legacy_manifest_fields <= set(manifest['allowed_fields'])
fields=manifest['fields']
assert {'depends_on','on_dependency_failure','previous_failure'} <= set(fields['batch']['allowed_fields'])
assert {'commands','run_when_no_changes'} <= set(fields['post_patch']['allowed_fields'])
assert {'fail_handoff','collect_on_source_drift','rollback'} <= set(fields['recovery']['allowed_fields'])
assert {'profiles'} <= set(fields['validation']['allowed_fields'])
assert {'key'} <= set(fields['project']['allowed_fields'])

collect=json.loads((HERE/'docs'/'COLLECT_ACTION_SCHEMA.json').read_text(encoding='utf-8'))
assert {'pack','overview','find','search','git'} <= set(collect['actions'])
search_fields=set(collect['actions']['search']['allowed_fields'])
assert {
    'query','regex','paths','context_lines','max_matches','backend','source_scope','filesystem',
    'respect_gitignore','follow_symlinks','must_find','diagnose_on_zero','fallback_search',
    'report_coverage','report_skipped_dirs','module_discovery','anchor_paths','expected_files'
} <= search_fields

sh=(ROOT/'tools'/'run_python_patches.sh').read_text(encoding='utf-8')
ps=(ROOT/'tools'/'run_python_patches.ps1').read_text(encoding='utf-8')
bat=(ROOT/'tools'/'run_python_patches.bat').read_text(encoding='utf-8')
assert 'health-search' in sh and 'health-search' in ps
assert 'python_patch_queue_dispatcher.py' in sh and 'python_patch_queue_dispatcher.py' in ps
assert 'powershell' in bat.lower()

print('PASS: v6.18.1 upgrade continuity guard preserves established queue/history/recovery/report/batch/schema/launcher capabilities while retaining v6.18 search additions')
