#!/usr/bin/env python3
from __future__ import annotations
import json, os, subprocess, tempfile, zipfile
from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
import python_patch_project_state as ps
import python_patch_runner as rr
assert ps.VERSION==rr.VERSION=='6.20.1'
RUNNER=HERE/'python_patch_runner.py'

def fixture(root:Path, *, dangerous=False):
    (root/'patchs').mkdir(); (root/'src').mkdir(); (root/'src/main.c').write_text('OLD\n')
    (root/'scripts').mkdir(); marker=root/'rerun.marker'
    (root/'scripts/check.py').write_text(
      'import pathlib,sys\n'
      f'm=pathlib.Path({str(marker)!r})\n'
      'if "--diag" in sys.argv or "ota" in sys.argv: m.write_text("RERUN")\n'
      'print("CHECK",sys.argv[1:])\n'
      'raise SystemExit(0 if "--diag" in sys.argv else 7)\n')
    append=['ota'] if dangerous else ['--diag']
    cfg={'validation_profiles':{
      'base':{'argv':['python3','-c','print("BASE")']},
      'cbuild':{'argv':['python3','scripts/check.py'],'cwd':'.','timeout_seconds':30,
                'diagnostic_rerun':{'enabled':True,'safe':True,'name':'C diagnostic','append_args':append,'timeout_seconds':30}},
      'fallback':{'argv':['python3','-c','print("FALLBACK")']}
    },'validation':{'selection':{'mode':'append','fallback_profiles':['fallback'],'rules':[{'name':'C source','include':['src/*.c'],'exclude':['src/generated/**'],'profiles':['cbuild']}]},'diagnostic_rerun':{'max_commands':1,'on_timeout':False}}}
    (root/'.python_patch_tool.json').write_text(json.dumps(cfg))
    manifest={'schema_version':1,'patch':{'id':'validation-auto'},'targets':['src/main.c']}
    ops={'patch_name':'validation-auto','ops':[{'kind':'replace','file':'src/main.c','old':'OLD','new':'NEW'}]}
    z=root/'patchs/p.zip'
    with zipfile.ZipFile(z,'w') as f:
        f.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest)); f.writestr('PATCH_TOOL_OPS.json',json.dumps(ops))
    return z,marker

# Local resolver: append matched profile; fallback only when no rule matches.
with tempfile.TemporaryDirectory(prefix='ptv_val_unit_') as td:
    root=Path(td); z,marker=fixture(root)
    rows,rep=ps.resolve_effective_validation_profiles(root,{'validation':{'profiles':['base']}},['src/main.c'])
    assert [x['name'] for x in rows]==['base','cbuild'] and rep['matched_rules'][0]['name']=='C source'
    rows,rep=ps.resolve_effective_validation_profiles(root,{'validation':{'profiles':['base']}},['docs/readme.md'])
    assert [x['name'] for x in rows]==['base','fallback'] and rep['fallback_used'] is True

# Actual post-payload delta selects cbuild. Diagnostic rerun may pass but primary FAIL remains FAIL.
with tempfile.TemporaryDirectory(prefix='ptv_val_run_') as td:
    root=Path(td); z,marker=fixture(root); result=root/'result.json'; env=os.environ.copy(); env['PTV_PATCH_RESULT_FILE']=str(result); env['PYTHONDONTWRITEBYTECODE']='1'
    cp=subprocess.run([sys.executable,str(RUNNER),'--patch','patchs/p.zip','--transaction','off'],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=60)
    data=json.loads(result.read_text())
    assert cp.returncode==7,(cp.returncode,cp.stdout)
    assert data['validation_selection']['changed_paths']==['src/main.c'] and data['validation_selection']['final_profiles']==['cbuild'],data['validation_selection']
    row=data['validation']['profiles'][0]; assert row['status']=='FAIL' and row['diagnostic_rerun']['status']=='PASS',row
    assert data['status']=='FAIL' and marker.read_text()=='RERUN'

# Dangerous-action hint blocks the diagnostic rerun entirely.
with tempfile.TemporaryDirectory(prefix='ptv_val_danger_') as td:
    root=Path(td); z,marker=fixture(root,dangerous=True); result=root/'result.json'; env=os.environ.copy(); env['PTV_PATCH_RESULT_FILE']=str(result); env['PYTHONDONTWRITEBYTECODE']='1'
    cp=subprocess.run([sys.executable,str(RUNNER),'--patch','patchs/p.zip','--transaction','off'],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=60)
    data=json.loads(result.read_text()); row=data['validation']['profiles'][0]
    assert cp.returncode==7 and row['diagnostic_rerun']['status']=='SKIPPED' and 'dangerous_action_hint:ota'==row['diagnostic_rerun']['reason'],row
    assert not marker.exists(),'dangerous rerun executed'

# Explicit no-validation disables both manifest-requested and delta-selected validation.
with tempfile.TemporaryDirectory(prefix='ptv_val_off_') as td:
    root=Path(td); z,marker=fixture(root); result=root/'result.json'; env=os.environ.copy(); env['PTV_PATCH_RESULT_FILE']=str(result); env['PYTHONDONTWRITEBYTECODE']='1'
    cp=subprocess.run([sys.executable,str(RUNNER),'--patch','patchs/p.zip','--transaction','off','--no-validation'],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=60)
    data=json.loads(result.read_text()); assert cp.returncode==0,(cp.returncode,cp.stdout)
    assert data['validation_selection']['status']=='DISABLED_BY_CLI' and data['status']=='PASS'
    assert not marker.exists() and (root/'src/main.c').read_text()=='NEW\n'
print('PASS: v6.20.1 delta-based validation selection, safe diagnostic rerun and --no-validation')
