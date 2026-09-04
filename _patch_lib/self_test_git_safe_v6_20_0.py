#!/usr/bin/env python3
from __future__ import annotations
import os, shutil, subprocess, tempfile
from pathlib import Path

from python_patch_collect_schema import validate_request_data, CollectSchemaError
from python_patch_git_safe import validate_git_action, run_git_operations, GitSafeError
from python_patch_package_schema import validate_manifest, PatchSchemaError
import python_patch_runner as runner

assert not hasattr(runner, '_run_git_policy'), 'PATCH Git mutation execution function must be absent'
source=(Path(runner.__file__).read_text(encoding='utf-8'))
for token in ['git add', 'git commit', 'git push']:
    assert token not in source, token

# Requirement-driven retirement is explicit, not silent.
for field,value in [('add','changed'),('commit','auto'),('push','auto')]:
    manifest={'schema_version':1,'patch':{'id':'reject-git'},'git':{'add':'off','commit':'off','push':'off'}}
    manifest['git'][field]=value
    if field=='commit': manifest['git']['commit_message']='x'
    try: validate_manifest(manifest)
    except PatchSchemaError as exc: assert exc.kind=='git_mutation_forbidden',(field,exc.kind,exc)
    else: raise AssertionError(f'PATCH Git mutation accepted: {field}')
validate_manifest({'schema_version':1,'patch':{'id':'git-off'},'git':{'add':'off','commit':'off','push':'off'}})

# No raw command escape hatch or mutation operation survives COLLECT validation.
for action in [
    {'type':'git','operations':[{'op':'add'}]},
    {'type':'git','operations':[{'op':'commit'}]},
    {'type':'git','operations':[{'op':'merge'}]},
    {'type':'git','operations':[{'op':'status','argv':['status']}]},
    {'type':'git','operations':[{'op':'status','command':'git status'}]},
    {'type':'git','raw_git':'status','operations':[{'op':'status'}]},
]:
    try: validate_request_data({'actions':[action]})
    except CollectSchemaError: pass
    else: raise AssertionError(f'unsafe Git action accepted: {action}')

legacy=validate_request_data({'actions':[{'type':'git','sections':['status','log','diff_stat','diff'],'log_entries':7}]})['actions'][0]
assert [x['op'] for x in legacy['operations']]==['status','log','diff_worktree','diff_worktree'],legacy
assert legacy['operations'][1]['max_entries']==7

if shutil.which('git'):
    with tempfile.TemporaryDirectory(prefix='ptv620_git_safe_') as td:
        project=Path(td); repo=project/'projects'/'m3-client'; repo.mkdir(parents=True)
        subprocess.run(['git','init','-q'],cwd=repo,check=True)
        subprocess.run(['git','config','user.email','ptv@example.invalid'],cwd=repo,check=True)
        subprocess.run(['git','config','user.name','PTV'],cwd=repo,check=True)
        (repo/'a.txt').write_text('base\n')
        subprocess.run(['git','add','a.txt'],cwd=repo,check=True); subprocess.run(['git','commit','-qm','base'],cwd=repo,check=True)
        base_branch=subprocess.run(['git','branch','--show-current'],cwd=repo,text=True,capture_output=True,check=True).stdout.strip()
        subprocess.run(['git','switch','-c','feature'],cwd=repo,check=True,stdout=subprocess.DEVNULL)
        (repo/'a.txt').write_text('feature\n'); subprocess.run(['git','commit','-am','feature','-q'],cwd=repo,check=True)
        feature_sha=subprocess.run(['git','rev-parse','HEAD'],cwd=repo,text=True,capture_output=True,check=True).stdout.strip()
        subprocess.run(['git','switch',base_branch],cwd=repo,check=True,stdout=subprocess.DEVNULL)
        # Worktree and staged diff operations are data-only.
        (repo/'a.txt').write_text('worktree\n')
        (repo/'staged.txt').write_text('staged\n'); subprocess.run(['git','add','staged.txt'],cwd=repo,check=True)
        action={'type':'git','repo':'projects/m3-client','operations':[
            {'op':'status'},{'op':'current_branch'},{'op':'branches'},
            {'op':'log','ref':'HEAD','max_entries':5},{'op':'show','ref':'HEAD'},
            {'op':'diff_worktree'},{'op':'diff_staged'},
            {'op':'diff_refs','from':base_branch,'to':'feature'},
            {'op':'diff_ref_worktree','ref':'HEAD'},
        ]}
        report=run_git_operations(project,validate_git_action(action))
        for token in ['# Git safe operations','## 1. status','## 7. diff_staged','feature']:
            assert token in report,token
        # switch refuses all local/untracked/index changes.
        try:
            from python_patch_git_safe import execute_git_operation
            execute_git_operation(repo,{'op':'switch','branch':'feature'})
        except GitSafeError as exc: assert 'clean' in str(exc)
        else: raise AssertionError('dirty switch was accepted')
        subprocess.run(['git','reset','--hard','-q'],cwd=repo,check=True); subprocess.run(['git','clean','-fdq'],cwd=repo,check=True)
        # Existing local branch only and Git hooks disabled for the allowed switch exception.
        hook=repo/'.git'/'hooks'/'post-checkout'; marker=repo/'HOOK_SHOULD_NOT_RUN'
        if os.name!='nt':
            hook.write_text(f'#!/bin/sh\nprintf bad > {marker.name}\n'); hook.chmod(0o755)
        from python_patch_git_safe import execute_git_operation
        kind,text=execute_git_operation(repo,{'op':'switch','branch':'feature'})
        assert kind=='switch' and subprocess.run(['git','branch','--show-current'],cwd=repo,text=True,capture_output=True).stdout.strip()=='feature'
        if os.name!='nt': assert not marker.exists(),'checkout hook executed despite core.hooksPath=/dev/null'
        try: execute_git_operation(repo,{'op':'switch','branch':'does-not-exist'})
        except GitSafeError as exc: assert 'existing local branch' in str(exc)
        else: raise AssertionError('non-local/non-existing switch accepted')
        assert subprocess.run(['git','rev-parse','HEAD'],cwd=repo,text=True,capture_output=True).stdout.strip()==feature_sha

print('PASS: v6.20.0 Git interface is strict allowlist-only; inspect/diff/log/safe local switch work, hooks are disabled, and PATCH add/commit/push execution code is absent')
