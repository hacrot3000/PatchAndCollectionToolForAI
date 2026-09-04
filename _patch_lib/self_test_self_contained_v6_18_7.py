#!/usr/bin/env python3
from __future__ import annotations
import json, os, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent

def install(root: Path):
    shutil.copytree(TOOLS,root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()

def run_zero(root: Path, user_input='\n'):
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    return subprocess.run([str(root/'tools'/'run_python_patches.sh')],cwd=root,input=user_input,text=True,capture_output=True,env=env,timeout=30)

def manifest():
    return {'schema_version':1,'patch':{'id':'self-contained-test'},'execution':{'timeout_seconds':30}}

# Clean project: no preinstalled/private core. ZIP Python patch must run, archive,
# and a post-patch argv command must run from the self-contained package only.
with tempfile.TemporaryDirectory(prefix='ptv612_self_py_') as td:
    root=Path(td); install(root); (root/'target.txt').write_text('old\n')
    package=root/'patchs'/'patch_python.zip'
    m=manifest(); m['post_patch']={'commands':[{'name':'marker','argv':[sys.executable,'-c','from pathlib import Path; Path("post.txt").write_text("ok")'],'cwd':'.','timeout_seconds':10}],'run_when_no_changes':False}
    with zipfile.ZipFile(package,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(m))
        z.writestr('apply.py','from pathlib import Path\np=Path("target.txt")\np.write_text(p.read_text().replace("old","new"))\n')
    cp=run_zero(root); assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert (root/'target.txt').read_text()=='new\n'
    assert (root/'post.txt').read_text()=='ok'
    assert not package.exists() and (root/'patchs'/'patched'/package.name).is_file()
    assert 'Execution: IN-PLACE' in cp.stdout and 'RUN SUMMARY: PASS' in cp.stdout

# OPS data-only package is supported without external modules.
with tempfile.TemporaryDirectory(prefix='ptv612_self_ops_') as td:
    root=Path(td); install(root); (root/'src.txt').write_text('alpha\nbeta\n')
    package=root/'patchs'/'patch_ops.zip'
    ops={'schema_version':1,'patch_name':'ops-test','ops':[{'kind':'replace','file':'src.txt','old':'beta','new':'gamma','mode':'exact'}]}
    with zipfile.ZipFile(package,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest()))
        z.writestr('PATCH_TOOL_OPS.json',json.dumps(ops))
    cp=run_zero(root); assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert (root/'src.txt').read_text()=='alpha\ngamma\n'
    assert (root/'patchs'/'patched'/package.name).is_file()

# Resource-preserving extraction: Python entrypoint can read package resources
# relative to __file__, matching real OTA PATCH structure.
with tempfile.TemporaryDirectory(prefix='ptv612_self_resource_') as td:
    root=Path(td); install(root)
    package=root/'patchs'/'patch_resource.zip'
    with zipfile.ZipFile(package,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest()))
        z.writestr('patch_resource.py','from pathlib import Path\nsrc=Path(__file__).parent/"resources"/"final.txt"\nPath("out.txt").write_bytes(src.read_bytes())\n')
        z.writestr('resources/final.txt',b'resource-ok\n')
    cp=run_zero(root); assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert (root/'out.txt').read_bytes()==b'resource-ok\n'

# Ambiguous two-entrypoint package fails closed and remains queued.
with tempfile.TemporaryDirectory(prefix='ptv612_self_ambig_') as td:
    root=Path(td); install(root); package=root/'patchs'/'patch_ambiguous.zip'
    with zipfile.ZipFile(package,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest()))
        z.writestr('a.py','print("a")')
        z.writestr('b.py','print("b")')
    cp=run_zero(root); assert cp.returncode!=0,(cp.stdout,cp.stderr)
    assert package.is_file() and not (root/'patchs'/'patched'/package.name).exists()
    assert 'exactly one Python patch entrypoint' in cp.stdout+cp.stderr

print('PASS: v6.18.7 full self-contained clean-project PATCH runner for Python/OPS/resources')
