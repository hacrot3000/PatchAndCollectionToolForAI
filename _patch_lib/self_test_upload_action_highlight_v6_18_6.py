#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, os, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent

def load(name: str, file: str):
    spec=importlib.util.spec_from_file_location(name,HERE/file)
    mod=importlib.util.module_from_spec(spec); sys.modules[name]=mod; assert spec.loader; spec.loader.exec_module(mod)
    return mod

q=load('ptv_upload_highlight_dispatcher','python_patch_queue_dispatcher.py')
c=load('ptv_upload_highlight_collect','python_patch_collect_progress_v6_7.py')
assert q.VERSION=='6.18.6' and c.VERSION=='6.18.6'

class FakeTTY(io.StringIO):
    def isatty(self): return True

# FAIL_HANDOFF / PATCH upload action: PRIMARY, ACTION REQUIRED, and exact path
# must all be high-contrast yellow on a real TTY. The path stays underlined.
buf=FakeTTY()
q._print_upload_action_block('/tmp/FAIL_HANDOFF_patch_example.zip',patch_failure=True,stream=buf)
out=buf.getvalue()
assert '!!! [PRIMARY - UPLOAD THIS FILE] PATCH FAIL HANDOFF !!!' in out,out
assert '>>> ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER <<<' in out,out
assert '/tmp/FAIL_HANDOFF_patch_example.zip' in out,out
assert out.count('\x1b[1;30;103m') >= 2,out
assert '\x1b[1;4;30;103m/tmp/FAIL_HANDOFF_patch_example.zip\x1b[0m' in out,out

# NO_COLOR/plain output remains authoritative and copyable.
old_no_color=os.environ.get('NO_COLOR')
try:
    os.environ['NO_COLOR']='1'
    plain=FakeTTY(); q._print_upload_action_block('/tmp/plain.zip',patch_failure=True,stream=plain)
finally:
    if old_no_color is None: os.environ.pop('NO_COLOR',None)
    else: os.environ['NO_COLOR']=old_no_color
plain_out=plain.getvalue()
assert '\x1b[' not in plain_out,plain_out
assert 'ACTION REQUIRED' in plain_out and '/tmp/plain.zip' in plain_out,plain_out

# COLLECT success uses the same yellow-background action hierarchy.
with tempfile.TemporaryDirectory(prefix='ptv_upload_highlight_') as td:
    root=Path(td)
    result=root/'artifacts'/'patch_tool_code_collections'/'result.zip'
    result.parent.mkdir(parents=True,exist_ok=True)
    with zipfile.ZipFile(result,'w') as zf:
        zf.writestr('COLLECTION_MANIFEST.json','{}')
    old_stdout=c.sys.stdout
    try:
        tty=FakeTTY(); c.sys.stdout=tty
        ok=c._print_collect_success(root,[f'ZIP : {result}'])
    finally:
        c.sys.stdout=old_stdout
    cout=tty.getvalue(); assert ok is True,cout
    assert cout.count('\x1b[1;30;103m') >= 2,cout
    assert f'\x1b[1;4;30;103m{result}\x1b[0m' in cout,cout
    assert 'ACTION REQUIRED' in cout,cout

print('PASS: v6.18.6 upload-required highlight contract for FAIL_HANDOFF and COLLECT')
