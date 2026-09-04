#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, json, os, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
MOD=HERE/'python_patch_queue_dispatcher.py'
spec=importlib.util.spec_from_file_location('ptv_recovery_menu',MOD)
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; assert spec.loader; spec.loader.exec_module(m)
assert m.VERSION=='6.17.13'

class FakeTTYIn:
    def isatty(self): return True
    def fileno(self): return 123
class FakeTTYOut(io.StringIO):
    def isatty(self): return True
class DummyTermios:
    TCSADRAIN=0
    @staticmethod
    def tcgetattr(fd): return ['old']
    @staticmethod
    def tcsetattr(fd, when, old): return None
class DummyTty:
    @staticmethod
    def setcbreak(fd): return None

# Arrow-key recovery menu + dynamic description + multi-select failed PATCHes.
with tempfile.TemporaryDirectory(prefix='ptv_recovery_ui_') as td:
    root=Path(td); (root/'patchs').mkdir()
    for name in ('p1.zip','p2.zip','p3.zip'): (root/'patchs'/name).write_text('x',encoding='utf-8')
    previous={'status':'FAIL','results':[
        {'name':'p1.zip','kind':'PATCH','status':'FAIL','diagnosis':{'kind':'source_drift'}},
        {'name':'p2.zip','kind':'PATCH','status':'FAIL','diagnosis':{'kind':'python_exception'}},
        {'name':'p3.zip','kind':'PATCH','status':'NOT_EXECUTED'},
    ]}
    items=[m.QueueItem(x,'PATCH') for x in ('p1.zip','p2.zip','p3.zip')]
    keys=iter(['DOWN','ENTER','SPACE','DOWN','SPACE','ENTER'])
    old=(m.sys.stdin,m.sys.stdout,m.termios,m.tty,m._read_key)
    try:
        m.sys.stdin=FakeTTYIn(); capture=FakeTTYOut(); m.sys.stdout=capture
        m.termios=DummyTermios; m.tty=DummyTty; m._read_key=lambda fd: next(keys)
        decision=m._resume_selection(root,items,previous)
    finally:
        m.sys.stdin,m.sys.stdout,m.termios,m.tty,m._read_key=old
    assert decision['action']=='run',decision
    assert [x.name for x in decision['items']]==['p1.zip','p2.zip'],decision
    text=m._ANSI_RE.sub('',capture.getvalue())
    assert 'MÔ TẢ MỤC ĐANG CHỌN' in text,text
    assert 'Space: chọn/bỏ' in text,text
    assert 'COLLECT source của PATCH lỗi' in text,text
    assert 'Xóa PATCH lỗi khỏi hàng đợi' in text,text

# Any failed PATCH can build and run an exact-source COLLECT; multiple selected
# failures are executed sequentially while preserving one-COLLECT-per-unit.
with tempfile.TemporaryDirectory(prefix='ptv_recovery_collect_') as td:
    root=Path(td); shutil.copytree(HERE.parent,root/'tools'); (root/'patchs').mkdir(); (root/'src').mkdir()
    (root/'src/a.py').write_text('A\n',encoding='utf-8'); (root/'src/b.py').write_text('B\n',encoding='utf-8')
    rows=[]
    for idx, rel in enumerate(('src/a.py','src/b.py'),1):
        name=f'failed_{idx}.zip'
        manifest={'schema_version':1,'patch':{'id':f'f{idx}'},'execution':{'timeout_seconds':30},'targets':[rel]}
        with zipfile.ZipFile(root/'patchs'/name,'w') as zf:
            zf.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest)); zf.writestr('apply.py','raise SystemExit(1)\n')
        sha=m._sha256_file(root/'patchs'/name)
        rows.append({'name':name,'kind':'PATCH','status':'FAIL','patch_result':{
            'patch_file':name,'patch_sha256':sha,
            'diagnosis':{'kind':'python_exception','affected_paths':[]},
            'partial_modification':{'detected':False,'changed_paths':[]},
        }})
    m._ACTIVE_RUN_ID='recovery_collect_test'
    rc, requests, executed=m._execute_failed_patch_collects(root,rows)
    assert rc==0,(rc,requests,executed,m._LAST_EXECUTION_DETAILS)
    assert len(requests)==2 and len(executed)==2,(requests,executed)
    assert all(x.get('status')=='PASS' for x in m._LAST_EXECUTION_DETAILS),m._LAST_EXECUTION_DETAILS
    assert len(list((root/'patchs/patched').glob('CODE_COLLECTION_REQUEST_failed_patch_*.zip')))==2
    results=list((root/'artifacts/patch_tool_code_collections').glob('CODE_COLLECTION_RESULT_*.zip'))
    assert len(results)==2,results
    retired,warnings=m._retire_failed_rows(root,rows)
    assert not warnings,warnings
    assert {x['source'] for x in retired}=={'failed_1.zip','failed_2.zip'},retired
    assert not (root/'patchs/failed_1.zip').exists() and not (root/'patchs/failed_2.zip').exists()
    assert len(list((root/'patchs/ignore').glob('*failed_*.zip')))>=2

