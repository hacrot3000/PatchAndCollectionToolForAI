package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	updater "bletonfc/vscode_tasks_menu/internal/selfupdate"
	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSelfUpdateProtectsTerminalStateFromTeardownSnapshot(t *testing.T) {
	workspace := t.TempDir()
	original := projectTerminalState{
		Version:     2,
		Terminals:   []terminalStateItem{{Cwd: workspace}, {Cwd: workspace}},
		ActiveIndex: 1,
		Splits:      []terminalSplitState{{Left: 0, Right: 1, Ratio: 0.5, Orientation: "vertical"}},
	}
	if err := writeProjectTerminalState(workspace, original); err != nil {
		t.Fatal(err)
	}
	req, err := updater.CreateRequest(workspace, "0123456789abcdef", "http://127.0.0.1:1234", true)
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: workspace}
	r := httptest.NewRequest("PUT", "/api/state/tasks?scope=terminals", strings.NewReader(`{"session_ids":[],"active_session_id":"","splits":[]}`))
	w := httptest.NewRecorder()
	s.taskState(w, r)
	if w.Code != 200 {
		t.Fatalf("protected terminal PUT status=%d body=%s", w.Code, w.Body.String())
	}
	got, err := readProjectTerminalState(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Terminals) != 2 || len(got.Splits) != 1 || got.ActiveIndex != 1 {
		t.Fatalf("self-update teardown overwrote saved terminal layout: %#v", got)
	}

	if _, err := updater.Update(workspace, req.ID, "failed", "failed", "", "test"); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("PUT", "/api/state/tasks?scope=terminals", strings.NewReader(`{"session_ids":[],"active_session_id":"","splits":[]}`))
	w = httptest.NewRecorder()
	s.taskState(w, r)
	if w.Code != 200 {
		t.Fatalf("unprotected terminal PUT status=%d body=%s", w.Code, w.Body.String())
	}
	got, err = readProjectTerminalState(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Terminals) != 0 {
		t.Fatalf("terminal writes did not resume after failed update: %#v", got)
	}
}

func TestSelfUpdateUIFreezesTerminalPersistenceBeforeConfirm(t *testing.T) {
	restoreData, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil {
		t.Fatal(err)
	}
	restoreJS := string(restoreData)
	for _, want := range []string{
		"freezeForSelfUpdate",
		"persistenceFrozen=true",
		"if(restoring||persistenceFrozen)return",
		"resumeAfterSelfUpdate",
		"if(restoring||persistenceFrozen)return;",
	} {
		if !strings.Contains(restoreJS, want) {
			t.Fatalf("terminalrestore.js missing %q", want)
		}
	}

	updateData, err := webassets.Files.ReadFile("featuremods/selfupdate.js")
	if err != nil {
		t.Fatal(err)
	}
	updateJS := string(updateData)
	freezeAt := strings.Index(updateJS, "freezeForSelfUpdate")
	confirmAt := strings.Index(updateJS, "postAction('confirm'")
	if freezeAt < 0 || confirmAt < 0 || freezeAt > confirmAt {
		t.Fatalf("self-update must freeze terminal persistence before confirm: freeze=%d confirm=%d", freezeAt, confirmAt)
	}
	if !strings.Contains(updateJS, "resumeAfterSelfUpdate") {
		t.Fatal("self-update must resume terminal persistence after failure/cancel")
	}
}
