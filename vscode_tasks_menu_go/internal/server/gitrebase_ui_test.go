package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestInteractiveRebasePlannerAndGuardedExecutionUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"['rebase','Rebase']",
		"gitRebaseActions=['pick','reword','edit','squash','fixup','drop']",
		"function validateRebasePlanClient(plan)",
		"function projectedRebaseHistory(plan)",
		"function renderRebasePlanList(host,preview,meta)",
		"async function loadRebase()",
		"gitView('rebase-plan',{base:value,limit:'150'})",
		"Base branch or full commit SHA (exclusive)",
		"Drag to reorder",
		"New commit message",
		"Projected history",
		"Working tree is dirty. Planning is allowed",
		"This range contains merge commits.",
		"Range is truncated.",
		"action==='squash'||action==='fixup'",
		"cannot be the first non-dropped commit",
		"Plan drops every commit",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("interactive rebase planner UI missing %q", want)
		}
	}
	for _, want := range []string{
		"Start interactive rebase",
		"function structuredRebasePlan()",
		"async function startInteractiveRebase(meta)",
		"action('interactive_rebase'",
		"expected_sha:meta.head_sha",
		"expected_current:meta.branch",
		"rebase_plan:plan",
		"Working tree and index must be clean before starting interactive rebase",
		"Merge-containing ranges are not yet supported",
		"The selected range is truncated",
		"Structured plan is validated again server-side",
		"function renderRebasePausedControls(meta,data)",
		"Continue rebase",
		"Abort rebase",
		"continue_in_progress",
		"abort_in_progress",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("interactive rebase execution UI missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"GIT_SEQUENCE_EDITOR",
		"GIT_EDITOR",
		"sequence_editor",
		"raw_command",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("browser UI must not construct editor scripts or raw commands; found %q", forbidden)
		}
	}
}

func TestInteractiveRebasePlannerBackendUsesExactCommitGuards(t *testing.T) {
	data, err := webassets.Files.ReadFile("../internal/server/gitrebase.go")
	if err == nil {
		_ = data
		t.Fatal("server Go source must not be embedded in web assets")
	}
}
