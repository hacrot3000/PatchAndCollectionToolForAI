#!/usr/bin/env python3
from __future__ import annotations
import json, os, shutil, subprocess, tempfile, zipfile
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent

def install(root: Path):
    shutil.copytree(TOOLS, root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()

def pack(path: Path, manifest: dict, script='print("ok")'):
    with zipfile.ZipFile(path,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        z.writestr('patch_apply.py',script)

def run(root: Path,*args,input_text='\n',timeout=120):
    return subprocess.run([str(root/'tools'/'run_python_patches.sh'),*args],cwd=root,input=input_text,text=True,capture_output=True,timeout=timeout,env={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'})

# A. A later related successor cannot bypass previous_failure because an unrelated PATCH is first.
with tempfile.TemporaryDirectory(prefix='ptv61710_later_successor_') as td:
    r=Path(td); install(r); (r/'shared.txt').write_text('x')
    pack(r/'patchs'/'failed.zip',{'schema_version':1,'patch':{'id':'failed'},'targets':['shared.txt']},'raise SystemExit(9)')
    cp=run(r); assert cp.returncode==9,(cp.stdout,cp.stderr)
    (r/'patchs'/'failed.zip').unlink(missing_ok=True)
    pack(r/'patchs'/'unrelated.zip',{'schema_version':1,'patch':{'id':'unrelated'},'targets':['other.txt']},'from pathlib import Path; Path("other.txt").write_text("ok")')
    pack(r/'patchs'/'related.zip',{'schema_version':1,'patch':{'id':'related'},'targets':['shared.txt']},'from pathlib import Path; Path("related.txt").write_text("bad")')
    cp=run(r,'plan'); text=cp.stdout+cp.stderr
    assert cp.returncode==2,text
    assert 'previous_failure_action_required' in text,text

# B. Unrelated first PATCH is not forced to declare previous_failure; the related successor may own it.
with tempfile.TemporaryDirectory(prefix='ptv61710_related_owner_') as td:
    r=Path(td); install(r); (r/'shared.txt').write_text('x')
    pack(r/'patchs'/'failed.zip',{'schema_version':1,'patch':{'id':'failed'},'targets':['shared.txt']},'raise SystemExit(9)')
    cp=run(r); assert cp.returncode==9
    # Remove failed bytes from queue; an unrelated patch comes first while a later related successor owns the explicit delete.
    (r/'patchs'/'failed.zip').unlink(missing_ok=True)
    pack(r/'patchs'/'00_unrelated.zip',{'schema_version':1,'patch':{'id':'unrelated'},'targets':['other.txt']},'print("u")')
    pack(r/'patchs'/'zz_related.zip',{
        'schema_version':1,'patch':{'id':'related'},'targets':['shared.txt'],
        'batch':{'previous_failure':{'patch_id':'failed','patch_file':'failed.zip','action':'delete','reason':'superseded'}}
    },'print("r")')
    cp=run(r,'plan'); text=cp.stdout+cp.stderr
    assert cp.returncode==0,text
    assert 'PREVIOUS FAILURE ACTION: delete' in text,text

# C. More than one unresolved related predecessor is fail-closed; singular manifest field cannot resolve both.
with tempfile.TemporaryDirectory(prefix='ptv61710_multi_prev_') as td:
    r=Path(td); install(r); (r/'a.txt').write_text('a'); (r/'b.txt').write_text('b')
    pack(r/'patchs'/'fa.zip',{'schema_version':1,'patch':{'id':'fa'},'targets':['a.txt']},'raise SystemExit(3)')
    pack(r/'patchs'/'fb.zip',{'schema_version':1,'patch':{'id':'fb'},'targets':['b.txt']},'raise SystemExit(4)')
    cp=run(r,input_text='a\n'); assert cp.returncode in {3,4}
    (r/'patchs'/'fa.zip').unlink(missing_ok=True); (r/'patchs'/'fb.zip').unlink(missing_ok=True)
    pack(r/'patchs'/'succ.zip',{'schema_version':1,'patch':{'id':'succ'},'targets':['a.txt','b.txt']},'print("s")')
    cp=run(r,'plan'); text=cp.stdout+cp.stderr
    assert cp.returncode==2,text
    assert 'multiple_previous_failures_action_required' in text,text

# D. Same patch.id with different SHA PASS must not auto-resolve older exact failure.
with tempfile.TemporaryDirectory(prefix='ptv61710_exact_registry_') as td:
    r=Path(td); install(r); (r/'shared.txt').write_text('x')
    pack(r/'patchs'/'old.zip',{'schema_version':1,'patch':{'id':'same'},'targets':['shared.txt']},'raise SystemExit(5)')
    cp=run(r); assert cp.returncode==5
    (r/'patchs'/'old.zip').unlink(missing_ok=True)
    pack(r/'patchs'/'new.zip',{'schema_version':1,'patch':{'id':'same'},'targets':['other.txt']},'print("different bytes pass")')
    cp=run(r); assert cp.returncode==0,(cp.stdout,cp.stderr)
    reg=json.loads((r/'artifacts'/'patch_tool'/'UNRESOLVED_FAILURES.json').read_text())
    assert any(not e.get('resolved') and e.get('row',{}).get('name')=='old.zip' for e in reg['entries']),reg

# E. run_anyway is accepted for compatibility but cannot bypass a failed dependency.
with tempfile.TemporaryDirectory(prefix='ptv61710_run_anyway_') as td:
    r=Path(td); install(r); (r/'dep.txt').write_text('x')
    pack(r/'patchs'/'a.zip',{'schema_version':1,'patch':{'id':'a'},'targets':['dep.txt']},'raise SystemExit(6)')
    pack(r/'patchs'/'b.zip',{'schema_version':1,'patch':{'id':'b'},'batch':{'depends_on':['a'],'on_dependency_failure':'run_anyway'}},'from pathlib import Path; Path("BAD").write_text("x")')
    cp=run(r,input_text='a\n'); text=cp.stdout+cp.stderr
    assert cp.returncode==6,text
    assert '[BLOCKED]' in text and not (r/'BAD').exists(),text
    assert 'run_anyway is deprecated/ignored' in text,text

# F. plan CLI/config policy is effective and exported exactly.
with tempfile.TemporaryDirectory(prefix='ptv61710_recipe_policy_') as td:
    r=Path(td); install(r)
    (r/'.python_patch_tool.json').write_text(json.dumps({'batch':{'failure_policy':'fail_fast','transaction_policy':'patch'}}))
    pack(r/'patchs'/'p.zip',{'schema_version':1,'patch':{'id':'p'}},'print("p")')
    cp=run(r,'plan','--export-recipe','recipe.json'); text=cp.stdout+cp.stderr
    assert cp.returncode==0,text
    recipe=json.loads((r/'recipe.json').read_text())
    assert recipe['failure_policy']=='fail_fast' and recipe['transaction_policy']=='patch',recipe
    cp=run(r,'plan','--failure-policy','continue_independent','--export-recipe','recipe2.json')
    assert cp.returncode==0,(cp.stdout,cp.stderr)
    recipe2=json.loads((r/'recipe2.json').read_text())
    assert recipe2['failure_policy']=='continue_independent',recipe2

# G. plan transaction=batch must reject target-unbounded command side effects just like execute.
with tempfile.TemporaryDirectory(prefix='ptv61710_plan_tx_') as td:
    r=Path(td); install(r); (r/'.python_patch_tool.json').write_text(json.dumps({'batch':{'transaction_policy':'batch'}}))
    pack(r/'patchs'/'p.zip',{
        'schema_version':1,'patch':{'id':'p'},'targets':['x.txt'],
        'post_patch':{'commands':[{'argv':['python3','-c','print(1)'],'cwd':'.','timeout_seconds':10}]}
    },'print("p")')
    cp=run(r,'plan'); text=cp.stdout+cp.stderr
    assert cp.returncode==2,text
    assert 'transaction_policy=batch is incompatible' in text,text

# H. zero-argument config uses the same strict parser: symlink config is not consumed as automation policy.
if os.name != 'nt':
    with tempfile.TemporaryDirectory(prefix='ptv61710_config_parser_') as td:
        r=Path(td); install(r)
        outside=r/'outside.json'; outside.write_text(json.dumps({'automation':{'zero_argument':{'selection':'all','non_interactive_confirmed':True}}}))
        (r/'.python_patch_tool.json').symlink_to(outside)
        pack(r/'patchs'/'p.zip',{'schema_version':1,'patch':{'id':'p'}},'print("p")')
        # Invalid/symlinked project config is a global policy boundary and must fail closed.
        cp=run(r,'plan'); text=cp.stdout+cp.stderr
        assert cp.returncode==2,text
        assert 'PROJECT CONFIG' in text and 'non-symlink' in text,text

print('PASS: v6.18.8 contract consistency for unresolved failures, dependency blocking, plan policy/recipe and project config parsing')

# I. previous_failure delete resolves only the exact SHA-bound registry entry when patch.id was reused.
import sys
sys.path.insert(0,str(HERE))
import python_patch_queue_dispatcher as qd
with tempfile.TemporaryDirectory(prefix='ptv61710_delete_exact_') as td:
    r=Path(td); (r/'artifacts'/'patch_tool').mkdir(parents=True)
    row1={'name':'old-a.zip','kind':'PATCH','status':'FAIL','patch_id':'dup','patch_result':{'patch_sha256':'a'*64,'manifest_patch':{'id':'dup'}}}
    row2={'name':'old-b.zip','kind':'PATCH','status':'FAIL','patch_id':'dup','patch_result':{'patch_sha256':'b'*64,'manifest_patch':{'id':'dup'}}}
    (r/'artifacts'/'patch_tool'/'UNRESOLVED_FAILURES.json').write_text(json.dumps({
        'format':'python-patch-tool-unresolved-failures','format_version':1,'entries':[
            {'resolved':False,'row':row1},{'resolved':False,'row':row2}
        ]
    }))
    action={'action':'delete','result':'already_absent','patch_id':'dup','patch_file':'old-a.zip','patch_sha256':'a'*64}
    qd._resolve_registry_previous_action(r,action)
    reg=json.loads((r/'artifacts'/'patch_tool'/'UNRESOLVED_FAILURES.json').read_text())
    states={e['row']['name']:e.get('resolved') for e in reg['entries']}
    assert states['old-a.zip'] is True and states['old-b.zip'] is False,states

print('PASS: v6.18.8 exact-SHA previous_failure delete resolution under patch.id reuse')

# J. Invalid/tampered recipe fails before duplicate-history cleanup can mutate the queue.
with tempfile.TemporaryDirectory(prefix='ptv61710_recipe_no_mutation_') as td:
    r=Path(td); install(r)
    pack(r/'patchs'/'p.zip',{'schema_version':1,'patch':{'id':'p'}},'print("p")')
    (r/'patchs'/'patched').mkdir()
    shutil.copy2(r/'patchs'/'p.zip',r/'patchs'/'patched'/'p.zip')
    recipe={
        'format':'python-patch-tool-batch-recipe','format_version':1,'tool_version':'6.18.8',
        'project_key':None,'failure_policy':'continue_independent','transaction_policy':'patch',
        'packages':[{'name':'p.zip','patch_id':'p','sha256':'0'*64}],
    }
    (r/'bad_recipe.json').write_text(json.dumps(recipe))
    cp=run(r,'run','--recipe','bad_recipe.json'); text=cp.stdout+cp.stderr
    assert cp.returncode==2,text
    assert (r/'patchs'/'p.zip').is_file(),text
    assert not list((r/'patchs'/'ignore').glob('*p.zip')) if (r/'patchs'/'ignore').exists() else True
    assert 'package_input_changed' in text or 'SHA mismatch' in text,text

print('PASS: v6.18.8 recipe validation precedes duplicate-history queue mutation')

# K. Valid recipe execution must not move unrelated queue duplicates to ignore.
with tempfile.TemporaryDirectory(prefix='ptv61710_recipe_scope_') as td:
    r=Path(td); install(r)
    pack(r/'patchs'/'selected.zip',{'schema_version':1,'patch':{'id':'selected'}},'print("selected")')
    pack(r/'patchs'/'unrelated.zip',{'schema_version':1,'patch':{'id':'unrelated'}},'print("unrelated")')
    (r/'patchs'/'patched').mkdir()
    shutil.copy2(r/'patchs'/'unrelated.zip',r/'patchs'/'patched'/'unrelated.zip')
    import hashlib
    sha=hashlib.sha256((r/'patchs'/'selected.zip').read_bytes()).hexdigest()
    recipe={
        'format':'python-patch-tool-batch-recipe','format_version':1,'tool_version':'6.18.8',
        'project_key':None,'failure_policy':'continue_independent','transaction_policy':'patch',
        'packages':[{'name':'selected.zip','patch_id':'selected','sha256':sha}],
    }
    (r/'recipe.json').write_text(json.dumps(recipe))
    cp=run(r,'run','--recipe','recipe.json'); text=cp.stdout+cp.stderr
    assert cp.returncode==0,text
    assert (r/'patchs'/'unrelated.zip').is_file(),text
    assert not list((r/'patchs'/'ignore').glob('*unrelated.zip')) if (r/'patchs'/'ignore').exists() else True
    assert (r/'patchs'/'patched'/'selected.zip').is_file(),text

print('PASS: v6.18.8 recipe execution leaves unrelated queue entries untouched')

# L. Recipe patch_id is part of the exact identity and must match package metadata.
with tempfile.TemporaryDirectory(prefix='ptv61710_recipe_patch_id_') as td:
    r=Path(td); install(r)
    pack(r/'patchs'/'p.zip',{'schema_version':1,'patch':{'id':'actual-id'}},'print("p")')
    import hashlib
    sha=hashlib.sha256((r/'patchs'/'p.zip').read_bytes()).hexdigest()
    recipe={
        'format':'python-patch-tool-batch-recipe','format_version':1,'tool_version':'6.18.8',
        'project_key':None,'failure_policy':'continue_independent','transaction_policy':'patch',
        'packages':[{'name':'p.zip','patch_id':'wrong-id','sha256':sha}],
    }
    (r/'recipe.json').write_text(json.dumps(recipe))
    cp=run(r,'run','--recipe','recipe.json'); text=cp.stdout+cp.stderr
    assert cp.returncode==2,text
    assert 'patch_id mismatch' in text,text
    assert (r/'patchs'/'p.zip').is_file(),text
    assert not (r/'patchs'/'patched'/'p.zip').exists(),text

print('PASS: v6.18.8 recipe patch_id is SHA-bound execution identity')


# M. Recipe owns batch policies; CLI overrides would make replay non-reproducible and are rejected.
with tempfile.TemporaryDirectory(prefix='ptv61710_recipe_policy_override_') as td:
    r=Path(td); install(r)
    pack(r/'patchs'/'p.zip',{'schema_version':1,'patch':{'id':'p'}},'print("p")')
    import hashlib
    sha=hashlib.sha256((r/'patchs'/'p.zip').read_bytes()).hexdigest()
    recipe={
        'format':'python-patch-tool-batch-recipe','format_version':1,'tool_version':'6.18.8',
        'project_key':None,'failure_policy':'fail_fast','transaction_policy':'patch',
        'packages':[{'name':'p.zip','patch_id':'p','sha256':sha}],
    }
    (r/'recipe.json').write_text(json.dumps(recipe))
    cp=run(r,'run','--recipe','recipe.json','--failure-policy','continue_independent'); text=cp.stdout+cp.stderr
    assert cp.returncode==2,text
    assert 'CLI policy overrides are not allowed with --recipe' in text,text
    assert (r/'patchs'/'p.zip').is_file(),text
    assert not (r/'patchs'/'patched'/'p.zip').exists(),text

print('PASS: v6.18.8 recipe policies cannot be overridden during replay')
