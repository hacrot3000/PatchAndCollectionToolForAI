#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, sys, tempfile, zipfile
from contextlib import redirect_stdout, redirect_stderr
from pathlib import Path

HERE=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('ptv_result_clarity',HERE/'python_patch_queue_dispatcher.py')
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; assert spec.loader; spec.loader.exec_module(m)
assert m.VERSION=='6.19.0'

def make_patch(path: Path) -> None:
    path.parent.mkdir(parents=True,exist_ok=True)
    with zipfile.ZipFile(path,'w') as zf:
        zf.writestr('PATCH_TOOL_MANIFEST.json','{"schema_version":1,"project":{"key":"x"},"patch":{"id":"x"}}')
        zf.writestr('patch_dummy.py','print("unused")\n')

# PASS: final summary is followed by a visually prominent patch banner near the end.
with tempfile.TemporaryDirectory(prefix='ptv6151_result_pass_') as td:
    root=Path(td); patch=root/'patchs'/'patch_visual_pass.zip'; make_patch(patch)
    original_execute=m.execute_items; original_select=m.select_items
    try:
        def fake_execute(_root, chosen, **kwargs):
            m._LAST_EXECUTION_DETAILS=[{'name':chosen[0].name,'kind':'PATCH','status':'PASS','rc':0}]
            return 0,[(chosen[0].name,0)],[],[],[]
        m.execute_items=fake_execute; m.select_items=lambda root,items,**kwargs: list(items)
        out=io.StringIO()
        with redirect_stdout(out):
            rc=m._run_queue(root)
    finally:
        m.execute_items=original_execute; m.select_items=original_select
    text=out.getvalue(); assert rc==0,text
    assert 'SUMMARY: PASS | 1 item(s) completed' in text,text
    assert '=== PATCH COMPLETED ===' in text,text
    assert 'PATCH: patch_visual_pass.zip' in text,text
    assert text.rfind('PATCH: patch_visual_pass.zip') > text.rfind('SUMMARY: PASS'),text

# FAIL: final warning banner is unmistakable and follows the ordinary summary/path output.
with tempfile.TemporaryDirectory(prefix='ptv6151_result_fail_') as td:
    root=Path(td); patch=root/'patchs'/'patch_visual_fail.zip'; make_patch(patch)
    original_execute=m.execute_items; original_select=m.select_items
    try:
        def fake_execute(_root, chosen, **kwargs):
            m._LAST_EXECUTION_DETAILS=[{'name':chosen[0].name,'kind':'PATCH','status':'FAIL','rc':7}]
            return 7,[(chosen[0].name,7)],[],[],[]
        m.execute_items=fake_execute; m.select_items=lambda root,items,**kwargs: list(items)
        out,err=io.StringIO(),io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            rc=m._run_queue(root)
    finally:
        m.execute_items=original_execute; m.select_items=original_select
    text=err.getvalue(); assert rc==7,(out.getvalue(),text)
    assert 'SUMMARY: FAIL | failed=1 | policy=continue_independent | last=patch_visual_fail.zip rc=7' in text,text
    assert '!!! PATCH FAILED !!!' in text,text
    assert 'PATCH: patch_visual_fail.zip | rc=7' in text,text
    assert text.rfind('PATCH: patch_visual_fail.zip | rc=7') > text.rfind('SUMMARY: FAIL'),text

# Real TTY style contract: FAIL is bold yellow text on a red background.
class FakeTTY(io.StringIO):
    def isatty(self): return True
buf=FakeTTY()
m._print_patch_result_banner('FAIL','patch_color.zip',rc=9,stream=buf)
colored=buf.getvalue()
assert '\x1b[1;93;41m' in colored,colored
assert '!!! PATCH FAILED !!!' in colored and 'PATCH: patch_color.zip | rc=9' in colored,colored
assert '\x1b[0m' in colored,colored

# Banner helper flushes pending stdout before writing FAIL to stderr, preserving
# handoff/path-before-banner ordering when callers combine redirected streams.
class FlushProbe(io.StringIO):
    def __init__(self): super().__init__(); self.flushed=False
    def flush(self): self.flushed=True; return super().flush()
probe=FlushProbe(); old_stdout=m.sys.stdout
try:
    m.sys.stdout=probe
    m._print_patch_result_banner('FAIL','flush_order.zip',rc=4,stream=io.StringIO())
finally:
    m.sys.stdout=old_stdout
assert probe.flushed

print('PASS: v6.19.0 final result clarity, patch highlighting and red/yellow FAIL banner contract')
