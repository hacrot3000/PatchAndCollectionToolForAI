#!/usr/bin/env python3
from __future__ import annotations
import json, os, subprocess, sys, tempfile, zipfile
from pathlib import Path

from python_patch_manual_workflow import validate_manual_execution, run_manual_workflow, ManualWorkflowError, _read_exit_code, _append_manual_exit
from python_patch_package_schema import validate_manifest

base={'stop_on_failure':True,'package_result':True,'steps':[{'id':'build-server','title':'Build server','description':'manual evidence test','cwd':'.','argv':['touch','SHOULD_NOT_BE_CREATED'],'expected_exit_codes':[0]}]}
validate_manual_execution(base)
validate_manifest({'schema_version':1,'patch':{'id':'manual-only'},'payload':'manual_only','manual_execution':base})

for bad in [
    {'stop_on_failure':True,'steps':[{'id':'x','command':'echo x','argv':['echo','x']}]},
    {'steps':[{'id':'x','argv':['bash','-c','echo x']}]},
    {'steps':[{'id':'x','argv':['sh','-c','echo x']}]},
    {'steps':[{'id':'x','argv':['python','-c','print(1)']}]},
    {'steps':[{'id':'x','argv':['node','-e','1+1']}]},
    {'steps':[{'id':'x','argv':['powershell','-Command','Get-ChildItem']}]},
    {'steps':[{'id':'x','argv':['python3','-cprint(123)']}]},
    {'steps':[{'id':'x','argv':['node','-econsole.log(1)']}]},
    {'steps':[{'id':'x','argv':['bash','-cprintf x']}]},
    {'steps':[{'id':'x','argv':['bash','-lc','echo x']}]},
    {'steps':[{'id':'x','argv':['sh','-xc','echo x']}]},
    {'steps':[{'id':'x','argv':['/bin/bash','-ec','echo x']}]},
    {'steps':[{'id':'x','argv':['env','bash','-c','echo x']}]},
    {'steps':[{'id':'x','argv':['env','-S','bash -c echo x']}]},
    {'steps':[{'id':'x','argv':['env','--split-string','python3 -c print(1)']}]},
    {'steps':[{'id':'x','argv':['env','--split-string=bash -lc echo x']}]},
    {'steps':[{'id':'x','argv':['/usr/bin/env','python3','-c','print(1)']}]},
    {'steps':[{'id':'x','argv':['powershell.exe','-Command:Get-ChildItem']}]},
    {'steps':[{'id':'x','argv':['cmd.exe','/cecho x']}]},
    {'steps':[{'id':'x','argv':['busybox','sh','-c','echo x']}]},
]:
    try: validate_manual_execution(bad)
    except ManualWorkflowError: pass
    else: raise AssertionError(f'unsafe manual command escape accepted: {bad}')

# Human-only workflow: the test fabricates the evidence log. The declared touch
# command must never execute inside Patch Tool.
with tempfile.TemporaryDirectory(prefix='ptv620_manual_') as td:
    root=Path(td); root.mkdir(exist_ok=True)
    def evidence_input(prompt:str)->str:
        if prompt.startswith('Manual step'):
            instruction=next((root/'artifacts'/'ptv_manual').glob('M_*/steps/001_*_instruction.txt'))
            log=instruction.with_name(instruction.name.replace('_instruction.txt','_console.log'))
            log.write_text('simulated user console\n[PTV_MANUAL_EXIT_CODE=0]\n',encoding='utf-8')
            return ''
        raise AssertionError(prompt)
    report=run_manual_workflow(root,{'manual_execution':base},'manual_semantic',input_fn=evidence_input)
    assert report and report['status']=='PASS' and report['rc']==0,report
    assert report['steps'][0]['log_size_bytes'] > 0
    assert len(report['steps'][0]['log_sha256']) == 64
    assert not (root/'SHOULD_NOT_BE_CREATED').exists(),'Patch Tool executed the manual command'
    z=root/report['result_zip']; t=root/report['result_text']; assert z.is_file() and t.is_file()
    with zipfile.ZipFile(z) as f:
        names=set(f.namelist()); assert {'MANUAL_EXECUTION.json','MANUAL_EXECUTION_REPORT.md'}<=names,names
        assert any(x.startswith('steps/001_') and x.endswith('_instruction.txt') for x in names)
        assert any(x.startswith('steps/001_') and x.endswith('_console.log') for x in names)

# Manual fallback: copied console log + explicit exit code is accepted and the
# tool writes the evidence marker itself.
with tempfile.TemporaryDirectory(prefix='ptv620_manual_fallback_') as td:
    root=Path(td); state={'n':0}
    def fallback_input(prompt:str)->str:
        state['n']+=1
        if prompt.startswith('Manual step'):
            instruction=next((root/'artifacts'/'ptv_manual').glob('M_*/steps/001_*_instruction.txt'))
            log=instruction.with_name(instruction.name.replace('_instruction.txt','_console.log'))
            log.write_text('copied console output\n',encoding='utf-8')
            return 'm'
        if prompt.startswith('Exit code'): return '0'
        raise AssertionError(prompt)
    report=run_manual_workflow(root,{'manual_execution':base},'manual_fallback',input_fn=fallback_input)
    assert report and report['status']=='PASS',report
    log=root/report['steps'][0]['log_file']; assert '[PTV_MANUAL_EXIT_CODE=0]' in log.read_text()

# Large logs are verified from a bounded tail; full evidence remains on disk.
with tempfile.TemporaryDirectory(prefix='ptv620_manual_large_') as td:
    root=Path(td); log=root/'large.log'
    with log.open('wb') as fh:
        fh.write(b'x' * (2 * 1024 * 1024))
        fh.write(b'\n[PTV_MANUAL_EXIT_CODE=7]\n')
    assert _read_exit_code(log) == 7

