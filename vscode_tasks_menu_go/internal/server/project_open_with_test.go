package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectOpenWithPreviewAndDownload(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "note.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "README.md"), []byte("# Title\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 24)...)
	if err := os.WriteFile(filepath.Join(root, "docs", "image.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}

	cases := []struct {
		path string
		kind string
	}{
		{"docs/note.txt", "text"},
		{"docs/README.md", "markdown"},
		{"docs/image.png", "image"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/project/preview?path="+tc.path, nil)
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("preview %s status=%d body=%s", tc.path, rr.Code, rr.Body.String())
		}
		var got filePreviewResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Kind != tc.kind || got.ProjectPath != tc.path {
			t.Fatalf("preview %s=%+v", tc.path, got)
		}
		if tc.kind == "markdown" && !strings.Contains(got.Content, "# Title") {
			t.Fatalf("markdown preview missing content: %+v", got)
		}
		if tc.kind == "image" && !strings.Contains(got.URL, "/api/project/preview?mode=content") {
			t.Fatalf("image preview missing project content URL: %+v", got)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/project/download?path=docs/note.txt", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "hello\n" {
		t.Fatalf("download body=%q", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Disposition"); !strings.Contains(got, "note.txt") {
		t.Fatalf("content disposition=%q", got)
	}
}

func TestProjectOpenWithRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	s := &Server{Workspace: root}
	for _, endpoint := range []string{
		"/api/project/preview?path=../secret.txt",
		"/api/project/download?path=../secret.txt",
	} {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code == http.StatusOK {
			t.Fatalf("traversal unexpectedly accepted by %s", endpoint)
		}
	}
}
