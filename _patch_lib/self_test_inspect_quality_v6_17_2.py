#!/usr/bin/env python3
from __future__ import annotations
import json, os, shutil, subprocess, tempfile, zipfile
from pathlib import Path
HERE=Path(__file__).resolve().parent; TOOLS=HERE.parent

def install(root: Path):
    shutil.copytree(TOOLS,root/'tools'); (root/'tools'/'run_python_patches.sh').chmod(0o755); (root/'patchs').mkdir()

def run(root: Path, text: str):
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    return subprocess.run([str(root/'tools'/'run_python_patches.sh')],cwd=root,input=text,text=True,capture_output=True,env=env,timeout=50)

# line-mode inspect/dry-run never executes or archives the PATCH.
with tempfile.TemporaryDirectory(prefix='ptv613_inspect_') as td:
    root=Path(td); install(root); (root/'target.txt').write_text('old')
    manifest={'schema_version':1,'patch':{'id':'inspect'},'targets':['target.txt'],'compatibility':{'min_tool_version':'6.12.0','max_tested_version':'6.17.2'}}
    package=root/'patchs'/'p.zip'
    with zipfile.ZipFile(package,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest)); z.writestr('p.py','from pathlib import Path\nPath("target.txt").write_text("new")')
    cp=run(root,'i 1\nq\n'); assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert 'INSPECT RESULT: READY_TO_APPLY — project unchanged' in cp.stdout
    assert (root/'target.txt').read_text()=='old' and package.is_file() and not (root/'patchs'/'patched').exists()

# COLLECT quality summary reports bounded/truncated evidence and is captured in LAST_RUN.
with tempfile.TemporaryDirectory(prefix='ptv613_quality_') as td:
    root=Path(td); install(root); (root/'a.txt').write_text('needle\nneedle\n')
    req={'id':'quality','actions':[{'type':'search','query':'needle','paths':['.'],'max_matches':1,'context_lines':0}]}
    q=root/'patchs'/'CODE_COLLECTION_REQUEST_quality.zip'
    with zipfile.ZipFile(q,'w') as z:z.writestr('CODE_COLLECTION_REQUEST_quality.json',json.dumps(req))
    cp=run(root,'\n'); assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert 'COLLECT QUALITY:' in cp.stdout and 'truncated=1' in cp.stdout
    assert 'evidence is bounded/truncated' in cp.stdout
    last=json.loads((root/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text())
    quality=last['results'][0]['collect_result']['quality']; assert quality['truncated_reports']==1 and quality['missing']==0
print('PASS: v6.17.2 inspect/dry-run and COLLECT quality summary')
