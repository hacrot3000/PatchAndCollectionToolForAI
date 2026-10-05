package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitVisualDiffShowsThreeStatePairs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function gitDiffModeMeta(mode)",
		"HEAD ↔ STAGED",
		"STAGED ↔ WORKING",
		"HEAD ↔ WORKING",
		"HEAD · committed",
		"INDEX · staged",
		"WORKTREE · not staged",
		"WORKTREE · current",
		"function gitVisualHunkRows(hunk)",
		"function renderGitVisualHunk(hunk,mode)",
		"git-diff-columns-head",
		"git-diff-visual-row",
		".git-diff-cell.removed",
		".git-diff-cell.added",
		"Raw unified patch",
		"git-panel-wide",
		"showDiff(path,'staged')",
		"showDiff(change.path,'worktree')",
		"showDiff(change.path,'head-worktree')",
		"HEAD ↔ Staged",
		"Staged ↔ Working",
		"HEAD ↔ Working",
		"3-state file: HEAD → staged → working",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("git visual diff UI missing %q", want)
		}
	}
}

func TestGitHeadWorkingDiffIsReadOnlyForHunkMutations(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	start := strings.Index(js, "async function showDiff(path,mode='head-worktree')")
	end := strings.Index(js[start:], "async function openIgnoreWizard")
	if start < 0 || end < 0 {
		t.Fatal("showDiff function not found")
	}
	block := js[start : start+end]
	if !strings.Contains(block, "if(mode==='staged')") || !strings.Contains(block, "}else if(mode==='worktree'){") {
		t.Fatal("showDiff must limit hunk mutations to staged/worktree modes")
	}
	if strings.Contains(block, "mode==='head-worktree'") && strings.Contains(block, "runHunkAction(path,mode") {
		t.Fatal("HEAD-to-working visual diff must remain read-only")
	}
}
