#!/usr/bin/env python3
from __future__ import annotations
import builtins, importlib.util, io, json, os, shutil, subprocess, sys, tempfile, zipfile
from contextlib import redirect_stdout
from pathlib import Path

HERE=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('ptv_batch_report',HERE/'python_patch_queue_dispatcher.py')
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; assert spec.loader; spec.loader.exec_module(m)
assert m.VERSION=='6.18.4'

with tempfile.TemporaryDirectory(prefix='ptv6160_batch_') as td:
    root=Path(td)
    run_id='run_pass'
    run_dir=m._batch_run_dir(root,run_id); (run_dir/'items').mkdir(parents=True)
    logs=[]
    for i,name in enumerate(['a.zip','b.zip','c.zip'],1):
        p=run_dir/'items'/f'{i:03d}_{name}.log'; p.write_text(f'LOG-{name}\n',encoding='utf-8'); logs.append(p)
    report={
        'run_id':run_id,'status':'PASS','selected':['a.zip','b.zip','c.zip'],'not_executed':[],
        'results':[
            {'name':'a.zip','kind':'PATCH','status':'PASS','rc':0,'elapsed_seconds':1.2,'log_path':logs[0].relative_to(root).as_posix(),'patch_result':{'project_delta':{'changed_paths':['x']}}},
            {'name':'b.zip','kind':'PATCH','status':'PASS','rc':0,'elapsed_seconds':0.4,'log_path':logs[1].relative_to(root).as_posix(),'patch_result':{'project_delta':{'changed_paths':['y','z']}}},
            {'name':'c.zip','kind':'PATCH','status':'PASS','rc':0,'elapsed_seconds':0.2,'log_path':logs[2].relative_to(root).as_posix(),'patch_result':{'project_delta':{'changed_paths':[]}}},
        ],
    }
    m._finalize_batch_artifacts(root,report)
    summary=(root/report['batch_summary']).read_text(encoding='utf-8')
    aggregate=(root/report['batch_log']).read_text(encoding='utf-8')
    assert 'PASS=3' in summary and 'NOT_EXECUTED=0' in summary,summary
    assert all(f'LOG-{n}' in aggregate for n in ['a.zip','b.zip','c.zip']),aggregate

    # Interactive report menu: choose item 2 and see its detail log.
    answers=iter(['2','q']); old_input=builtins.input
    builtins.input=lambda _prompt='': next(answers)
    try:
        out=io.StringIO()
        with redirect_stdout(out): m._batch_report_menu(root,report)
    finally:
        builtins.input=old_input
    text=out.getvalue(); assert 'PATCH DETAIL' in text and 'b.zip' in text and 'LOG-b.zip' in text,text

# Report rendering must distinguish PASS/FAIL/NOT_EXECUTED correctly. A run can
# still contain NOT_EXECUTED rows under explicit fail_fast or a global safety-stop
# even though v6.18.4 defaults to continue_independent.
mixed={
    'run_id':'mixed','status':'FAIL','selected':['a.zip','b.zip','c.zip','d.zip'],
    'not_executed':['c.zip','d.zip'],
    'results':[
        {'name':'a.zip','kind':'PATCH','status':'PASS','rc':0},
        {'name':'b.zip','kind':'PATCH','status':'FAIL','rc':2,'fail_handoff':'FAIL_HANDOFF_b.zip','patch_result':{'diagnosis':{'kind':'source_drift'}}},
    ],
}
rows=m._report_rows(mixed); counts=m._batch_counts(rows)
assert [r['status'] for r in rows]==['PASS','FAIL','NOT_EXECUTED','NOT_EXECUTED'],rows
assert counts['PASS']==1 and counts['FAIL']==1 and counts['NOT_EXECUTED']==2,counts
assert 'not executed' in m._batch_summary_text(mixed)
assert 'source_drift' in m._batch_summary_text(mixed)

# Renderer supports the v6.18.4 continue_independent default: multiple FAIL rows
# are counted/rendered correctly when independent PATCHes fail in one invocation.
all_failed={
    'run_id':'future-multi-fail','status':'FAIL','selected':['a.zip','b.zip'], 'not_executed':[],
    'results':[{'name':'a.zip','kind':'PATCH','status':'FAIL','rc':2},{'name':'b.zip','kind':'PATCH','status':'FAIL','rc':3}],
}
assert m._batch_counts(m._report_rows(all_failed))['FAIL']==2
assert 'PASS=0' in m._batch_summary_text(all_failed) and 'FAIL=2' in m._batch_summary_text(all_failed) and 'NOT_EXECUTED=0' in m._batch_summary_text(all_failed)

