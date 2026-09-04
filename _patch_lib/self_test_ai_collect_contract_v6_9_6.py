#!/usr/bin/env python3
from pathlib import Path

HERE=Path(__file__).resolve().parent
docs=HERE/'docs'
ai=(docs/'AI_USAGE_CONTRACT.md').read_text(encoding='utf-8')
portable=(docs/'PORTABLE_USAGE.md').read_text(encoding='utf-8')
package=(HERE/'PACKAGE_CONTENTS.txt').read_text(encoding='utf-8')

for text,name in [(ai,'AI_USAGE_CONTRACT.md'),(portable,'PORTABLE_USAGE.md'),(package,'PACKAGE_CONTENTS.txt')]:
    assert 'ZIP' in text, name
    assert 'CODE_COLLECTION_REQUEST' in text, name
    assert './tools/run_python_patches.sh' in text, name

assert 'NEVER give the user a loose `.json` COLLECT request' in portable
assert 'Do not provide a loose request `.json`' in ai
assert 'exactly one' in ai.lower()
assert 'run_python_patches.sh collect' not in ai.lower()
assert 'run_python_patches.sh collect' not in portable.lower()
assert 'must NOT tell the user' in ai
assert 'request ZIP' in ai and 'Result collection ZIP' in ai


assert '[PRIMARY - UPLOAD THIS FILE]' in ai
assert 'Destination: ChatGPT / AI server' in ai
assert 'must not replay' in ai

print('PASS: v6.9.6 AI COLLECT ZIP-only documentation contract')
