#!/usr/bin/env python3
from pathlib import Path
root=Path(__file__).resolve().parent
version=(root/'VERSION').read_text(encoding='utf-8').strip()
assert version=='6.10.0',version
for rel in ['python_patch_queue_dispatcher.py','python_patch_collect_progress_v6_7.py']:
    text=(root/rel).read_text(encoding='utf-8')
    assert 'VERSION = "6.10.0"' in text,(rel,version)
launcher=(root.parent/'run_python_patches.sh').read_text(encoding='utf-8')
assert 'v6.10.0' in launcher
master=(root/'self_test_python_patch_tool_v6_10_0.py').read_text(encoding='utf-8')
assert 'self_test_collect_progress_v6_10_0.py' in master
assert 'self_test_local_duplicate_v6_10_0.py' in master
assert 'self_test_collect_exclusivity_v6_10_0.py' in master
dispatcher=(root/'python_patch_queue_dispatcher.py').read_text(encoding='utf-8')
assert '_acquire_project_queue_lock' not in dispatcher
assert '.ptv_queue.lock' not in dispatcher
for path in [root/'python_patch_queue_dispatcher.py',root/'python_patch_collect_progress_v6_7.py',root.parent/'run_python_patches.sh']:
    text=path.read_text(encoding='utf-8')
    assert '6.7.6' not in text,(path,'stale 6.7.6 marker')
print('PASS: v6.10.0 executable version markers and master coverage are synchronized')
