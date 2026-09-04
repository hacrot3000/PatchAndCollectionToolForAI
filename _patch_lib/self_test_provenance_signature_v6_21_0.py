#!/usr/bin/env python3
from __future__ import annotations
import base64, hashlib, json, os, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent
sys.path.insert(0,str(HERE))
import python_patch_provenance as prov


def _secret_scalar(seed: bytes):
    h=hashlib.sha512(seed).digest()
    a=int.from_bytes(h[:32],'little')
    a &= (1<<254)-8
    a |= 1<<254
    return a,h[32:]


def _keypair(seed: bytes):
    a,_=_secret_scalar(seed)
    return seed,prov._encodepoint(prov._scalarmult(prov._B,a))


def _sign(seed: bytes, message: bytes) -> bytes:
    a,prefix=_secret_scalar(seed)
    public=prov._encodepoint(prov._scalarmult(prov._B,a))
    r=int.from_bytes(hashlib.sha512(prefix+message).digest(),'little') % prov._L
    r_enc=prov._encodepoint(prov._scalarmult(prov._B,r))
    k=int.from_bytes(hashlib.sha512(r_enc+public+message).digest(),'little') % prov._L
    s=(r+k*a) % prov._L
    return r_enc+s.to_bytes(32,'little')


def install(root: Path):
    shutil.copytree(TOOLS,root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()


def run(root: Path, text='\n', args=None):
    env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
    cmd=[str(root/'tools'/'run_python_patches.sh')]+list(args or [])
    return subprocess.run(cmd,cwd=root,input=text,text=True,capture_output=True,env=env,timeout=50)


def policy(root: Path, public: bytes, *, required=True, key_id='test-key'):
    (root/'.python_patch_tool.json').write_text(json.dumps({'provenance':{
        'require_signature':required,
        'trusted_ed25519_keys':{key_id:base64.b64encode(public).decode('ascii')},
    }}),encoding='utf-8')


def base(pid: str):
    old='old\n'
    return {
        'schema_version':1,
        'patch':{'id':pid},
        'targets':['state.txt'],
        'preflight':{'files':[{'path':'state.txt','exists':True,'sha256':hashlib.sha256(old.encode()).hexdigest()}]},
    }


def pack_signed(path: Path, manifest: dict, script: str, seed: bytes, *, key_id='test-key', mutate_payload=False, mutate_manifest=False):
    with tempfile.TemporaryDirectory(prefix='ptv-sign-build-') as td:
        d=Path(td)
        payload=d/'patch_apply.py'; payload.write_text(script,encoding='utf-8')
        m=json.loads(json.dumps(manifest))
        m['provenance']={'format':prov.SIGNED_FORMAT,'algorithm':prov.ALGORITHM,'key_id':key_id,'signature':''}
        (d/'PATCH_TOOL_MANIFEST.json').write_text(json.dumps(m,ensure_ascii=False),encoding='utf-8')
        sig=_sign(seed,prov.canonical_signed_message(m,d))
        m['provenance']['signature']=base64.b64encode(sig).decode('ascii')
        if mutate_manifest:
            m['patch']['summary']='tampered-after-signing'
        payload_bytes=(script+'\n# tampered-after-signing\n' if mutate_payload else script).encode()
        with zipfile.ZipFile(path,'w') as z:
            z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(m,ensure_ascii=False))
            z.writestr('patch_apply.py',payload_bytes)


# RFC 8032 test vector 1 proves verifier interoperability for empty message.
rfc_pk=bytes.fromhex('d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a')
rfc_sig=bytes.fromhex('e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b')
assert prov.verify_ed25519(rfc_pk,b'',rfc_sig)
assert not prov.verify_ed25519(rfc_pk,b'x',rfc_sig)

seed=bytes(range(32)); _,public=_keypair(seed)
script='from pathlib import Path\nPath("state.txt").write_text("new\\n")\n'

