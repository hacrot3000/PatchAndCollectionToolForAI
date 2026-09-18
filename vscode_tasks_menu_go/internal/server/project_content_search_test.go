package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectContentSearchFallbackBoundsAndSkipsBinaryAndSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.txt"), []byte("needle one\nneedle two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "binary.bin"), []byte{'n', 'e', 0, 'e', 'd', 'l', 'e'}, 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("needle outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(outside, filepath.Join(root, "src", "outside-link.txt"))

	results, err := searchProjectContentFallback(context.Background(), root, "needle", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%d want 1: %#v", len(results), results)
	}
	if results[0].Path != "src/a.txt" || results[0].Line != 1 || results[0].Column != 1 {
		t.Fatalf("unexpected first result: %#v", results[0])
	}
}

func TestProjectContentSearchAPIEmptyAndBounded(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hit\nhit\nhit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/content/search?q=", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("empty query status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"results":[]`) {
		t.Fatalf("empty query body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/content/search?q=hit&limit=2", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Results []projectContentSearchResult `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) > 2 {
		t.Fatalf("results=%d want <=2", len(got.Results))
	}
}

func TestProjectContentSearchFallbackHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(strings.Repeat("data\n", 100)), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := searchProjectContentFallback(ctx, root, "data", 10)
	if err == nil || err != context.Canceled {
		t.Fatalf("err=%v want context.Canceled", err)
	}
}


func TestProjectContentSearchFallbackHonorsRootGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored", "secret.txt"), []byte("needle ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scratch.tmp"), []byte("needle temp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("needle keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := searchProjectContentFallback(context.Background(), root, "needle", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "keep.txt" {
		t.Fatalf("ignore filtering results=%#v", results)
	}
}
