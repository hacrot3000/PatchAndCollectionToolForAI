#!/usr/bin/env python3
from __future__ import annotations
import json, os, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent; TOOLS=HERE.parent
sys.path.insert(0,str(HERE))
import python_patch_ai_sync as sync
import python_patch_queue_dispatcher as q
assert sync.VERSION==q.VERSION=='6.19.4'
env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'; env['NO_COLOR']='1'

def copy_tools(root: Path) -> None:
    shutil.copytree(TOOLS,root/'tools')
    (root/'tools/run_python_patches.sh').chmod(0o755)

def collect(root: Path, req_id: str, ai_context=None):
    p=root/'patchs'/f'CODE_COLLECTION_REQUEST_{req_id}.zip'
    body={'id':req_id,'actions':[{'type':'pack','paths':['src/a.txt']}]}
    if ai_context is not None: body['ai_context']=ai_context
    with zipfile.ZipFile(p,'w') as zf: zf.writestr(f'CODE_COLLECTION_REQUEST_{req_id}.json',json.dumps(body))
    cp=subprocess.run([sys.executable,str(root/'tools/_patch_lib/python_patch_collect_compat.py'),'--project-root',str(root),'request','patchs/'+p.name],cwd=root,env=env,text=True,capture_output=True,timeout=40)
    assert cp.returncode==0,(cp.stdout,cp.stderr)
    zips=sorted((root/'artifacts/patch_tool_code_collections').glob(f'CODE_COLLECTION_RESULT_{req_id}_*.zip'))
    assert len(zips)==1,zips
    return zips[0]

# 1) First stale COLLECT carries full docs + companion; second stale request for
# the same AI agent suppresses repeated docs. A different agent gets its own sync.
with tempfile.TemporaryDirectory(prefix='ptv_ai_sync_collect_') as td:
    root=Path(td); copy_tools(root); (root/'patchs').mkdir(); (root/'src').mkdir(); (root/'src/a.txt').write_text('evidence\n')
    old={'known_tool_version':'6.19.0','agent_id':'agent-a'}
    first=collect(root,'sync1',old)
    with zipfile.ZipFile(first) as zf:
        names=set(zf.namelist()); assert 'AI_TOOL_SYNC/ACTION_REQUIRED_AI_UPDATE.md' in names
        assert 'AI_TOOL_SYNC/AI_SYNC_MANIFEST.json' in names
        assert 'AI_TOOL_SYNC/docs/tools/_patch_lib/docs/AI_USAGE_CONTRACT.md' in names
        sm=json.loads(zf.read('AI_TOOL_SYNC/AI_SYNC_MANIFEST.json')); token=sm['sync_token']
        cm=json.loads(zf.read('COLLECTION_MANIFEST.json')); assert cm['ai_tool_sync']['full_update_attached'] is True
    txt=first.with_suffix('.txt').read_text(encoding='utf-8')
    assert 'ACTION REQUIRED — UPDATE AI KNOWLEDGE' in txt and token in txt
    second=collect(root,'sync2',old)
    with zipfile.ZipFile(second) as zf:
        names=set(zf.namelist()); assert not any(n.startswith('AI_TOOL_SYNC/') for n in names), sorted(names)[:10]
        cm=json.loads(zf.read('COLLECTION_MANIFEST.json')); assert cm['ai_tool_sync']['full_update_attached'] is False
        assert cm['ai_tool_sync']['reason']=='already_delivered_to_agent',cm['ai_tool_sync']
    third=collect(root,'sync3',{'known_tool_version':'6.19.0','agent_id':'agent-b'})
    with zipfile.ZipFile(third) as zf: assert 'AI_TOOL_SYNC/AI_SYNC_MANIFEST.json' in zf.namelist()

    # A current token is an explicit acknowledgement and needs no repeated docs.
    fourth=collect(root,'sync4',{'known_tool_version':'6.19.4','sync_token':token,'agent_id':'agent-c'})
    with zipfile.ZipFile(fourth) as zf:
        cm=json.loads(zf.read('COLLECTION_MANIFEST.json')); assert cm['ai_tool_sync']['full_update_attached'] is False
        assert cm['ai_tool_sync']['reason']=='sync_token_current'

