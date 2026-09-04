#!/usr/bin/env python3
from __future__ import annotations
import hashlib, json, os, shutil, subprocess, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent

def sha(p: Path)->str:
    return hashlib.sha256(p.read_bytes()).hexdigest()

def install(root: Path):
    shutil.copytree(TOOLS,root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()

def mk(path: Path, manifest: dict, script: str):
    with zipfile.ZipFile(path,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        z.writestr('patch_apply.py',script)

def run_patch(root: Path, name: str):
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    result=root/'result.json'; env['PTV_PATCH_RESULT_FILE']=str(result)
    cp=subprocess.run([str(root/'tools'/'run_python_patches.sh'),'--patch',f'patchs/{name}'],cwd=root,text=True,capture_output=True,env=env,timeout=30)
    data=json.loads(result.read_text())
    return cp,data

def manifest_for(root: Path, *, post=False):
    old=root/'old.txt'
    m={
      'schema_version':1,
      'patch':{'id':'rollback-test'},
      'targets':['old.txt','new.txt'],
      'preflight':{'files':[
        {'path':'old.txt','exists':True,'sha256':sha(old)},
        {'path':'new.txt','exists':False},
      ]},
      'recovery':{'rollback':{
        'targets':['old.txt','new.txt'],
        'on':['payload_failure','post_patch_failure'],
        'max_total_bytes':1048576,
      }},
    }
    if post:
        m['post_patch']={'commands':[{'name':'forced validation failure','argv':['python3','-c','raise SystemExit(7)'],'cwd':'.','timeout_seconds':30}]}
    return m

# Insufficient metadata must fail before payload and preserve the project.
with tempfile.TemporaryDirectory(prefix='ptv614_rollback_contract_') as td:
    root=Path(td); install(root); (root/'old.txt').write_text('BASE\n')
    bad={
      'schema_version':1,'patch':{'id':'bad-rollback'},'targets':['old.txt'],
      'preflight':{'files':[{'path':'old.txt','exists':True}]},
      'recovery':{'rollback':{'targets':['old.txt']}}
    }
    mk(root/'patchs'/'bad.zip',bad,"from pathlib import Path\nPath('old.txt').write_text('MUTATED')")
    cp,data=run_patch(root,'bad.zip')
    assert cp.returncode==2,(cp.stdout,cp.stderr)
    assert (root/'old.txt').read_text()=='BASE\n'
    assert data['stage']=='preflight' and data['diagnosis']['kind']=='rollback_contract_invalid',data
    assert 'PREFLIGHT FAIL' in cp.stderr

# Payload failure restores existing target bytes and removes a target that was initially missing.
with tempfile.TemporaryDirectory(prefix='ptv614_rollback_payload_') as td:
    root=Path(td); install(root); (root/'old.txt').write_text('BASE\n')
    m=manifest_for(root)
    script="""from pathlib import Path
Path('old.txt').write_text('CHANGED\\n')
Path('new.txt').write_text('CREATED\\n')
raise SystemExit(9)
"""
    mk(root/'patchs'/'payload_fail.zip',m,script)
    cp,data=run_patch(root,'payload_fail.zip')
    assert cp.returncode==9,(cp.stdout,cp.stderr)
    assert (root/'old.txt').read_text()=='BASE\n'
    assert not (root/'new.txt').exists()
    assert data['rollback']['status']=='PASS',data
    assert data['rollback']['trigger']=='payload_failure'
    assert data['partial_modification']['detected'] is False,data
    assert (root/'patchs'/'payload_fail.zip').exists()  # failed patch remains retryable
    assert 'ROLLBACK: PASS' in cp.stdout

# Post-patch validation failure also restores the declared target baseline.
with tempfile.TemporaryDirectory(prefix='ptv614_rollback_post_') as td:
    root=Path(td); install(root); (root/'old.txt').write_text('BASE\n')
    m=manifest_for(root,post=True)
    script="""from pathlib import Path
Path('old.txt').write_text('CHANGED\\n')
Path('new.txt').write_text('CREATED\\n')
"""
    mk(root/'patchs'/'post_fail.zip',m,script)
    cp,data=run_patch(root,'post_fail.zip')
    assert cp.returncode==7,(cp.stdout,cp.stderr)
    assert (root/'old.txt').read_text()=='BASE\n' and not (root/'new.txt').exists()
    assert data['rollback']['status']=='PASS' and data['rollback']['trigger']=='post_patch_failure',data

# Git fingerprint catches modifications outside declared rollback scope; rollback must report PARTIAL.
with tempfile.TemporaryDirectory(prefix='ptv614_rollback_outside_') as td:
    root=Path(td); install(root)
    (root/'old.txt').write_text('BASE\n'); (root/'outside.txt').write_text('OUTSIDE BASE\n')
    subprocess.run(['git','init','-q'],cwd=root,check=True)
    subprocess.run(['git','config','user.email','ptv@example.invalid'],cwd=root,check=True)
    subprocess.run(['git','config','user.name','PTV Test'],cwd=root,check=True)
    subprocess.run(['git','add','old.txt','outside.txt'],cwd=root,check=True)
    subprocess.run(['git','commit','-qm','baseline'],cwd=root,check=True)
    # Package/patchs/tools are untracked at the baseline and thus included in the fingerprint.
    # Recompute a clean fingerprint baseline by committing only project source is not enough,
    # so add tool/queue to .gitignore to keep the test focused on source changes.
    (root/'.gitignore').write_text('tools/\npatchs/\nresult.json\nartifacts/\n')
    subprocess.run(['git','add','.gitignore'],cwd=root,check=True)
    subprocess.run(['git','commit','-qm','ignore test harness'],cwd=root,check=True)
    m={
      'schema_version':1,'patch':{'id':'outside-change'},'targets':['old.txt'],
      'preflight':{'files':[{'path':'old.txt','exists':True,'sha256':sha(root/'old.txt')}]},
      'recovery':{'rollback':{'targets':['old.txt'],'on':['payload_failure']}}
    }
    script="""from pathlib import Path
Path('old.txt').write_text('CHANGED\\n')
Path('outside.txt').write_text('UNDECLARED CHANGE\\n')
raise SystemExit(11)
"""
    mk(root/'patchs'/'outside_fail.zip',m,script)
    cp,data=run_patch(root,'outside_fail.zip')
    assert cp.returncode==11,(cp.stdout,cp.stderr)
    assert (root/'old.txt').read_text()=='BASE\n'
    assert (root/'outside.txt').read_text()=='UNDECLARED CHANGE\n'
    assert data['rollback']['status']=='PARTIAL',data
    assert data['partial_modification']['detected'] is True,data
    assert 'ROLLBACK: PARTIAL' in cp.stderr


# TOCTOU: a target changed after preflight but before snapshot must fail instead of snapshotting a new baseline.
import importlib.util, sys
spec_s=importlib.util.spec_from_file_location('ptv_schema_rb',HERE/'python_patch_package_schema.py')
schema_mod=importlib.util.module_from_spec(spec_s); sys.modules[spec_s.name]=schema_mod; spec_s.loader.exec_module(schema_mod)
spec_r=importlib.util.spec_from_file_location('ptv_runner_rb',HERE/'python_patch_runner.py')
runner_mod=importlib.util.module_from_spec(spec_r); sys.modules[spec_r.name]=runner_mod; spec_r.loader.exec_module(runner_mod)
with tempfile.TemporaryDirectory(prefix='ptv614_rollback_toctou_') as td:
    root=Path(td); (root/'old.txt').write_text('BASE\n')
    manifest={
      'schema_version':1,'patch':{'id':'toctou'},'targets':['old.txt'],
      'preflight':{'files':[{'path':'old.txt','exists':True,'sha256':sha(root/'old.txt')}]},
      'recovery':{'rollback':{'targets':['old.txt'],'on':['payload_failure']}}
    }
    payload=root/'dummy.py'; payload.write_text('pass\n')
    report=schema_mod.run_preflight(root,manifest,extracted=None,kind='python',payload=payload,ops_data=None)
    (root/'old.txt').write_text('CHANGED BETWEEN PREFLIGHT AND SNAPSHOT\n')
    try:
        runner_mod._prepare_rollback_snapshot(root,report['rollback'])
    except Exception as exc:
        assert type(exc).__name__=='PatchSchemaError',type(exc).__name__
        assert getattr(exc,'kind',None)=='rollback_snapshot_race',getattr(exc,'kind',None)
    else:
        raise AssertionError('TOCTOU baseline change must fail before payload')

print('PASS: v6.20.1 metadata-driven in-place rollback is opt-in, bounded and fail-closed')
