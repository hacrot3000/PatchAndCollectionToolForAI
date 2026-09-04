#!/usr/bin/env python3
from __future__ import annotations
import hashlib, json, subprocess, sys, tempfile, zipfile
from pathlib import Path
from python_patch_package_schema import collect_manifest_issues

HERE=Path(__file__).resolve().parent
RUNNER=HERE/'python_patch_runner.py'

bad={'schema_version':1,'patch':{'id':'bad'},'source_baseline':{'files':[]},'execution':{'timeout_seconds':0}}
issues=collect_manifest_issues(bad)
assert len(issues)>=2,issues
fields={x.get('field') for x in issues}
assert 'manifest.source_baseline' in fields,issues
assert 'manifest.execution.timeout_seconds' in fields,issues
assert any('preflight.files' in str(x.get('suggestion','')) for x in issues),issues

def pack(path:Path, manifest:dict, *, ops:dict|None=None, py:str='print("ok")\n'):
    with zipfile.ZipFile(path,'w',compression=zipfile.ZIP_DEFLATED) as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        if ops is not None: z.writestr('PATCH_TOOL_OPS.json',json.dumps(ops))
        else: z.writestr('apply.py',py)

def run_validate(root:Path, patch:Path):
    cp=subprocess.run([sys.executable,str(RUNNER),'validate','--patch',str(patch),'--transaction','off'],cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=20)
    return cp,cp.stdout+cp.stderr

with tempfile.TemporaryDirectory() as td:
    root=Path(td); (root/'a.txt').write_text('alpha\n'); (root/'b.txt').write_text('beta\n')
    p=root/'bad.zip'; pack(p,bad)
    cp,out=run_validate(root,p)
    assert cp.returncode==2,(cp.returncode,out)
    assert 'PATCH_INVALID' in out,out
    assert 'manifest.source_baseline' in out and 'manifest.execution.timeout_seconds' in out,out

    # Aggregate source drift: two SHA mismatches must be reported in one read-only pass.
    manifest={'schema_version':1,'patch':{'id':'drift'},'preflight':{'files':[
        {'path':'a.txt','exists':True,'sha256':'0'*64,'anchors':['alpha']},
        {'path':'b.txt','exists':True,'sha256':'1'*64,'anchors':['missing-anchor']},
    ]}}
    p=root/'drift.zip'; pack(p,manifest)
    cp,out=run_validate(root,p)
    assert cp.returncode==2,(cp.returncode,out)
    assert 'SOURCE_DRIFT' in out,out
    assert 'a.txt' in out and 'b.txt' in out,out
    assert 'expected=' in out and 'actual=' in out,out
    assert (root/'a.txt').read_text()=='alpha\n' and (root/'b.txt').read_text()=='beta\n'

    # Sequential OPS dry-run succeeds without touching the real source.
    sha=hashlib.sha256((root/'a.txt').read_bytes()).hexdigest()
    manifest={'schema_version':1,'patch':{'id':'ops-ready'},'targets':['a.txt'],'preflight':{'files':[{'path':'a.txt','exists':True,'sha256':sha}]}}
    ops={'ops':[{'id':'first','kind':'replace','file':'a.txt','old':'alpha','new':'middle'}, {'id':'second','kind':'replace','file':'a.txt','old':'middle','new':'omega'}]}
    p=root/'ops-ready.zip'; pack(p,manifest,ops=ops)
    cp,out=run_validate(root,p)
    assert cp.returncode==0,(cp.returncode,out)
    assert 'READY_TO_APPLY' in out and 'OPS dry-run: PASS' in out,out
    assert (root/'a.txt').read_text()=='alpha\n'

    # Missing OPS match is source drift before payload and names the operation.
    ops_bad={'ops':[{'id':'need-current-source','kind':'replace','file':'a.txt','old':'not-present','new':'x'}]}
    p=root/'ops-drift.zip'; pack(p,manifest,ops=ops_bad)
    cp,out=run_validate(root,p)
    assert cp.returncode==2,(cp.returncode,out)
    assert 'SOURCE_DRIFT' in out and 'need-current-source' in out and 'a.txt' in out,out
    assert (root/'a.txt').read_text()=='alpha\n'

print('PASS: v6.18.7 multi-error lint, classified validate, aggregate source drift and sequential OPS dry-run')