# Default policy preserves compatibility: an unsigned current PATCH still runs.
with tempfile.TemporaryDirectory(prefix='ptv621_unsigned_optional_') as td:
    root=Path(td); install(root); (root/'state.txt').write_text('old\n')
    m=base('unsigned-optional')
    with zipfile.ZipFile(root/'patchs'/'p.zip','w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(m)); z.writestr('patch_apply.py',script)
    cp=run(root); assert cp.returncode==0,(cp.stdout,cp.stderr); assert (root/'state.txt').read_text()=='new\n'

# Required policy rejects unsigned package before payload.
with tempfile.TemporaryDirectory(prefix='ptv621_unsigned_required_') as td:
    root=Path(td); install(root); (root/'state.txt').write_text('old\n'); policy(root,public,required=True)
    m=base('unsigned-required')
    with zipfile.ZipFile(root/'patchs'/'p.zip','w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(m)); z.writestr('patch_apply.py',script)
    cp=run(root); assert cp.returncode==2,(cp.stdout,cp.stderr); assert (root/'state.txt').read_text()=='old\n'; assert 'signature_required' in cp.stdout+cp.stderr

# Trusted valid signature passes and provenance evidence is retained in preflight.
with tempfile.TemporaryDirectory(prefix='ptv621_signed_ok_') as td:
    root=Path(td); install(root); (root/'state.txt').write_text('old\n'); policy(root,public,required=True)
    pack_signed(root/'patchs'/'p.zip',base('signed-ok'),script,seed)
    cp=run(root); assert cp.returncode==0,(cp.stdout,cp.stderr); assert (root/'state.txt').read_text()=='new\n'
    data=json.loads((root/'artifacts'/'patch_tool'/'LAST_RUN.json').read_text())
    pf=data['results'][0]['patch_result']['preflight']; rows=[x for x in pf['checks'] if x.get('kind')=='provenance']
    assert rows and rows[0]['status']=='PASS' and rows[0]['key_id']=='test-key',rows

# Payload mutation after signing is rejected before payload.
with tempfile.TemporaryDirectory(prefix='ptv621_signed_payload_tamper_') as td:
    root=Path(td); install(root); (root/'state.txt').write_text('old\n'); policy(root,public,required=True)
    pack_signed(root/'patchs'/'p.zip',base('payload-tamper'),script,seed,mutate_payload=True)
    cp=run(root); assert cp.returncode==2,(cp.stdout,cp.stderr); assert (root/'state.txt').read_text()=='old\n'; assert 'signature_invalid' in cp.stdout+cp.stderr

# Manifest mutation after signing is rejected.
with tempfile.TemporaryDirectory(prefix='ptv621_signed_manifest_tamper_') as td:
    root=Path(td); install(root); (root/'state.txt').write_text('old\n'); policy(root,public,required=True)
    pack_signed(root/'patchs'/'p.zip',base('manifest-tamper'),script,seed,mutate_manifest=True)
    cp=run(root); assert cp.returncode==2,(cp.stdout,cp.stderr); assert (root/'state.txt').read_text()=='old\n'; assert 'signature_invalid' in cp.stdout+cp.stderr

# A syntactically valid signature from a key not present in local trust is rejected.
with tempfile.TemporaryDirectory(prefix='ptv621_signed_untrusted_') as td:
    root=Path(td); install(root); (root/'state.txt').write_text('old\n'); policy(root,public,required=False,key_id='trusted-other')
    pack_signed(root/'patchs'/'p.zip',base('untrusted'),script,seed,key_id='test-key')
    cp=run(root); assert cp.returncode==2,(cp.stdout,cp.stderr); assert (root/'state.txt').read_text()=='old\n'; assert 'signer_untrusted' in cp.stdout+cp.stderr

# Required policy also rejects a recognized manifestless legacy archive before execution.
with tempfile.TemporaryDirectory(prefix='ptv621_legacy_required_') as td:
    root=Path(td); install(root); (root/'state.txt').write_text('old\n'); policy(root,public,required=True)
    with zipfile.ZipFile(root/'patchs'/'patch_legacy_required.zip','w') as z:
        z.writestr('patch_01.py',script)
    cp=run(root,args=['--patch','patchs/patch_legacy_required.zip']); assert cp.returncode==2,(cp.stdout,cp.stderr); assert (root/'state.txt').read_text()=='old\n'; assert 'signature_required' in cp.stdout+cp.stderr

print('PASS: v6.21.0 Ed25519 PATCH provenance/trust is local-policy, tamper-evident, fail-closed, and pre-payload')
