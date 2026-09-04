#!/usr/bin/env python3
from pathlib import Path
import json
HERE=Path(__file__).resolve().parent; TOOLS=HERE.parent; DOCS=HERE/'docs'
impl=(TOOLS/'implementing.md').read_text(encoding='utf-8')
features=(TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md').read_text(encoding='utf-8')
html=(TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html').read_text(encoding='utf-8')
ai=(DOCS/'AI_USAGE_CONTRACT.md').read_text(encoding='utf-8')
portable=(DOCS/'PORTABLE_USAGE.md').read_text(encoding='utf-8')
status=(DOCS/'PYTHON_PATCH_TOOL_FEATURE_STATUS.md').read_text(encoding='utf-8')
patch_guide=(DOCS/'PATCH_PACKAGE_GUIDE.md').read_text(encoding='utf-8')
patch_schema=json.loads((DOCS/'PATCH_PACKAGE_SCHEMA.json').read_text(encoding='utf-8'))
checklist=json.loads((DOCS/'PATCH_PACKAGE_CHECKLIST.json').read_text(encoding='utf-8'))
collect_schema=json.loads((DOCS/'COLLECT_ACTION_SCHEMA.json').read_text(encoding='utf-8'))

layout=(DOCS/'LAYOUT_AND_MIGRATION.md').read_text(encoding='utf-8')
output_guide=(DOCS/'OUTPUT_FILES_GUIDE.md').read_text(encoding='utf-8')
historical_status=json.loads((DOCS/'HISTORICAL_FEATURE_STATUS_V5_15.json').read_text(encoding='utf-8'))
for path in [TOOLS/'implementing.md',TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md',TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html',TOOLS/'run_python_patches.ps1',TOOLS/'run_python_patches.bat',DOCS/'PATCH_PACKAGE_SCHEMA.json',DOCS/'PATCH_PACKAGE_CHECKLIST.json',DOCS/'PATCH_PACKAGE_GUIDE.md',DOCS/'PORTABLE_USAGE.md']:
    assert path.is_file(),path
for phrase in [
    'Continue-on-failure có kiểm soát', 'Dependency giữa các PATCH',
    'Bắt buộc successor xử lý predecessor FAIL', 'Whole-batch preflight',
    'Batch transaction / rollback policy', 'Smart resume', 'Report browser nâng cao',
    'Source before/after', 'Run history management', 'Support bundle từ report',
    'Native Windows runtime test lane', 'Static target-overlap/conflict analyzer trước execution',
    'continue_independent', 'previous_failure', 'retry_before', 'run_after', 'BLOCKED',
    'DEFERRED_AFTER_DEPENDENCY', 'transaction_policy', 'report --pin', 'support-item',
    'package_input_changed', 'Execution-byte binding', 'O_NOFOLLOW',
    'menu mũi tên', 'COLLECT source của PATCH lỗi', 'runtime failed-target overlap guard'
]:
    assert phrase in impl,phrase
for phrase in [
    'continue_independent', 'Dependency', 'Smart Resume', 'BLOCKED', 'PREFLIGHT_FAIL',
    'Source before/after unified diff', 'Support bundle ZIP', 'History list/pin/unpin/export/delete/cleanup', 'Native Windows',
    'Target-overlap', 'Local identity/provenance-light', 'Planned package SHA binding', 'Batch replay snapshot SHA/size',
    'Recovery multi-select', 'Recovery COLLECT source', 'effective target với PATCH lỗi'
]:
    assert phrase.lower() in features.lower(),phrase
assert patch_schema['tool_version']=='6.19.1' and patch_schema['schema_version']==1
assert 'batch' in patch_schema['manifest']['allowed_fields']
assert 'on_failure' in patch_schema['manifest']['allowed_fields']
on_failure_spec=patch_schema['manifest']['fields']['on_failure']; assert 'commands' in on_failure_spec['allowed_fields']
assert 'timeout_seconds' in patch_schema['manifest']['fields']['git']['allowed_fields']
batch_spec=patch_schema['manifest']['fields']['batch']
assert 'depends_on' in batch_spec['allowed_fields'] and 'previous_failure' in batch_spec['allowed_fields']
prev=batch_spec['fields']['previous_failure']
assert set(prev['fields']['action']['enum'])=={'delete','retry_before','run_after','block'}
assert checklist['tool_version']=='6.19.1' and 'READY_TO_APPLY' in checklist['result_classes']
assert collect_schema['tool_version']=='6.19.1'
for phrase in [
    'batch.previous_failure', 'retry_before', 'run_after', '`reason` là bắt buộc',
    'depends_on', 'on_dependency_failure', 'FAIL_HANDOFF',
    'tools\\run_python_patches.bat', '.\\tools\\run_python_patches.ps1',
    'package_input_changed', 'execution-integrity check only'
]:
    assert phrase in ai,phrase
for phrase in ['tools\\run_python_patches.bat','.\\tools\\run_python_patches.ps1','Python 3.10+','report --list','batch.log','NOT_EXECUTED']:
    assert phrase in portable,phrase
for phrase in ['Per-run batch summary','Interactive/reopenable `report`']:
    assert phrase in status,phrase
assert 'failure-only commands and managed script execution' in patch_guide
assert '`post_patch.run_when_no_changes`' in patch_guide and 'descriptive, not implicit runtime gates' in patch_guide
assert 'post_patch.run_when_no_changes' in ai and 'descriptive metadata' in ai
assert 'manifest.on_failure.commands' in ai and 'non-interactive' in ai
for phrase in ['Metadata-driven safe rollback','recovery.rollback.targets','Rollback path/runtime safety','package_input_changed','SHA-256 + size']:
    assert phrase in patch_guide,phrase
for phrase in ['Project Identity Guard','Trusted Validation Profiles','UNRESOLVED_FAILURES.json','Static Batch Conflict Analyzer','plan --export-recipe','PATCH_LEDGER.json','Disk/resource preflight','Queue search/filter']:
    assert phrase in impl,phrase
for phrase in ['project.key','validation_profiles','PATCH_LEDGER.json','plan','BATCH_RECIPE']:
    assert phrase in (impl+features+ai+portable),phrase
assert patch_schema['manifest']['fields']['validation']['fields']['profiles']['items']['type']=='string'
# User HTML stays minimal; only user-facing launcher/selector/validate guidance is added.
assert 'Action COLLECT' not in html and '<table' not in html.lower()
assert 'PATCH_PACKAGE_SCHEMA' not in html and 'COLLECT_ACTION_SCHEMA' not in html
assert 'recovery.rollback' not in html and 'payload_failure' not in html
assert '[PRIMARY - UPLOAD THIS FILE] PATCH FAIL HANDOFF' in html
assert 'Tool Health' in html and '<code>h</code>' in html
assert 'Python <strong>3.10+</strong>' in html
assert 'tools\\run_python_patches.bat' in html and '.\\tools\\run_python_patches.ps1' in html
assert 'validate --patch' in html and 'phím mũi tên/Space' in html
assert 'patchs/ignore/YYYY-MM-DD-' in html and 'nền đỏ/chữ vàng' in html
assert 'PASS / FAIL / BLOCKED / PREFLIGHT_FAIL / NOT_EXECUTED / SKIPPED' in html
assert 'Smart Resume' in html and 'COLLECT source của PATCH lỗi' in html and 'Space' in html and '<code>report</code>' in html and 'artifacts/patch_tool/runs/' in html
for prompt_id in ['prompt-vi','prompt-en','prompt-ru']:
    assert f'id="{prompt_id}"' in html and f"selectPrompt('{prompt_id}')" in html and f"copyPrompt('{prompt_id}',this)" in html
assert html.count('>Select all</button>')==3 and html.count('>Copy</button>')==3

for phrase in ['HISTORY','PTV_DISABLE_LIVE_STATUS','live PATCH status header']:
    assert phrase in (impl+features+portable+status),phrase
assert 'HISTORY' in html and 'AUTO STATUS: IDLE' in html and 'PTV_DISABLE_LIVE_STATUS=1' in html

protected_collect={'pack','zip','overview','ls','tree','find','search','search_files','content','research','file','range','head','tail','symbol','references','callgraph','dependencies','directory','symbol_graph','decompile','ida','ghidra','git'}
assert protected_collect <= set(collect_schema['actions'])
assert collect_schema['tool_version']=='6.19.1'
for token in ['NO_SILENT_REMOVAL_POLICY.md','CAPABILITY_LEDGER.md','search_files','symbol_graph','decompile']:
    assert token in ai or token in status or token in (DOCS/'CODE_COLLECTION_GUIDE.md').read_text(encoding='utf-8'),token


for phrase in ['install_python_patch_tool_v6.py','install_python_patch_tool_v5.py','fixed list','never overwritten']:
    assert phrase.lower() in (layout+portable+ai).lower(),phrase
assert historical_status['inventory_count']==107 and len(historical_status['complete_ids'])==95
assert set(historical_status['not_started_ids'])=={56,60,61,63,65,66}
for phrase in ['FAIL_HANDOFF','COLLECT','LAST_RUN.json','historical v5']:
    assert phrase.lower() in output_guide.lower(),phrase

print('PASS: v6.19.1 diagnostics, Windows parity docs, schemas/checklist and minimal user HTML contract')
