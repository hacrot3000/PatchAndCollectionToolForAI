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
		"'log','Log'",
		"'ahead-behind','Ahead / Behind'",
		"'stashes','Stash'",
		"'repositories','Repositories'",
		"'compare','Compare'",
		"git-repo-select",
		"view:'repositories'",
		"repoStorageKey()",
		"if(activeRepoID)query.set('repo',activeRepoID)",
		"repoID+'|'+actionName",
		"auto_select_from_terminal_cwd",
		"repositoryForCWD(cwd)",
		"taskmenu:view-activated",
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
		"Merge this branch into the current branch",
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
