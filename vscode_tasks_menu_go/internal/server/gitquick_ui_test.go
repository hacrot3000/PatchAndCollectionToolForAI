package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)


func TestGitQuickPanelIsLeftAlignedAndCommandLogIsTaller(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"z-index:2100",
		"left:12px;right:auto",
		"body.task-sidebar-auto-hide .git-panel{left:60px;width:min(1080px,calc(100vw - 72px))}",
		"max-height:min(46vh,460px)",
		"max-height:min(38vh,360px)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitstatus.js missing Git panel/log layout contract %q", want)
		}
	}
	if strings.Contains(js, "top:calc(var(--taskmenu-header-height,30px) + 6px);right:12px;") {
		t.Fatal("Git Quick Actions must not remain right-aligned")
	}

	if strings.Contains(js, "z-index:1500") {
		t.Fatal("Git Quick Actions must stay above the task activity bar")
	}
}

func TestGitQuickActionsUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Git Quick Actions",
		"git-quick-groups",
		"git pull --ff-only",
		"Commit + Push",
		"Stage all",
		"URLSearchParams({view,...params})",
		"'changes','Changes'",
		"'branches','Branches'",
		"'tags','Tags'",
		"async function loadTags()",
		"Create at HEAD",
		"action('create_tag'",
		"action('delete_tag'",
		"Delete local Git tag",
		"git-branch-section-toggle",
		"remoteList.hidden=!expanded",
		"remoteToggle.setAttribute('aria-expanded',expanded?'true':'false')",
		"'▸ ')+'REMOTE-TRACKING REFS ('+remoteRows.length+')'",
		"'▾ ':'▸ '",
		"remoteList.hidden=expanded",
		"'log','Log'",
		"action('cherry_pick'",
		"action('revert_commit'",
		"Cherry-pick",
		"Revert…",
		"openGitCommitSemantics(commit,'revert')",
		"expected_sha:commit.sha",
		"'file-history','File History'",
		"function openGitFileView(path,mode='history')",
		"function gitFileControls()",
		"async function loadFileHistory()",
		"gitView('file-history',{path:gitFilePath,limit:'100'})",
		"gitView('blame',{path:gitFilePath})",
		"Show commit history for this file",
		"Show line authorship for this file",
		"'ahead-behind','Ahead / Behind'",
		"'stashes','Stash'",
		"'repositories','Repositories'",
		"'compare','Compare'",
		"git-repo-select",
		"view:'repositories'",
		"repoStorageKey()",
		"if(repoID)query.set('repo',repoID)",
		"repoID+'|'+actionName",
		"auto_select_from_terminal_cwd",
		"repositoryForCWD(cwd)",
		"else if(workspaceRoot&&(value.startsWith('/')||/^[A-Za-z]:\\//.test(value)))return null",
		"taskmenu:view-activated",
		"autoSelectRepositoryForTerminal(id,{reload=true}={})",
		"autoSelectRepositoryForTerminal(app.active,{reload:false})",
		"app.views.has(String(app.active||''))",
		"const repoAtStart=activeRepoID",
		"String(app.active||'')!==String(id)||activeRepoID!==repoAtStart",
		"refreshSeq=0",
		"if(changed)refreshSeq++",
		"return repoID===activeRepoID?data:null",
		"if(!data)return false",
		"Repository selection changed; merge canceled",
		"action('stage'",
		"function runHunkAction(path,mode,data,hunk,kind)",
		"Stage hunk",
		"Unstage hunk",
		"Discard hunk",
		"expected_diff_sha",
		"git-hunk",
		"action('unstage'",
		"action('switch'",
		"action('create_branch'",
		"const actionName=force?'force_delete_branch':'delete_branch'",
		"action(actionName,{branch:branch.name,confirmed:true}",
		"action('delete_remote_tracking'",
		"action('delete_remote_branch'",
		"Prune remotes",
		"git branch -d",
		"git branch -D",
		"git branch -dr",
		"Delete local",
		"Force delete local",
		"unmerged commits can be lost",
		"Delete upstream remote",
		"Forget local ref",
		"Delete on remote",
		"REMOTE-TRACKING REFS",
		"This action deletes LOCAL only",
		"It DOES NOT delete the branch from the remote server",
		"action('stash_pop'",
		"Copy command",
		"Copy SHA",
		"Copy path",
		"git-action-running",
		"git-action-success",
		"git-action-error",
		"const runningActions=new Map()",
		"let activeGitJob=null",
		"operationCancel",
		"function gitUIAsyncAction(action)",
		"function waitGitJob(initial,command)",
		"/api/git/jobs?id=",
		"/api/git/jobs/control",
		"async:gitUIAsyncAction(actionName)",
		"Canceling…",
		"if(runningActions.has(key))return runningActions.get(key)",
		"beginOperation(command)",
		"showOperation(command,message,'','running')",
		"button.textContent='⏳ '+label",
		"button.textContent=(state==='success'?'✓ ':'✕ ')+label",
		"action('merge'",
		"'merge-preflight'",
		"Merge From",
		"Merge the selected branch into the current branch",
		"Merge To",
		"mergeToBranch(branch)",
		"'merge-to-preflight'",
		"target_source:targetSource",
		"allow_dirty:allowDirty",
		"allow_slow_fallback:allowSlowFallback",
		"let allowSlowFallback=false",
		"Only committed HEAD",
		"will remain untouched",
		"slow_fallback",
		"temporary worktree",
		"no-checkout merge-tree",
		"expected_source_sha:check.current_sha",
		"expected_target_sha:check.target_sha",
		"Local and remote versions differ",
		"Choose source version for Merge From",
		"gitBranchChoiceWizard.open({",
		"Merge selected version",
		"Remote branch deleted · delete local too?",
		"Delete local branch safely",
		"defaultChoice:'keep'",
		"deleteRemoteBranch({name:branch.upstream},{localBranch:branch})",
		"expected_sha:expectedSHA",
		"expected_current:check.current",
		"expectedSHA.slice(0,12)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitstatus.js missing Git quick action behavior %q", want)
		}
	}
}

