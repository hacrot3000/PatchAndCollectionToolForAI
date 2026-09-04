#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, json, os, subprocess, sys, tarfile, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent

def load(name,file):
    spec=importlib.util.spec_from_file_location(name,HERE/file)
    m=importlib.util.module_from_spec(spec); sys.modules[name]=m; assert spec.loader; spec.loader.exec_module(m); return m

q=load('ptv_audit_q','python_patch_queue_dispatcher.py')
r=load('ptv_audit_r','python_patch_runner.py')
u=load('ptv_audit_u','python_patch_utils.py')
b=load('ptv_audit_b','python_patch_batch.py')
c=load('ptv_audit_c','python_patch_collect_compat.py')
cs=load('ptv_audit_cs','python_patch_collect_schema.py')

# v6.18.1: even an internal/tool failure may continue independent work only when
# post-failure evidence proves the project was not modified. Unknown/changed state stops.
ok,reason=q._safe_to_continue_after_failure({'rc':2,'patch_result':{'diagnosis':{'kind':'internal_error'},'partial_modification':{'detected':False}}})
assert ok is True and reason=='failure_contained_project_unchanged',(ok,reason)
ok,reason=q._safe_to_continue_after_failure({'rc':2,'patch_result':{'diagnosis':{'kind':'internal_error'},'partial_modification':{'detected':None}}})
assert ok is False and reason=='unsafe_partial_or_unknown_state',(ok,reason)

# Implicit `new anywhere` is not idempotency proof; explicit already remains supported.
text='HEADER\nnew-value\nTARGET=old-value\n'
assert u._already(text,'new-value',None) is False
assert u._already(text,'new-value','new-value') is True

# Ignored target changes must still count as project modification.
with tempfile.TemporaryDirectory(prefix='ptv6171_ignored_') as td:
    root=Path(td)
    subprocess.run(['git','init','-q'],cwd=root,check=True)
    subprocess.run(['git','config','user.email','test@example.invalid'],cwd=root,check=True)
    subprocess.run(['git','config','user.name','PTV Test'],cwd=root,check=True)
    (root/'.gitignore').write_text('ignored.txt\n')
    subprocess.run(['git','add','.gitignore'],cwd=root,check=True)
    subprocess.run(['git','commit','-qm','base'],cwd=root,check=True)
    (root/'ignored.txt').write_text('A\n')
    before_fp=r._git_worktree_fingerprint(root); before_dirty=r._dirty_paths(root); before_targets=r._snapshot_declared_paths(root,['ignored.txt'])
    (root/'ignored.txt').write_text('B\n')
    delta=r._partial_state(root,before_fp=before_fp,before_dirty=before_dirty,before_targets=before_targets,target_paths=['ignored.txt'])
    assert delta['detected'] is True and 'ignored.txt' in delta['changed_paths'],delta