# 2) Old PATCH failure embeds the update directly in mandatory FAIL_HANDOFF.
with tempfile.TemporaryDirectory(prefix='ptv_ai_sync_handoff_') as td:
    root=Path(td); copy_tools(root); (root/'patchs').mkdir(); (root/'src').mkdir(); (root/'src/fail.txt').write_text('old\n')
    patch=root/'patchs'/'patch_old_fail.zip'
    manifest={'schema_version':1,'patch':{'id':'old-fail'},'compatibility':{'max_tested_version':'6.19.0'},'ai_context':{'known_tool_version':'6.19.0','agent_id':'agent-fail'},'targets':['src/fail.txt']}
    with zipfile.ZipFile(patch,'w') as zf:
        zf.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest)); zf.writestr('apply.py','raise SystemExit(7)\n')
    pr={'diagnosis':{'kind':'runtime_error','affected_paths':['src/fail.txt']},'patch_sha256':q._sha256_file(patch)}
    handoff=q._create_fail_handoff(root,q.QueueItem(patch.name,'PATCH'),7,'failed\n',pr,None)
    assert handoff and handoff.with_suffix('.txt').is_file()
    with zipfile.ZipFile(handoff) as zf:
        assert 'AI_TOOL_SYNC/AI_SYNC_MANIFEST.json' in zf.namelist()
        summary=json.loads(zf.read('FAIL_SUMMARY.json')); assert summary['ai_tool_sync']['full_update_attached'] is True
    assert 'ACTION REQUIRED — UPDATE AI KNOWLEDGE' in handoff.with_suffix('.txt').read_text(encoding='utf-8')

# 3) Old PATCH success has no FAIL_HANDOFF, so dispatcher emits a standalone
# AI_TOOL_SYNC_RESULT ZIP + TXT and records/highlights it in run history.
with tempfile.TemporaryDirectory(prefix='ptv_ai_sync_patch_pass_') as td:
    root=Path(td); copy_tools(root); (root/'patchs').mkdir(); (root/'src').mkdir(); (root/'src/a.txt').write_text('OLD\n')
    manifest={
        'schema_version':1,'patch':{'id':'old-pass'},
        'compatibility':{'max_tested_version':'6.19.0'},
        'ai_context':{'known_tool_version':'6.19.0','agent_id':'agent-pass'},
        'targets':['src/a.txt'],
    }
    ops={'patch_name':'old-pass','ops':[{'kind':'replace','file':'src/a.txt','old':'OLD','new':'NEW'}]}
    with zipfile.ZipFile(root/'patchs/p.zip','w') as zf:
        zf.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest)); zf.writestr('PATCH_TOOL_OPS.json',json.dumps(ops))
    cp=subprocess.run([str(root/'tools/run_python_patches.sh'),'run','--all','--no-validation'],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,stdin=subprocess.DEVNULL,timeout=60)
    assert cp.returncode==0,(cp.returncode,cp.stdout)
    assert (root/'src/a.txt').read_text()=='NEW\n'
    zips=list((root/'artifacts/patch_tool/ai_sync').glob('AI_TOOL_SYNC_RESULT_*.zip')); assert len(zips)==1,zips
    assert zips[0].with_suffix('.txt').is_file()
    with zipfile.ZipFile(zips[0]) as zf: assert 'AI_TOOL_SYNC/AI_SYNC_MANIFEST.json' in zf.namelist()
    last=json.loads((root/'artifacts/patch_tool/LAST_RUN.json').read_text())
    row=last['results'][0]; assert row.get('ai_sync_result') and row.get('ai_sync_result_text'),row
    labels=[x[0] for x in q._important_row_artifacts(root,row)]
    assert 'AI sync result' in labels and 'AI sync TXT' in labels,labels

print('PASS: v6.19.4 AI tool-version/capability sync is stateful, one-shot, artifact-contained and legacy-compatible')
