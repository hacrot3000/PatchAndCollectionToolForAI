#!/usr/bin/env python3
import io
import os
from contextlib import redirect_stdout
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
import python_patch_queue_dispatcher as d

class TTYBuffer(io.StringIO):
    def isatty(self):
        return True

with tempfile.TemporaryDirectory(prefix='ptv-history-highlight-') as td:
    root = Path(td)
    result = root / 'artifacts' / 'patch_tool_code_collections' / 'RESULT.zip'
    handoff = root / 'artifacts' / 'patch_tool' / 'fail_handoffs' / 'FAIL_HANDOFF.zip'
    request = root / 'patchs' / 'patched' / 'REQUEST.zip'
    for p in (result, handoff, request):
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_bytes(b'x')

    rows = [
        {
            'name': 'CODE_COLLECTION_REQUEST_demo.zip',
            'status': 'INCOMPLETE',
            'collect_result': {
                'result_zip': result.relative_to(root).as_posix(),
                'request_archive': request.relative_to(root).as_posix(),
            },
        },
        {
            'name': 'patch_demo.zip',
            'status': 'PREFLIGHT_FAIL',
            'fail_handoff': handoff.relative_to(root).as_posix(),
            'recovery_collect_request': 'RECOVERY_MISSING.zip',
            'preflight_log_path': 'artifacts/patch_tool/runs/demo/preflight.log',
        },
    ]
    report = {
        'status': 'INCOMPLETE',
        'selected': [r['name'] for r in rows],
        'results': rows,
        'failure_policy': 'continue_independent',
        'transaction_policy': 'patch',
    }

    tty = TTYBuffer()
    d._print_batch_overview(root, report, stream=tty)
    text = tty.getvalue()
    assert '\x1b[1;30;103mBATCH RESULT — INCOMPLETE\x1b[0m' in text, repr(text)
    assert '\x1b[1;30;103m[INCOMPLETE]\x1b[0m' in text, repr(text)
    assert '\x1b[1;93;41m[PREFLIGHT_FAIL]\x1b[0m' in text, repr(text)
    assert 'COLLECT result' in text and '\x1b[1;4;30;103m' + str(result.absolute()) in text, repr(text)
    assert 'FAIL handoff' in text and '\x1b[1;4;30;103m' + str(handoff.absolute()) in text, repr(text)
    assert 'Recovery COLLECT' in text and '\x1b[1;4;93;41m' in text and '[missing]' in text, repr(text)
    # Request/archive and ordinary logs remain readable without upload-required background.
    assert f'Request archive : {request.absolute()}' in d._ANSI_RE.sub('', text), repr(text)

    detail_tty = TTYBuffer()
    with redirect_stdout(detail_tty):
        d._print_item_detail(root, rows[1])
        d._print_item_detail(root, rows[0])
    detail_text = detail_tty.getvalue()
    assert 'FAIL handoff' in detail_text and '\x1b[1;4;30;103m' + str(handoff.absolute()) in detail_text, repr(detail_text)
    assert 'Recovery COLLECT' in detail_text and '\x1b[1;4;93;41m' in detail_text, repr(detail_text)
    assert 'COLLECT ZIP' in detail_text and '\x1b[1;4;30;103m' + str(result.absolute()) in detail_text, repr(detail_text)

    plain = io.StringIO()
    d._print_batch_overview(root, report, stream=plain)
    plain_text = plain.getvalue()
    assert '\x1b[' not in plain_text, repr(plain_text)
    assert f'COLLECT result  : {result.absolute()}' in plain_text, plain_text
    assert f'FAIL handoff    : {handoff.absolute()}' in plain_text, plain_text
    assert 'Recovery COLLECT: ' in plain_text and '[missing]' in plain_text, plain_text

    old = os.environ.get('NO_COLOR')
    os.environ['NO_COLOR'] = '1'
    try:
        no_color = TTYBuffer()
        d._print_batch_overview(root, report, stream=no_color)
        assert '\x1b[' not in no_color.getvalue(), repr(no_color.getvalue())
    finally:
        if old is None:
            os.environ.pop('NO_COLOR', None)
        else:
            os.environ['NO_COLOR'] = old

print('PASS: report/history highlights AI-upload artifacts and problem statuses without changing plain output')
