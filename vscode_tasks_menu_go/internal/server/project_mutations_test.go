package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func projectMutationRequestForTest(t *testing.T, h http.Handler, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/project/mutate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestProjectMutationsCreateRenameAndMove(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	h := s.Handler()

	if rr := projectMutationRequestForTest(t, h, map[string]any{"action": "mkdir", "path": "docs"}); rr.Code != http.StatusCreated {
		t.Fatalf("mkdir status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := projectMutationRequestForTest(t, h, map[string]any{"action": "create_file", "path": "notes.txt"}); rr.Code != http.StatusCreated {
		t.Fatalf("create file status=%d body=%s", rr.Code, rr.Body.String())
	}
	if info, err := os.Stat(filepath.Join(workspace, "notes.txt")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("created file info=%v err=%v", info, err)
	}

	if rr := projectMutationRequestForTest(t, h, map[string]any{"action": "rename", "path": "notes.txt", "new_path": "docs/renamed.txt"}); rr.Code != http.StatusOK {
		t.Fatalf("rename/move status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("old file still exists: %v", err)
	}
	if info, err := os.Stat(filepath.Join(workspace, "docs", "renamed.txt")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("moved file info=%v err=%v", info, err)
	}

	if rr := projectMutationRequestForTest(t, h, map[string]any{"action": "rename", "path": "docs", "new_path": "archive"}); rr.Code != http.StatusOK {
		t.Fatalf("rename folder status=%d body=%s", rr.Code, rr.Body.String())
	}
	if info, err := os.Stat(filepath.Join(workspace, "archive")); err != nil || !info.IsDir() {
		t.Fatalf("renamed folder info=%v err=%v", info, err)
	}
}

func TestProjectMutationsRejectTraversalAndExistingDestination(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	for _, payload := range []map[string]any{
		{"action": "create_file", "path": "../escape.txt"},
		{"action": "mkdir", "path": "/tmp/escape"},
		{"action": "rename", "path": "a.txt", "new_path": "../escape.txt"},
		{"action": "rename", "path": "a.txt", "new_path": "b.txt"},
	} {
		if rr := projectMutationRequestForTest(t, h, payload); rr.Code < 400 {
			t.Fatalf("unsafe mutation unexpectedly succeeded payload=%v status=%d body=%s", payload, rr.Code, rr.Body.String())
		}
	}

	data, err := os.ReadFile(filepath.Join(workspace, "a.txt"))
	if err != nil || string(data) != "a" {
		t.Fatalf("source changed after rejected mutations data=%q err=%v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(workspace, "b.txt"))
	if err != nil || string(data) != "b" {
		t.Fatalf("destination changed after rejected mutations data=%q err=%v", data, err)
	}
}

func TestProjectMutationsRejectSymlinkSourceAndParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), filepath.Join(workspace, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "outside-dir")); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	if rr := projectMutationRequestForTest(t, h, map[string]any{"action": "rename", "path": "link.txt", "new_path": "renamed.txt"}); rr.Code < 400 {
		t.Fatalf("symlink source unexpectedly renamed: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := projectMutationRequestForTest(t, h, map[string]any{"action": "create_file", "path": "outside-dir/new.txt"}); rr.Code < 400 {
		t.Fatalf("symlink destination parent unexpectedly accepted: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(outside, "outside.txt")); err != nil || string(data) != "outside" {
		t.Fatalf("outside file changed data=%q err=%v", data, err)
	}
}