# Symlink evidence must never be followed for append/read.
if hasattr(os, 'symlink'):
    with tempfile.TemporaryDirectory(prefix='ptv620_manual_link_') as td:
        root=Path(td); target=root/'target.log'; target.write_text('safe\n',encoding='utf-8')
        link=root/'link.log'
        try:
            link.symlink_to(target)
        except OSError:
            pass
        else:
            assert _read_exit_code(link) is None
            try: _append_manual_exit(link,0)
            except ManualWorkflowError: pass
            else: raise AssertionError('manual evidence append followed symlink')
            assert target.read_text(encoding='utf-8') == 'safe\n'

# Ctrl+C finalizes the current step and packages the evidence/report before the
# interrupt propagates to the caller.
with tempfile.TemporaryDirectory(prefix='ptv620_manual_interrupt_') as td:
    root=Path(td)
    try:
        run_manual_workflow(root,{'manual_execution':base},'manual_interrupt',input_fn=lambda _prompt: (_ for _ in ()).throw(KeyboardInterrupt()))
    except KeyboardInterrupt:
        pass
    else:
        raise AssertionError('KeyboardInterrupt did not propagate')
    state_files=list((root/'artifacts'/'ptv_manual').glob('M_*/MANUAL_EXECUTION.json'))
    assert len(state_files)==1,state_files
    state=json.loads(state_files[0].read_text(encoding='utf-8'))
    assert state['status']=='ABORTED' and state['rc']==130,state
    assert state['steps'][0]['status']=='ABORTED',state
    assert list((root/'artifacts'/'patch_tool'/'manual').glob('MANUAL_EXECUTION_RESULT_*.zip'))
    assert list((root/'artifacts'/'patch_tool'/'manual').glob('MANUAL_EXECUTION_RESULT_*.txt'))

# Non-TTY gate is before any Python payload mutation.
with tempfile.TemporaryDirectory(prefix='ptv620_manual_nontty_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'patchs'/'patched').mkdir()
    manifest={'schema_version':1,'patch':{'id':'nontty'},'manual_execution':base}
    package=root/'patchs'/'nontty.zip'
    with zipfile.ZipFile(package,'w',compression=zipfile.ZIP_DEFLATED) as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        z.writestr('patch_nontty.py','from pathlib import Path\nPath("PAYLOAD_SHOULD_NOT_RUN").write_text("bad")\n')
    runner=Path(__file__).resolve().parent/'python_patch_runner.py'
    cp=subprocess.run([sys.executable,'-S',str(runner),'--patch',str(package)],cwd=root,stdin=subprocess.DEVNULL,text=True,capture_output=True,env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'},timeout=30)
    assert cp.returncode==2,(cp.stdout,cp.stderr)
    assert 'manual_execution requires interactive TTY' in cp.stdout+cp.stderr
    assert not (root/'PAYLOAD_SHOULD_NOT_RUN').exists(),'payload mutated project before non-TTY manual gate'


# Dispatcher/HISTORY and FAIL_HANDOFF preserve manual result evidence.
import python_patch_queue_dispatcher as dispatcher
with tempfile.TemporaryDirectory(prefix='ptv620_manual_handoff_') as td:
    root=Path(td)
    work=root/'artifacts'/'ptv_manual'/'M_demo'; (work/'steps').mkdir(parents=True)
    (work/'steps'/'001_x_instruction.txt').write_text('instruction\n',encoding='utf-8')
    (work/'steps'/'001_x_console.log').write_text('console\n[PTV_MANUAL_EXIT_CODE=1]\n',encoding='utf-8')
    out=root/'artifacts'/'patch_tool'/'manual'; out.mkdir(parents=True)
    rz=out/'MANUAL_EXECUTION_RESULT_demo.zip'; rt=out/'MANUAL_EXECUTION_RESULT_demo.txt'
    with zipfile.ZipFile(rz,'w') as z: z.writestr('MANUAL_EXECUTION.json','{}')
    rt.write_text('manual clear text\n',encoding='utf-8')
    manual={'status':'FAIL','rc':1,'result_zip':rz.relative_to(root).as_posix(),'result_text':rt.relative_to(root).as_posix(),'work_dir':work.relative_to(root).as_posix()}
    row={'name':'patch_demo.zip','patch_result':{'manual_execution':manual}}
    important=dispatcher._important_row_artifacts(root,row)
    labels=[x[0] for x in important]
    assert 'Manual result' in labels and 'Manual result TXT' in labels,important
    handoff=dispatcher._create_fail_handoff(root,dispatcher.QueueItem('patch_demo.zip','PATCH'),1,'manual failed\n',{'manual_execution':manual,'diagnosis':{'kind':'manual_execution_failed','message':'manual failed','affected_paths':[]}},None)
    assert handoff and handoff.is_file(),handoff
    with zipfile.ZipFile(handoff) as z:
        names=set(z.namelist())
        assert 'manual_execution/MANUAL_EXECUTION_RESULT_demo.zip' in names,names
        assert 'manual_execution/MANUAL_EXECUTION_RESULT_demo.txt' in names,names
        assert 'manual_execution/work/steps/001_x_console.log' in names,names
        summary=json.loads(z.read('FAIL_SUMMARY.json'))
        assert summary['manual_execution']['status']=='FAIL',summary

print('PASS: v6.20.2 manual_execution is human-only, stepwise, structured-argv, log-verified, ZIP+TXT packaged, and has no raw shell command escape hatch')
