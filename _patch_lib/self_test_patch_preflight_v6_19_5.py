#!/usr/bin/env python3
from __future__ import annotations
import hashlib, json, os, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent

def install(root: Path):
    shutil.copytree(TOOLS,root/'tools'); (root/'tools'/'run_python_patches.sh').chmod(0o755); (root/'patchs').mkdir()

def run(root: Path, text='\n'):
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    return subprocess.run([str(root/'tools'/'run_python_patches.sh')],cwd=root,input=text,text=True,capture_output=True,env=env,timeout=40)

def pack(path: Path, manifest: dict, script: str, resources: dict[str,bytes]|None=None):
    with zipfile.ZipFile(path,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        z.writestr('patch_apply.py',script)
        for name,data in (resources or {}).items(): z.writestr(name,data)

def base(pid='pf'):
    return {'schema_version':1,'patch':{'id':pid},'execution':{'timeout_seconds':30}}

# Preflight SHA mismatch must fail before payload and leave project unchanged.
with tempfile.TemporaryDirectory(prefix='ptv613_pf_hash_') as td:
    root=Path(td); install(root); (root/'a.txt').write_text('current\n')
    m=base('hash'); m['targets']=['a.txt']; m['preflight']={'files':[{'path':'a.txt','sha256':'0'*64}]}
    pack(root/'patchs'/'p.zip',m,'from pathlib import Path\nPath("a.txt").write_text("WRONG")\n')
    cp=run(root); assert cp.returncode==2,(cp.stdout,cp.stderr); assert (root/'a.txt').read_text()=='current\n'
    assert 'PREFLIGHT FAIL — project unchanged' in cp.stdout+cp.stderr
    data=json.loads((root/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text())
    pr=data['results'][0]['patch_result']; assert pr['diagnosis']['kind']=='source_drift'; assert pr['partial_modification']['detected'] is False

# Tool compatibility mismatch must fail before payload.
with tempfile.TemporaryDirectory(prefix='ptv613_pf_version_') as td:
    root=Path(td); install(root); (root/'a.txt').write_text('old')
    m=base('compat'); m['compatibility']={'min_tool_version':'99.0.0'}; m['targets']=['a.txt']
    pack(root/'patchs'/'p.zip',m,'from pathlib import Path\nPath("a.txt").write_text("bad")')
    cp=run(root); assert cp.returncode==2,(cp.stdout,cp.stderr); assert (root/'a.txt').read_text()=='old'
    assert 'tool_version_incompatible' in cp.stdout+cp.stderr

# Unknown manifest fields and missing declared resources are rejected by exact schema/preflight.
with tempfile.TemporaryDirectory(prefix='ptv613_pf_schema_') as td:
    root=Path(td); install(root)
    m=base('schema'); m['invented_field']=True
    pack(root/'patchs'/'p.zip',m,'print("must-not-run")')
    cp=run(root); assert cp.returncode==2; assert 'unsupported field' in cp.stdout+cp.stderr
with tempfile.TemporaryDirectory(prefix='ptv613_pf_resource_') as td:
    root=Path(td); install(root)
    m=base('resource'); m['resources']=['resources/required.bin']
    pack(root/'patchs'/'p.zip',m,'print("must-not-run")')
    cp=run(root); assert cp.returncode==2; assert 'resource_missing' in cp.stdout+cp.stderr

# Post command executable/cwd are checked before payload.
with tempfile.TemporaryDirectory(prefix='ptv613_pf_cmd_') as td:
    root=Path(td); install(root); (root/'a.txt').write_text('old')
    m=base('cmd'); m['targets']=['a.txt']; m['post_patch']={'commands':[{'argv':['definitely-not-a-real-ptv-command-xyz'],'cwd':'.','timeout_seconds':10}]}
    pack(root/'patchs'/'p.zip',m,'from pathlib import Path\nPath("a.txt").write_text("bad")')
    cp=run(root); assert cp.returncode==2; assert (root/'a.txt').read_text()=='old'; assert 'command_missing' in cp.stdout+cp.stderr

# Partial modification is detected when payload modifies a declared target then fails.
with tempfile.TemporaryDirectory(prefix='ptv613_pf_partial_') as td:
    root=Path(td); install(root); (root/'a.txt').write_text('old')
    m=base('partial'); m['targets']=['a.txt']
    pack(root/'patchs'/'p.zip',m,'from pathlib import Path\nPath("a.txt").write_text("changed")\nraise SystemExit(7)')
    cp=run(root); assert cp.returncode==7,(cp.stdout,cp.stderr); assert (root/'a.txt').read_text()=='changed'
    assert 'PARTIAL MODIFICATION DETECTED' in cp.stdout+cp.stderr
    data=json.loads((root/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text())
    assert data['results'][0]['patch_result']['partial_modification']['detected'] is True

# A representative legacy/current manifest shape remains accepted.
from python_patch_package_schema import validate_manifest, check_compatibility
representative={
 'schema_version':1,'project':{'key':'bletonfc'},
 'patch':{'id':'nfc-test','version':'v5.15.0','phase':'NFC','phase_under_test':'NFC','summary':'s','regression_scope':'r'},
 'execution':{'timeout_seconds':300},'validation':{'profiles':[]},
 'post_patch':{'commands':[{'name':'t','argv':['python3','test.py'],'cwd':'.','timeout_seconds':900}],'run_when_no_changes':False},
 'git':{'add':'changed','commit':'auto','commit_message':'New fix NFC: test','push':'off','fail_on_error':True},
}
validate_manifest(representative); assert check_compatibility(representative,'6.19.5')==[]
print('PASS: v6.19.5 exact PATCH schema, preflight, compatibility and partial-modification detection')
