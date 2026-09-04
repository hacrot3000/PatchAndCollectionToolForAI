#!/usr/bin/env python3
from pathlib import Path
HERE=Path(__file__).resolve().parent
docs=HERE/'docs'
ai=(docs/'AI_USAGE_CONTRACT.md').read_text(encoding='utf-8')
portable=(docs/'PORTABLE_USAGE.md').read_text(encoding='utf-8')
guide=(docs/'CODE_COLLECTION_GUIDE.md').read_text(encoding='utf-8')
package=(HERE/'PACKAGE_CONTENTS.txt').read_text(encoding='utf-8')
for text,name in [(ai,'AI_USAGE_CONTRACT.md'),(portable,'PORTABLE_USAGE.md'),(guide,'CODE_COLLECTION_GUIDE.md'),(package,'PACKAGE_CONTENTS.txt')]:
    assert 'ZIP' in text and 'CODE_COLLECTION_REQUEST' in text,name
    assert './tools/run_python_patches.sh' in text,name
assert 'run_python_patches.sh collect' not in ai.lower()
assert 'run_python_patches.sh collect' not in portable.lower()
assert 'exactly one' in ai.lower()
assert 'at most one' in ai.lower()
assert 'never select `[collect]` together' in ai.lower()
assert 'no project/process queue lock' in portable.lower()
assert 'do not use the old v5 action table' in guide.lower()
assert 'not authoritative' in guide.lower()
assert 'unknown action type: overview' in guide.lower()
assert 'never guess an action type' in ai.lower()
assert '[PRIMARY - UPLOAD THIS FILE]' in ai
print('PASS: v6.10.1 AI COLLECT ZIP/schema/exclusive-selection documentation contract')
