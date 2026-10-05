package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitMergeEditorLoadsAndProvidesThreeWayWorkflow(t *testing.T) {
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(next), "import '/featuremods/gitmergeeditor.js';") {
		t.Fatal("Git 3-way merge editor feature is not loaded")
	}

	data, err := webassets.Files.ReadFile("featuremods/gitmergeeditor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Git 3-way conflict editor",
		"BASE · stage 1",
		"CURRENT / OURS · stage 2",
		"INCOMING / THEIRS · stage 3",
		"RESULT · working tree",
		"Accept Current",
		"Accept Incoming",
		"Accept Both",
		"Copy selected block",
		"Previous conflict",
		"Next conflict",
		"function parseConflictBlocks(text)",
		"^<<<<<<<",
		"^\\|\\|\\|\\|\\|\\|\\|",
		"^=======",
		"^>>>>>>>",
		"function replaceSelectedBlock(kind)",
		"function saveResult()",
		"/api/git/conflict-file?",
		"expected_sha256:expected",
		"Resolve all conflict marker blocks before Mark resolved",
		"conflict_mark_resolved",
		"conflict_take_side",
		"abort_in_progress",
		"globalThis.TaskMenuGitMergeEditor={open,close,parseConflictBlocks",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git merge editor missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("Git merge editor must not render conflict content through innerHTML")
	}
}

func TestGitMergeEditorIntegratedWithChangesAndRecovery(t *testing.T) {
	gitData, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	gitJS := string(gitData)
	for _, want := range []string{
		"3-way editor",
		"TaskMenuGitMergeEditor",
		"editor.open({repoID:activeRepoID,path:change.path})",
		"openMergeEditor:path=>",
		"runRepair:(repair,payload={},repoID=activeRepoID)=>repairAction",
	} {
		if !strings.Contains(gitJS, want) {
			t.Fatalf("Git panel merge editor integration missing %q", want)
		}
	}

	recoveryData, err := webassets.Files.ReadFile("featuremods/gitrecovery.js")
	if err != nil {
		t.Fatal(err)
	}
	recoveryJS := string(recoveryData)
	for _, want := range []string{
		"Open 3-way merge editor",
		"Base / Current / Incoming / Result",
		"ctx.openMergeEditor(values.path)",
	} {
		if !strings.Contains(recoveryJS, want) {
			t.Fatalf("Git recovery merge editor integration missing %q", want)
		}
	}
}
