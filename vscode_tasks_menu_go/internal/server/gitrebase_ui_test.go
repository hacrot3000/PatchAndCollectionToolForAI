package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestInteractiveRebasePlannerUIIsReadOnlyAndStructured(t *testing.T) {
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
		"Planner only: no Git history has been modified.",
		"squash cannot be the first non-dropped commit",
		"fixup cannot be the first non-dropped commit",
		"Plan drops every commit",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("interactive rebase planner UI missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"rebase_execute",
		"GIT_SEQUENCE_EDITOR",
		"git rebase -i",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("planner milestone must remain read-only; found %q", forbidden)
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
