package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitSemanticsWizardExplainsHeadIndexAndWorkingTree(t *testing.T) {
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	nextJS := string(next)
	wizardImport := "import '/featuremods/gitsemanticswizard.js';"
	gitImport := "import '/featuremods/gitstatus.js';"
	if !strings.Contains(nextJS, wizardImport) {
		t.Fatal("Git semantics wizard is not loaded")
	}
	if strings.Index(nextJS, wizardImport) > strings.Index(nextJS, gitImport) {
		t.Fatal("Git semantics wizard must load before Git panel integration")
	}

	data, err := webassets.Files.ReadFile("featuremods/gitsemanticswizard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Git reset / revert semantics",
		"Reset soft",
		"Reset mixed",
		"Reset hard",
		"Revert commit",
		"Restore file from commit",
		"Restore staged only",
		"Move HEAD to target",
		"Keep Index unchanged",
		"Reset Index to target",
		"Keep Working tree unchanged",
		"Reset tracked files to target",
		"Destructive: tracked staged and working-tree changes are discarded.",
		"HEAD unchanged",
		"Index unchanged",
		"Only what would be committed for this path changes.",
		"Visual diff",
		"Snapshot guard",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git semantics wizard missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("Git semantics wizard must not render repository text through innerHTML")
	}
	for _, forbidden := range []string{
		"git reset --hard",
		"git restore --source",
		"git revert ",
		"raw_command",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("Git semantics wizard must emit structured operations, found raw command %q", forbidden)
		}
	}
}

func TestGitPanelUsesSemanticsWizardAndSnapshotGuards(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function gitSemanticsPreview(ref,path='')",
		"gitView('semantics-preview',params)",
		"async function openGitCommitSemantics(commit,preferred='mixed')",
		"TaskDeckGitSemanticsWizard",
		"expected_head_sha:preview.head_sha",
		"expected_sha:preview.target_sha",
		"restore_file_commit",
		"restore_staged_commit",
		"openGitFileRestoreSemantics(commit,gitFilePath)",
		"Save or close the unsaved editor before restoring this Working tree file",
		"gitReloadWorkspaceEditor(workspacePath)",
		"Revert…",
		"Restore…",
		"openGitCommitFileDiffBetween",
		"openGitCommitAgainstProject",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git panel semantics integration missing %q", want)
		}
	}
}
