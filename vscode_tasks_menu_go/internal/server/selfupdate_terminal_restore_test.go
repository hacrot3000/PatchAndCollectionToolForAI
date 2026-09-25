package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	updater "bletonfc/vscode_tasks_menu/internal/selfupdate"
	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSelfUpdateProtectsTerminalStateOnlyDuringActivation(t *testing.T) {
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
	putEmpty := func() projectTerminalState {
		r := httptest.NewRequest("PUT", "/api/state/tasks?scope=terminals", strings.NewReader(`{"session_ids":[],"active_session_id":"","splits":[]}`))
		w := httptest.NewRecorder()
		s.taskState(w, r)
		if w.Code != 200 {
			t.Fatalf("terminal PUT status=%d body=%s", w.Code, w.Body.String())
		}
		got, err := readProjectTerminalState(workspace)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// Confirmed/downloading/testing/building are dry-run phases. Normal browser
	// persistence must remain writable so a failed candidate never freezes work.
	if got := putEmpty(); len(got.Terminals) != 0 {
		t.Fatalf("dry-run unexpectedly protected terminal writes: %#v", got)
	}

	if err := writeProjectTerminalState(workspace, original); err != nil {
		t.Fatal(err)
	}
	if _, err := updater.Update(workspace, req.ID, "ready_restart", "ready", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := putEmpty(); len(got.Terminals) != 2 || len(got.Splits) != 1 || got.ActiveIndex != 1 {
		t.Fatalf("activation teardown overwrote saved terminal layout: %#v", got)
	}

	if _, err := updater.Update(workspace, req.ID, "failed", "failed", "", "test"); err != nil {
		t.Fatal(err)
	}
	if got := putEmpty(); len(got.Terminals) != 0 {
		t.Fatalf("terminal writes did not resume after failed update: %#v", got)
	}
}

func TestSelfUpdateUIDoesNotFreezeTerminalPersistenceDuringDryRun(t *testing.T) {
	restoreData, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil {
		t.Fatal(err)
	}
	restoreJS := string(restoreData)
	for _, want := range []string{
		"freezeForSelfUpdate",
		"persistenceFrozen=true",
		"resumeAfterSelfUpdate",
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
	if strings.Contains(updateJS, ".freezeForSelfUpdate(") {
		t.Fatal("self-update UI must not freeze terminal persistence during candidate validation")
	}
	if !strings.Contains(updateJS, "persistSnapshot") {
		t.Fatal("self-update UI must persist a recovery snapshot before starting")
	}
	if !strings.Contains(updateJS, "resumeAfterSelfUpdate") {
		t.Fatal("self-update UI must remain compatible with older pages that may already be frozen")
	}
}

func TestSelfUpdateGuardProtectsRequestedMobileProfileWithoutTouchingDesktop(t *testing.T) {
	workspace := t.TempDir()
	desktop := projectTerminalState{
		Version: 3,
		Terminals: []terminalStateItem{{SessionID: "desktop-a", Cwd: workspace}, {SessionID: "desktop-b", Cwd: workspace}},
		ActiveIndex: 1,
		Splits: []terminalSplitState{{Left: 0, Right: 1, Ratio: 0.6, Orientation: "vertical"}},
	}
	mobile := projectTerminalState{
		Version: 3,
		Terminals: []terminalStateItem{{SessionID: "mobile-a", Cwd: workspace}},
		ActiveIndex: 0,
	}
	if err := writeProjectTerminalStateProfile(workspace, "desktop", desktop); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectTerminalStateProfile(workspace, "mobile", mobile); err != nil {
		t.Fatal(err)
	}
	if _, err := updater.CreateRequest(workspace, "0123456789abcdef", "http://127.0.0.1:1234", true); err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: workspace}
	r := httptest.NewRequest("PUT", "/api/state/tasks?scope=terminals&profile=mobile", strings.NewReader(`{"session_ids":[],"active_session_id":"","splits":[]}`))
	w := httptest.NewRecorder()
	s.taskState(w, r)
	if w.Code != 200 {
		t.Fatalf("protected mobile terminal PUT status=%d body=%s", w.Code, w.Body.String())
	}

	gotMobile, err := readProjectTerminalStateProfile(workspace, "mobile")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotMobile.Terminals) != 1 || gotMobile.Terminals[0].SessionID != "mobile-a" || len(gotMobile.Splits) != 0 {
		t.Fatalf("mobile recovery snapshot was overwritten: %#v", gotMobile)
	}
	gotDesktop, err := readProjectTerminalStateProfile(workspace, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotDesktop.Terminals) != 2 || len(gotDesktop.Splits) != 1 || gotDesktop.ActiveIndex != 1 {
		t.Fatalf("desktop layout changed while protecting mobile snapshot: %#v", gotDesktop)
	}
	if strings.Contains(w.Body.String(), "desktop-a") || strings.Contains(w.Body.String(), `"splits":[`) {
		t.Fatalf("mobile protected response leaked desktop layout: %s", w.Body.String())
	}
}
