#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, json, os, shutil, subprocess, sys, tempfile
sys.dont_write_bytecode = True
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent

spec=importlib.util.spec_from_file_location('ptv_health',HERE/'python_patch_health.py')
h=importlib.util.module_from_spec(spec); sys.modules[spec.name]=h; spec.loader.exec_module(h)
assert h.VERSION=='6.18.4'

# The installed source tree itself must pass its managed-file/schema audit.
root=TOOLS.parent
report=h.audit_tool(root)
assert report['status']=='PASS',report

# A corrupted managed runtime file must be detected by SHA256SUMS.
with tempfile.TemporaryDirectory(prefix='ptv614_health_corrupt_') as td:
    proj=Path(td); shutil.copytree(TOOLS,proj/'tools'); (proj/'patchs').mkdir()
    target=proj/'tools'/'_patch_lib'/'python_patch_utils.py'
    target.write_text(target.read_text(encoding='utf-8')+'\n# corruption\n',encoding='utf-8')
    bad=h.audit_tool(proj)
    assert bad['status']=='FAIL',bad
    assert any('checksum mismatch: tools/_patch_lib/python_patch_utils.py' in x for x in bad['errors']),bad

# Line selector exposes h/health without changing selection or executing work.
spec=importlib.util.spec_from_file_location('ptv_dispatcher_health',HERE/'python_patch_queue_dispatcher.py')
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)
called=[]
orig=m.print_health
m.print_health=lambda root,compact=False: called.append((Path(root),compact)) or 0
with tempfile.TemporaryDirectory(prefix='ptv614_health_line_') as td:
    proj=Path(td); (proj/'patchs').mkdir(); (proj/'patchs'/'patch.zip').write_text('x')
    old_in,old_out=m.sys.stdin,m.sys.stdout
    try:
        m.sys.stdin=io.StringIO('h\nq\n'); cap=io.StringIO(); m.sys.stdout=cap
        chosen=m._select_items_line(proj,[m.QueueItem('patch.zip','PATCH')],'none')
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    assert chosen is None
    assert called==[(proj,False)],called
    assert 'TOOL HEALTH rc=0' in cap.getvalue(),cap.getvalue()
# Fullscreen TTY h must run the same health audit and preserve selection.
class _FakeTTYIn:
    def isatty(self): return True
    def fileno(self): return 123
class _FakeTTYOut(io.StringIO):
    def isatty(self): return True
class _DummyTermios:
    TCSADRAIN=0
    @staticmethod
    def tcgetattr(fd): return ['old']
    @staticmethod
    def tcsetattr(fd,when,old): return None
class _DummyTty:
    @staticmethod
    def setcbreak(fd): return None
called=[]
m.print_health=lambda root,compact=False: called.append((Path(root),compact)) or 0
keys=iter(['h','ENTER'])
old_stdin,old_stdout=m.sys.stdin,m.sys.stdout
old_termios,old_tty,old_read=m.termios,m.tty,m._read_key
try:
    m.sys.stdin=_FakeTTYIn(); m.sys.stdout=_FakeTTYOut(); m.termios=_DummyTermios; m.tty=_DummyTty
    m._read_key=lambda fd: next(keys)
    chosen=m.select_items(Path('.'),[m.QueueItem('patch.zip','PATCH')],initial_selection='all',selector_ui='auto')
finally:
    m.sys.stdin,m.sys.stdout=old_stdin,old_stdout; m.termios,m.tty,m._read_key=old_termios,old_tty,old_read
assert [x.name for x in chosen]==['patch.zip'],chosen
assert called==[(Path('.'),False)],called
m.print_health=orig

# Empty zero-argument queue automatically shows one compact health summary.
with tempfile.TemporaryDirectory(prefix='ptv614_health_idle_') as td:
    proj=Path(td); shutil.copytree(TOOLS,proj/'tools'); (proj/'tools'/'run_python_patches.sh').chmod(0o755); (proj/'patchs').mkdir()
    env=dict(os.environ); env.pop('PYTHONDONTWRITEBYTECODE',None)
    cp=subprocess.run([str(proj/'tools'/'run_python_patches.sh')],cwd=proj,text=True,capture_output=True,env=env,timeout=30)
    assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert 'AUTO STATUS: IDLE' in cp.stdout,cp.stdout
    assert 'TOOL HEALTH: PASS' in cp.stdout,cp.stdout
    assert not list((proj/'tools').rglob('__pycache__')),list((proj/'tools').rglob('__pycache__'))
    assert not list((proj/'tools').rglob('*.pyc')),list((proj/'tools').rglob('*.pyc'))

print('PASS: v6.18.4 zero-argument Tool Health self-audit detects install corruption and never executes PATCH')
