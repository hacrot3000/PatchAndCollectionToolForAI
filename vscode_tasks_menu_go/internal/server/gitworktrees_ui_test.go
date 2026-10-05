package server

import (
	"os"
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
		"worktree_open",
		"Open TaskDeck",
		"taskdeck --workspace <selected worktree>",
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

func TestGitWorktreeLauncherIsWiredFromMain(t *testing.T) {
	data, err := os.ReadFile("../../cmd/vscode_tasks_menu/main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"OpenWorkspace: launchTaskdeckWorkspace",
		"func launchTaskdeckWorkspace(workspace string) error",
		"preferredTaskdeckExecutable()",
		`exec.Command(exe, "--workspace", abs)`,
		"cmd.Process.Release()",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("TaskDeck worktree launcher missing %q", want)
		}
	}
}