func TestGitQuickActionsCanCopyFullOperationOutput(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"operationCopy.textContent='Copy command'",
		"operationCopyOutput.textContent='Copy output'",
		"operationCopyOutput.disabled=true",
		"operationHead.append(operationCommand,operationCopy,operationCopyOutput,operationCancel,operationClear)",
		"lastCommand='',lastOutput=''",
		"lastOutput=[error&&('ERROR: '+error),output].filter(Boolean).join('\\n')",
		"operationOutput.textContent=lastOutput||'(no output)'",
		"operationCopyOutput.disabled=!lastOutput",
		"operationCopy.onclick=()=>copyText(lastCommand)",
		"operationCopyOutput.onclick=()=>{if(!lastOutput)return;copyText(lastOutput).catch(app.showError);}",
		"lastCommand='';lastOutput='';operationCopyOutput.disabled=true",
		"renderGitJob(data,command)",
		"showOperation(command,data.output||''",
	} {
		if !strings.Contains(js,want) { t.Errorf("Git operation output copy missing %q",want) }
	}
}

func TestGitQuickRemoteTrackingSwitchDedupAndInPlaceDelete(t *testing.T) {
	data,err:=webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err!=nil {t.Fatal(err)}
	js:=string(data)
	for _,want:=range []string{
		"case 'switch_remote':",
		"return 'git switch -c '+q(local)+' --track '+q(ref)",
		"actionButton('Switch',()=>action('switch_remote',{branch:branch.name})",
		"function visibleRemoteTrackingBranches(data)",
		"const localNames=new Set(local.map(branch=>String(branch?.name||'')))",
		"const localUpstreams=new Set(local.map(branch=>String(branch?.upstream||'')).filter(Boolean))",
		"const candidate=ref.slice(slash+1)",
		"ref.endsWith('/HEAD')",
		"!localNames.has(candidate)&&!localUpstreams.has(ref)",
		"const remoteRows=visibleRemoteTrackingBranches(data)",
		"const expandedRemoteBranchSections=new Map()",
		"expandedRemoteBranchSections.get(repoID)===true",
		"expandedRemoteBranchSections.set(repoID,!expanded)",
		"function removeRenderedBranchRow(repoID,branchName,remote)",
		"row.remove();state.rows.delete(key)",
		"state.remoteRows.splice(index,1)",
		"state.renderRemoteToggle()",
		"removeRenderedBranchRow(repoID,branch.name,false)",
		"removeRenderedBranchRow(repoID,branch.name,true)",
		"removeRenderedBranchRow(repoID,remoteRef,true)",
		"{repoID,refresh:false}",
		"if(!data||activeRepoID!==repoID||currentView!=='branches')return false",
	}{
		if !strings.Contains(js,want){t.Errorf("Git remote branch functionality missing %q",want)}
	}
	for _,marker:=range []string{
		"async function deleteLocalBranch(branch,force=false){",
		"async function deleteRemoteTracking(branch){",
		"async function deleteRemoteBranch(branch,{localBranch=null}={}){",
	}{
		from:=strings.Index(js,marker)
		if from<0 {t.Fatalf("missing function %s",marker)}
		rest:=js[from+len(marker):]
		end:=strings.Index(rest,"\nasync function ")
		if end<0 {end=len(rest)}
		body:=rest[:end]
		if strings.Contains(body,"return loadBranches()")||strings.Contains(body,"return result===false?false:loadBranches()"){
			t.Errorf("%s triggers full branch-list reload",marker)
		}
	}
}

func TestGitRemoteTrackingAgeSortedNewestAndRefreshesWithoutReload(t *testing.T) {
	data,err:=webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err!=nil {t.Fatal(err)}
	js:=string(data)
	for _,want:=range []string{
		"function gitBranchCommitTimestamp(branch)",
		"Number(branch?.commit_time)",
		"Number.isSafeInteger(unix)&&unix>0",
		"function gitRelativeCommitAge(unix,now=Date.now())",
		"if(elapsed<60)return 'now'",
		"minute'+(minutes===1?'':'s')+' ago'",
		"hour'+(hours===1?'':'s')+' ago'",
		"day'+(days===1?'':'s')+' ago'",
		"month'+(months===1?'':'s')+' ago'",
		"year'+(years===1?'':'s')+' ago'",
		".sort((a,b)=>gitBranchCommitTimestamp(b)-gitBranchCommitTimestamp(a)||",
		"String(a.name||'').localeCompare(String(b.name||''))",
		"const remoteRows=visibleRemoteTrackingBranches(data)",
		"const age=el('span','git-branch-age',gitRelativeCommitAge(unix))",
		"subtitle.append(document.createTextNode(' · Last commit: '),age)",
		"new Date(unix*1000).toLocaleString()",
		"Fetch to update",
		"state.ageLabels.push({node:age,unix})",
		"state.ageLabels=state.ageLabels.filter(item=>item.node.isConnected)",
		"for(const item of state.ageLabels)item.node.textContent=gitRelativeCommitAge(item.unix)",
		"},60000)",
		"content.replaceChildren()",
		"const expanded=expandedRemoteBranchSections.get(repoID)===true",
	}{
		if !strings.Contains(js,want){t.Errorf("Git remote relative tip time missing %q",want)}
	}
	if strings.Contains(js,"git log -1") || strings.Contains(js,"git show --format="){
		t.Fatal("Git branch ages must come from the batched Git backend, not separate subprocesses in the UI")
	}
}