# Batch effective target set includes OPS files beyond manifest.targets; rollback verifies both.
with tempfile.TemporaryDirectory(prefix='ptv6171_batch_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'a.txt').write_text('A0\n'); (root/'b.txt').write_text('B0\n')
    manifest={'schema_version':1,'patch':{'id':'audit-targets'},'execution':{'timeout_seconds':30},'targets':['a.txt']}
    ops={'patch_name':'audit-targets','ops':[{'kind':'replace','file':'b.txt','old':'B0','new':'B1'}]}
    with zipfile.ZipFile(root/'patchs'/'patch_audit.zip','w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest)); z.writestr('PATCH_TOOL_OPS.json',json.dumps(ops))
    meta=b.load_patch_meta(root,'patch_audit.zip')
    assert set(meta.effective_targets)=={'a.txt','b.txt'},meta.effective_targets
    snap_root=root/'snap'; snap=b.snapshot_targets(root,meta.effective_targets,snap_root)
    (root/'a.txt').write_text('A1\n'); (root/'b.txt').write_text('B1\n')
    restored=b.restore_targets(root,snap_root,snap)
    assert restored['status']=='PASS' and restored.get('verified') is True,restored
    assert (root/'a.txt').read_text()=='A0\n' and (root/'b.txt').read_text()=='B0\n'

# COLLECT never enumerates or copies its own internal artifacts.
with tempfile.TemporaryDirectory(prefix='ptv6171_collect_') as td:
    root=Path(td); (root/'artifacts'/'patch_tool_code_collections').mkdir(parents=True); (root/'src').mkdir(); (root/'src'/'x.py').write_text('print(1)\n')
    internal=root/'artifacts'/'patch_tool_code_collections'/'old.zip'; internal.write_bytes(b'x')
    listed=[p.relative_to(root).as_posix() for p in c._iter_files(root,root,max_files=100)]
    assert 'src/x.py' in listed and not any(x.startswith('artifacts/patch_tool_code_collections/') for x in listed),listed
    req={'id':'audit','title':'audit','actions':[{'type':'find','patterns':['*.zip'],'paths':['.'],'collect':True,'max_results':10}], 'limits':dict(cs.DEFAULT_LIMITS)}
    builder=c.ResultBuilder(root,req,'CODE_COLLECTION_REQUEST.json')
    try:
        try: builder.add_exact_file(builder.temp.relative_to(root).as_posix(),builder.temp,source_action=1)
        except ValueError as exc: assert 'collector output' in str(exc) or 'internal artifact' in str(exc)
        else: raise AssertionError('collector accepted its own temp ZIP')
    finally: builder.abort()

# Regex COLLECT is isolated in a worker: normal regex works and catastrophic regex times out.
with tempfile.TemporaryDirectory(prefix='ptv6171_regex_') as td:
    root=Path(td); (root/'x.txt').write_text('alpha 123\n',encoding='utf-8')
    action={'type':'search','query':r'alpha\s+\d+','regex':True,'paths':['.'],'context_lines':0,'max_matches':10}
    payload=c._search_action(root,action,dict(cs.DEFAULT_LIMITS))
    report=payload['report']
    assert 'Matches: 1' in report and 'x.txt:1' in report,payload
    (root/'bad.txt').write_text('a'*50000+'!\n',encoding='utf-8')
    old_timeout=c.REGEX_SEARCH_TIMEOUT_SECONDS; c.REGEX_SEARCH_TIMEOUT_SECONDS=0.20
    try:
        bad={'type':'search','query':r'(a+)+$','regex':True,'paths':['bad.txt'],'context_lines':0,'max_matches':1}
        try: c._search_action(root,bad,dict(cs.DEFAULT_LIMITS))
        except ValueError as exc: assert 'hard timeout' in str(exc),exc
        else: raise AssertionError('catastrophic regex was not timeout-contained')
    finally:
        c.REGEX_SEARCH_TIMEOUT_SECONDS=old_timeout

# Request-controlled limits cannot raise the hard ceiling.
try:
    cs.validate_request_data({'actions':[{'type':'overview'}],'limits':{'max_total_bytes':cs.HARD_LIMITS['max_total_bytes']+1}})
except cs.CollectSchemaError as exc:
    assert 'hard ceiling' in str(exc)
else:
    raise AssertionError('COLLECT hard ceiling was bypassed')

# Git commit rc=1 from a rejecting hook is failure; pre-existing dirty target is refused.
with tempfile.TemporaryDirectory(prefix='ptv6171_git_') as td:
    root=Path(td); subprocess.run(['git','init','-q'],cwd=root,check=True); subprocess.run(['git','config','user.email','t@example.invalid'],cwd=root,check=True); subprocess.run(['git','config','user.name','PTV'],cwd=root,check=True)
    (root/'a.txt').write_text('A\n'); subprocess.run(['git','add','a.txt'],cwd=root,check=True); subprocess.run(['git','commit','-qm','base'],cwd=root,check=True)
    before=r._dirty_paths(root); (root/'a.txt').write_text('B\n'); after=r._dirty_paths(root)
    hook=root/'.git'/'hooks'/'pre-commit'; hook.write_text('#!/bin/sh\nexit 1\n'); hook.chmod(0o755)
    rc=r._run_git_policy(root,{'git':{'add':'auto','commit':'auto','commit_message':'audit','fail_on_error':True}},before,after)
    assert rc==1,rc
    staged=subprocess.run(['git','diff','--cached','--name-only'],cwd=root,text=True,capture_output=True).stdout.strip()
    assert staged=='',f'failed auto-commit leaked staged paths: {staged}'
    hook.unlink(); (root/'a.txt').write_text('USER\n'); before=r._dirty_paths(root); (root/'a.txt').write_text('USER+PATCH\n'); after=r._dirty_paths(root)
    rc=r._run_git_policy(root,{'git':{'add':'auto','commit':'auto','commit_message':'audit','fail_on_error':True}},before,after)
    assert rc==1,rc
    staged=subprocess.run(['git','diff','--cached','--name-only'],cwd=root,text=True,capture_output=True).stdout.strip()
    assert staged=='',f'dirty-target guard changed index: {staged}'

# Archive hardening: Windows drive/ADS path and case-fold collisions are rejected.
for bad in ['D:evil.txt','dir/file.txt:ads']:
    try: r._safe_archive_name(bad)
    except ValueError: pass
    else: raise AssertionError(f'unsafe archive name accepted: {bad}')
with tempfile.TemporaryDirectory(prefix='ptv6171_zip_') as td:
    z=Path(td)/'x.zip'; out=Path(td)/'out'; out.mkdir()
    with zipfile.ZipFile(z,'w') as f: f.writestr('A.txt','1'); f.writestr('a.txt','2')
    try: r._safe_extract_zip(z,out)
    except ValueError as exc: assert 'colliding' in str(exc)
    else: raise AssertionError('case-fold collision archive accepted')


# Legacy TAR/ZIP archives recognized by discovery are also runnable by planner/runner.
with tempfile.TemporaryDirectory(prefix='ptv6171_legacy_') as td:
    root=Path(td); (root/'patchs').mkdir()
    tar_path=root/'patchs'/'legacy.tgz'
    with tarfile.open(tar_path,'w:gz') as tf:
        data=b'print("legacy ok")\n'; info=tarfile.TarInfo('patch_legacy.py'); info.size=len(data); tf.addfile(info,io.BytesIO(data))
    meta=b.load_patch_meta(root,'legacy.tgz')
    assert meta.patch_id=='legacy:legacy.tgz' and not meta.effective_targets,meta
    tmp,extracted,manifest,kind,payload,ops_data,preflight=r._prepare_package(root,tar_path)
    try:
        assert kind=='python' and preflight.get('legacy_archive') is True and payload.name=='patch_legacy.py',preflight
    finally:
        if tmp is not None: tmp.cleanup()


# Batch replay requeue never overwrites a concurrent queue replacement.
with tempfile.TemporaryDirectory(prefix='ptv6173_requeue_') as td:
    root=Path(td); (root/'patchs').mkdir(); snap=root/'snap'; snap.mkdir(); (snap/'pkg.bin').write_bytes(b'EXECUTED')
    (root/'patchs'/'patch_x.zip').write_bytes(b'NEW-QUEUE')
    replay=b.requeue_packages(root,snap,{'patch_x.zip':'pkg.bin'})
    assert (root/'patchs'/'patch_x.zip').read_bytes()==b'NEW-QUEUE'
    assert replay['patch_x.zip']!='patch_x.zip' and (root/'patchs'/replay['patch_x.zip']).read_bytes()==b'EXECUTED'
    for child in (root/'patchs').iterdir(): child.unlink()
    real_link=b.os.link; first={'v':True}
    def race_link(src,dst,*args,**kwargs):
        if first['v']:
            first['v']=False; Path(dst).write_bytes(b'RACE-WINNER'); raise FileExistsError()
        return real_link(src,dst,*args,**kwargs)
    b.os.link=race_link
    try: replay=b.requeue_packages(root,snap,{'patch_x.zip':'pkg.bin'})
    finally: b.os.link=real_link
    assert (root/'patchs'/'patch_x.zip').read_bytes()==b'RACE-WINNER'
    assert (root/'patchs'/replay['patch_x.zip']).read_bytes()==b'EXECUTED'

# A transaction may never report replay success when its exact package snapshot vanished.
with tempfile.TemporaryDirectory(prefix='ptv6173_requeue_missing_') as td:
    root=Path(td); (root/'patchs').mkdir(); snap=root/'snap'; snap.mkdir()
    try:
        b.requeue_packages(root,snap,{'patch_missing.zip':'missing.bin'})
    except b.BatchPlanError as exc:
        assert exc.kind=='batch_requeue_failed' and 'missing or unsafe' in str(exc),exc
    else:
        raise AssertionError('missing replay package snapshot was silently accepted')

# Hard-link publication has an exclusive-copy fallback and still verifies bytes.
with tempfile.TemporaryDirectory(prefix='ptv6171_publish_') as td:
    d=Path(td); src=d/'snapshot.bin'; dst=d/'published.bin'; src.write_bytes(b'audit-bytes')
    digest=r._sha256(src); real_link=r.os.link
    def no_links(*args,**kwargs): raise OSError('hardlinks unavailable')
    r.os.link=no_links
    try:
        r._publish_executed_patch(src,dst,digest)
    finally:
        r.os.link=real_link
    assert dst.read_bytes()==b'audit-bytes' and r._sha256(dst)==digest

# FAIL_HANDOFF sensitive-content detection warns without embedding secret values in metadata.
warn=q._sensitive_handoff_warnings('Authorization: Bearer super-secret-value',[])
assert any('authorization_header' in x for x in warn) and all('super-secret-value' not in x for x in warn),warn

# Batch dispatcher can hold the mutation lock across child PATCHes without deadlock,
# while an unrelated runner blocks until the batch owner releases it.
with tempfile.TemporaryDirectory(prefix='ptv6171_batchlock_') as td:
    root=Path(td); (root/'patchs').mkdir()
    p1=root/'patchs'/'patch_inside.py'; p1.write_text('from pathlib import Path\nPath("inside.txt").write_text("ok")\n',encoding='utf-8')
    p2=root/'patchs'/'patch_outside.py'; p2.write_text('from pathlib import Path\nPath("outside.txt").write_text("ok")\n',encoding='utf-8')
    lock,key,token=q._acquire_batch_mutation_lock(root)
    old_key=os.environ.get('PTV_PARENT_MUTATION_LOCK_KEY'); old_token=os.environ.get('PTV_PARENT_MUTATION_LOCK_TOKEN')
    try:
        env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1','PTV_PARENT_MUTATION_LOCK_KEY':key,'PTV_PARENT_MUTATION_LOCK_TOKEN':token}
        cp=subprocess.run([sys.executable,str(HERE/'python_patch_runner.py'),'--patch','patchs/patch_inside.py','--transaction','off'],cwd=root,env=env,text=True,capture_output=True,timeout=10)
        assert cp.returncode==0,(cp.stdout,cp.stderr)
        assert (root/'inside.txt').read_text()=='ok'
        env2={k:v for k,v in os.environ.items() if k not in {'PTV_PARENT_MUTATION_LOCK_KEY','PTV_PARENT_MUTATION_LOCK_TOKEN'}}; env2['PYTHONDONTWRITEBYTECODE']='1'
        proc=subprocess.Popen([sys.executable,str(HERE/'python_patch_runner.py'),'--patch','patchs/patch_outside.py','--transaction','off'],cwd=root,env=env2,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
        import time as _time; _time.sleep(.25)
        assert proc.poll() is None and not (root/'outside.txt').exists(), 'independent PATCH bypassed batch mutation lock'
        q._release_batch_mutation_lock(lock); lock=None
        out,_=proc.communicate(timeout=10)
        assert proc.returncode==0,out
        assert (root/'outside.txt').read_text()=='ok'
    finally:
        if lock is not None: q._release_batch_mutation_lock(lock)
        for name,value in [('PTV_PARENT_MUTATION_LOCK_KEY',old_key),('PTV_PARENT_MUTATION_LOCK_TOKEN',old_token)]:
            if value is None: os.environ.pop(name,None)
            else: os.environ[name]=value

# Source writes use temporary+replace rather than direct Path.write_text on target.
source=(HERE/'python_patch_utils.py').read_text(encoding='utf-8')
assert 'tempfile.mkstemp(prefix=f".{path.name}.ptv-write-"' in source and 'os.replace(tmp, path)' in source
assert '_acquire_project_mutation_lock' in (HERE/'python_patch_runner.py').read_text(encoding='utf-8')
assert 'python_patch_ops_worker.py' in (HERE/'python_patch_runner.py').read_text(encoding='utf-8')

print('PASS: v6.18.1 audit regressions cover rollback completeness, contained-failure continuation, COLLECT self-output, mutation integrity, idempotency, Git, archives and limits')
