package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitWorktreeManagerUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"['worktrees','Worktrees']",
		"async function loadWorktrees()",
		"gitView('worktrees')",
		"Existing local branch",
		"New branch + worktree",
		"sibling directory name",
		"worktree_add_branch",
		"worktree_add_new_branch",
		"worktree_remove",
		"worktree_prune",
		"Prune stale",
		"already checked out",
		"Git will refuse if the worktree contains uncommitted changes. No force removal is used.",
		"worktreeDirectorySuggestion",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git Worktree Manager UI missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"worktree_remove',{path:",
		"worktree_add_branch',{path:",
		"worktree_add_new_branch',{path:",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("Git Worktree Manager must not send arbitrary filesystem path: %q", forbidden)
		}
	}
}
