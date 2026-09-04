#!/usr/bin/env python3
from __future__ import annotations
import hashlib, importlib.util, json, os, shutil, signal, subprocess, sys, tempfile, time, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent

def install(root: Path):
    shutil.copytree(TOOLS,root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()

def mk_patch(path: Path, manifest: dict, script: str):
    with zipfile.ZipFile(path,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        z.writestr('patch.py',script)

def run(root: Path, user_input='\n', timeout=30):
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    return subprocess.run([str(root/'tools'/'run_python_patches.sh')],cwd=root,input=user_input,text=True,capture_output=True,env=env,timeout=timeout)

def sha(path: Path): return hashlib.sha256(path.read_bytes()).hexdigest()

# Queue root itself must never redirect execution to another project/shared path.
spec=importlib.util.spec_from_file_location('ptv6141q',HERE/'python_patch_queue_dispatcher.py')
q=importlib.util.module_from_spec(spec); sys.modules[spec.name]=q; spec.loader.exec_module(q)
with tempfile.TemporaryDirectory(prefix='ptv6141_qroot_') as td, tempfile.TemporaryDirectory(prefix='ptv6141_qext_') as ext:
    root=Path(td); (root/'patchs').symlink_to(Path(ext),target_is_directory=True)
    try: q.discover_queue(root)
    except q.QueueSafetyError: pass
    else: raise AssertionError('symlinked patchs/ must fail closed')

# Rollback for initially-missing files requires a real existing parent and no
# symlink ancestor. This prevents outside-project unlink and false PASS with a
# leftover newly-created directory.
with tempfile.TemporaryDirectory(prefix='ptv6141_rbpath_') as td, tempfile.TemporaryDirectory(prefix='ptv6141_out_') as ext:
    root=Path(td); install(root); (root/'link').symlink_to(Path(ext),target_is_directory=True)
    target='link/evil.txt'
    man={'schema_version':1,'patch':{'id':'unsafe-rb'},'targets':[target],
         'preflight':{'files':[{'path':target,'exists':False}]},
         'recovery':{'rollback':{'targets':[target]}}}
    mk_patch(root/'patchs'/'unsafe.zip',man,f"from pathlib import Path\nPath({target!r}).write_text('x')\nraise SystemExit(9)\n")
    cp=run(root); assert cp.returncode==2,(cp.stdout,cp.stderr)
    assert 'rollback_path_unsafe' in cp.stdout+cp.stderr
    assert not (Path(ext)/'evil.txt').exists()
with tempfile.TemporaryDirectory(prefix='ptv6141_rbparent_') as td:
    root=Path(td); install(root); target='newdir/file.txt'
    man={'schema_version':1,'patch':{'id':'missing-parent'},'targets':[target],
         'preflight':{'files':[{'path':target,'exists':False}]},
         'recovery':{'rollback':{'targets':[target]}}}
    mk_patch(root/'patchs'/'parent.zip',man,"raise SystemExit(9)\n")
    cp=run(root); assert cp.returncode==2; assert 'rollback_parent_missing' in cp.stdout+cp.stderr
    assert not (root/'newdir').exists()

# The archived history must be the exact package bytes that were executed, even
# if the patch overwrites its own queue filename while running. The replacement
# must remain queued rather than being archived/deleted as if it had executed.
with tempfile.TemporaryDirectory(prefix='ptv6141_archiveid_') as td:
    root=Path(td); install(root)
    man={'schema_version':1,'patch':{'id':'archive-identity'},'targets':['out.txt']}
    script="from pathlib import Path\nPath('out.txt').write_text('ok')\nPath('patchs/p.zip').write_bytes(b'NEW_NOT_EXECUTED')\n"
    pkg=root/'patchs'/'p.zip'; mk_patch(pkg,man,script); original=sha(pkg)
    cp=run(root); assert cp.returncode==0,(cp.stdout,cp.stderr)
    archived=root/'patchs'/'patched'/'p.zip'
    assert sha(archived)==original
    assert (root/'patchs'/'p.zip').read_bytes()==b'NEW_NOT_EXECUTED'
    assert 'replacement was kept for a later run' in cp.stdout+cp.stderr

# Parent-only SIGINT/SIGTERM must be forwarded into the isolated PATCH process
# group; configured rollback must finish and descendants must not keep running.
def signal_case(sig: int, expected_rc: int):
    with tempfile.TemporaryDirectory(prefix='ptv6141_signal_') as td:
        root=Path(td); install(root); target=root/'a.txt'; target.write_text('original\n'); digest=sha(target)
        man={'schema_version':1,'patch':{'id':'signal'},'targets':['a.txt'],
             'preflight':{'files':[{'path':'a.txt','exists':True,'sha256':digest}]},
             'recovery':{'rollback':{'targets':['a.txt']}},'execution':{'timeout_seconds':20}}
        script="from pathlib import Path\nimport subprocess,sys,time\nPath('a.txt').write_text('changed')\nsubprocess.Popen([sys.executable,'-c',\"import time; from pathlib import Path; time.sleep(1); Path('orphan.txt').write_text('bad')\"])\ntime.sleep(20)\n"
        mk_patch(root/'patchs'/'p.zip',man,script)
        env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
        proc=subprocess.Popen([str(root/'tools'/'run_python_patches.sh')],cwd=root,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,env=env)
        assert proc.stdin is not None; proc.stdin.write('\n'); proc.stdin.flush()
        deadline=time.time()+6
        while time.time()<deadline and target.read_text()=='original\n': time.sleep(.03)
        assert target.read_text()=='changed'
        os.kill(proc.pid,sig)
        out,_=proc.communicate(timeout=15)
        assert proc.returncode==expected_rc,(proc.returncode,out)
        assert target.read_text()=='original\n',out
        time.sleep(1.3)
        assert not (root/'orphan.txt').exists(),out
        assert 'ROLLBACK: PASS' in out,out
signal_case(signal.SIGINT,130)
signal_case(signal.SIGTERM,143)

# Timeout must terminate the whole payload process group before rollback.
with tempfile.TemporaryDirectory(prefix='ptv6141_timeout_') as td:
    root=Path(td); install(root); target=root/'a.txt'; target.write_text('original\n'); digest=sha(target)
    man={'schema_version':1,'patch':{'id':'timeout'},'targets':['a.txt'],
         'preflight':{'files':[{'path':'a.txt','exists':True,'sha256':digest}]},
         'recovery':{'rollback':{'targets':['a.txt']}},'execution':{'timeout_seconds':1}}
    script="from pathlib import Path\nimport subprocess,sys,time\nPath('a.txt').write_text('changed')\nsubprocess.Popen([sys.executable,'-c',\"import time; from pathlib import Path; time.sleep(2); Path('a.txt').write_text('orphan-change')\"] )\ntime.sleep(20)\n"
    mk_patch(root/'patchs'/'p.zip',man,script)
    cp=run(root,timeout=15); assert cp.returncode==124,(cp.stdout,cp.stderr)
    assert target.read_text()=='original\n'
    time.sleep(2.3); assert target.read_text()=='original\n'

# COLLECT archives the exact request it executed and keeps a same-name
# replacement queued. Test the lifecycle primitive directly to avoid a slow
# artificial collection.
cspec=importlib.util.spec_from_file_location('ptv6141c',HERE/'python_patch_collect_compat.py')
c=importlib.util.module_from_spec(cspec); sys.modules[cspec.name]=c; cspec.loader.exec_module(c)
with tempfile.TemporaryDirectory(prefix='ptv6141_collectid_') as td:
    root=Path(td); (root/'patchs').mkdir(); req=root/'patchs'/'CODE_COLLECTION_REQUEST_x.zip'
    with zipfile.ZipFile(req,'w') as z: z.writestr('CODE_COLLECTION_REQUEST_x.json',json.dumps({'actions':[{'type':'overview'}]}))
    snap_dir,snap,req_sha=c._snapshot_request_input(req)
    try:
        with zipfile.ZipFile(req,'w') as z: z.writestr('CODE_COLLECTION_REQUEST_x.json',json.dumps({'actions':[{'type':'git','sections':['status']}]}))
        replacement_sha=sha(req); assert replacement_sha!=req_sha
        archived,lifecycle=c._archive_request(root,req,snap,req_sha)
        assert sha(archived)==req_sha
        assert sha(req)==replacement_sha
        assert lifecycle=='replacement_restored',lifecycle
    finally: snap_dir.cleanup()

# Tool Health must not be bypassable by deleting the checksum row for a
# corrupted/changed required runtime file.
hspec=importlib.util.spec_from_file_location('ptv6141h',HERE/'python_patch_health.py')
h=importlib.util.module_from_spec(hspec); sys.modules[hspec.name]=h; hspec.loader.exec_module(h)
with tempfile.TemporaryDirectory(prefix='ptv6141_health_') as td:
    root=Path(td); shutil.copytree(TOOLS,root/'tools'); (root/'tools'/'run_python_patches.sh').chmod(0o755)
    manifest=root/'tools'/'_patch_lib'/'SHA256SUMS'
    rows=[]
    for path in sorted((root/'tools').rglob('*')):
        if path.is_file() and path!=manifest and '__pycache__' not in path.parts and path.suffix!='.pyc':
            rows.append(f"{sha(path)}  {path.relative_to(root).as_posix()}\n")
    manifest.write_text(''.join(rows))
    base=h.audit_tool(root); assert base['status'] in {'PASS','WARN'},base
    victim='tools/_patch_lib/python_patch_queue_dispatcher.py'
    manifest.write_text(''.join(line for line in manifest.read_text().splitlines(True) if not line.rstrip().endswith(victim)))
    bad=h.audit_tool(root); assert bad['status']=='FAIL',bad
    assert any('missing required managed path' in e and victim in e for e in bad['errors']),bad

print('PASS: v6.18.8 robustness audit fixes path safety, exact input lifecycle, signals/descendants, and health coverage')
