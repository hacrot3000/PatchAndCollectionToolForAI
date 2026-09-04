#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, io, os, sys, tempfile
from pathlib import Path

HERE=Path(__file__).resolve().parent

def load(name: str, file: str):
    spec=importlib.util.spec_from_file_location(name,HERE/file)
    mod=importlib.util.module_from_spec(spec); sys.modules[name]=mod; assert spec.loader; spec.loader.exec_module(mod)
    return mod

a=load('ptv_upload_alias','python_patch_upload_alias.py')
q=load('ptv_copy_path_dispatcher','python_patch_queue_dispatcher.py')
assert a.VERSION==q.VERSION=='6.20.0'

class FakeTTY(io.StringIO):
    def isatty(self): return True

with tempfile.TemporaryDirectory(prefix='ptv_short_upload_') as td:
    root=Path(td)
    long_dir=root/'artifacts'/'patch_tool'/'fail_handoffs'; long_dir.mkdir(parents=True)
    stem='FAIL_HANDOFF_' + ('patch_m3_camp_arena_final_combat_sanitize_hp_penalty_'*3) + '20260903_145936_809078'
    z=long_dir/(stem+'.zip'); t=long_dir/(stem+'.txt')
    z.write_bytes(b'zip-bytes'); t.write_text('text-evidence',encoding='utf-8')
    az,at,used=a.create_upload_aliases(root,z,t,kind='FAIL_HANDOFF')
    assert used and at is not None,(az,at,used)
    assert az.parent==root/'artifacts'/'ptv_to_ai'
    assert az.name.startswith('FH_') and len(az.name) <= len('FH_12345678.zip')
    assert at.name==az.with_suffix('.txt').name
    assert os.path.samefile(z,az) and os.path.samefile(t,at)
    assert az.stat().st_ino==z.stat().st_ino and at.stat().st_ino==t.stat().st_ino

    old_no=os.environ.get('NO_COLOR')
    try:
        os.environ['NO_COLOR']='1'
        buf=FakeTTY(); q._print_upload_action_block(z,patch_failure=True,companion_path=t,root=root,stream=buf)
    finally:
        if old_no is None: os.environ.pop('NO_COLOR',None)
        else: os.environ['NO_COLOR']=old_no
    lines=buf.getvalue().splitlines()
    path_lines=[x for x in lines if '/artifacts/ptv_to_ai/' in x]
    assert len(path_lines)==2,buf.getvalue()
    assert all(x.startswith(str(root/'artifacts'/'ptv_to_ai')) for x in path_lines),path_lines
    assert all('ZIP (preferred)' not in x and 'Clear-text TXT' not in x for x in path_lines)
    assert str(z) not in path_lines and str(t) not in path_lines
    assert 'ZIP (preferred) — copy path below:' in lines
    assert 'Clear-text TXT — copy path below:' in lines

# Alias failure is presentation-only: unsafe alias dir falls back to canonical artifact.
with tempfile.TemporaryDirectory(prefix='ptv_short_upload_unsafe_') as td:
    root=Path(td); (root/'artifacts').mkdir(); (root/'real').mkdir(); (root/'artifacts'/'ptv_to_ai').symlink_to(root/'real',target_is_directory=True)
    z=root/'canonical.zip'; z.write_bytes(b'x')
    az,at,used=a.create_upload_aliases(root,z,None,kind='FAIL_HANDOFF')
    assert not used and az==z.absolute() and at is None

print('PASS: v6.20.0 copy-friendly short hard-link upload aliases keep ACTION REQUIRED paths on dedicated rows with safe fallback')
