#!/usr/bin/env python3
from pathlib import Path
HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent
impl=(TOOLS/'implementing.md').read_text(encoding='utf-8')
features=(TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md').read_text(encoding='utf-8')
html=(TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html').read_text(encoding='utf-8')
ai=(HERE/'docs'/'AI_USAGE_CONTRACT.md').read_text(encoding='utf-8')
guide=(HERE/'docs'/'CODE_COLLECTION_GUIDE.md').read_text(encoding='utf-8')
schema=(HERE/'docs'/'COLLECT_ACTION_SCHEMA.json').read_text(encoding='utf-8')

for path in [TOOLS/'implementing.md',TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md',TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html']:
    assert path.is_file(),path

assert 'STOP. Chờ người dùng xác nhận' in impl
for task in ['DOC-UX-1','DOC-UX-2','DOC-UX-3','DOC-AI-1','DUP-1','COLLECT-1','COLLECT-2','CORE-1','STOP-1']:
    assert task in impl,task
assert '**COMPLETE**' in impl
assert 'cập nhật ở mỗi release' in features
assert 'Duplicate current session' in features
assert 'Không tự đoán/alias action' in features
assert 'Full self-contained runtime' in features

# User guide: no internal Action COLLECT section/table; user only sees the workflow.
assert 'Action COLLECT' not in html
assert '<table' not in html.lower()
assert 'Người dùng không cần tự chọn kiểu thu thập' in html
assert 'Công cụ này làm được gì?' in html
assert 'Quy trình thông thường khi dùng cùng Chat AI' in html
assert 'tools/_patch_lib/docs/' in html
assert 'Tiếng Việt' in html and 'English' in html and 'Русский' in html
assert 'CODE_COLLECTION_REQUEST' in html and './tools/run_python_patches.sh' in html

# Each of the three prompt blocks must have Select all + Copy controls.
for prompt_id in ['prompt-vi','prompt-en','prompt-ru']:
    assert f'id="{prompt_id}"' in html,prompt_id
    assert f"selectPrompt('{prompt_id}')" in html,prompt_id
    assert f"copyPrompt('{prompt_id}',this)" in html,prompt_id
assert html.count('>Select all</button>') == 3
assert html.count('>Copy</button>') == 3
assert 'navigator.clipboard' in html and "document.execCommand('copy')" in html
assert 'function selectPrompt' in html and 'function copyPrompt' in html

# AI-facing docs keep the technical schema and explicitly keep it out of user HTML.
assert 'User-guide boundary' in ai
assert 'must **not** expose or require the user to understand the internal COLLECT action list/schema' in ai
assert 'AI/tool-facing technical document' in guide
for action in ['"pack"','"overview"','"find"','"search"','"git"']:
    assert action in schema,action

print('PASS: v6.12.1 implementing/AI-doc/user-HTML workflow and prompt-control contract')
