#!/usr/bin/env python3
from __future__ import annotations
import json, os, subprocess, tempfile, sys, shlex, zipfile
from pathlib import Path

HERE=Path(__file__).resolve()
LAUNCHER=HERE.parents[1]/'run_python_patches.sh'
PROGRESS=HERE.parent/'python_patch_collect_progress_v6_7.py'
COMPAT=HERE.parent/'python_patch_collect_compat.py'
SCHEMA_MOD=HERE.parent/'python_patch_collect_schema.py'
REGEX_WORKER=HERE.parent/'python_patch_collect_regex_worker.py'
SCHEMA_JSON=HERE.parent/'docs'/'COLLECT_ACTION_SCHEMA.json'

def make_project(tmp: Path):
    tools=tmp/'tools'; lib=tools/'_patch_lib'; lib.mkdir(parents=True)
    (tools/'run_python_patches.sh').write_bytes(LAUNCHER.read_bytes())
    (tools/'run_python_patches.sh').chmod(0o755)
    (lib/'python_patch_runner.py').write_text('import json,sys\nprint(json.dumps(sys.argv[1:]))\n', encoding='utf-8')
    (lib/'python_patch_queue_dispatcher.py').write_text('import json,sys\nprint(json.dumps(sys.argv[1:]))\n', encoding='utf-8')
    (lib/'python_patch_collect_progress_v6_7.py').write_bytes(PROGRESS.read_bytes())
    (lib/'python_patch_collect_compat.py').write_bytes(COMPAT.read_bytes())
    (lib/'python_patch_collect_schema.py').write_bytes(SCHEMA_MOD.read_bytes())
    (lib/'python_patch_collect_regex_worker.py').write_bytes(REGEX_WORKER.read_bytes())
    docs=lib/'docs'; docs.mkdir()
    (docs/'COLLECT_ACTION_SCHEMA.json').write_bytes(SCHEMA_JSON.read_bytes())
    bindir=tmp/'bin'; bindir.mkdir()
    py=bindir/'python3'
    py.write_text('#!/usr/bin/env bash\nexec '+shlex.quote(sys.executable)+' -S "$@"\n',encoding='utf-8')
    py.chmod(0o755)
    return tools/'run_python_patches.sh'

def test_env(launcher: Path, extra=None):
    result=os.environ.copy()
    if extra: result.update(extra)
    result['PATH']=str(launcher.parent.parent/'bin')+os.pathsep+result.get('PATH','')
    return result

def run(launcher: Path, *args: str, env=None):
    cp=subprocess.run([str(launcher),*args], cwd=launcher.parent.parent, text=True, capture_output=True, env=test_env(launcher,env))
    assert cp.returncode==0, (cp.returncode,cp.stdout,cp.stderr)
    return json.loads(cp.stdout.strip().splitlines()[-1])

with tempfile.TemporaryDirectory() as td:
    launcher=make_project(Path(td))
    for args in [
        ('--patch','patchs/a.zip'),
        ('--all',),
        ('--select',),
        ('patchs/a.zip',),
        ('patchs/UPPER.ZIP',),
        ('patchs/UPPER.TAR.GZ',),
        ('--patch=patchs/UPPER.ZIP',),
        ('--transaction','--all'),
        ('--transaction','--select'),
        ('--transaction','--patch','patchs/demo.zip'),
        ('--select','--transaction','required','--keep-failed-sandbox'),
        ('--transaction=auto','--patch','patchs/a.zip'),
        ('-a','-y','--zip-failed','--keep-failed-zip','--move'),
        ('-y','--move'),
    ]:
        got=run(launcher,*args)
        automation=any(a in {'--all','-a','--select','--zip-failed','--keep-failed-zip','--move','-y'} for a in args)
        if automation:
            assert 'run' in got,(args,got)
            assert '--transaction' not in got and '--transaction=auto' not in got and 'required' not in got,(args,got)
        else:
            assert got[-2:]==['--transaction','off'], (args,got)
        assert '--keep-failed-sandbox' not in got, (args,got)


    # Obsolete transaction/SANDBOX-only invocations must fail closed.  After
    # stripping those flags the launcher must never call the legacy core with
    # zero arguments, where an old default could recreate a worktree.
    for args in [
        ('--transaction','auto'),
        ('--transaction','required'),
        ('--transaction=auto',),
        ('--transaction=required',),
        ('--keep-failed-sandbox',),
        ('--transaction',),
    ]:
        cp=subprocess.run([str(launcher),*args],cwd=launcher.parent.parent,text=True,capture_output=True,env=test_env(launcher))
        assert cp.returncode==2,(args,cp.returncode,cp.stdout,cp.stderr)
        assert 'obsolete transaction/SANDBOX flags' in cp.stderr,(args,cp.stderr)
        assert cp.stdout.strip()=='',(args,cp.stdout)

    env=os.environ.copy(); env['PTV_USE_RUNTIME_GUARD']='1'
    got=run(launcher,'--patch','patchs/a.zip',env=env)
    assert got[-2:]==['--transaction','off'], got

    for utility in [('paths',),('help',),('--help',),('-h',),('version',),('--version',)]:
        got=run(launcher,*utility)
        assert '--transaction' not in got, (utility,got)

    root=launcher.parent.parent
    (root/'patchs').mkdir(exist_ok=True)
    request=root/'patchs'/'CODE_COLLECTION_REQUEST_delegate_route.zip'
    with zipfile.ZipFile(request,'w') as zf:
        zf.writestr('CODE_COLLECTION_REQUEST_delegate_route.json', json.dumps({'id':'delegate-route','actions':[{'type':'overview'}]}))
    cp=subprocess.run([str(launcher),'collect','request','patchs/CODE_COLLECTION_REQUEST_delegate_route.zip'],cwd=root,text=True,capture_output=True,env=test_env(launcher))
    assert cp.returncode==0,(cp.returncode,cp.stdout,cp.stderr)
    assert '[PRIMARY - UPLOAD THIS FILE]' in cp.stdout,cp.stdout
    assert 'CODE_COLLECTION_RESULT_delegate-route_' in cp.stdout,cp.stdout

text=LAUNCHER.read_text(encoding='utf-8')
assert 'python_patch_runtime_guard.py' not in text
assert 'PTV_USE_RUNTIME_GUARD' not in text
assert 'git worktree add' not in text
assert 'exec python3 "$RUNNER" "${filtered[@]}" --transaction off' in text
print('PASS: v6.18.8 all documented PATCH execution routes force in-place mode')
