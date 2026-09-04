#!/usr/bin/env python3
from __future__ import annotations
import hashlib, json, os, shlex, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
LAUNCHER=HERE.parent/'run_python_patches.sh'
FILES=['python_patch_collect_progress_v6_7.py','python_patch_collect_compat.py','python_patch_collect_schema.py']

def sha(data: bytes)->str: return hashlib.sha256(data).hexdigest()
def make_request(path: Path,data: dict):
    with zipfile.ZipFile(path,'w',compression=zipfile.ZIP_DEFLATED) as z:
        z.writestr(path.stem+'.json',json.dumps(data,ensure_ascii=False,indent=2)+'\n')

def make_project(root: Path)->Path:
    lib=root/'tools'/'_patch_lib'; docs=lib/'docs'; docs.mkdir(parents=True)
    launcher=root/'tools'/'run_python_patches.sh'; launcher.write_bytes(LAUNCHER.read_bytes()); launcher.chmod(0o755)
    for name in FILES: (lib/name).write_bytes((HERE/name).read_bytes())
    (docs/'COLLECT_ACTION_SCHEMA.json').write_bytes((HERE/'docs'/'COLLECT_ACTION_SCHEMA.json').read_bytes())
    (root/'patchs').mkdir()
    bindir=root/'bin'; bindir.mkdir(); py=bindir/'python3'
    py.write_text('#!/usr/bin/env bash\nexec '+shlex.quote(sys.executable)+' -S "$@"\n'); py.chmod(0o755)
    return launcher

def run_collect(root: Path,launcher: Path,name: str):
    env=dict(os.environ); env['PATH']=str(root/'bin')+os.pathsep+env.get('PATH','')
    return subprocess.run([str(launcher),'collect','request',f'patchs/{name}'],cwd=root,env=env,text=True,capture_output=True,timeout=20)

# pack exact files
with tempfile.TemporaryDirectory(prefix='ptv612_pack_') as td:
    root=Path(td); launcher=make_project(root)
    payloads={'a/source.c':b'int a=1;\n','b/source.h':b'#pragma once\n'}
    for rel,content in payloads.items():
        p=root/rel; p.parent.mkdir(parents=True,exist_ok=True); p.write_bytes(content)
    name='CODE_COLLECTION_REQUEST_pack.zip'; req=root/'patchs'/name
    make_request(req,{'id':'pack-test','actions':[{'type':'pack','paths':list(payloads)}]})
    cp=run_collect(root,launcher,name); assert cp.returncode==0,(cp.stdout,cp.stderr)
    results=list((root/'artifacts'/'patch_tool_code_collections').glob('CODE_COLLECTION_RESULT_pack-test_*.zip')); assert len(results)==1
    with zipfile.ZipFile(results[0]) as z:
        manifest=json.loads(z.read('COLLECTION_MANIFEST.json'))
        assert manifest['tool_version']=='6.15.1' and manifest['file_count']==2
        by={e['path']:e for e in manifest['files']}
        for rel,content in payloads.items():
            assert by[rel]['sha256']==sha(content); assert z.read(by[rel]['archive_path'])==content

# overview/find/search/git are exact supported actions, including the shape that
# previously failed with "Unknown action type: overview".
with tempfile.TemporaryDirectory(prefix='ptv612_actions_') as td:
    root=Path(td); launcher=make_project(root)
    (root/'src').mkdir(); (root/'src'/'App.java').write_text('class App { String getPassword(){ return "x"; } }\n')
    (root/'db-game.properties').write_text('password=secret\n')
    subprocess.run(['git','init','-q'],cwd=root,check=True)
    subprocess.run(['git','config','user.email','ptv@example.invalid'],cwd=root,check=True)
    subprocess.run(['git','config','user.name','PTV Test'],cwd=root,check=True)
    subprocess.run(['git','add','src/App.java','db-game.properties'],cwd=root,check=True)
    subprocess.run(['git','commit','-qm','base'],cwd=root,check=True)
    (root/'src'/'App.java').write_text('class App { String getPassword(){ return "changed"; } }\n')
    name='CODE_COLLECTION_REQUEST_server-log-root-causes.zip'; req=root/'patchs'/name
    data={'id':'server-log-root-causes','actions':[
        {'type':'overview','path':'.','tree_depth':3},
        {'type':'find','paths':['.'],'patterns':['*.java','db-game.properties'],'collect':True},
        {'type':'search','query':'getPassword\\(|password=','regex':True,'paths':['.'],'context_lines':2},
        {'type':'git','sections':['status','log','diff_stat','diff']},
    ]}
    make_request(req,data); cp=run_collect(root,launcher,name); assert cp.returncode==0,(cp.stdout,cp.stderr)
    result=list((root/'artifacts'/'patch_tool_code_collections').glob('CODE_COLLECTION_RESULT_server-log-root-causes_*.zip'))[0]
    with zipfile.ZipFile(result) as z:
        names=set(z.namelist())
        assert 'reports/001_overview.md' in names and 'reports/002_find.md' in names
        assert 'reports/003_search.md' in names and 'reports/004_git.md' in names
        assert 'files/src/App.java' in names and 'files/db-game.properties' in names
        search=z.read('reports/003_search.md').decode(); assert 'getPassword' in search
        # report redaction does not alter exact collected files
        assert '<REDACTED>' in search or 'password=' in search

# Unsupported historical action fails exact-schema preflight, not private delegation.
with tempfile.TemporaryDirectory(prefix='ptv612_schema_bad_') as td:
    root=Path(td); launcher=make_project(root)
    name='CODE_COLLECTION_REQUEST_bad.zip'; req=root/'patchs'/name
    make_request(req,{'id':'bad','actions':[{'type':'symbol_graph','paths':['.'],'symbols':['x']}]})
    cp=run_collect(root,launcher,name); assert cp.returncode==2,(cp.stdout,cp.stderr)
    assert 'unsupported action type: symbol_graph' in cp.stdout+cp.stderr,cp.stdout+cp.stderr
    assert req.exists()

print('PASS: v6.15.1 self-contained COLLECT actions pack/overview/find/search/git and exact schema')
