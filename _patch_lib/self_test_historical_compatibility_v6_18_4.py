#!/usr/bin/env python3
from __future__ import annotations
import json, os, shutil, subprocess, tempfile, zipfile, sys
from pathlib import Path
HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent
ENV=dict(os.environ); ENV['PYTHONDONTWRITEBYTECODE']='1'; ENV['PTV_DISABLE_LIVE_STATUS']='1'

def install(root: Path):
    shutil.copytree(TOOLS,root/'tools'); (root/'tools'/'run_python_patches.sh').chmod(0o755); (root/'patchs').mkdir()

def manifest(pid: str): return {'schema_version':1,'patch':{'id':pid},'execution':{'timeout_seconds':20}}

def pack(root: Path, name: str, pid: str, marker: str):
    with zipfile.ZipFile(root/'patchs'/name,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest(pid)))
        z.writestr('patch_apply.py',f'from pathlib import Path\np=Path("executed.txt")\np.write_text(p.read_text() + {marker!r} if p.exists() else {marker!r})\n')

def run(root: Path,*args: str):
    return subprocess.run([str(root/'tools'/'run_python_patches.sh'),*args],cwd=root,text=True,capture_output=True,env=ENV,timeout=50)

# --all and historical automation flags execute all current PATCH inputs.
with tempfile.TemporaryDirectory(prefix='ptv6182_all_') as td:
    r=Path(td); install(r); pack(r,'a.zip','a','A'); pack(r,'b.zip','b','B')
    cp=run(r,'-a','-y','--zip-failed','--keep-failed-zip','--move'); assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert (r/'executed.txt').read_text()=='AB'; assert len(list((r/'patchs'/'patched').glob('*.zip')))==2

# Repeated --patch must execute every explicit item, not silently retain only the last argument.
with tempfile.TemporaryDirectory(prefix='ptv6182_repeat_') as td:
    r=Path(td); install(r); pack(r,'a.zip','a','A'); pack(r,'b.zip','b','B')
    cp=run(r,'--patch','a.zip','--patch','b.zip'); assert cp.returncode==0,(cp.stdout,cp.stderr); assert (r/'executed.txt').read_text()=='AB'

# --select numeric/range route must execute selected queue items only.
with tempfile.TemporaryDirectory(prefix='ptv6182_select_') as td:
    r=Path(td); install(r); pack(r,'a.zip','a','A'); pack(r,'b.zip','b','B'); pack(r,'c.zip','c','C')
    cp=run(r,'--select','1,3'); assert cp.returncode==0,(cp.stdout,cp.stderr); assert (r/'executed.txt').read_text()=='AC'; assert (r/'patchs'/'b.zip').exists()
    report=json.loads((r/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text(encoding='utf-8'))
    assert report.get('user_not_selected')==['b.zip'],report.get('user_not_selected')
    assert 'USER NOT SELECTED: 1 runnable package(s) preserved in patchs/' in cp.stdout,cp.stdout

# Legacy v4 multi-script archive: recognized scripts run in deterministic sorted relative path order.
with tempfile.TemporaryDirectory(prefix='ptv6182_v4_') as td:
    r=Path(td); install(r)
    with zipfile.ZipFile(r/'patchs'/'patch_legacy_multi.zip','w') as z:
        z.writestr('patch_01.py','from pathlib import Path\np=Path("legacy.txt"); p.write_text((p.read_text() if p.exists() else "")+"1")\n')
        z.writestr('zz/patch_02.py','from pathlib import Path\np=Path("legacy.txt"); p.write_text((p.read_text() if p.exists() else "")+"2")\n')
    cp=run(r,'--patch','patchs/patch_legacy_multi.zip'); assert cp.returncode==0,(cp.stdout,cp.stderr); assert (r/'legacy.txt').read_text()=='12'
    out=cp.stdout+cp.stderr; assert 'LEGACY_V4_COMPATIBILITY: TRUE' in out and 'PROJECT_SCOPE_VERIFIED: FALSE' in out

# Historical helper API remains callable beside the current API.
sys.path.insert(0,str(HERE)); import python_patch_utils as u
for name in ['run_patch','apply_ops','print_summary','zip_failed_files','maybe_prompt_zip_failed_files','find_project_root']:
    assert hasattr(u,name),name
with tempfile.TemporaryDirectory(prefix='ptv6182_helper_') as td:
    r=Path(td); (r/'a.txt').write_text('old')
    st=u.apply_ops(r,'legacy',[{'kind':'replace_exact','file':'a.txt','old':'old','new':'new'}]); assert not st.failures; assert (r/'a.txt').read_text()=='new'; assert 'a.txt' in st.changed_files

# Command-only package works in strict lane; inline interpreter execution is rejected.
def command_only(root: Path,name: str,argv):
    m=manifest(name); m['post_patch']={'commands':[{'name':'compat','argv':argv,'cwd':'.','timeout_seconds':10}], 'run_when_no_changes':True,'no_change_reason':'Historical compatibility diagnostic requires one safe command.'}
    with zipfile.ZipFile(root/'patchs'/f'{name}.zip','w') as z: z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(m))
with tempfile.TemporaryDirectory(prefix='ptv6182_cmdok_') as td:
    r=Path(td); install(r); command_only(r,'ok',['pwd']); cp=run(r,'--patch','patchs/ok.zip'); assert cp.returncode==0,(cp.stdout,cp.stderr); assert 'COMMAND_ONLY_PACKAGE' in cp.stdout
with tempfile.TemporaryDirectory(prefix='ptv6182_cmdbad_') as td:
    r=Path(td); install(r); command_only(r,'bad',['python3','-c','print(1)']); cp=run(r,'--patch','patchs/bad.zip'); assert cp.returncode==2,(cp.stdout,cp.stderr); assert 'command_not_allowed' in cp.stdout+cp.stderr

# The strict compatibility lane is additive: normal source-changing v6 post-patch keeps current behavior.
with tempfile.TemporaryDirectory(prefix='ptv6182_currentcmd_') as td:
    r=Path(td); install(r); (r/'x.txt').write_text('old')
    m=manifest('currentcmd'); m['targets']=['x.txt']; m['post_patch']={'commands':[{'argv':['python3','-c','print("CURRENT_OK")'],'cwd':'.','timeout_seconds':10}],'run_when_no_changes':False}
    with zipfile.ZipFile(r/'patchs'/'p.zip','w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(m)); z.writestr('patch_apply.py','from pathlib import Path\nPath("x.txt").write_text("new")\n')
    cp=run(r,'--patch','patchs/p.zip'); assert cp.returncode==0,(cp.stdout,cp.stderr); assert (r/'x.txt').read_text()=='new'; assert 'CURRENT_OK' in cp.stdout

print('PASS: v6.18.4 behavioral historical compatibility (--all/repeated --patch/--select/v4 archive/helper API/command-only strict lane)')
