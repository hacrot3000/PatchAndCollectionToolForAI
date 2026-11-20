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

func projectReplaceRequestForTest(t *testing.T, h http.Handler, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/project/content/replace", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestProjectReplacePreviewApplyUndoRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "a.txt"), []byte("foo one\nfoo two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "b.txt"), []byte("keep foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	rr := projectReplaceRequestForTest(t, h, map[string]any{
		"action": "preview", "query": "foo", "replacement": "bar",
		"case_sensitive": true, "mode": "all",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rr.Code, rr.Body.String())
	}
	var preview struct {
		Token string `json:"token"`
		Total int    `json:"total_replacements"`
		Files []projectReplacePreviewFile `json:"files"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Token) != 32 || preview.Total != 3 || len(preview.Files) != 2 {
		t.Fatalf("preview=%+v", preview)
	}
	if !strings.Contains(preview.Files[0].Diff, "--- ") || !strings.Contains(preview.Files[0].Diff, "+++") {
		t.Fatalf("missing preview diff: %+v", preview.Files[0])
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "a.txt")); string(data) != "foo one\nfoo two\n" {
		t.Fatalf("preview modified file: %q", data)
	}

	rr = projectReplaceRequestForTest(t, h, map[string]any{"action": "apply", "token": preview.Token})
	if rr.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "a.txt")); string(data) != "bar one\nbar two\n" {
		t.Fatalf("applied a.txt=%q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "b.txt")); string(data) != "keep bar\n" {
		t.Fatalf("applied b.txt=%q", data)
	}

	rr = projectReplaceRequestForTest(t, h, map[string]any{"action": "undo", "token": preview.Token})
	if rr.Code != http.StatusOK {
		t.Fatalf("undo status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "a.txt")); string(data) != "foo one\nfoo two\n" {
		t.Fatalf("undone a.txt=%q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "b.txt")); string(data) != "keep foo\n" {
		t.Fatalf("undone b.txt=%q", data)
	}
}

func TestProjectReplaceOneMatchUsesLineAndUTF16Column(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "a.txt"), []byte("😀 foo foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	// 😀 occupies two UTF-16 units. First foo starts at column 4; second at 8.
	rr := projectReplaceRequestForTest(t, h, map[string]any{
		"action": "preview", "query": "foo", "replacement": "BAR",
		"case_sensitive": true, "mode": "match", "target_path": "a.txt",
		"line": 1, "column": 8,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rr.Code, rr.Body.String())
	}
	var preview struct {
		Token string `json:"token"`
		Total int    `json:"total_replacements"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Total != 1 || preview.Token == "" {
		t.Fatalf("preview=%+v", preview)
	}
	rr = projectReplaceRequestForTest(t, h, map[string]any{"action": "apply", "token": preview.Token})
	if rr.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "a.txt")); string(data) != "😀 foo BAR\n" {
		t.Fatalf("match replacement=%q", data)
	}
}

func TestProjectReplaceRegexCaptureAndFileScope(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "a.txt"), []byte("ID-12 ID-34\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "b.txt"), []byte("ID-56\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	rr := projectReplaceRequestForTest(t, h, map[string]any{
		"action": "preview", "query": `ID-([0-9]+)`, "replacement": "item-$1",
		"regex": true, "case_sensitive": true, "mode": "file", "target_path": "a.txt",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rr.Code, rr.Body.String())
	}
	var preview struct {
		Token string `json:"token"`
		Total int    `json:"total_replacements"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Total != 2 {
		t.Fatalf("preview total=%d", preview.Total)
	}
	rr = projectReplaceRequestForTest(t, h, map[string]any{"action": "apply", "token": preview.Token})
	if rr.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "a.txt")); string(data) != "item-12 item-34\n" {
		t.Fatalf("regex replacement=%q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(workspace, "b.txt")); string(data) != "ID-56\n" {
		t.Fatalf("file scope changed b.txt=%q", data)
	}
}

func TestProjectReplaceApplyRejectsStalePreview(t *testing.T) {
	workspace := t.TempDir()
	pathValue := filepath.Join(workspace, "a.txt")
	if err := os.WriteFile(pathValue, []byte("foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: workspace}
	h := s.Handler()

	rr := projectReplaceRequestForTest(t, h, map[string]any{
		"action": "preview", "query": "foo", "replacement": "bar",
		"case_sensitive": true, "mode": "all",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rr.Code, rr.Body.String())
	}
	var preview struct{ Token string `json:"token"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathValue, []byte("external change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr = projectReplaceRequestForTest(t, h, map[string]any{"action": "apply", "token": preview.Token})
	if rr.Code != http.StatusConflict {
		t.Fatalf("stale apply status=%d body=%s", rr.Code, rr.Body.String())
	}
	if data, _ := os.ReadFile(pathValue); string(data) != "external change\n" {
		t.Fatalf("stale apply overwrote file=%q", data)
	}
}