# Full detail log capture is independent from the bounded 8 MiB handoff capture.
with tempfile.TemporaryDirectory(prefix='ptv6160_log_') as td:
    root=Path(td); log=root/'detail.log'
    cmd=[sys.executable,'-c','print("alpha"); print("beta")']
    rc,capture,result=m._run_patch_child(root,cmd,m.QueueItem('demo.zip','PATCH'),full_log_path=log)
    assert rc==0 and 'alpha' in capture and 'beta' in capture,(rc,capture)
    assert log.read_text(encoding='utf-8')=='alpha\nbeta\n'

sh=(HERE.parent/'run_python_patches.sh').read_text(encoding='utf-8')
ps=(HERE.parent/'run_python_patches.ps1').read_text(encoding='utf-8')
assert 'report' in sh and 'report' in ps

def install(root: Path) -> None:
    shutil.copytree(HERE.parent, root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()

def make_python_patch(path: Path, body: str) -> None:
    manifest={'schema_version':1,'patch':{'id':path.stem},'execution':{'timeout_seconds':30}}
    with zipfile.ZipFile(path,'w') as zf:
        zf.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        zf.writestr('apply.py',body)

def run_zero(root: Path):
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    return subprocess.run([str(root/'tools'/'run_python_patches.sh')],cwd=root,input='a\n',text=True,capture_output=True,env=env,timeout=40)

# End-to-end all-PASS batch: static report is emitted on redirected output,
# logs persist, and `report` can reopen the most recent useful run.
with tempfile.TemporaryDirectory(prefix='ptv6160_e2e_pass_') as td:
    root=Path(td); install(root)
    for i in range(1,4):
        make_python_patch(root/'patchs'/f'patch_{i}.zip',f'print("PASS-LOG-{i}")\n')
    cp=run_zero(root); text=cp.stdout+cp.stderr
    assert cp.returncode==0,(cp.returncode,text)
    assert 'BATCH RESULT — PASS' in text and 'PASS=3' in text and 'NOT EXECUTED=0' in text,text
    last=json.loads((root/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text(encoding='utf-8'))
    assert len(last['results'])==3 and all(x['status']=='PASS' for x in last['results']),last
    assert (root/last['batch_log']).is_file() and 'PASS-LOG-3' in (root/last['batch_log']).read_text(encoding='utf-8')
    useful_run_id=str(last['run_id'])
    idle=run_zero(root); assert idle.returncode==0,(idle.stdout,idle.stderr)
    # Zero-work probes are not runs and must not overwrite LAST_RUN/history.
    idle_last=json.loads((root/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text(encoding='utf-8'))
    assert idle_last['status']=='PASS' and str(idle_last['run_id'])==useful_run_id,idle_last
    reopened=subprocess.run([str(root/'tools'/'run_python_patches.sh'),'report'],cwd=root,text=True,capture_output=True,env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'},timeout=20)
    assert reopened.returncode==0 and 'PASS=3' in reopened.stdout+reopened.stderr,(reopened.stdout,reopened.stderr)
    listed=subprocess.run([str(root/'tools'/'run_python_patches.sh'),'report','--list'],cwd=root,text=True,capture_output=True,env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'},timeout=20)
    assert listed.returncode==0 and useful_run_id in listed.stdout+listed.stderr,(listed.stdout,listed.stderr)
    by_id=subprocess.run([str(root/'tools'/'run_python_patches.sh'),'report','--run-id',useful_run_id],cwd=root,text=True,capture_output=True,env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'},timeout=20)
    assert by_id.returncode==0 and 'PASS=3' in by_id.stdout+by_id.stderr,(by_id.stdout,by_id.stderr)

# End-to-end mixed batch with an uncontained/unknown failure state safety-stops:
# prior PASS, first FAIL, later selected PATCH is NOT_EXECUTED and remains queued.
with tempfile.TemporaryDirectory(prefix='ptv6160_e2e_fail_') as td:
    root=Path(td); install(root)
    make_python_patch(root/'patchs'/'patch_1.zip','print("FIRST-PASS")\n')
    make_python_patch(root/'patchs'/'patch_2.zip','print("SECOND-FAIL")\nraise SystemExit(7)\n')
    make_python_patch(root/'patchs'/'patch_3.zip','print("MUST-NOT-RUN")\n')
    cp=run_zero(root); text=cp.stdout+cp.stderr
    assert cp.returncode==7,(cp.returncode,text)
    assert 'BATCH RESULT — FAIL' in text and 'PASS=1' in text and 'FAIL=1' in text and 'NOT EXECUTED=1' in text,text
    last=json.loads((root/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text(encoding='utf-8'))
    assert [r['status'] for r in m._report_rows(last)]==['PASS','FAIL','NOT_EXECUTED'],last
    assert (root/'patchs'/'patch_3.zip').is_file()
    aggregate=(root/last['batch_log']).read_text(encoding='utf-8')
    assert 'FIRST-PASS' in aggregate and 'SECOND-FAIL' in aggregate and 'MUST-NOT-RUN' not in aggregate,aggregate

print('PASS: v6.18.4 persistent batch summary, aggregate/detail logs, continuation/safety-stop status model and report browser')
