#!/usr/bin/env python3
from __future__ import annotations

import contextlib
import io
import os
import shutil
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import python_patch_queue_dispatcher as m

assert m.VERSION == '6.17.11'

# Zero-argument detection ignores the internal project-root handoff only.
assert m._is_zero_argument_dispatch([])
assert m._is_zero_argument_dispatch(['--project-root', '/tmp/example'])
assert m._is_zero_argument_dispatch(['--project-root=/tmp/example'])
assert not m._is_zero_argument_dispatch(['run'])
assert not m._is_zero_argument_dispatch(['--project-root', '/tmp/example', 'report'])

# Opening history after an idle zero-argument probe must default to the most
# recent meaningful PASS rather than the newer empty IDLE record.
entries = [
    (Path('idle.json'), {'run_id': 'idle-newest', 'status': 'IDLE', 'selected': []}),
    (Path('pass.json'), {'run_id': 'pass-recent', 'status': 'PASS', 'selected': ['p.zip']}),
    (Path('fail.json'), {'run_id': 'fail-old', 'status': 'FAIL', 'selected': ['q.zip']}),
]
assert m._history_default_index(entries) == 1

# The live status panel is best-effort TTY presentation. Raw detail logs are
# elsewhere; terminal-destructive child ANSI is stripped only from live view.
class FakeIn:
    def isatty(self): return True

class FakeOut(io.StringIO):
    def isatty(self): return True
    def flush(self): pass

old_in, old_out = sys.stdin, sys.stdout
old_term = os.environ.get('TERM')
old_size = m.shutil.get_terminal_size
fake = FakeOut()
try:
    sys.stdin = FakeIn()
    sys.stdout = fake
    os.environ['TERM'] = 'xterm-256color'
    m.shutil.get_terminal_size = lambda fallback=(120, 40): os.terminal_size((100, 24))
    items = [m.QueueItem('patch_a.zip', 'PATCH'), m.QueueItem('patch_b.zip', 'PATCH')]
    panel = m._LivePatchStatus.start_for(items)
    assert panel.active
    assert m._ACTIVE_LIVE_STATUS_PANEL is panel
    panel.set_status('patch_a.zip', 'RUNNING')
    before = len(fake.getvalue())
    panel.write_log('\x1b[2Jchild-log\n')
    delta = fake.getvalue()[before:]
    assert 'child-log' in delta
    assert '\x1b[2J' not in delta
    panel.set_status('patch_a.zip', 'PASS')
    panel.set_status('patch_b.zip', 'FAILED')
    panel.close()
    assert m._ACTIVE_LIVE_STATUS_PANEL is None
    visible = fake.getvalue()
    assert 'patch_a.zip' in visible and 'PASS' in visible
    assert 'patch_b.zip' in visible and 'FAILED' in visible
    assert '\x1b[r' in visible  # scroll region restored on close
finally:
    sys.stdin, sys.stdout = old_in, old_out
    m.shutil.get_terminal_size = old_size
    if old_term is None: os.environ.pop('TERM', None)
    else: os.environ['TERM'] = old_term

# Redirected output must retain the historical plain-console path.
class NonTty(io.StringIO):
    def isatty(self): return False
old_in, old_out = sys.stdin, sys.stdout
try:
    sys.stdin = NonTty(); sys.stdout = NonTty()
    panel = m._LivePatchStatus.start_for([m.QueueItem('patch.zip', 'PATCH')])
    assert not panel.active
finally:
    sys.stdin, sys.stdout = old_in, old_out

# Reopened details expose archived PATCH/COLLECT artifacts through the same
# report browser data used immediately after a run.
with tempfile.TemporaryDirectory() as td:
    root = Path(td)
    (root/'patchs'/'patched').mkdir(parents=True)
    (root/'patchs'/'patched'/'ok.zip').write_bytes(b'zip')
    row = {
        'name': 'ok.zip', 'kind': 'PATCH', 'status': 'PASS', 'rc': 0,
        'collect_result': {
            'result_zip': 'artifacts/patch_tool_code_collections/RESULT.zip',
            'request_archive': 'patchs/patched/CODE_COLLECTION_REQUEST_demo.zip',
            'quality': {'files': 4, 'reports': 1, 'missing': 0, 'truncated_reports': 0},
        },
    }
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        m._print_item_detail(root, row)
    text = buf.getvalue()
    assert 'Archived pkg:' in text and 'patchs/patched/ok.zip' in text
    assert 'COLLECT ZIP' in text and 'RESULT.zip' in text
    assert 'Request ZIP' in text and 'CODE_COLLECTION_REQUEST_demo.zip' in text

# Source-level orchestration contract: zero-argument interactive queue exposes
# HISTORY, an empty queue automatically opens it after health, and batch status
# transitions are wired to the fixed panel.
src = (HERE/'python_patch_queue_dispatcher.py').read_text(encoding='utf-8')
for needle in [
    'show_history=zero_argument_invocation',
    'AUTO STATUS: IDLE — no runnable patch/collect package is waiting in patchs/.',
    'if zero_argument_invocation and sys.stdin.isatty() and sys.stdout.isatty():\n            _history_browser(root)',
    'H. [HISTORY] Xem lại lịch sử chạy gần đây',
    '_LivePatchStatus.start_for(chosen)',
    'live_status.set_status(item.name, "RUNNING")',
    'live_status.set_status(item.name, "PASS" if rc == 0 else "FAIL")',
    'PTV_DISABLE_LIVE_STATUS',
]:
    assert needle in src, needle

print('PASS: v6.17.11 zero-argument HISTORY browser, archived artifact detail and best-effort fixed live PATCH status header')
