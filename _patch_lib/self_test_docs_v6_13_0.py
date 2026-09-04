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
for task in ['A1','A2','A3','A4','B1','B2','B3','B4','C1','C2','C3','C4']:
    assert f'| {task} |' in impl,task
assert 'COMPLETE. DỪNG.' in impl and 'Không tự bắt đầu task/tính năng tiếp theo' in impl
for phrase in ['Exact machine-readable `PATCH_PACKAGE_SCHEMA.json`','Partial-modification detection','PATCH FAIL HANDOFF','Bounded local run history','COLLECT quality']:
    assert phrase in features,phrase
assert patch_schema['tool_version']=='6.13.0' and patch_schema['schema_version']==1
assert set(patch_schema['manifest']['allowed_fields']) >= {'patch','compatibility','targets','preflight','post_patch','git','recovery'}
assert collect_schema['tool_version']=='6.13.0'
for phrase in ['PATCH_PACKAGE_SCHEMA.json','PREFLIGHT FAIL','FAIL_HANDOFF','LAST_RUN.json','COLLECT QUALITY']:
    assert phrase in ai,phrase
assert 'No guessed rollback' in patch_guide
# User HTML remains minimal: no internal action/schema tables. Only the changed fail-handoff workflow was added.
assert 'Action COLLECT' not in html and '<table' not in html.lower()
assert 'PATCH_PACKAGE_SCHEMA' not in html and 'COLLECT_ACTION_SCHEMA' not in html
assert '[PRIMARY - UPLOAD THIS FILE] PATCH FAIL HANDOFF' in html
assert 'Công cụ này làm được gì?' in html and 'Quy trình thông thường khi dùng cùng Chat AI' in html
for prompt_id in ['prompt-vi','prompt-en','prompt-ru']:
    assert f'id="{prompt_id}"' in html and f"selectPrompt('{prompt_id}')" in html and f"copyPrompt('{prompt_id}',this)" in html
assert html.count('>Select all</button>')==3 and html.count('>Copy</button>')==3
print('PASS: v6.13.0 implementation tracker, AI schemas/docs and minimal user HTML contract')
