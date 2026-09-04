#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, json, os, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent

def install(root: Path):
    shutil.copytree(TOOLS,root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()

def mk(path: Path, manifest: dict, script: str):
    with zipfile.ZipFile(path,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        z.writestr('patch_apply.py',script)

def run(root: Path, user_input: str):
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    return subprocess.run([str(root/'tools'/'run_python_patches.sh')],cwd=root,input=user_input,text=True,capture_output=True,env=env,timeout=60)

# Every runtime PATCH failure must attach structured target source even when the
# payload never changed it and diagnosis.affected_paths remains empty.
with tempfile.TemporaryDirectory(prefix='ptv_handoff_all_fail_') as td:
    root=Path(td); install(root)
    (root/'source.c').write_text('#include "source.h"\nint value = 1;\n')
    (root/'source.h').write_text('#pragma once\n')
    manifest={
      'schema_version':1,
      'patch':{'id':'runtime-fail'},
      'targets':['source.c'],
      # Backward-compatible field is accepted but must no longer disable the
      # mandatory failure handoff/source collection contract.
      'recovery':{'fail_handoff':False},
    }
    mk(root/'patchs'/'patch_runtime_fail.zip',manifest,'raise SystemExit(9)')
    cp=run(root,'\n')
    assert cp.returncode==9,(cp.stdout,cp.stderr)
    assert 'deprecated/ignored' in cp.stderr,(cp.stdout,cp.stderr)
    handoffs=list((root/'artifacts'/'patch_tool'/'fail_handoffs').glob('FAIL_HANDOFF_*.zip'))
    assert len(handoffs)==1,(cp.stdout,cp.stderr,handoffs)
    with zipfile.ZipFile(handoffs[0]) as z:
        names=set(z.namelist())
        assert 'current_source/source.c' in names,names
        assert 'current_source/source.h' in names,names
        assert 'SOURCE_DISCOVERY.json' in names,names
        discovery=json.loads(z.read('SOURCE_DISCOVERY.json'))
        included={x['path']:x for x in discovery['included_files']}
        assert 'preflight.target_paths' in included['source.c']['reasons'],included
        assert any(r.startswith(('same_stem_companion:','one_hop_reference:')) for r in included['source.h']['reasons']),included
        summary=json.loads(z.read('FAIL_SUMMARY.json'))
        assert summary['source_discovery']['mode']=='automatic_on_every_patch_failure'
        assert summary['source_discovery']['included_files']>=2
    assert 'FAIL HANDOFF SOURCES: included=' in cp.stdout

# A bare compiler/log basename must trigger a bounded repository scan so the
# source can still be attached without a structured diagnosis path.
spec=importlib.util.spec_from_file_location('ptv_dispatcher',HERE/'python_patch_queue_dispatcher.py')
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)
with tempfile.TemporaryDirectory(prefix='ptv_handoff_scan_') as td:
    root=Path(td); (root/'src'/'deep').mkdir(parents=True)
    (root/'src'/'deep'/'Widget.ts').write_text('export const x = 1\n')
    attachments,discovery=m._discover_fail_handoff_sources(
        root,m.QueueItem('missing.zip','PATCH'),None,'Widget.ts:41:9: error TS1000\n'
    )
    assert [rel for rel,_ in attachments]==['src/deep/Widget.ts'],(attachments,discovery)
    assert discovery['basename_scan_files_examined']>=1
    assert discovery['mode']=='automatic_on_every_patch_failure'

# Absolute project paths from Python tracebacks are attached, and hidden source
# files remain normalized correctly.
with tempfile.TemporaryDirectory(prefix='ptv_handoff_traceback_') as td:
    root=Path(td); (root/'pkg').mkdir(); src=root/'pkg'/'worker.py'; src.write_text('raise RuntimeError()\n')
    rows,_=m._handoff_console_path_evidence(root,f'  File "{src}", line 1, in <module>\n')
    assert rows==[('pkg/worker.py','console_path')],rows
    (root/'.env').write_text('X=1\n')
    assert m._normalize_handoff_candidate(root,'.env')=='.env'

print('PASS: v6.17.2 every PATCH failure auto-discovers and embeds related current source in FAIL_HANDOFF')
