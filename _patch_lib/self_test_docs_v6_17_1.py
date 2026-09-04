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
for path in [TOOLS/'implementing.md',TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md',TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html',TOOLS/'run_python_patches.ps1',TOOLS/'run_python_patches.bat',DOCS/'PATCH_PACKAGE_SCHEMA.json',DOCS/'PATCH_PACKAGE_CHECKLIST.json',DOCS/'PATCH_PACKAGE_GUIDE.md',DOCS/'PORTABLE_USAGE.md']:
    assert path.is_file(),path
for phrase in [
    'Continue-on-failure có kiểm soát', 'Dependency giữa các PATCH',
    'Bắt buộc successor xử lý predecessor FAIL', 'Whole-batch preflight',
    'Batch transaction / rollback policy', 'Smart resume', 'Report browser nâng cao',
    'Source before/after', 'Run history management', 'Support bundle từ report',
    'Native Windows runtime test lane', 'NOT IMPLEMENTED — BY REQUIREMENT',
    'continue_independent', 'previous_failure', 'retry_before', 'run_after', 'BLOCKED',
    'DEFERRED_AFTER_DEPENDENCY', 'transaction_policy', 'report --pin', 'support-item'
]:
    assert phrase in impl,phrase
for phrase in [
    'continue_independent', 'Dependency', 'Smart Resume', 'BLOCKED', 'PREFLIGHT_FAIL',
    'Source before/after unified diff', 'Support bundle ZIP', 'History list/pin/unpin/export/delete/cleanup', 'Native Windows',
    'Target-overlap', 'Patch provenance'
]:
    assert phrase.lower() in features.lower(),phrase
assert patch_schema['tool_version']=='6.17.1' and patch_schema['schema_version']==1
assert 'batch' in patch_schema['manifest']['allowed_fields']
batch_spec=patch_schema['manifest']['fields']['batch']
assert 'depends_on' in batch_spec['allowed_fields'] and 'previous_failure' in batch_spec['allowed_fields']
prev=batch_spec['fields']['previous_failure']
assert set(prev['fields']['action']['enum'])=={'delete','retry_before','run_after','block'}
assert checklist['tool_version']=='6.17.1' and 'READY_TO_APPLY' in checklist['result_classes']
assert collect_schema['tool_version']=='6.17.1'
for phrase in [
    'batch.previous_failure', 'retry_before', 'run_after', '`reason` là bắt buộc',
    'depends_on', 'on_dependency_failure', 'FAIL_HANDOFF',
    'tools\\run_python_patches.bat', '.\\tools\\run_python_patches.ps1'
]:
    assert phrase in ai,phrase
for phrase in ['tools\\run_python_patches.bat','.\\tools\\run_python_patches.ps1','Python 3.10+','report --list','batch.log','NOT_EXECUTED']:
    assert phrase in portable,phrase
for phrase in ['Per-run batch summary','Interactive/reopenable `report`']:
    assert phrase in status,phrase
for phrase in ['Metadata-driven safe rollback','recovery.rollback.targets','Rollback path/runtime safety']:
    assert phrase in patch_guide,phrase
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
assert 'PASS / FAIL / BLOCKED / PREFLIGHT_FAIL / NOT_EXECUTED / SKIPPED' in html and '<code>report</code>' in html and 'artifacts/patch_tool/runs/' in html
for prompt_id in ['prompt-vi','prompt-en','prompt-ru']:
    assert f'id="{prompt_id}"' in html and f"selectPrompt('{prompt_id}')" in html and f"copyPrompt('{prompt_id}',this)" in html
assert html.count('>Select all</button>')==3 and html.count('>Copy</button>')==3
print('PASS: v6.17.1 diagnostics, Windows parity docs, schemas/checklist and minimal user HTML contract')
