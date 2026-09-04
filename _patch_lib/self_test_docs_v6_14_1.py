#!/usr/bin/env python3
from pathlib import Path
import json
HERE=Path(__file__).resolve().parent; TOOLS=HERE.parent; DOCS=HERE/'docs'
impl=(TOOLS/'implementing.md').read_text(encoding='utf-8')
features=(TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md').read_text(encoding='utf-8')
html=(TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html').read_text(encoding='utf-8')
ai=(DOCS/'AI_USAGE_CONTRACT.md').read_text(encoding='utf-8')
patch_guide=(DOCS/'PATCH_PACKAGE_GUIDE.md').read_text(encoding='utf-8')
patch_schema=json.loads((DOCS/'PATCH_PACKAGE_SCHEMA.json').read_text(encoding='utf-8'))
collect_schema=json.loads((DOCS/'COLLECT_ACTION_SCHEMA.json').read_text(encoding='utf-8'))
for path in [TOOLS/'implementing.md',TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md',TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html',DOCS/'PATCH_PACKAGE_SCHEMA.json',DOCS/'PATCH_PACKAGE_GUIDE.md']:
    assert path.is_file(),path
for task in ['RB-1','RB-2','RB-3','SIG-1','PROC-1','LIFE-1','LIFE-2','HANDOFF-1','HEALTH-1','HEALTH-2','HEALTH-3','QUEUE-1','DUP-1','TEST-1','STOP-1']:
    assert f'| {task} |' in impl,task
assert 'ROBUSTNESS AUDIT' in impl
assert 'COMPLETE. DỪNG.' in impl and 'Không tự bắt đầu task/tính năng tiếp theo' in impl
for phrase in ['Rollback reject symlink','Tool Health','Full self-contained runtime','Duplicate current-session safe-removal','Exact PATCH input snapshot']:
    assert phrase in features,phrase
assert patch_schema['tool_version']=='6.14.1' and patch_schema['schema_version']==1
recovery=patch_schema['manifest']['fields']['recovery']
assert 'rollback' in recovery['allowed_fields'] and 'rollback' in recovery['fields']
rb=recovery['fields']['rollback']; assert set(rb['allowed_fields'])=={'targets','on','max_total_bytes'}
assert collect_schema['tool_version']=='6.14.1'
for phrase in ['PATCH_PACKAGE_SCHEMA.json','PREFLIGHT FAIL','FAIL_HANDOFF','LAST_RUN.json','COLLECT QUALITY','Optional safe rollback','Tool Health','runtime robustness invariants']:
    assert phrase in ai,phrase
for phrase in ['Metadata-driven safe rollback','recovery.rollback.targets','payload_failure','post_patch_failure','ROLLBACK: PASS','Rollback path/runtime safety','Exact input lifecycle']:
    assert phrase in patch_guide,phrase
# User HTML remains minimal: one direct health hint was necessary; no internal schemas/action tables/rollback mechanics.
assert 'Action COLLECT' not in html and '<table' not in html.lower()
assert 'PATCH_PACKAGE_SCHEMA' not in html and 'COLLECT_ACTION_SCHEMA' not in html
assert 'recovery.rollback' not in html and 'payload_failure' not in html
assert '[PRIMARY - UPLOAD THIS FILE] PATCH FAIL HANDOFF' in html
assert 'Tool Health' in html and '<code>h</code>' in html
assert 'Công cụ này làm được gì?' in html and 'Quy trình thông thường khi dùng cùng Chat AI' in html
for prompt_id in ['prompt-vi','prompt-en','prompt-ru']:
    assert f'id="{prompt_id}"' in html and f"selectPrompt('{prompt_id}')" in html and f"copyPrompt('{prompt_id}',this)" in html
assert html.count('>Select all</button>')==3 and html.count('>Copy</button>')==3
print('PASS: v6.14.1 robustness tracker, runtime-safety AI docs and minimal user HTML contract')
