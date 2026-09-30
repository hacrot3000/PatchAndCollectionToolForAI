package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

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
		"git-branch-section-toggle",
		"remoteList.hidden=true",
		"remoteToggle.setAttribute('aria-expanded','false')",
		"'▸ ')+'REMOTE ('+remoteRows.length+')'",
		"'▾ ':'▸ '",
		"remoteList.hidden=expanded",
		"'log','Log'",
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
		"action('unstage'",
		"action('switch'",
		"action('create_branch'",
		"action('stash_pop'",
		"Copy command",
		"Copy SHA",
		"Copy path",
		"git-action-running",
		"git-action-success",
		"git-action-error",
		"const runningActions=new Map()",
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
		"Type LOCAL or REMOTE",
		"expected_sha:expectedSHA",
		"expected_current:check.current",
		"expectedSHA.slice(0,12)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitstatus.js missing Git quick action behavior %q", want)
		}
	}
}
