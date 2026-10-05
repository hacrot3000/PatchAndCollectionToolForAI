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

func TestProjectMutationsCopyFilesAndDirectories(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "src", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "a.txt"), []byte("alpha"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "nested", "b.txt"), []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	if rr := projectMutationRequestForTest(t, h, map[string]any{"action": "copy", "path": "src/a.txt", "new_path": "copied.txt"}); rr.Code != http.StatusCreated {
		t.Fatalf("copy file status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "copied.txt")); err != nil || string(data) != "alpha" {
		t.Fatalf("copied file data=%q err=%v", data, err)
	}

	if rr := projectMutationRequestForTest(t, h, map[string]any{"action": "copy", "path": "src", "new_path": "src-copy"}); rr.Code != http.StatusCreated {
		t.Fatalf("copy directory status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "src-copy", "nested", "b.txt")); err != nil || string(data) != "beta" {
		t.Fatalf("copied directory nested data=%q err=%v", data, err)
	}
}

func TestProjectMutationCopyRejectsNestedSymlinkAndCleansDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "src", "link.txt")); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	rr := projectMutationRequestForTest(t, h, map[string]any{"action": "copy", "path": "src", "new_path": "copy"})
	if rr.Code < 400 {
		t.Fatalf("copy with nested symlink unexpectedly succeeded status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "copy")); !os.IsNotExist(err) {
		t.Fatalf("failed copy destination was not cleaned: %v", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside" {
		t.Fatalf("outside file changed data=%q err=%v", data, err)
	}
}

func TestProjectMutationTrashAndRestore(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "docs", "note.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	rr := projectMutationRequestForTest(t, h, map[string]any{"action": "trash", "path": "docs"})
	if rr.Code != http.StatusOK {
		t.Fatalf("trash status=%d body=%s", rr.Code, rr.Body.String())
	}
	var trashed struct {
		Path  string `json:"path"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &trashed); err != nil {
		t.Fatal(err)
	}
	if trashed.Path != "docs" || len(trashed.Token) != 32 {
		t.Fatalf("trash response=%+v", trashed)
	}
	if _, err := os.Stat(filepath.Join(workspace, "docs")); !os.IsNotExist(err) {
		t.Fatalf("trashed source still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".taskdeck-trash")); !os.IsNotExist(err) {
		t.Fatalf("trash must not be stored inside workspace: %v", err)
	}

	rr = projectMutationRequestForTest(t, h, map[string]any{"action": "restore", "path": "docs", "token": trashed.Token})
	if rr.Code != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "docs", "note.txt")); err != nil || string(data) != "keep me" {
		t.Fatalf("restored data=%q err=%v", data, err)
	}

	rr = projectMutationRequestForTest(t, h, map[string]any{"action": "restore", "path": "docs2", "token": trashed.Token})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("reused trash token status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestProjectMutationRestoreRejectsExistingDestination(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	rr := projectMutationRequestForTest(t, h, map[string]any{"action": "trash", "path": "note.txt"})
	if rr.Code != http.StatusOK {
		t.Fatalf("trash status=%d body=%s", rr.Code, rr.Body.String())
	}
	var trashed struct{ Token string `json:"token"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &trashed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr = projectMutationRequestForTest(t, h, map[string]any{"action": "restore", "path": "note.txt", "token": trashed.Token})
	if rr.Code != http.StatusConflict {
		t.Fatalf("restore conflict status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(workspace, "note.txt")); err != nil || string(data) != "replacement" {
		t.Fatalf("existing destination changed data=%q err=%v", data, err)
	}
}
