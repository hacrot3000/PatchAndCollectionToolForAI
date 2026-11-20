package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectTaskStatePersistsAndLimitsHistory(t *testing.T) {
	workspace := t.TempDir()
	state := defaultProjectTaskState()
	state.Favorites = []int{3, 3, 2, 1}
	state.Recent = []int{9, 8, 9, 7}
	for i := 0; i < 14; i++ {
		code := i
		duration := int64(i + 1)
		state.History = append(state.History, taskHistoryItem{
			SessionID: "session-" + string(rune('a'+i)),
			TaskID:    i + 1,
			Label:     "task",
			Status:    "PASS",
			ExitCode:  &code,
			Duration:  &duration,
		})
	}
	if err := writeProjectTaskState(workspace, state); err != nil {
		t.Fatal(err)
	}
	path := projectTaskStatePath(workspace)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("state permission = %o, want 600", got)
	}
	got, err := readProjectTaskState(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.History) != 10 {
		t.Fatalf("history len = %d, want 10", len(got.History))
	}
	if len(got.Favorites) != 3 || got.Favorites[0] != 3 || got.Favorites[1] != 2 || got.Favorites[2] != 1 {
		t.Fatalf("favorites not normalized: %#v", got.Favorites)
	}
	if len(got.Recent) != 3 || got.Recent[0] != 9 || got.Recent[1] != 8 || got.Recent[2] != 7 {
		t.Fatalf("recent not normalized: %#v", got.Recent)
	}
}

func TestProjectTaskStateAPI(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	body := []byte(`{"version":1,"favorites":[5,4],"recent":[7],"history":[{"session_id":"abc","task_id":5,"label":"Build","status":"PASS","exit_code":0,"started_at":"2026-09-17T10:00:00Z","ended_at":"2026-09-17T10:00:02Z","duration":2}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/state/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.taskState(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/state/tasks", nil)
	rr = httptest.NewRecorder()
	s.taskState(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", rr.Code, rr.Body.String())
	}
	var got projectTaskState
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Favorites) != 2 || got.Favorites[0] != 5 || len(got.History) != 1 || got.History[0].Label != "Build" {
		t.Fatalf("unexpected state: %#v", got)
	}
}


func TestProjectTaskStateReadMigratesLegacyRootFile(t *testing.T) {
	workspace := t.TempDir()
	legacy := filepath.Join(workspace, projectTaskStateFile)
	if err := os.WriteFile(legacy, []byte("{\"version\":1,\"favorites\":[7],\"recent\":[],\"history\":[]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readProjectTaskState(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Favorites) != 1 || got.Favorites[0] != 7 {
		t.Fatalf("migrated state=%#v", got)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy state still exists: %v", err)
	}
	if _, err := os.Stat(projectTaskStatePath(workspace)); err != nil {
		t.Fatalf("migrated state missing: %v", err)
	}
}
