#!/usr/bin/env python3
from __future__ import annotations
import json, os, subprocess, tempfile
from pathlib import Path

HERE=Path(__file__).resolve()
LAUNCHER=HERE.parents[1]/'run_python_patches.sh'
PROGRESS=HERE.parent/'python_patch_collect_progress_v6_7.py'

def make_project(tmp: Path):
    tools=tmp/'tools'; lib=tools/'_patch_lib'; lib.mkdir(parents=True)
    (tools/'run_python_patches.sh').write_bytes(LAUNCHER.read_bytes())
    (tools/'run_python_patches.sh').chmod(0o755)
    (lib/'python_patch_runner.py').write_text('import json,sys\nprint(json.dumps(sys.argv[1:]))\n', encoding='utf-8')
    (lib/'python_patch_collect_progress_v6_7.py').write_bytes(PROGRESS.read_bytes())
    (lib/'python_patch_readonly_collector.py').write_text("from pathlib import Path\nimport zipfile\nroot=Path.cwd()\nout=root/'artifacts'/'patch_tool_code_collections'/'route.zip'\nout.parent.mkdir(parents=True,exist_ok=True)\nwith zipfile.ZipFile(out,'w') as zf: zf.writestr('manifest.json','{}')\nprint(f'ZIP : {out}')\nprint('PASS collect route')\n", encoding='utf-8')
    return tools/'run_python_patches.sh'

def run(launcher: Path, *args: str, env=None):
    cp=subprocess.run([str(launcher),*args], cwd=launcher.parent.parent, text=True, capture_output=True, env=env)
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
    ]:
        got=run(launcher,*args)
        assert got[-2:]==['--transaction','off'], (args,got)
        assert '--transaction=auto' not in got and 'required' not in got, (args,got)
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
        cp=subprocess.run([str(launcher),*args],cwd=launcher.parent.parent,text=True,capture_output=True)
        assert cp.returncode==2,(args,cp.returncode,cp.stdout,cp.stderr)
        assert 'obsolete transaction/SANDBOX flags' in cp.stderr,(args,cp.stderr)
        assert cp.stdout.strip()=='',(args,cp.stdout)

    env=os.environ.copy(); env['PTV_USE_RUNTIME_GUARD']='1'
    got=run(launcher,'--patch','patchs/a.zip',env=env)
    assert got[-2:]==['--transaction','off'], got

    for utility in [('paths',),('--help',)]:
        got=run(launcher,*utility)
        assert '--transaction' not in got, (utility,got)

    cp=subprocess.run([str(launcher),'collect','request','dummy.zip'],cwd=launcher.parent.parent,text=True,capture_output=True)
    assert cp.returncode==0,(cp.returncode,cp.stdout,cp.stderr)
    assert '[PRIMARY - UPLOAD THIS FILE]' in cp.stdout,cp.stdout
    assert 'route.zip' in cp.stdout,cp.stdout

text=LAUNCHER.read_text(encoding='utf-8')
assert 'python_patch_runtime_guard.py' not in text
assert 'PTV_USE_RUNTIME_GUARD' not in text
assert 'git worktree add' not in text
assert 'exec python3 "$RUNNER" "${filtered[@]}" --transaction off' in text
print('PASS: v6.8.0 all documented PATCH execution routes force in-place mode')
