package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitConflictFileReturnsThreeStagesAndWorkingResult(t *testing.T) {
	workspace, s, _, feature := setupMergeConflictRepo(t)
	startMergeConflict(t, workspace, feature)
	h := s.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/git/conflict-file?repo=.&path=tracked.txt", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Operation   string              `json:"operation"`
		Path        string              `json:"path"`
		ProjectPath string              `json:"project_path"`
		Base        gitConflictTextSide `json:"base"`
		Current     gitConflictTextSide `json:"current"`
		Incoming    gitConflictTextSide `json:"incoming"`
		Result      gitConflictTextSide `json:"result"`
		Conflicts   []gitConflictFile   `json:"conflicts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Operation != "merge" || got.Path != "tracked.txt" || got.ProjectPath != "tracked.txt" || len(got.Conflicts) != 1 {
		t.Fatalf("response=%+v", got)
	}
	if !got.Base.Exists || got.Base.Content != "one\n" {
		t.Fatalf("base=%+v", got.Base)
	}
	if !got.Current.Exists || got.Current.Content != "main side\n" {
		t.Fatalf("current=%+v", got.Current)
	}
	if !got.Incoming.Exists || got.Incoming.Content != "feature side\n" {
		t.Fatalf("incoming=%+v", got.Incoming)
	}
	if !got.Result.Exists || got.Result.SHA256 == "" {
		t.Fatalf("result=%+v", got.Result)
	}
	for _, marker := range []string{"<<<<<<<", "=======", ">>>>>>>"} {
		if !strings.Contains(got.Result.Content, marker) {
			t.Fatalf("working result missing %q: %q", marker, got.Result.Content)
		}
	}
}

func TestGitConflictFileWriteUsesCASAndDoesNotStageAutomatically(t *testing.T) {
	workspace, s, _, feature := setupMergeConflictRepo(t)
	startMergeConflict(t, workspace, feature)
	h := s.Handler()

	readRR := httptest.NewRecorder()
	h.ServeHTTP(readRR, httptest.NewRequest(http.MethodGet, "/api/git/conflict-file?repo=.&path=tracked.txt", nil))
	if readRR.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", readRR.Code, readRR.Body.String())
	}
	var loaded struct {
		Result gitConflictTextSide `json:"result"`
	}
	if err := json.Unmarshal(readRR.Body.Bytes(), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Result.SHA256 == "" {
		t.Fatal("missing result SHA")
	}

	payload, err := json.Marshal(map[string]any{
		"path": "tracked.txt",
		"content": "manual result\n",
		"expected_sha256": loaded.Result.SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeReq := httptest.NewRequest(http.MethodPut, "/api/git/conflict-file?repo=.", bytes.NewReader(payload))
	writeReq.Header.Set("Content-Type", "application/json")
	writeRR := httptest.NewRecorder()
	h.ServeHTTP(writeRR, writeReq)
	if writeRR.Code != http.StatusOK {
		t.Fatalf("write status=%d body=%s", writeRR.Code, writeRR.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt")); err != nil || string(data) != "manual result\n" {
		t.Fatalf("working result=%q err=%v", data, err)
	}
	state, err := s.gitConflictState(withGitRepository(t.Context(), gitRepository{ID: ".", Name: filepath.Base(workspace), Path: ".", Root: workspace}))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Files) != 1 {
		t.Fatalf("manual save unexpectedly staged conflict: %+v", state)
	}

	staleReq := httptest.NewRequest(http.MethodPut, "/api/git/conflict-file?repo=.", bytes.NewReader(payload))
	staleReq.Header.Set("Content-Type", "application/json")
	staleRR := httptest.NewRecorder()
	h.ServeHTTP(staleRR, staleReq)
	if staleRR.Code != http.StatusConflict {
		t.Fatalf("stale status=%d body=%s", staleRR.Code, staleRR.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "tracked.txt")); err != nil || string(data) != "manual result\n" {
		t.Fatalf("stale write changed result=%q err=%v", data, err)
	}
}
