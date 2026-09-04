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
for task in ['D1','D2']:
    assert f'| {task} |' in impl,task
assert 'mục 2→11' in impl or 'mục **2→11' in impl
assert 'COMPLETE. DỪNG.' in impl and 'Không tự bắt đầu task/tính năng tiếp theo' in impl
for phrase in ['Metadata-driven rollback','Tool Health','Full self-contained runtime','Duplicate current-session']:
    assert phrase in features,phrase
assert patch_schema['tool_version']=='6.14.0' and patch_schema['schema_version']==1
recovery=patch_schema['manifest']['fields']['recovery']
assert 'rollback' in recovery['allowed_fields'] and 'rollback' in recovery['fields']
rb=recovery['fields']['rollback']; assert set(rb['allowed_fields'])=={'targets','on','max_total_bytes'}
assert collect_schema['tool_version']=='6.14.0'
for phrase in ['PATCH_PACKAGE_SCHEMA.json','PREFLIGHT FAIL','FAIL_HANDOFF','LAST_RUN.json','COLLECT QUALITY','Optional safe rollback','Tool Health']:
    assert phrase in ai,phrase
for phrase in ['Metadata-driven safe rollback','recovery.rollback.targets','payload_failure','post_patch_failure','ROLLBACK: PASS']:
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
print('PASS: v6.14.0 ordered remaining-scope tracker, rollback/health AI docs and minimal user HTML contract')
