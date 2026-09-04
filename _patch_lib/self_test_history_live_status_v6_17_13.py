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

assert m.VERSION == '6.17.13'

# Zero-argument detection ignores the internal project-root handoff only.
assert m._is_zero_argument_dispatch([])
assert m._is_zero_argument_dispatch(['--project-root', '/tmp/example'])
assert m._is_zero_argument_dispatch(['--project-root=/tmp/example'])
assert not m._is_zero_argument_dispatch(['run'])
assert not m._is_zero_argument_dispatch(['--project-root', '/tmp/example', 'report'])

# User-facing history hides empty IDLE probes and renders useful package-first
# rows: package name -> time -> status.
entries = [
    (Path('idle.json'), {'run_id': 'idle-newest', 'status': 'IDLE', 'selected': [], 'started_at':'2026-08-10T06:48:46'}),
    (Path('pass.json'), {'run_id': 'pass-recent', 'status': 'PASS', 'selected': ['p.zip'], 'started_at':'2026-08-10T06:32:43'}),
    (Path('fail.json'), {'run_id': 'fail-old', 'status': 'FAIL', 'selected': ['q.zip'], 'started_at':'2026-08-10T06:20:10'}),
]
assert m._history_default_index(entries) == 1
assert m._history_row_text(entries[1][1]) == 'p.zip | 2026-08-10 06:32:43 | PASS'

# IDLE updates LAST_RUN but must not create another history/*.json entry.
with tempfile.TemporaryDirectory() as td:
    root = Path(td)
    history = root/'artifacts'/'patch_tool'/'history'; history.mkdir(parents=True)
    (history/'old_idle.json').write_text('{"run_id":"old-idle","status":"IDLE","selected":[]}', encoding='utf-8')
    (history/'pass.json').write_text('{"run_id":"pass","status":"PASS","selected":["good.zip"],"started_at":"2026-08-10T06:32:43"}', encoding='utf-8')
    assert [r.get('run_id') for _p,r in m._visible_history_entries(root)] == ['pass']
    before = sorted(x.name for x in history.glob('*.json'))
    m._write_run_report(root, {'run_id':'idle-new','status':'IDLE','selected':[],'results':[],'started_at':'2026-08-10T07:00:00'})
    after = sorted(x.name for x in history.glob('*.json'))
    assert after == before, (before, after)
    assert m._load_previous_run(root).get('run_id') == 'idle-new'
    assert m._latest_meaningful_run(root, m._load_previous_run(root)).get('run_id') == 'pass'

    cleanup = m._cleanup_history(root)
    assert cleanup['remaining'] == 1, cleanup
    assert not (history/'old_idle.json').exists()

    # An older unresolved failure remains a planner constraint but must not
    # redefine the latest meaningful execution for automatic SMART RESUME.
    registry = root/'artifacts'/'patch_tool'/'UNRESOLVED_FAILURES.json'
    registry.write_text('{"format":"python-patch-tool-unresolved-failures","entries":[{"resolved":false,"row":{"name":"old_fail.zip","kind":"PATCH","status":"FAIL"}}]}', encoding='utf-8')
    latest_meaningful = m._latest_meaningful_run(root, m._load_previous_run(root))
    assert latest_meaningful.get('status') == 'PASS', latest_meaningful
    planning_previous = m._planning_previous(root, m._load_previous_run(root))
    assert planning_previous and planning_previous.get('status') == 'FAIL' and planning_previous.get('source') == 'UNRESOLVED_FAILURES.json'

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

with tempfile.TemporaryDirectory() as td:
    root = Path(td)
    result_zip = root/'artifacts'/'patch_tool_code_collections'/'RESULT.zip'
    request_zip = root/'patchs'/'patched'/'CODE_COLLECTION_REQUEST_demo.zip'
    handoff_zip = root/'artifacts'/'patch_tool'/'fail_handoffs'/'FAIL_HANDOFF_demo.zip'
    detail_log = root/'artifacts'/'patch_tool'/'runs'/'run1'/'items'/'002.log'
    for path in (result_zip, request_zip, handoff_zip, detail_log):
        path.parent.mkdir(parents=True, exist_ok=True); path.write_bytes(b'x')
    report = {'run_id':'run1','status':'FAIL','failure_policy':'continue_independent','transaction_policy':'patch',
              'batch_log':'artifacts/patch_tool/runs/run1/batch.log','batch_report_dir':'artifacts/patch_tool/runs/run1',
              'results':[
                  {'name':'CODE_COLLECTION_REQUEST_demo.zip','kind':'COLLECT','status':'PASS','rc':0,'collect_result':{'result_zip':str(result_zip),'request_archive':'patchs/patched/CODE_COLLECTION_REQUEST_demo.zip'}},
                  {'name':'bad_patch.zip','kind':'PATCH','status':'FAIL','rc':2,'fail_handoff':'artifacts/patch_tool/fail_handoffs/FAIL_HANDOFF_demo.zip','log_path':'artifacts/patch_tool/runs/run1/items/002.log'}]}
    buf = io.StringIO(); m._print_batch_overview(root, report, stream=buf); text = buf.getvalue()
    assert 'Important files:' in text
    assert f'COLLECT result  : {result_zip.absolute()}' in text
    assert f'Request archive : {request_zip.absolute()}' in text
    assert f'FAIL handoff    : {handoff_zip.absolute()}' in text
    assert f'Detail log      : {detail_log.absolute()}' in text
    assert f'Aggregate log: {(root/"artifacts/patch_tool/runs/run1/batch.log").absolute()}' in text
    request_zip.unlink(); buf = io.StringIO(); m._print_batch_overview(root, report, stream=buf)
    assert f'Request archive : {request_zip.absolute()} [missing]' in buf.getvalue()

# Source-level orchestration contract: zero-argument interactive queue exposes
# HISTORY, an empty queue automatically opens it after health, and batch status
# transitions are wired to the fixed panel.
src = (HERE/'python_patch_queue_dispatcher.py').read_text(encoding='utf-8')
for needle in [
    'show_history=zero_argument_invocation',
    'AUTO STATUS: IDLE — no runnable patch/collect package is waiting in patchs/.',
    'QUEUE CLEANUP SUMMARY — queue ban đầu có package nhưng tất cả đã bị duplicate/auto-filter.',
    'H. [HISTORY] Xem lại lịch sử chạy gần đây',
    '_LivePatchStatus.start_for(chosen)',
    'live_status.set_status(item.name, "RUNNING")',
    'live_status.set_status(item.name, "PASS" if rc == 0 else "FAIL")',
    'PTV_DISABLE_LIVE_STATUS',
    'should_auto_resume = isinstance(meaningful_previous, dict) and meaningful_previous.get("status") == "FAIL"',
    'SMART RESUME — LẦN CHẠY CÓ CÔNG VIỆC GẦN NHẤT CÓ PATCH LỖI',
    'Menu: 1..{len(rows)}=detail',
]:
    assert needle in src, needle

print('PASS: v6.17.13 zero-argument HISTORY browser, archived artifact detail and best-effort fixed live PATCH status header')
