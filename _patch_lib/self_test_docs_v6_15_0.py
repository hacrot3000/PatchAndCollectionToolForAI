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
for phase in map(str,range(1,9)):
    assert f'| {phase} |' in impl,phase
for phrase in ['DIAGNOSTICS + WINDOWS ROBUSTNESS','COMPLETE. DỪNG.','Multi-error','SOURCE_DRIFT','Windows fullscreen']:
    assert phrase in impl,phrase
for phrase in ['Fullscreen Windows selector','Multi-error schema lint','validate --patch','OPS sequential dry-run','taskkill /T /F','PATCH_PACKAGE_CHECKLIST.json']:
    assert phrase in features,phrase
assert patch_schema['tool_version']=='6.15.0' and patch_schema['schema_version']==1
assert checklist['tool_version']=='6.15.0' and 'READY_TO_APPLY' in checklist['result_classes']
assert collect_schema['tool_version']=='6.15.0'
for phrase in ['tools\\run_python_patches.bat','.\\tools\\run_python_patches.ps1','Python 3.10+','PATCH_PACKAGE_SCHEMA.json','PATCH_PACKAGE_CHECKLIST.json','source_baseline','FAIL_HANDOFF','v6.15.0 diagnostics contract']:
    assert phrase in ai,phrase
for phrase in ['tools\\run_python_patches.bat','.\\tools\\run_python_patches.ps1','Expand-Archive','Python 3.10+','fullscreen selector','validate --patch']:
    assert phrase in portable,phrase
for phrase in ['Windows internal PATCH/COLLECT routing without Bash','Windows fullscreen selector','Multi-error manifest lint','Sequential data-only OPS dry-run']:
    assert phrase in status,phrase
for phrase in ['Metadata-driven safe rollback','recovery.rollback.targets','Rollback path/runtime safety','Exact input lifecycle','v6.15.0 package lint / validate','source_baseline','READY_TO_APPLY']:
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
for prompt_id in ['prompt-vi','prompt-en','prompt-ru']:
    assert f'id="{prompt_id}"' in html and f"selectPrompt('{prompt_id}')" in html and f"copyPrompt('{prompt_id}',this)" in html
assert html.count('>Select all</button>')==3 and html.count('>Copy</button>')==3
print('PASS: v6.15.0 diagnostics, Windows parity docs, schemas/checklist and minimal user HTML contract')
