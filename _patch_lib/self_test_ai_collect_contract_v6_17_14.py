#!/usr/bin/env python3
import json
from pathlib import Path
HERE=Path(__file__).resolve().parent; docs=HERE/'docs'
ai=(docs/'AI_USAGE_CONTRACT.md').read_text(encoding='utf-8')
portable=(docs/'PORTABLE_USAGE.md').read_text(encoding='utf-8')
guide=(docs/'CODE_COLLECTION_GUIDE.md').read_text(encoding='utf-8')
schema=json.loads((docs/'COLLECT_ACTION_SCHEMA.json').read_text(encoding='utf-8'))
package=(HERE/'PACKAGE_CONTENTS.txt').read_text(encoding='utf-8')
for text,name in [(ai,'AI_USAGE_CONTRACT.md'),(portable,'PORTABLE_USAGE.md'),(guide,'CODE_COLLECTION_GUIDE.md')]:
    assert 'CODE_COLLECTION_REQUEST' in text,name
    assert './tools/run_python_patches.sh' in text,name
assert 'run_python_patches.sh collect' not in ai.lower()
assert 'at most one `[collect]`' in ai.lower()
assert 'never mix collect and patch' in ai.lower()
assert 'no global queue/selector lock' in ai.lower()
assert 'serializes only the patch source-mutation lane per project' in ai.lower()
assert 'COLLECT_ACTION_SCHEMA.json' in ai and 'COLLECT_ACTION_SCHEMA.json' in guide
assert set(schema['actions'])=={'pack','overview','find','search','git'},schema['actions']
assert 'unsupported actions/fields become `collect invalid`' in ai.lower()
assert 'self-contained' in ai.lower() and 'private core' in ai.lower()
for name in ['python_patch_runner.py','python_patch_utils.py','python_patch_readonly_collector.py']:
    assert name in package,name
assert '[PRIMARY - UPLOAD THIS FILE]' in ai
print('PASS: v6.17.14 exact self-contained AI COLLECT/PATCH documentation contract')
