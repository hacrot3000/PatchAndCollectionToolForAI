#!/usr/bin/env python3
from pathlib import Path
import hashlib, json, subprocess, tempfile, zipfile

import python_patch_queue_dispatcher as q
import python_patch_runner as r

assert q.VERSION == '6.18.2'
assert r.VERSION == '6.18.2'

# 1) No declared targets + unchanged Git fingerprint is not proof that ignored
# files were untouched. A failed legacy/unbounded payload must stop fail-safe.
with tempfile.TemporaryDirectory(prefix='ptv6176_ignored_unknown_') as td:
    root=Path(td)
    subprocess.run(['git','init','-q'],cwd=root,check=True)
    subprocess.run(['git','config','user.email','ptv@example.invalid'],cwd=root,check=True)
    subprocess.run(['git','config','user.name','PTV'],cwd=root,check=True)
    (root/'.gitignore').write_text('ignored.txt\n',encoding='utf-8')
    (root/'tracked.txt').write_text('tracked\n',encoding='utf-8')
    subprocess.run(['git','add','.gitignore','tracked.txt'],cwd=root,check=True)
    subprocess.run(['git','commit','-qm','base'],cwd=root,check=True)
    before_fp=r._git_worktree_fingerprint(root)
    before_dirty=r._dirty_paths(root)
    before_targets=r._snapshot_declared_paths(root,[])
    (root/'ignored.txt').write_text('changed\n',encoding='utf-8')
    partial=r._partial_state(root,before_fp=before_fp,before_dirty=before_dirty,before_targets=before_targets,target_paths=[])
    assert partial['detected'] is None,partial
    assert 'ignored_paths_unbounded' in partial['evidence'],partial

# 2) Exact batch rollback replay is allowed through PASS-history duplicate
# suppression, and protected replay identity wins over an ordinary same-session
# duplicate that sorts before it.
with tempfile.TemporaryDirectory(prefix='ptv6176_replay_duplicate_') as td:
    root=Path(td); (root/'patchs/patched').mkdir(parents=True)
    data=b'exact-replay-package'
    sha=hashlib.sha256(data).hexdigest()
    (root/'patchs'/'copy.zip').write_bytes(data)
    (root/'patchs'/'RETRY-old.zip').write_bytes(data)
    (root/'patchs/patched'/'old.zip').write_bytes(data)
    prev={'status':'FAIL','results':[{
        'name':'old.zip','kind':'PATCH','status':'PASS','batch_rolled_back':True,
        'requeued_as':'RETRY-old.zip','patch_result':{'patch_sha256':sha},
    }]}
    replay=q._previous_replay_identities(prev)
    assert replay=={'RETRY-old.zip':sha},replay
    items=[q.QueueItem('copy.zip','PATCH'),q.QueueItem('RETRY-old.zip','PATCH')]
    runnable,dups,warnings=q._split_session_duplicate_patches(root,items,history_replay_sha=replay)
    assert [x.name for x in runnable]==['RETRY-old.zip'],([x.name for x in runnable],dups,warnings)
    assert not (root/'patchs/copy.zip').exists()
    runnable,dups,warnings=q._split_local_duplicate_patches(root,runnable,history_replay_sha=replay)
    assert [x.name for x in runnable]==['RETRY-old.zip'] and not dups,(runnable,dups,warnings)

# 3) Recovery actions must never bind a same-name replacement with different
# bytes to the historical failed row.
with tempfile.TemporaryDirectory(prefix='ptv6176_recovery_identity_') as td:
    root=Path(td); (root/'patchs').mkdir()
    (root/'patchs/failed.zip').write_bytes(b'new-unrelated-package')
    oldsha=hashlib.sha256(b'old-failed-package').hexdigest()
    row={'name':'failed.zip','kind':'PATCH','status':'FAIL','patch_result':{'patch_sha256':oldsha}}
    assert q._queued_failed_rows(root,[row])==[]

# 4) Logical previous_failure validation keeps the historical name/id, while
# retry/delete/run_after filesystem operations bind to exact RETRY-* bytes.
def pack(path: Path, patch_id: str, previous_failure=None):
    manifest={'schema_version':1,'patch':{'id':patch_id},'execution':{'timeout_seconds':30},'targets':['x.txt']}
    if previous_failure is not None:
        manifest['batch']={'previous_failure':previous_failure}
    with zipfile.ZipFile(path,'w') as zf:
        zf.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        zf.writestr('apply.py','print("ok")\n')

with tempfile.TemporaryDirectory(prefix='ptv6176_predecessor_binding_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'x.txt').write_text('x\n',encoding='utf-8')
    pack(root/'patchs/RETRY-old.zip','old-id')
    retry_sha=q.stable_package_sha256(root/'patchs/RETRY-old.zip')
    pack(root/'patchs/old.zip','unrelated-new-id')
    pack(root/'patchs/successor.zip','successor-id',{
        'action':'retry_before','patch_file':'old.zip','patch_id':'old-id','reason':'retry exact predecessor',
    })
    previous={'status':'FAIL','failed_item':'old.zip','results':[{
        'name':'old.zip','kind':'PATCH','status':'FAIL','requeued_as':'RETRY-old.zip',
        'patch_result':{'patch_sha256':retry_sha,'manifest_patch':{'id':'old-id'}},
    }]}
    available=[q.QueueItem('old.zip','PATCH'),q.QueueItem('RETRY-old.zip','PATCH'),q.QueueItem('successor.zip','PATCH')]
    ordered,metas,action=q._build_batch_plan(root,[q.QueueItem('successor.zip','PATCH')],available,previous)
    assert [x.name for x in ordered]==['RETRY-old.zip','successor.zip'],[x.name for x in ordered]
    assert action['patch_file']=='old.zip' and action['queue_file']=='RETRY-old.zip',action

# 5) A true zero-work probe must not touch artifact state at all, even if an
# artifact path is unsafe.  As soon as runnable PATCH work exists, the same
# unsafe boundary remains a clean fail-closed CLI result.
with tempfile.TemporaryDirectory(prefix='ptv6176_artifact_safety_') as td, tempfile.TemporaryDirectory(prefix='ptv6176_artifact_out_') as out:
    root=Path(td); (root/'patchs').mkdir(); (root/'artifacts').symlink_to(Path(out),target_is_directory=True)
    rc=q.main(['--project-root',str(root)])
    assert rc==0,rc
    pack(root/'patchs/work.zip','artifact-safety-work')
    rc=q.main(['--project-root',str(root)])
    assert rc==2,rc

print('PASS: v6.18.2 recovery identity, rollback replay dedupe exception, unbounded ignored-file fail-safe and clean artifact safety error')
