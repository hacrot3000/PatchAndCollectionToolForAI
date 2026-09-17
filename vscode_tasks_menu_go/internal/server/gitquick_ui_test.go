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
		"'compare','Compare'",
		"action('stage'",
		"action('unstage'",
		"action('switch'",
		"action('create_branch'",
		"action('stash_pop'",
		"Copy command",
		"Copy SHA",
		"Copy path",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitstatus.js missing Git quick action behavior %q", want)
		}
	}
}
