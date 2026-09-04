#!/usr/bin/env python3
from __future__ import annotations
import json, os, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
HERE=Path(__file__).resolve().parent; TOOLS=HERE.parent
D=HERE/'python_patch_queue_dispatcher.py'; R=HERE/'python_patch_runner.py'
env=os.environ.copy(); env['PYTHONDONTWRITEBYTECODE']='1'
# Public parsers/launchers must actually construct and return, not merely contain option strings.
for cmd in ([sys.executable,str(D),'--project-root',str(TOOLS.parent),'--help'], ['bash',str(TOOLS/'run_python_patches.sh'),'--help']):
    cp=subprocess.run(cmd,cwd=TOOLS.parent,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,stdin=subprocess.DEVNULL,timeout=20)
    assert cp.returncode==0,(cmd,cp.returncode,cp.stdout)
assert '--no-validation' in subprocess.run([sys.executable,str(D),'--project-root',str(TOOLS.parent),'--help'],cwd=TOOLS.parent,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=20).stdout
# Captured/non-TTY zero-work entry prints HISTORY and returns without waiting for input.
with tempfile.TemporaryDirectory(prefix='ptv_public_') as td:
    root=Path(td); (root/'patchs').mkdir(); h=root/'artifacts/patch_tool/history'; h.mkdir(parents=True)
    (h/'run.json').write_text(json.dumps({'run_id':'r1','status':'PASS','selected':['old.zip'],'started_at':'2026-08-12T01:00:00'}))
    cp=subprocess.run([sys.executable,str(D),'--project-root',str(root)],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,stdin=subprocess.DEVNULL,timeout=20)
    assert cp.returncode==0,(cp.returncode,cp.stdout)
    assert 'AUTO STATUS: IDLE' in cp.stdout and 'PATCH TOOL RUN HISTORY' in cp.stdout and 'old.zip' in cp.stdout,cp.stdout
    # no zero-work state was manufactured
    assert not (root/'artifacts/patch_tool/LAST_RUN.json').exists()

# --no-validation must survive the real launcher -> dispatcher -> execute_items -> runner path.
with tempfile.TemporaryDirectory(prefix='ptv_public_noval_') as td:
    root=Path(td); shutil.copytree(TOOLS,root/'tools'); (root/'tools/run_python_patches.sh').chmod(0o755); (root/'patchs').mkdir(); (root/'src').mkdir(); (root/'src/a.txt').write_text('OLD\n')
    (root/'.python_patch_tool.json').write_text(json.dumps({'validation_profiles':{'must_fail':{'argv':['python3','-c','raise SystemExit(9)']}}}))
    manifest={'schema_version':1,'patch':{'id':'public-no-validation'},'targets':['src/a.txt'],'validation':{'profiles':['must_fail']}}
    ops={'patch_name':'public-no-validation','ops':[{'kind':'replace','file':'src/a.txt','old':'OLD','new':'NEW'}]}
    with zipfile.ZipFile(root/'patchs/p.zip','w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest)); z.writestr('PATCH_TOOL_OPS.json',json.dumps(ops))
    cp=subprocess.run([str(root/'tools/run_python_patches.sh'),'run','--all','--no-validation'],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,stdin=subprocess.DEVNULL,timeout=40)
    assert cp.returncode==0,(cp.returncode,cp.stdout)
    assert (root/'src/a.txt').read_text()=='NEW\n',cp.stdout
    last=json.loads((root/'artifacts/patch_tool/LAST_RUN.json').read_text())
    row=last['results'][0]['patch_result']; assert row['validation_selection']['status']=='DISABLED_BY_CLI',row

print('PASS: v6.20.2 public entry parser/launcher, non-TTY HISTORY and --no-validation routing')
