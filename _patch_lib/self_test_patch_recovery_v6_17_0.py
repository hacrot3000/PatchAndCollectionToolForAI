#!/usr/bin/env python3
from __future__ import annotations
import hashlib, importlib.util, json, os, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
HERE=Path(__file__).resolve().parent; TOOLS=HERE.parent

def install(root: Path):
    shutil.copytree(TOOLS,root/'tools'); (root/'tools'/'run_python_patches.sh').chmod(0o755); (root/'patchs').mkdir()

def mk(path: Path, manifest: dict, script: str):
    with zipfile.ZipFile(path,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest)); z.writestr('patch_apply.py',script)

def run(root: Path, user_input: str):
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    return subprocess.run([str(root/'tools'/'run_python_patches.sh')],cwd=root,input=user_input,text=True,capture_output=True,env=env,timeout=50)

with tempfile.TemporaryDirectory(prefix='ptv613_recovery_') as td:
    root=Path(td); install(root); (root/'source.txt').write_text('current source\n')
    bad={'schema_version':1,'patch':{'id':'fail-drift'},'targets':['source.txt'],'preflight':{'files':[{'path':'source.txt','sha256':'0'*64}]}}
    good={'schema_version':1,'patch':{'id':'later'},'targets':['later.txt']}
    mk(root/'patchs'/'patch_1_fail.zip',bad,'raise SystemExit(9)')
    mk(root/'patchs'/'patch_2_later.zip',good,'from pathlib import Path\nPath("later.txt").write_text("ok")')
    cp=run(root,'a\n'); assert cp.returncode==2,(cp.stdout,cp.stderr)
    assert not (root/'later.txt').exists()
    requests=list((root/'patchs').glob('CODE_COLLECTION_REQUEST_patch_recovery_*.zip')); assert len(requests)==1,requests
    with zipfile.ZipFile(requests[0]) as z:
        inner=[n for n in z.namelist() if n.endswith('.json')][0]; req=json.loads(z.read(inner)); assert req['actions']==[{'type':'pack','paths':['source.txt']}]
    handoffs=list((root/'artifacts'/'patch_tool'/'fail_handoffs').glob('FAIL_HANDOFF_*.zip')); assert len(handoffs)==1,handoffs
    with zipfile.ZipFile(handoffs[0]) as z:
        names=z.namelist(); assert 'FAIL_SUMMARY.json' in names and 'console.log' in names
        assert 'current_source/source.txt' in names
        assert any(n.startswith('recovery/CODE_COLLECTION_REQUEST_') for n in names)
        summary=json.loads(z.read('FAIL_SUMMARY.json')); assert summary['patch_result']['diagnosis']['kind']=='source_drift'
    last=json.loads((root/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text())
    assert last['status']=='FAIL' and last['failed_item']=='patch_1_fail.zip'
    assert last['not_executed']==['patch_2_later.zip']
    assert last['results'][0]['fail_handoff'].startswith('artifacts/patch_tool/fail_handoffs/')
    # Cancel does not erase resume knowledge; the following invocation still shows the hint.
    cp2=run(root,'q\n'); assert cp2.returncode==0; assert 'PREVIOUS RUN: FAIL' in cp2.stdout
    cp3=run(root,'q\n'); assert cp3.returncode==0; assert 'selected item(s) remain unexecuted' in cp3.stdout

# Bounded local run history retains only the most recent 30 records.
spec=importlib.util.spec_from_file_location('ptv613_dispatcher',HERE/'python_patch_queue_dispatcher.py')
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)
with tempfile.TemporaryDirectory(prefix='ptv613_history_') as td:
    root=Path(td)
    for i in range(35):
        m._write_run_report(root,{'run_id':f'r{i:02d}','started_at':f'2026-08-09T00:00:{i:02d}+00:00','status':'PASS','exit_code':0})
    hist=list((root/'artifacts'/'patch_tool'/'history').glob('*.json')); assert len(hist)==30,len(hist)
    assert (root/'artifacts'/'patch_tool'/'LAST_RUN.json').is_file()
# Deterministic recovery request publication is idempotent without a process lock.
with tempfile.TemporaryDirectory(prefix='ptv613_recovery_publish_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'src.c').write_text('x')
    item=m.QueueItem('p.zip','PATCH')
    result={'patch_sha256':'1'*64,'diagnosis':{'kind':'source_drift','affected_paths':['src.c']}}
    a=m._create_recovery_collect_request(root,item,result); b=m._create_recovery_collect_request(root,item,result)
    assert a is not None and a==b and a.is_file()
    assert len(list((root/'patchs').glob('CODE_COLLECTION_REQUEST_patch_recovery_*.zip')))==1

print('PASS: v6.17.0 diagnosis, FAIL_HANDOFF, source-drift recollection, LAST_RUN/resume/history')