# Default multi-PATCH policy continues independent work, blocks a later PATCH
# sharing an effective target with the failed one, and preserves explicit
# fail_fast as an opt-in override.
def make_patch(root: Path, name: str, pid: str, target: str, body: str):
    manifest={'schema_version':1,'patch':{'id':pid},'execution':{'timeout_seconds':30},'targets':[target]}
    with zipfile.ZipFile(root/'patchs'/name,'w') as zf:
        zf.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest)); zf.writestr('apply.py',body)

def install(root: Path):
    shutil.copytree(HERE.parent,root/'tools'); (root/'tools/run_python_patches.sh').chmod(0o755); (root/'patchs').mkdir()

def run(root: Path, *args):
    return subprocess.run([str(root/'tools/run_python_patches.sh'),*args],cwd=root,text=True,capture_output=True,
                          env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'},timeout=120)

with tempfile.TemporaryDirectory(prefix='ptv_default_continue_') as td:
    root=Path(td); install(root); (root/'shared.txt').write_text('S\n'); (root/'other.txt').write_text('O\n')
    (root/'.python_patch_tool.json').write_text(json.dumps({'automation':{'zero_argument':{'selection':'all','non_interactive_confirmed':True}}}),encoding='utf-8')
    make_patch(root,'patch_1.zip','p1','shared.txt','raise SystemExit(7)\n')
    make_patch(root,'patch_2.zip','p2','shared.txt','from pathlib import Path\nPath("shared.txt").write_text("BAD\\n")\n')
    make_patch(root,'patch_3.zip','p3','other.txt','from pathlib import Path\nPath("other.txt").write_text("GOOD\\n")\n')
    cp=run(root); assert cp.returncode==7,(cp.returncode,cp.stdout,cp.stderr)
    last=json.loads((root/'artifacts/patch_tool/LAST_RUN.json').read_text())
    assert last['failure_policy']=='continue_independent',last
    assert [x['status'] for x in last['results']]==['FAIL','BLOCKED','PASS'],last['results']
    assert last['results'][1]['diagnosis']['kind']=='related_target_failed',last['results'][1]
    assert (root/'shared.txt').read_text()=='S\n' and (root/'other.txt').read_text()=='GOOD\n'

with tempfile.TemporaryDirectory(prefix='ptv_explicit_fail_fast_') as td:
    root=Path(td); install(root); (root/'a.txt').write_text('A\n'); (root/'b.txt').write_text('B\n')
    (root/'.python_patch_tool.json').write_text(json.dumps({'automation':{'zero_argument':{'selection':'all','non_interactive_confirmed':True}}}),encoding='utf-8')
    make_patch(root,'patch_1.zip','p1','a.txt','raise SystemExit(8)\n')
    make_patch(root,'patch_2.zip','p2','b.txt','from pathlib import Path\nPath("b.txt").write_text("SHOULD_NOT_RUN\\n")\n')
    cp=subprocess.run([sys.executable,'-S',str(MOD),'--project-root',str(root),'--failure-policy','fail_fast'],cwd=root,text=True,capture_output=True,env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'},timeout=120); assert cp.returncode==8,(cp.returncode,cp.stdout,cp.stderr)
    last=json.loads((root/'artifacts/patch_tool/LAST_RUN.json').read_text())
    assert last['failure_policy']=='fail_fast',last
    assert [x['status'] for x in last['results']]==['FAIL'],last['results']
    assert last['not_executed']==['patch_2.zip'],last
    assert (root/'b.txt').read_text()=='B\n'

print('PASS: v6.17.13 arrow recovery menu, failed-PATCH multi-select delete/COLLECT, and default dependency-aware continuation')
