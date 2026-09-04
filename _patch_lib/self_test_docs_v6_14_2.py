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
collect_schema=json.loads((DOCS/'COLLECT_ACTION_SCHEMA.json').read_text(encoding='utf-8'))
for path in [TOOLS/'implementing.md',TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md',TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html',TOOLS/'run_python_patches.ps1',TOOLS/'run_python_patches.bat',DOCS/'PATCH_PACKAGE_SCHEMA.json',DOCS/'PATCH_PACKAGE_GUIDE.md',DOCS/'PORTABLE_USAGE.md']:
    assert path.is_file(),path
for task in ['WIN-1','WIN-2','WIN-3','WIN-4','WIN-5','WIN-6','WIN-7','STOP-1']:
    assert f'| {task} |' in impl,task
assert 'WINDOWS LAUNCHER SUPPORT' in impl
assert 'COMPLETE. DỪNG.' in impl and 'Không tự bắt đầu task/tính năng tiếp theo' in impl
for phrase in ['Windows CMD entry point','Windows PowerShell entry point','Python 3.10+','Windows/non-TTY line selector','Tool Health']:
    assert phrase in features,phrase
assert patch_schema['tool_version']=='6.14.2' and patch_schema['schema_version']==1
recovery=patch_schema['manifest']['fields']['recovery']
assert 'rollback' in recovery['allowed_fields'] and 'rollback' in recovery['fields']
rb=recovery['fields']['rollback']; assert set(rb['allowed_fields'])=={'targets','on','max_total_bytes'}
assert collect_schema['tool_version']=='6.14.2'
for phrase in ['tools\\run_python_patches.bat','.\\tools\\run_python_patches.ps1','Python 3.10+','Windows portability boundary','PATCH_PACKAGE_SCHEMA.json','FAIL_HANDOFF','Tool Health']:
    assert phrase in ai,phrase
for phrase in ['tools\\run_python_patches.bat','.\\tools\\run_python_patches.ps1','Expand-Archive','Python 3.10+','line selector']:
    assert phrase in portable,phrase
for phrase in ['Windows `.bat` + PowerShell launcher','Windows launchers included in Tool Health/SHA256 coverage']:
    assert phrase in status,phrase
for phrase in ['Metadata-driven safe rollback','recovery.rollback.targets','payload_failure','post_patch_failure','ROLLBACK: PASS','Rollback path/runtime safety','Exact input lifecycle']:
    assert phrase in patch_guide,phrase
# User HTML stays minimal: Windows install/run is now user-facing, but internal schemas/action tables/rollback mechanics remain hidden.
assert 'Action COLLECT' not in html and '<table' not in html.lower()
assert 'PATCH_PACKAGE_SCHEMA' not in html and 'COLLECT_ACTION_SCHEMA' not in html
assert 'recovery.rollback' not in html and 'payload_failure' not in html
assert '[PRIMARY - UPLOAD THIS FILE] PATCH FAIL HANDOFF' in html
assert 'Tool Health' in html and '<code>h</code>' in html
assert 'Python <strong>3.10+</strong>' in html
assert 'tools\\run_python_patches.bat' in html and '.\\tools\\run_python_patches.ps1' in html
assert 'Công cụ này làm được gì?' in html and 'Quy trình thông thường khi dùng cùng Chat AI' in html
for prompt_id in ['prompt-vi','prompt-en','prompt-ru']:
    assert f'id="{prompt_id}"' in html and f"selectPrompt('{prompt_id}')" in html and f"copyPrompt('{prompt_id}',this)" in html
assert html.count('>Select all</button>')==3 and html.count('>Copy</button>')==3
print('PASS: v6.14.2 Windows launcher docs, schemas and minimal user HTML contract')
