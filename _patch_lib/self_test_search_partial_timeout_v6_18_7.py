#!/usr/bin/env python3
from pathlib import Path
import json
import tempfile
import zipfile

import python_patch_collect_compat as c
import python_patch_collect_schema as cs

# Positive primary evidence must not trigger a second full Python content scan by default.
with tempfile.TemporaryDirectory(prefix='ptv6187_positive_') as td:
    root=Path(td)
    for i in range(1200):
        (root/f'f{i:04d}.ts').write_text('console.log(1)\n' if i % 100 == 0 else 'const x=1;\n',encoding='utf-8')
    action={
        'type':'search','query':r'\bconsole\.log\s*\(','regex':True,'paths':['.'],
        'context_lines':0,'max_matches':100,'backend':'auto','source_scope':'filesystem',
        'filesystem':True,'respect_gitignore':False,'follow_symlinks':False,
        'must_find':False,'diagnose_on_zero':True,'fallback_search':True,
        'verify_nonzero_with_fallback':False,'report_coverage':True,
        'report_skipped_dirs':True,'module_discovery':True,'anchor_paths':[],'expected_files':[],
    }
    out=c._search_action(root,action,dict(cs.DEFAULT_LIMITS))
    assert out['matches']==12 and out['incomplete'] is False,out
    assert 'Search execution status: COMPLETED' in out['report'],out['report']
    assert 'Fallback backend: not run (positive primary result; zero/error verification policy)' in out['report'],out['report']

# Opt-in non-zero consistency verification remains available.  A catastrophic fallback
# is hard-contained and the already-published primary checkpoint survives as PARTIAL.
with tempfile.TemporaryDirectory(prefix='ptv6187_checkpoint_') as td:
    root=Path(td)
    (root/'good.txt').write_text('a'*100+'\n',encoding='utf-8')
    (root/'bad.txt').write_text('a'*50000+'!\n',encoding='utf-8')
    action={
        'type':'search','query':r'(a+)+$','regex':True,'paths':['.'],'context_lines':0,
        'max_matches':10,'backend':'auto','source_scope':'filesystem','filesystem':True,
        'respect_gitignore':False,'follow_symlinks':False,'must_find':False,
        'diagnose_on_zero':True,'fallback_search':True,'verify_nonzero_with_fallback':True,
        'report_coverage':True,'report_skipped_dirs':True,'module_discovery':True,
        'anchor_paths':[],'expected_files':[],
    }
    old=c.REGEX_SEARCH_TIMEOUT_SECONDS; c.REGEX_SEARCH_TIMEOUT_SECONDS=1.5
    try:
        out=c._search_action(root,action,dict(cs.DEFAULT_LIMITS))
    finally:
        c.REGEX_SEARCH_TIMEOUT_SECONDS=old
    assert out['incomplete'] is True and out['coverage_status']=='PARTIAL',out
    assert out['matches']>=1,out
    assert any('hard timeout' in x for x in out['reasons']),out
    assert 'REGEX TIMEOUT RECOVERY' in out['report'],out['report']
    assert 'Partial evidence above is preserved' in out['report'],out['report']

# max_matches is an evidence/output bound.  If additional match details exist, the
# result is PARTIAL/INCOMPLETE rather than silently reported as fully completed.
with tempfile.TemporaryDirectory(prefix='ptv6187_truncate_') as td:
    root=Path(td); (root/'x.txt').write_text('needle\nneedle\nneedle\n',encoding='utf-8')
    action={
        'type':'search','query':'needle','regex':False,'paths':['.'],'context_lines':0,
        'max_matches':1,'backend':'python','source_scope':'filesystem','filesystem':True,
        'respect_gitignore':False,'follow_symlinks':False,'must_find':False,
        'diagnose_on_zero':True,'fallback_search':True,'verify_nonzero_with_fallback':False,
        'report_coverage':True,'report_skipped_dirs':True,'module_discovery':True,
        'anchor_paths':[],'expected_files':[],
    }
    out=c._search_action(root,action,dict(cs.DEFAULT_LIMITS))
    assert out['matches']==3 and out['incomplete'] is True,out
    assert 'max_matches=1' in '\n'.join(out['reasons']),out
    assert 'Search execution status: PARTIAL' in out['report'],out['report']

