#!/usr/bin/env python3
from __future__ import annotations
import json, os, stat, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
from python_patch_collect_schema import validate_request_data
from python_patch_collect_compat import _search_action_payload, _run_request


def action(query: str, **kw):
    raw={'type':'search','query':query,'paths':['.'],'context_lines':0,'max_matches':100}
    raw.update(kw)
    req=validate_request_data({'actions':[raw]})
    return req['actions'][0],req['limits']

# Default contract is filesystem-first and independently verified.
a,limits=action('x')
assert a['source_scope']=='filesystem' and a['filesystem'] is True
assert a['backend']=='auto' and a['fallback_search'] is True and a['diagnose_on_zero'] is True
assert limits['max_search_files']>5000 and limits['max_search_file_bytes']>=64*1024*1024

with tempfile.TemporaryDirectory(prefix='ptv618_search_') as td:
    root=Path(td)
    (root/'early').mkdir(); (root/'trunk'/'jdqs_server').mkdir(parents=True)
    for i in range(5050): (root/'early'/f'n{i:05}.txt').touch()
    target=root/'trunk'/'jdqs_server'/'MineInfoCSHandler.java'
    target.write_text('CmdMineInfoCSReqMsg request;\n',encoding='utf-8')
    a,limits=action('CmdMineInfoCSReqMsg',must_find=True)
    out=_search_action_payload(root,a,limits)
    assert out['matches']==1 and not out['incomplete'] and out['coverage_status']=='VERIFIED',out
    report=out['report']
    assert 'Files considered: 5051' in report and 'trunk/jdqs_server: 1' in report,report
    assert 'Primary backend:' in report and 'Fallback backend:' in report

    # Filesystem mode sees ignored/untracked source; Git tracked mode is opt-in narrow behavior.
    subprocess.run(['git','init','-q'],cwd=root,check=True)
    (root/'.gitignore').write_text('ignored_module/\n',encoding='utf-8')
    (root/'ignored_module').mkdir(); (root/'ignored_module'/'Hidden.java').write_text('IGNORED_SYMBOL\n',encoding='utf-8')
    subprocess.run(['git','add','.gitignore',str(target.relative_to(root))],cwd=root,check=True)
    a,limits=action('IGNORED_SYMBOL')
    assert _search_action_payload(root,a,limits)['matches']==1
    a,limits=action('IGNORED_SYMBOL',source_scope='git_tracked')
    assert _search_action_payload(root,a,limits)['matches']==0

    # Force primary rg to return false zero; independent Python fallback must detect inconsistency.
    bindir=root/'fakebin'; bindir.mkdir(); fake=bindir/'rg'
    fake.write_text('#!/bin/sh\nexit 1\n',encoding='utf-8'); fake.chmod(fake.stat().st_mode|stat.S_IXUSR)
    old=os.environ.get('PATH',''); os.environ['PATH']=str(bindir)+os.pathsep+old
    try:
        a,limits=action('CmdMineInfoCSReqMsg',must_find=True)
        out=_search_action_payload(root,a,limits)
    finally:
        os.environ['PATH']=old
    assert out['matches']==1 and out['inconsistency'] and out['incomplete'],out
    assert 'SEARCH_INCONSISTENCY' in out['report'] and 'primary_matches=0' in out['report'] and 'fallback_matches=1' in out['report']

    # Zero + must_find creates a diagnostic result, never a normal PASS.
    a,limits=action('SYMBOL_THAT_DOES_NOT_EXIST',must_find=True,diagnose_on_zero=True)
    out=_search_action_payload(root,a,limits)
    assert out['matches']==0 and out['incomplete'] and out['must_find_failed']
    assert 'ZERO MATCH DIAGNOSTIC' in out['report'] and 'Zero-result interpretation: UNTRUSTED' in out['report']

# Integration: INCOMPLETE still publishes a result ZIP with diagnosis and archives exact request.
with tempfile.TemporaryDirectory(prefix='ptv618_collect_incomplete_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'src').mkdir(); (root/'src'/'A.java').write_text('nothing\n')
    request=root/'patchs'/'CODE_COLLECTION_REQUEST_zero.zip'
    body={'id':'zero','actions':[{'type':'search','paths':['src'],'query':'missing','must_find':True}]}
    with zipfile.ZipFile(request,'w') as z: z.writestr('CODE_COLLECTION_REQUEST_zero.json',json.dumps(body))
    result,archived,count,lifecycle,status=_run_request(root,request)
    assert status=='INCOMPLETE' and result.is_file() and archived.is_file() and count==1
    with zipfile.ZipFile(result) as z:
        manifest=json.loads(z.read('COLLECTION_MANIFEST.json'))
        assert manifest['collection_status']=='INCOMPLETE' and manifest['format_version']==3
        assert manifest['collection_warnings']
        text=z.read('reports/001_search.md').decode('utf-8')
        assert 'ZERO MATCH DIAGNOSTIC' in text

print('PASS: v6.19.5 filesystem-first search coverage, false-zero fallback, must_find, anchors/diagnostics and INCOMPLETE result contract')
