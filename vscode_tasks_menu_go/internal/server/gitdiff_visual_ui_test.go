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
		"['staged','HEAD ↔ Staged'",
		"['worktree','Staged ↔ Working'",
		"['head-worktree','HEAD ↔ Working'",
		"()=>showDiff(path,value)",
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

func TestGitPreviewReusesFullDiffColorsAndOneHorizontalScroller(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"decorateGitPreviewRows(rows,hunk.previewPath||'')",
		"compare.renderGitPreviewCode(code,spec,ranges)",
		".git-diff-cell.removed.important{background:rgba(229,72,86,.28)}",
		".git-diff-cell.added.important{background:rgba(232,174,55,.28)}",
		".git-diff-cell.removed.unimportant,.git-diff-cell.added.unimportant{background:rgba(58,149,214,.23)}",
		".git-diff-code{display:block;padding:1px 7px;white-space:pre;overflow:visible",
		".git-diff-preview-scroll{width:100%;min-width:0;overflow-x:auto",
		".git-diff-visual-row{display:contents}",
		"const preview=el('div','git-diff-preview-scroll')",
		"preview.append(card)",
		"content.append(preview)",
		"Open full diff tab ↗",
		"Full diff ↗",
		"openGenericGitStateCompare(path,mode)",
	} {
		if !strings.Contains(js,want) { t.Fatalf("Git diff preview missing %q",want) }
	}
	if strings.Contains(js,".git-diff-code{padding:1px 7px;white-space:pre;overflow-x:auto") {
		t.Fatal("Git diff code lines must never create independent horizontal scrollers")
	}
}
