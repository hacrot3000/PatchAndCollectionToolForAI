#!/usr/bin/env python3
from pathlib import Path
import io, tempfile
import python_patch_queue_dispatcher as m

assert m.VERSION == '6.20.1'

class _FakeTTYOut(io.StringIO):
    def isatty(self): return True

with tempfile.TemporaryDirectory(prefix='ptv-failed-group-') as td:
    root=Path(td); (root/'patchs').mkdir()
    for name in ['failed_patch.zip','new_patch.zip','failed_collect.zip','new_collect.zip']:
        (root/'patchs'/name).write_bytes(name.encode())
    items=[
        m.QueueItem('failed_patch.zip','PATCH'),
        m.QueueItem('new_patch.zip','PATCH'),
        m.QueueItem('failed_collect.zip','COLLECT'),
        m.QueueItem('new_collect.zip','COLLECT'),
    ]
    previous={
        'status':'FAIL','selected':['failed_patch.zip','failed_collect.zip'],
        'failed_item':'failed_patch.zip',
        'results':[
            {'name':'failed_patch.zip','kind':'PATCH','status':'FAIL'},
            {'name':'failed_collect.zip','kind':'COLLECT','status':'FAIL'},
        ],
    }
    failed=m._last_failed_queue_names(root,items,previous)
    assert failed == {'failed_patch.zip','failed_collect.zip'},failed
    grouped=m._group_selector_items(items,failed)
    assert [x.name for x in grouped] == ['new_patch.zip','new_collect.zip','failed_patch.zip','failed_collect.zip']
    # Grouping must not create wrappers or a second operation type.
    assert all(isinstance(x,m.QueueItem) for x in grouped)
    assert set(grouped)==set(items)

    old_in,old_out=m.sys.stdin,m.sys.stdout
    try:
        m.sys.stdin=io.StringIO('q\n'); cap=io.StringIO(); m.sys.stdout=cap
        m._select_items_line(root,list(grouped),'none',show_history=True,failed_group_names=failed)
    finally:
        m.sys.stdin,m.sys.stdout=old_in,old_out
    plain=cap.getvalue()
    assert plain.index('New patch/collect:') < plain.index('new_patch.zip')
    assert plain.index('new_collect.zip') < plain.index('last failed patch/collect:') < plain.index('failed_patch.zip')
    assert 'H. [HISTORY]' in plain

    # Fullscreen renderer: headers are non-selectable physical rows; cursor count
    # remains based only on actionable items (+ HISTORY).
    old_out=m.sys.stdout; old_size=m._selector_term_size
    try:
        cap=_FakeTTYOut(); m.sys.stdout=cap; m._selector_term_size=lambda:(100,20)
        m._render(grouped,2,{2},{},'',0,show_history=True,failed_group_names=failed)
    finally:
        m.sys.stdout=old_out; m._selector_term_size=old_size
    rendered=m._ANSI_RE.sub('',cap.getvalue()).replace('\r','')
    assert 'CON TRỎ 3/5' in rendered,rendered
    assert 'New patch/collect:' in rendered and 'Failed patch/collect (unresolved):' in rendered,rendered
    assert '› [x]   3. [PATCH] failed_patch.zip' in rendered,rendered

# Startup flow regression: previous FAIL must NOT invoke Smart Resume implicitly.
with tempfile.TemporaryDirectory(prefix='ptv-no-auto-resume-') as td:
    root=Path(td); (root/'patchs').mkdir()
    for name in ['failed.zip','new.zip']:(root/'patchs'/name).write_bytes(b'x')
    items=[m.QueueItem('failed.zip','PATCH'),m.QueueItem('new.zip','PATCH')]
    previous={'status':'FAIL','selected':['failed.zip'],'failed_item':'failed.zip','results':[{'name':'failed.zip','kind':'PATCH','status':'FAIL'}]}
    saved={name:getattr(m,name) for name in [
        'discover_queue','_load_previous_run','_unresolved_replay_identities',
        '_split_session_duplicate_patches','_split_local_duplicate_patches','_move_local_duplicates_to_ignore',
        '_load_zero_argument_config','select_items','_resume_selection','_finalize_batch_artifacts',
        '_write_run_report','_update_unresolved_registry','update_patch_ledger'
    ]}
    capture={}
    try:
        m.discover_queue=lambda _root:(list(items),[])
        m._load_previous_run=lambda _root:previous
        m._unresolved_replay_identities=lambda _root:{}
        m._split_session_duplicate_patches=lambda _root,x,history_replay_sha=None:(list(x),[],[])
        m._split_local_duplicate_patches=lambda _root,x,history_replay_sha=None:(list(x),[],[])
        m._move_local_duplicates_to_ignore=lambda _root,x:(list(x),[])
        m._load_zero_argument_config=lambda _root:({'initial_selection':'none','selector_ui':'line','failure_policy':'continue_independent','transaction_policy':'patch'},[])
        def fake_select(_root, shown, **kw):
            capture['names']=[x.name for x in shown]; capture['failed']=set(kw.get('failed_group_names') or ())
            return None
        m.select_items=fake_select
        m._resume_selection=lambda *a,**k: (_ for _ in ()).throw(AssertionError('ordinary run must not auto-open Smart Resume'))
        m._finalize_batch_artifacts=lambda *a,**k:None
        m._write_run_report=lambda *a,**k:None
        m._update_unresolved_registry=lambda *a,**k:None
        m.update_patch_ledger=lambda *a,**k:None
        rc=m._run_queue(root,zero_argument_invocation=True)
        assert rc==0
        assert capture['names']==['new.zip','failed.zip'],capture
        assert capture['failed']=={'failed.zip'}

        # Explicit resume command remains supported and is the only path that
        # invokes the old recovery menu automatically.
        called={'resume':0}
        def explicit_resume(*a,**k): called['resume']+=1; return None
        m._resume_selection=explicit_resume
        rc=m._run_queue(root,force_resume=True,zero_argument_invocation=False)
        assert rc==0 and called['resume']==1,(rc,called)
    finally:
        for name,value in saved.items(): setattr(m,name,value)

print('PASS: v6.20.1 previous failed PATCH/COLLECT are a normal second queue group; Smart Resume is explicit-only')
