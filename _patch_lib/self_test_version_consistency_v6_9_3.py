#!/usr/bin/env python3
from pathlib import Path
root=Path(__file__).resolve().parent
version=(root/'VERSION').read_text(encoding='utf-8').strip()
assert version=='6.9.3',version
for rel in ['python_patch_queue_dispatcher.py','python_patch_collect_progress_v6_7.py']:
    text=(root/rel).read_text(encoding='utf-8')
    assert 'VERSION = "6.9.3"' in text,(rel,version)
launcher=(root.parent/'run_python_patches.sh').read_text(encoding='utf-8')
assert 'v6.9.3' in launcher
master=(root/'self_test_python_patch_tool_v6_9_3.py').read_text(encoding='utf-8')
assert 'self_test_collect_progress_v6_9_3.py' in master
assert 'self_test_local_duplicate_v6_9_3.py' in master
for path in [root/'python_patch_queue_dispatcher.py',root/'python_patch_collect_progress_v6_7.py',root.parent/'run_python_patches.sh']:
    text=path.read_text(encoding='utf-8')
    assert '6.7.6' not in text,(path,'stale 6.7.6 marker')
print('PASS: v6.9.3 executable version markers and master coverage are synchronized')
