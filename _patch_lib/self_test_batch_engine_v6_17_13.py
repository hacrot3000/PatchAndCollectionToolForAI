#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, json, os, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('ptv617_batch',HERE/'python_patch_queue_dispatcher.py')
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; assert spec.loader; spec.loader.exec_module(m)
assert m.VERSION=='6.17.13'


def install(root: Path):
    shutil.copytree(HERE.parent, root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()


def make_patch(root: Path, name: str, pid: str, body: str, *, targets=None, batch=None, bad_timeout=False):
    manifest={'schema_version':1,'patch':{'id':pid},'execution':{'timeout_seconds':0 if bad_timeout else 30}}
    if targets is not None: manifest['targets']=targets
    if batch is not None: manifest['batch']=batch
    with zipfile.ZipFile(root/'patchs'/name,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        z.writestr('apply.py',body)


def config(root: Path, *, selection='all', failure='fail_fast', transaction='patch'):
    (root/'.python_patch_tool.json').write_text(json.dumps({
        'automation':{'zero_argument':{'selection':selection,'non_interactive_confirmed':True}},
        'batch':{'failure_policy':failure,'transaction_policy':transaction},
    }),encoding='utf-8')


def run(root: Path, *args):
    env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'}
    return subprocess.run([str(root/'tools'/'run_python_patches.sh'),*args],cwd=root,text=True,capture_output=True,env=env,timeout=80)

# Controlled continue: failure with declared target unchanged is safe, so an independent successor runs.
with tempfile.TemporaryDirectory(prefix='ptv617_continue_') as td:
    r=Path(td); install(r); (r/'sentinel.txt').write_text('stable\n')
    config(r,failure='continue_independent')
    make_patch(r,'patch_1.zip','p1','print("FAIL-1")\nraise SystemExit(7)\n',targets=['sentinel.txt'])
    make_patch(r,'patch_2.zip','p2','from pathlib import Path\nPath("done.txt").write_text("ok")\n',targets=['done.txt'])
    cp=run(r); text=cp.stdout+cp.stderr
    assert cp.returncode==7,(cp.returncode,text)
    last=json.loads((r/'artifacts/patch_tool/LAST_RUN.json').read_text())
    rows=m._report_rows(last); assert [x['status'] for x in rows]==['FAIL','PASS'],rows
    assert (r/'done.txt').read_text()=='ok'
    assert 'CONTINUE AFTER FAILURE' in text,text

# Dependency blocking under continue mode: dependent is BLOCKED, unrelated PATCH continues.
with tempfile.TemporaryDirectory(prefix='ptv617_dep_') as td:
    r=Path(td); install(r); (r/'sentinel.txt').write_text('stable\n'); config(r,failure='continue_independent')
    make_patch(r,'patch_1.zip','base','raise SystemExit(9)\n',targets=['sentinel.txt'])
    make_patch(r,'patch_2.zip','child','from pathlib import Path\nPath("child.txt").write_text("bad")\n',targets=['child.txt'],batch={'depends_on':['base'],'on_dependency_failure':'block'})
    make_patch(r,'patch_3.zip','other','from pathlib import Path\nPath("other.txt").write_text("good")\n',targets=['other.txt'])
    cp=run(r); last=json.loads((r/'artifacts/patch_tool/LAST_RUN.json').read_text()); rows=m._report_rows(last)
    assert cp.returncode==9,(cp.returncode,cp.stdout,cp.stderr)
    assert [x['status'] for x in rows]==['FAIL','BLOCKED','PASS'],rows
    assert not (r/'child.txt').exists() and (r/'other.txt').read_text()=='good'

# Whole-batch preflight: invalid later package prevents an earlier valid PATCH from writing.
with tempfile.TemporaryDirectory(prefix='ptv617_preflight_') as td:
    r=Path(td); install(r); config(r)
    make_patch(r,'patch_1.zip','p1','from pathlib import Path\nPath("must_not_exist.txt").write_text("x")\n',targets=['must_not_exist.txt'])
    make_patch(r,'patch_2.zip','p2','print("bad")\n',targets=['x.txt'],bad_timeout=True)
    cp=run(r); assert cp.returncode==2,(cp.stdout,cp.stderr)
    assert not (r/'must_not_exist.txt').exists(), 'batch preflight allowed earlier source write'
    assert 'project unchanged' in (cp.stdout+cp.stderr)

# Atomic batch transaction: prior PASS is source-rolled-back and its package is requeued after later FAIL.
with tempfile.TemporaryDirectory(prefix='ptv617_tx_') as td:
    r=Path(td); install(r); (r/'state.txt').write_text('A\n'); config(r,transaction='batch')
    make_patch(r,'patch_1.zip','tx1','from pathlib import Path\nPath("state.txt").write_text("B\\n")\n',targets=['state.txt'])
    make_patch(r,'patch_2.zip','tx2','raise SystemExit(5)\n',targets=['state.txt'])
    cp=run(r); text=cp.stdout+cp.stderr; assert cp.returncode==5,(cp.returncode,text)
    assert (r/'state.txt').read_text()=='A\n',text
    last=json.loads((r/'artifacts/patch_tool/LAST_RUN.json').read_text()); assert last['batch_transaction']['status']=='ROLLED_BACK',last
    rows=m._report_rows(last); assert rows[0].get('batch_rolled_back') is True,rows
    assert (r/'patchs'/'patch_1.zip').is_file(), 'PASS package was not requeued for atomic replay'

# Source rollback PASS + replay requeue failure is contained/reported as rc=71,
# never allowed to escape as an unstructured dispatcher exception.
with tempfile.TemporaryDirectory(prefix='ptv617_requeue_fail_') as td:
    r=Path(td); install(r); (r/'state.txt').write_text('A\n'); config(r,transaction='batch')
    make_patch(r,'patch_1.zip','rq1','from pathlib import Path\nPath("state.txt").write_text("B\\n")\n',targets=['state.txt'])
    make_patch(r,'patch_2.zip','rq2','raise SystemExit(5)\n',targets=['state.txt'])
    real_requeue=m.requeue_packages
    def fail_requeue(*_args,**_kwargs): raise OSError('simulated requeue failure')
    m.requeue_packages=fail_requeue
    try:
        rc=m._run_queue(r)
    finally:
        m.requeue_packages=real_requeue
    assert rc==71,rc
    assert (r/'state.txt').read_text()=='A\n'
    last=json.loads((r/'artifacts/patch_tool/LAST_RUN.json').read_text())
    assert last['batch_transaction']['status']=='REQUEUE_FAILED',last
    assert last['batch_transaction']['original_rc']==5,last
    assert 'simulated requeue failure' in last['batch_transaction']['requeue_error'],last

# An unrelated PATCH is not forced to handle an unresolved predecessor. Only an
# explicit dependency/target relation (or explicit previous_failure declaration)
# creates the cross-run successor constraint.
with tempfile.TemporaryDirectory(prefix='ptv617_prev_unrelated_') as td:
    r=Path(td); install(r); (r/'sentinel.txt').write_text('stable\n')
    config(r,selection='all')
    make_patch(r,'patch_1.zip','old','raise SystemExit(6)\n',targets=['sentinel.txt'])
    first=run(r); assert first.returncode==6
    config(r,selection='newest')
    make_patch(r,'patch_2.zip','new','from pathlib import Path\nPath("new.txt").write_text("x")\n',targets=['new.txt'])
    cp=run(r); assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert (r/'new.txt').read_text()=='x'

with tempfile.TemporaryDirectory(prefix='ptv617_prev_delete_') as td:
    r=Path(td); install(r); (r/'sentinel.txt').write_text('stable\n'); config(r)
    make_patch(r,'patch_1.zip','old','raise SystemExit(6)\n',targets=['sentinel.txt'])
    assert run(r).returncode==6
    config(r,selection='newest')
    make_patch(r,'patch_2.zip','new','from pathlib import Path\nPath("new.txt").write_text("x")\n',targets=['new.txt'],batch={'previous_failure':{'patch_id':'old','patch_file':'patch_1.zip','action':'delete','reason':'successor supersedes failed predecessor'}})
    cp=run(r); assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert (r/'new.txt').read_text()=='x'
    assert not (r/'patchs'/'patch_1.zip').exists()
    assert list((r/'patchs'/'ignore').glob('*patch_1.zip'))

# Source before/after diff, support bundle and run-history management are persistent report features.
with tempfile.TemporaryDirectory(prefix='ptv617_report_') as td:
    r=Path(td); install(r); (r/'source.txt').write_text('old\n'); config(r)
    make_patch(r,'patch_change.zip','change','from pathlib import Path\nPath("source.txt").write_text("new\\n")\n',targets=['source.txt'])
    cp=run(r); assert cp.returncode==0,(cp.stdout,cp.stderr)
    last=json.loads((r/'artifacts/patch_tool/LAST_RUN.json').read_text()); rid=str(last['run_id']); row=m._report_rows(last)[0]
    diff=r/row['source_compare']['diff_path']; assert diff.is_file(); dt=diff.read_text(); assert '-old' in dt and '+new' in dt,dt
    sup=run(r,'report','--run-id',rid,'--support-item','1'); assert sup.returncode==0,(sup.stdout,sup.stderr)
    assert list((r/'artifacts/patch_tool/support').glob('*.zip'))
    assert run(r,'report','--pin',rid).returncode==0
    assert rid in m._load_pinned_runs(r)
    ex=run(r,'report','--export',rid); assert ex.returncode==0 and list((r/'artifacts/patch_tool/exports').glob('*.zip'))
    assert run(r,'report','--unpin',rid).returncode==0
    assert run(r,'report','--delete',rid).returncode==0
    assert m._find_history_entry(r,rid) is None and not (r/'artifacts/patch_tool/runs'/rid).exists()

# Retention cleanup preserves pinned runs while pruning old unpinned history.
with tempfile.TemporaryDirectory(prefix='ptv617_history_cleanup_') as td:
    r=Path(td)
    for i in range(35):
        rid=f'h{i:02d}'
        m._write_run_report(r,{'run_id':rid,'started_at':f'2026-08-09T00:00:{i:02d}+00:00','status':'PASS','exit_code':0,'selected':[f'{rid}.zip'],'results':[{'name':f'{rid}.zip','kind':'PATCH','status':'PASS','rc':0}]})
        if i==0:
            m._save_pinned_runs(r,{rid})
    assert m._find_history_entry(r,'h00') is not None
    assert len(m._history_entries(r))==m.RUN_HISTORY_LIMIT
    result=m._cleanup_history(r); assert result['pinned']==1 and m._find_history_entry(r,'h00') is not None,result

# Smart-resume groups include successful PATCHes whose atomic batch changes were rolled back.
synthetic={'status':'FAIL','selected':['a','b','c'],'not_executed':['c'],'results':[{'name':'a','status':'PASS','batch_rolled_back':True},{'name':'b','status':'FAIL'}]}
g=m._resume_groups(synthetic); assert g=={'replay':['a'],'failed':['b'],'remaining':['c']},g

print('PASS: v6.17.13 controlled continue, dependencies, whole-batch preflight, atomic rollback/requeue, predecessor action, smart resume, source diff, support bundle and history management')