# End-to-end: a timeout no longer aborts the whole request.  The result ZIP is kept,
# later actions still execute, and the manifest is INCOMPLETE.
with tempfile.TemporaryDirectory(prefix='ptv6187_e2e_') as td:
    root=Path(td); (root/'patchs').mkdir()
    (root/'good.txt').write_text('a'*100+'\n',encoding='utf-8')
    (root/'bad.txt').write_text('a'*50000+'!\n',encoding='utf-8')
    (root/'after.txt').write_text('AFTER\n',encoding='utf-8')
    request={
        'id':'partial-timeout-e2e',
        'actions':[
            {'type':'search','query':r'(a+)+$','regex':True,'paths':['.'],'context_lines':0,'max_matches':10,'backend':'auto','source_scope':'filesystem','fallback_search':True,'verify_nonzero_with_fallback':True},
            {'type':'pack','paths':['after.txt']},
        ],
    }
    request_zip=root/'patchs'/'CODE_COLLECTION_REQUEST_partial-timeout-e2e.zip'
    with zipfile.ZipFile(request_zip,'w') as zf:
        zf.writestr('CODE_COLLECTION_REQUEST_partial-timeout-e2e.json',json.dumps(request))
    old=c.REGEX_SEARCH_TIMEOUT_SECONDS; c.REGEX_SEARCH_TIMEOUT_SECONDS=1.5
    try:
        result,_archived,action_count,_lifecycle,status=c._run_request(root,request_zip)
    finally:
        c.REGEX_SEARCH_TIMEOUT_SECONDS=old
    assert action_count==2 and status=='INCOMPLETE',status
    with zipfile.ZipFile(result) as zf:
        names=set(zf.namelist())
        assert 'files/after.txt' in names,names
        manifest=json.loads(zf.read('COLLECTION_MANIFEST.json'))
        assert manifest['collection_status']=='INCOMPLETE',manifest
        report=zf.read('reports/001_search.md').decode('utf-8')
        assert 'Search execution status: PARTIAL' in report and 'Matches: 1' in report,report

# Schema keeps full non-zero backend consistency as an explicit additive option.
validated=cs.validate_request_data({'actions':[{'type':'search','query':'x','verify_nonzero_with_fallback':True}]})
assert validated['actions'][0]['verify_nonzero_with_fallback'] is True,validated

print('PASS: v6.18.7 regex large-tree optimization, partial-timeout checkpoint and INCOMPLETE preservation contract')

# Discovery-driven file quotas are fail-partial: keep what fits and mark INCOMPLETE
# instead of aborting the whole result ZIP.  This preserves the historical
# "collect what was found, report omitted remainder" behavior.
with tempfile.TemporaryDirectory(prefix='ptv6187_filequota_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'src').mkdir()
    for i in range(5): (root/'src'/f'f{i}.ts').write_text(f'export const x{i}={i};\n',encoding='utf-8')
    request={
        'id':'discovery-file-quota',
        'actions':[{'type':'directory','path':'src','include':['**/*.ts'],'max_results':5}],
        'limits':{'max_files':2},
    }
    request_zip=root/'patchs'/'CODE_COLLECTION_REQUEST_discovery-file-quota.zip'
    with zipfile.ZipFile(request_zip,'w') as zf:
        zf.writestr('CODE_COLLECTION_REQUEST_discovery-file-quota.json',json.dumps(request))
    result,_archived,_count,_lifecycle,status=c._run_request(root,request_zip)
    assert status=='INCOMPLETE',status
    with zipfile.ZipFile(result) as zf:
        manifest=json.loads(zf.read('COLLECTION_MANIFEST.json'))
        assert manifest['file_count']==2,manifest
        assert manifest['collection_status']=='INCOMPLETE',manifest
        assert any('max_files=2' in ' '.join(w.get('reasons',[])) for w in manifest['collection_warnings']),manifest

print('PASS: v6.18.7 discovery/output quota partial-preservation contract')

# Exact pack remains mandatory evidence: quota conflict still fails closed rather
# than silently returning a partial exact-file package.
with tempfile.TemporaryDirectory(prefix='ptv6187_packquota_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'a.txt').write_text('A\n'); (root/'b.txt').write_text('B\n')
    request={'id':'exact-pack-quota','actions':[{'type':'pack','paths':['a.txt','b.txt']}],'limits':{'max_files':1}}
    request_zip=root/'patchs'/'CODE_COLLECTION_REQUEST_exact-pack-quota.zip'
    with zipfile.ZipFile(request_zip,'w') as zf:
        zf.writestr('CODE_COLLECTION_REQUEST_exact-pack-quota.json',json.dumps(request))
    try:
        c._run_request(root,request_zip)
    except ValueError as exc:
        assert 'max_files' in str(exc),exc
    else:
        raise AssertionError('exact pack quota unexpectedly degraded into partial success')

print('PASS: v6.18.7 exact pack quota remains fail-closed')
