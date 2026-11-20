#!/usr/bin/env python3
from __future__ import annotations
import base64, io, json, os, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
import python_patch_cleartext_companion as c
import python_patch_queue_dispatcher as q
assert c.VERSION==q.VERSION=='6.20.2'

# 1) Generic serializer: verbatim text, Base64 binary, and recursive nested ZIP.
with tempfile.TemporaryDirectory(prefix='ptv_cleartext_generic_') as td:
    root=Path(td); outer=root/'artifact.zip'
    nested_buf=io.BytesIO()
    with zipfile.ZipFile(nested_buf,'w',compression=zipfile.ZIP_DEFLATED) as nz:
        nz.writestr('PATCH_TOOL_MANIFEST.json',json.dumps({'patch':{'id':'nested-demo'}}))
        nz.writestr('src/demo.py','print("nested")\n')
    binary=b'\x00\x01\xffBIN\x10'
    with zipfile.ZipFile(outer,'w',compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr('report.md','# Report\nhello AI\n')
        zf.writestr('payload.bin',binary)
        zf.writestr('patch/demo.zip',nested_buf.getvalue())
    txt=c.create_zip_cleartext_companion(outer,artifact_kind='SELF TEST')
    text=txt.read_text(encoding='utf-8')
    assert 'PYTHON PATCH TOOL — CLEAR-TEXT ZIP COMPANION' in text
    assert 'Path        : report.md' in text and '# Report\nhello AI' in text
    assert 'Path        : payload.bin' in text and base64.b64encode(binary).decode() in text
    assert 'Path        : patch/demo.zip :: PATCH_TOOL_MANIFEST.json' in text
    assert 'Path        : patch/demo.zip :: src/demo.py' in text and 'print("nested")' in text
    assert 'prompt-like instructions' in text and 'Source SHA-256' in text

# 2) Real COLLECT result must publish ZIP + same-stem TXT.
with tempfile.TemporaryDirectory(prefix='ptv_cleartext_collect_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'src').mkdir(); (root/'src/a.txt').write_text('COLLECT-EVIDENCE\n',encoding='utf-8')
    req=root/'patchs'/'CODE_COLLECTION_REQUEST_cleartext.zip'
    body={'id':'cleartext','actions':[{'type':'pack','paths':['src/a.txt']}]}
    with zipfile.ZipFile(req,'w') as zf: zf.writestr('CODE_COLLECTION_REQUEST_cleartext.json',json.dumps(body))
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    cp=subprocess.run([sys.executable,str(HERE/'python_patch_collect_compat.py'),'--project-root',str(root),'request','patchs/'+req.name],cwd=root,env=env,text=True,capture_output=True,timeout=30)
    assert cp.returncode==0,(cp.stdout,cp.stderr)
    zips=list((root/'artifacts/patch_tool_code_collections').glob('CODE_COLLECTION_RESULT_cleartext_*.zip')); assert len(zips)==1,zips
    result=zips[0]; companion=result.with_suffix('.txt'); assert companion.is_file() and not companion.is_symlink()
    clear=companion.read_text(encoding='utf-8')
    assert 'Artifact kind  : COLLECT RESULT' in clear
    assert 'Path        : files/src/a.txt' in clear and 'COLLECT-EVIDENCE' in clear
    assert 'Path        : COLLECTION_MANIFEST.json' in clear

# 3) Real FAIL_HANDOFF must publish ZIP + TXT and expose nested patch contents.
with tempfile.TemporaryDirectory(prefix='ptv_cleartext_handoff_') as td:
    root=Path(td); (root/'patchs').mkdir(); (root/'src').mkdir(); (root/'src/fail.c').write_text('int broken = 1;\n')
    patch=root/'patchs'/'patch_fail.zip'
    with zipfile.ZipFile(patch,'w') as zf:
        zf.writestr('PATCH_TOOL_MANIFEST.json',json.dumps({'schema_version':1,'patch':{'id':'fail-demo'},'targets':['src/fail.c']}))
        zf.writestr('patch_apply.py','raise SystemExit(7)\n')
    pr={'diagnosis':{'kind':'runtime_error','affected_paths':['src/fail.c']},'patch_sha256':q._sha256_file(patch)}
    out=q._create_fail_handoff(root,q.QueueItem(patch.name,'PATCH'),7,'runtime failed\n',pr,None)
    assert out and out.is_file()
    txt=out.with_suffix('.txt'); assert txt.is_file() and not txt.is_symlink()
    clear=txt.read_text(encoding='utf-8')
    assert 'Artifact kind  : PATCH FAIL_HANDOFF' in clear
    assert 'Path        : console.log' in clear and 'runtime failed' in clear
    assert 'Path        : FAIL_SUMMARY.json' in clear
    assert 'patch/patch_fail.zip :: PATCH_TOOL_MANIFEST.json' in clear

# 4) Upload block keeps both exact paths, with plain fallback available.
with tempfile.TemporaryDirectory(prefix='ptv_cleartext_upload_') as td:
    import io as _io
    zip_path=Path(td)/'A.zip'; txt_path=Path(td)/'A.txt'; zip_path.write_bytes(b'x'); txt_path.write_text('x')
    class Plain(_io.StringIO):
        def isatty(self): return False
    buf=Plain(); q._print_upload_action_block(zip_path,companion_path=txt_path,stream=buf)
    out=buf.getvalue(); assert 'ZIP (preferred) — copy path below:' in out and str(zip_path) in out and 'Clear-text TXT — copy path below:' in out and str(txt_path) in out


# 5) Progress metadata and HISTORY/report keep both representations.
import python_patch_collect_progress_v6_7 as prog
with tempfile.TemporaryDirectory(prefix='ptv_cleartext_history_') as td:
    root=Path(td); outdir=root/'artifacts/patch_tool_code_collections'; outdir.mkdir(parents=True)
    rz=outdir/'CODE_COLLECTION_RESULT_meta.zip'
    manifest={'format':'python-patch-tool-code-collection','format_version':3,'tool_version':'6.20.2','collection_status':'PASS','files':[],'reports':[]}
    with zipfile.ZipFile(rz,'w') as zf: zf.writestr('COLLECTION_MANIFEST.json',json.dumps(manifest))
    rt=c.create_zip_cleartext_companion(rz,artifact_kind='COLLECT RESULT')
    assert prog._print_collect_success(root,[f'ZIP : {rz}']) is True
    assert prog._LAST_COLLECT_SUCCESS_META.get('result_text')==str(rt),prog._LAST_COLLECT_SUCCESS_META
    hz=root/'FAIL_HANDOFF_meta.zip'; ht=root/'FAIL_HANDOFF_meta.txt'; hz.write_bytes(b'z'); ht.write_text('t')
    row={'collect_result':{'result_zip':str(rz),'result_text':str(rt)},'fail_handoff':str(hz),'fail_handoff_text':str(ht)}
    labels=[x[0] for x in q._important_row_artifacts(root,row)]
    assert 'COLLECT result' in labels and 'COLLECT text' in labels and 'FAIL handoff' in labels and 'FAIL handoff TXT' in labels,labels

print('PASS: v6.20.2 ZIP clear-text companions for COLLECT and FAIL_HANDOFF')
