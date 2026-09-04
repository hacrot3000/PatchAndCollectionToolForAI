#!/usr/bin/env python3
from pathlib import Path
HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent
impl=(TOOLS/'implementing.md').read_text(encoding='utf-8')
features=(TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md').read_text(encoding='utf-8')
html=(TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html').read_text(encoding='utf-8')
schema=(HERE/'docs'/'COLLECT_ACTION_SCHEMA.json').read_text(encoding='utf-8')
for path in [TOOLS/'implementing.md',TOOLS/'PYTHON_PATCH_TOOL_FEATURES_VI.md',TOOLS/'HUONG_DAN_PYTHON_PATCH_TOOL.html']:
    assert path.is_file(),path
assert 'STOP. Chờ người dùng xác nhận' in impl
for task in ['DOC-1','DOC-2','DOC-3','DUP-1','COLLECT-1','COLLECT-2','CORE-1']:
    assert task in impl,task
assert 'cập nhật ở mỗi release' in features
for action in ['`pack`','`overview`','`find`','`search`','`git`']:
    assert action in features,action
assert 'tools/_patch_lib/docs/' in html
assert 'Tiếng Việt' in html and 'English' in html and 'Русский' in html
assert 'CODE_COLLECTION_REQUEST' in html and './tools/run_python_patches.sh' in html
for action in ['"pack"','"overview"','"find"','"search"','"git"']:
    assert action in schema,action
print('PASS: v6.12.0 implementing/features/HTML multilingual user-document contract')
