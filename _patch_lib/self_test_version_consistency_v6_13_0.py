#!/usr/bin/env python3
from pathlib import Path
root=Path(__file__).resolve().parent; tools=root.parent
version=(root/'VERSION').read_text(encoding='utf-8').strip(); assert version=='6.13.0',version
for rel in ['python_patch_queue_dispatcher.py','python_patch_collect_progress_v6_7.py','python_patch_collect_compat.py','python_patch_collect_schema.py','python_patch_runner.py','python_patch_utils.py','python_patch_package_schema.py']:
    text=(root/rel).read_text(encoding='utf-8'); assert 'VERSION = "6.13.0"' in text,(rel,version)
launcher=(tools/'run_python_patches.sh').read_text(encoding='utf-8'); assert 'v6.13.0' in launcher
master=(root/'self_test_python_patch_tool_v6_13_0.py').read_text(encoding='utf-8')
for name in [
 'self_test_collect_progress_v6_13_0.py','self_test_local_duplicate_v6_13_0.py','self_test_collect_exclusivity_v6_13_0.py',
 'self_test_collect_pack_v6_13_0.py','self_test_self_contained_v6_13_0.py','self_test_docs_v6_13_0.py',
 'self_test_patch_preflight_v6_13_0.py','self_test_patch_recovery_v6_13_0.py','self_test_inspect_quality_v6_13_0.py']:
    assert name in master,name
dispatcher=(root/'python_patch_queue_dispatcher.py').read_text(encoding='utf-8')
assert '_acquire_project_queue_lock' not in dispatcher and '.ptv_queue.lock' not in dispatcher
for root_doc in ['implementing.md','PYTHON_PATCH_TOOL_FEATURES_VI.md','HUONG_DAN_PYTHON_PATCH_TOOL.html']:
    assert (tools/root_doc).is_file(),root_doc
assert (root/'docs'/'PATCH_PACKAGE_SCHEMA.json').is_file()
assert (root/'docs'/'PATCH_PACKAGE_GUIDE.md').is_file()
# No stale immediately previous release marker in managed runtime/docs/tests.
old_version='6.12'+'.1'
for path in tools.rglob('*'):
    if not path.is_file() or path.name=='SHA256SUMS' or '__pycache__' in path.parts or path.suffix=='.pyc': continue
    try: text=path.read_text(encoding='utf-8')
    except UnicodeDecodeError: continue
    assert old_version not in text,(path,'stale previous-version marker')
print('PASS: v6.13.0 runtime/docs/version/master coverage synchronized')
