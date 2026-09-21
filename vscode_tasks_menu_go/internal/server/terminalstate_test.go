package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/session"
)

func waitTerminalCwd(t *testing.T, m *session.Manager, id, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		cwd, err := m.CurrentCwd(id)
		if err == nil && cwd == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("terminal %s cwd did not become %q; last=%q err=%v", id, want, cwd, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestProjectTerminalStateNormalizeMigratesLegacySplit(t *testing.T) {
	value := projectTerminalState{Version: 99, ActiveIndex: 39, Split: &terminalSplitState{Left: 0, Right: 31, Ratio: 9}}
	for i := 0; i < 40; i++ {
		value.Terminals = append(value.Terminals, terminalStateItem{Cwd: filepath.Join("/tmp", "terminal", string(rune('a'+i%26)))})
	}
	got := normalizeProjectTerminalState(value)
	if got.Version != 3 || len(got.Terminals) != projectTerminalMaxTabs {
		t.Fatalf("normalized terminal state = %#v", got)
	}
	if got.ActiveIndex != 0 {
		t.Fatalf("active index=%d want fallback 0", got.ActiveIndex)
	}
	if len(got.Splits) != 1 || got.Splits[0].Ratio != 0.8 || got.Splits[0].Orientation != "vertical" {
		t.Fatalf("splits=%#v want migrated/clamped legacy split", got.Splits)
	}
	if got.Split != nil {
		t.Fatalf("legacy split field must be cleared after normalization: %#v", got.Split)
	}
}

func TestProjectTerminalStateRejectsOverlappingSplitGroups(t *testing.T) {
	value := projectTerminalState{Terminals: []terminalStateItem{{Cwd: "/a"}, {Cwd: "/b"}, {Cwd: "/c"}}}
	value.Splits = []terminalSplitState{
		{Left: 0, Right: 1, Ratio: 0.5, Orientation: "horizontal"},
		{Left: 1, Right: 2, Ratio: 0.5, Orientation: "vertical"},
	}
	got := normalizeProjectTerminalState(value)
	if len(got.Splits) != 1 || got.Splits[0].Left != 0 || got.Splits[0].Right != 1 || got.Splits[0].Orientation != "horizontal" {
		t.Fatalf("overlapping split groups were not normalized safely: %#v", got.Splits)
	}
}

func TestProjectTerminalStateTracksCwdOrderMultipleSplitsAndRestores(t *testing.T) {
	root := t.TempDir()
	dirs := make([]string, 4)
	for i, name := range []string{"a", "b", "c", "d"} {
		dirs[i] = filepath.Join(root, name)
		if err := os.Mkdir(dirs[i], 0o755); err != nil {
			t.Fatal(err)
		}
	}

	m := session.NewManager(64 << 10)
	s := &Server{Workspace: root, Sessions: m}
	spec, err := workspaceTerminalExecutionAt(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	metas := make([]session.Metadata, 4)
	for i := range metas {
		metas[i], err = m.Start(spec)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Input(metas[i].ID, []byte("cd "+[]string{"a", "b", "c", "d"}[i]+"\n")); err != nil {
			t.Fatal(err)
		}
		waitTerminalCwd(t, m, metas[i].ID, dirs[i])
	}
	defer m.Shutdown(time.Second)

	order := []session.Metadata{metas[1], metas[0], metas[3], metas[2]}
	value, err := s.captureTerminalState(terminalSnapshotRequest{
		SessionIDs:      []string{order[0].ID, order[1].ID, order[2].ID, order[3].ID},
		ActiveSessionID: order[3].ID,
		Splits: []terminalSnapshotSplitRequest{
			{LeftSessionID: order[0].ID, RightSessionID: order[1].ID, Ratio: 0.65, Orientation: "vertical"},
			{LeftSessionID: order[2].ID, RightSessionID: order[3].ID, Ratio: 0.4, Orientation: "horizontal"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCwds := []string{dirs[1], dirs[0], dirs[3], dirs[2]}
	if len(value.Terminals) != len(wantCwds) {
		t.Fatalf("terminal count=%d want %d", len(value.Terminals), len(wantCwds))
	}
	for i, want := range wantCwds {
		if value.Terminals[i].Cwd != want {
			t.Fatalf("terminal[%d].cwd=%q want %q", i, value.Terminals[i].Cwd, want)
		}
		if value.Terminals[i].SessionID != order[i].ID {
			t.Fatalf("terminal[%d].session_id=%q want %q", i, value.Terminals[i].SessionID, order[i].ID)
		}
	}
	if value.ActiveIndex != 3 || len(value.Splits) != 2 {
		t.Fatalf("active/splits not preserved: %#v", value)
	}
	if value.Splits[0].Left != 0 || value.Splits[0].Right != 1 || value.Splits[0].Ratio != 0.65 || value.Splits[0].Orientation != "vertical" {
		t.Fatalf("first split=%#v", value.Splits[0])
	}
	if value.Splits[1].Left != 2 || value.Splits[1].Right != 3 || value.Splits[1].Ratio != 0.4 || value.Splits[1].Orientation != "horizontal" {
		t.Fatalf("second split=%#v", value.Splits[1])
	}
	if err := writeProjectTerminalState(root, value); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(projectTerminalStatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode=%o want 600", info.Mode().Perm())
	}

	m2 := session.NewManager(64 << 10)
	s2 := &Server{Workspace: root, Sessions: m2}
	resp, err := s2.restoreTerminalState()
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Shutdown(time.Second)
	if len(resp.Sessions) != 4 || resp.ActiveIndex != 3 || len(resp.Splits) != 2 {
		t.Fatalf("restore response=%#v", resp)
	}
	if resp.Splits[0].Orientation != "vertical" || resp.Splits[1].Orientation != "horizontal" {
		t.Fatalf("restore orientations=%#v", resp.Splits)
	}
	for i, want := range wantCwds {
		waitTerminalCwd(t, m2, resp.Sessions[i].ID, want)
	}

	persisted, err := readProjectTerminalState(root)
	if err != nil {
		t.Fatal(err)
	}
	for i, meta := range resp.Sessions {
		if persisted.Terminals[i].SessionID != meta.ID {
			t.Fatalf("restored terminal[%d] session_id=%q want new id %q", i, persisted.Terminals[i].SessionID, meta.ID)
		}
	}

	req := httptest.NewRequest("GET", "/api/state/tasks?scope=terminals", nil)
	rr := httptest.NewRecorder()
	s2.taskState(rr, req)
	if rr.Code != 200 {
		t.Fatalf("terminal state GET status=%d body=%s", rr.Code, rr.Body.String())
	}
}


func TestTerminalLayoutProfilesKeepDesktopSplitWhenMobileSaves(t *testing.T) {
	root := t.TempDir()
	desktop := projectTerminalState{
		Version: 3,
		Terminals: []terminalStateItem{
			{SessionID: "desktop-a", Cwd: root},
			{SessionID: "desktop-b", Cwd: root},
		},
		ActiveIndex: 1,
		Splits: []terminalSplitState{{Left: 0, Right: 1, Ratio: 0.6, Orientation: "vertical"}},
	}
	if err := writeProjectTerminalStateProfile(root, "desktop", desktop); err != nil {
		t.Fatal(err)
	}
	mobile := projectTerminalState{
		Version: 3,
		Terminals: []terminalStateItem{
			{SessionID: "mobile-b", Cwd: root},
			{SessionID: "mobile-a", Cwd: root},
		},
		ActiveIndex: 0,
		Splits: []terminalSplitState{{Left: 0, Right: 1, Ratio: 0.3, Orientation: "horizontal"}},
	}
	if err := writeProjectTerminalStateProfile(root, "mobile", mobile); err != nil {
		t.Fatal(err)
	}

	gotDesktop, err := readProjectTerminalStateProfile(root, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if gotDesktop.ActiveIndex != 1 || len(gotDesktop.Splits) != 1 {
		t.Fatalf("desktop layout was changed by mobile save: %#v", gotDesktop)
	}
	if gotDesktop.Splits[0].Ratio != 0.6 || gotDesktop.Splits[0].Orientation != "vertical" {
		t.Fatalf("desktop split was changed by mobile save: %#v", gotDesktop.Splits)
	}

	gotMobile, err := readProjectTerminalStateProfile(root, "mobile")
	if err != nil {
		t.Fatal(err)
	}
	if gotMobile.ActiveIndex != 0 || len(gotMobile.Splits) != 0 {
		t.Fatalf("mobile layout must never persist split state: %#v", gotMobile)
	}
	if projectTerminalStatePathForProfile(root, "desktop") == projectTerminalStatePathForProfile(root, "mobile") {
		t.Fatal("desktop and mobile terminal state paths must be different")
	}
}
