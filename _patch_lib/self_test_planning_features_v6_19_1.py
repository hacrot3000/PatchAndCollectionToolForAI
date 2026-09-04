#!/usr/bin/env python3
from __future__ import annotations
import json, os, shutil, subprocess, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent

def install(root: Path):
    shutil.copytree(TOOLS, root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()

def pack(path: Path, manifest: dict, *, script: str|None=None, ops: dict|None=None):
    with zipfile.ZipFile(path,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        if ops is not None: z.writestr('PATCH_TOOL_OPS.json',json.dumps(ops))
        else: z.writestr('patch_apply.py',script or 'print("ok")')

def run(root: Path,*args,input_text='\n',timeout=60):
    env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'}
    return subprocess.run([str(root/'tools'/'run_python_patches.sh'),*args],cwd=root,input=input_text,text=True,capture_output=True,env=env,timeout=timeout)

# 1. project identity is enforced before payload.
with tempfile.TemporaryDirectory(prefix='ptv6177_project_') as td:
    r=Path(td); install(r); (r/'.python_patch_tool.json').write_text(json.dumps({'project':{'key':'alpha'}}))
    pack(r/'patchs'/'p.zip',{'schema_version':1,'project':{'key':'beta'},'patch':{'id':'p'}},script='from pathlib import Path\nPath("BAD").write_text("x")')
    cp=run(r,'validate','--patch','patchs/p.zip'); assert cp.returncode==2,(cp.stdout,cp.stderr)
    assert 'project_mismatch' in cp.stdout+cp.stderr and not (r/'BAD').exists()

# 2. validation profile must exist locally and executes after payload.
with tempfile.TemporaryDirectory(prefix='ptv6177_profiles_') as td:
    r=Path(td); install(r)
    (r/'.python_patch_tool.json').write_text(json.dumps({'validation_profiles':{'unit':{'argv':['python3','-c','from pathlib import Path; assert Path("made.txt").read_text()=="yes"'],'timeout_seconds':10}}}))
    m={'schema_version':1,'patch':{'id':'vp'},'validation':{'profiles':['unit']}}
    pack(r/'patchs'/'p.zip',m,script='from pathlib import Path\nPath("made.txt").write_text("yes")')
    cp=run(r,input_text='\n'); assert cp.returncode==0,(cp.stdout,cp.stderr); assert 'VALIDATION PROFILE: unit PASS' in cp.stdout+cp.stderr
    result=json.loads((r/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text())['results'][0]['patch_result']
    assert result['preflight']['validation_profiles']==[{'name':'unit'}]
    assert '_resolved_validation_profiles' not in result['preflight']

with tempfile.TemporaryDirectory(prefix='ptv6177_profile_missing_') as td:
    r=Path(td); install(r)
    m={'schema_version':1,'patch':{'id':'vp'},'validation':{'profiles':['missing']}}
    pack(r/'patchs'/'p.zip',m,script='from pathlib import Path\nPath("BAD").write_text("x")')
    cp=run(r,input_text='\n'); assert cp.returncode==2; assert 'validation_profile_missing' in cp.stdout+cp.stderr; assert not (r/'BAD').exists()

# 3/4/5/7/8. plan is read-only, shows static conflicts + OPS diff, exports SHA recipe,
# and recipe execution binds exact bytes.
with tempfile.TemporaryDirectory(prefix='ptv6177_plan_') as td:
    r=Path(td); install(r); (r/'a.txt').write_text('old\n')
    ops={'ops':[{'op':'replace','file':'a.txt','old':'old','new':'new'}]}
    for name,pid in [('a.zip','a'),('b.zip','b')]:
        pack(r/'patchs'/name,{'schema_version':1,'patch':{'id':pid,'summary':'touch shared'},'targets':['a.txt']},ops=ops)
    cp=run(r,'plan','--export-recipe','recipe.json',timeout=90); text=cp.stdout+cp.stderr
    assert cp.returncode==0,text; assert 'ORDER-DEPENDENT' in text and 'PREVIEW DIFF' in text and 'RESOURCE PREFLIGHT: PASS' in text
    assert (r/'a.txt').read_text()=='old\n'
    recipe=json.loads((r/'recipe.json').read_text()); assert recipe['format']=='python-patch-tool-batch-recipe' and len(recipe['packages'])==2
    # Tamper one recipe package; exact SHA must reject before payload.
    with zipfile.ZipFile(r/'patchs'/'a.zip','a') as z: z.writestr('TAMPER.txt','x')
    cp=run(r,'run','--recipe','recipe.json',timeout=60); assert cp.returncode==2,(cp.stdout,cp.stderr); assert 'SHA mismatch' in cp.stdout+cp.stderr or 'package_input_changed' in cp.stdout+cp.stderr
    assert (r/'a.txt').read_text()=='old\n'
    failed_recipe_report=json.loads((r/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text())
    assert failed_recipe_report['status']=='FAIL' and failed_recipe_report['results'][0]['kind']=='RECIPE'
    assert failed_recipe_report['results'][0]['diagnosis']['kind']=='package_input_changed'

# 8b. resource failure stops plan before mirror preview/copy.
with tempfile.TemporaryDirectory(prefix='ptv6177_resource_fail_') as td:
    r=Path(td); install(r)
    free=shutil.disk_usage(r).free
    huge=r/'huge.bin'
    with huge.open('wb') as fh:
        fh.truncate(free//2 + 64*1024*1024)  # sparse: large logical size, tiny physical use
    pack(r/'patchs'/'huge.zip',{'schema_version':1,'patch':{'id':'huge'},'targets':['huge.bin']},ops={'ops':[{'op':'check','file':'huge.bin','contains':''}]})
    cp=run(r,'plan',timeout=60); text=cp.stdout+cp.stderr
    assert cp.returncode==2,text
    assert 'RESOURCE PREFLIGHT: FAIL' in text
    assert 'PREVIEW DIFF' not in text

# 6. ledger detects patch-id reuse with different package bytes.
with tempfile.TemporaryDirectory(prefix='ptv6177_ledger_') as td:
    r=Path(td); install(r)
    pack(r/'patchs'/'first.zip',{'schema_version':1,'patch':{'id':'same-id'}},script='print("one")')
    cp=run(r,input_text='\n'); assert cp.returncode==0,(cp.stdout,cp.stderr)
    ledger=r/'artifacts'/'patch_tool'/'PATCH_LEDGER.json'; assert ledger.is_file()
    pack(r/'patchs'/'second.zip',{'schema_version':1,'patch':{'id':'same-id'}},script='print("two")')
    cp=run(r,'plan',timeout=60); assert cp.returncode==0; assert 'PATCH ID REUSE' in cp.stdout+cp.stderr

# 3. unresolved failures survive a later unrelated successful LAST_RUN.
with tempfile.TemporaryDirectory(prefix='ptv6177_registry_') as td:
    r=Path(td); install(r)
    (r/'shared.txt').write_text('base')
    pack(r/'patchs'/'bad.zip',{'schema_version':1,'patch':{'id':'bad'},'targets':['shared.txt']},script='raise SystemExit(7)')
    cp=run(r,input_text='\n'); assert cp.returncode==7
    reg=r/'artifacts'/'patch_tool'/'UNRESOLVED_FAILURES.json'; assert reg.is_file()
    (r/'patchs'/'bad.zip').unlink(missing_ok=True)
    pack(r/'patchs'/'good.zip',{'schema_version':1,'patch':{'id':'good'}},script='print("good")')
    cp=run(r,input_text='\n'); assert cp.returncode==0,(cp.stdout,cp.stderr)
    data=json.loads(reg.read_text()); assert any(not x.get('resolved') for x in data['entries'])
    # The persistent registry is also a planner constraint after LAST_RUN has
    # been replaced by the unrelated PASS; normal queue/plan cannot bypass
    # batch.previous_failure merely by skipping SMART RESUME.
    pack(r/'patchs'/'successor.zip',{'schema_version':1,'patch':{'id':'successor'},'targets':['shared.txt']},script='print("successor")')
    cp=run(r,'plan',timeout=60)
    assert cp.returncode==2,(cp.stdout,cp.stderr)
    assert 'previous_failure_action_required' in cp.stdout+cp.stderr

# 9. selector search indexes filename/id/summary/effective targets.
import sys
sys.path.insert(0,str(HERE))
from python_patch_queue_dispatcher import QueueItem,_filter_selector_items
with tempfile.TemporaryDirectory(prefix='ptv6177_search_') as td:
    r=Path(td); (r/'patchs').mkdir()
    pack(r/'patchs'/'alpha.zip',{'schema_version':1,'patch':{'id':'NFC-999','summary':'special summary'},'targets':['src/needle.cpp']},script='print(1)')
    items=[QueueItem('alpha.zip','PATCH')]
    cache={}
    assert _filter_selector_items(r,items,'nfc-999',cache)==items
    assert _filter_selector_items(r,items,'needle.cpp',cache)==items
    assert _filter_selector_items(r,items,'special summary',cache)==items

print('PASS: v6.19.1 project identity, validation profiles, persistent failures, plan/conflicts/preview, ledger, recipe, resources and selector search')
