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


func TestProjectContentSearchUsesUTF16ColumnsAndBoundsPreview(t *testing.T) {
	line := "á😀prefix needle " + strings.Repeat("x", projectContentSearchMaxPreviewBytes*2)
	byteOffset := strings.Index(line, "needle")
	if byteOffset < 0 {
		t.Fatal("needle missing")
	}
	column := projectUTF16Column(line, byteOffset)
	// "á"=1 UTF-16 unit, "😀"=2, then "prefix " is 7.
	if column != 11 {
		t.Fatalf("column=%d want 11", column)
	}
	preview := boundedProjectSearchPreview(line, byteOffset)
	if len(preview) > projectContentSearchMaxPreviewBytes+len("……")*3 {
		t.Fatalf("preview too large: %d bytes", len(preview))
	}
	if !strings.Contains(preview, "needle") {
		t.Fatalf("preview lost match: %q", preview)
	}
}

func TestProjectContentSearchFallbackReportsUnicodeColumn(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "unicode.txt"), []byte("á😀 needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := searchProjectContentFallback(context.Background(), root, "needle", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%#v", results)
	}
	if results[0].Column != 5 {
		t.Fatalf("column=%d want 5", results[0].Column)
	}
}

func TestProjectContentSearchFallbackOptions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]string{
		"src/a.go":          "Alpha alpha alphabet\nID-123 id-456\n",
		"src/nested/b.txt":  "alpha\nID-999\n",
		"src/nested/skip.go":"alpha\n",
		"outside.go":        "alpha\n",
	}
	for rel, body := range fixtures {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	results, err := searchProjectContentFallbackWithOptions(context.Background(), root, projectContentSearchOptions{
		Query: "alpha", CaseSensitive: false, WholeWord: true,
		Include: []string{"*.go"}, Exclude: []string{"**/skip.go"}, Scope: "src",
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("filtered results=%#v, want 2 matches from src/a.go", results)
	}
	for _, result := range results {
		if result.Path != "src/a.go" {
			t.Fatalf("unexpected path outside filters: %#v", result)
		}
	}
}

func TestProjectContentSearchFallbackRegexAndCaseSensitivity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("ID-123 id-456 ID-999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := searchProjectContentFallbackWithOptions(context.Background(), root, projectContentSearchOptions{
		Query: `ID-[0-9]{3}`, Regex: true, CaseSensitive: true,
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("case-sensitive regex results=%#v", results)
	}
	results, err = searchProjectContentFallbackWithOptions(context.Background(), root, projectContentSearchOptions{
		Query: `ID-[0-9]{3}`, Regex: true, CaseSensitive: false,
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("case-insensitive regex results=%#v", results)
	}
}

func TestProjectContentSearchAPIRejectsInvalidScopeAndRegex(t *testing.T) {
	root := t.TempDir()
	s := &Server{Workspace: root}

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/content/search?q=x&path=../outside", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unsafe scope status=%d body=%s", rr.Code, rr.Body.String())
	}

	_, err := searchProjectContentWithOptions(context.Background(), root, projectContentSearchOptions{
		Query: "[", Regex: true,
	}, 10)
	if err == nil {
		t.Fatal("invalid regex unexpectedly accepted")
	}

	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/content/search?q=%5B&regex=1", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid regex API status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestProjectContentSearchAcrossAttachedRoots(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	if err := os.MkdirAll(primary, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(attached, "src"), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(primary, "server.txt"), []byte("needle primary\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(attached, "src", "client.txt"), []byte("needle attached\n"), 0o644); err != nil { t.Fatal(err) }
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client","name":"Client"}`)
	if create.Code != http.StatusCreated { t.Fatalf("attach status=%d body=%s", create.Code, create.Body.String()) }
	var root workspaceRootView
	if err := json.Unmarshal(create.Body.Bytes(), &root); err != nil { t.Fatal(err) }

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/content/search?q=needle&case=1", nil))
	if rr.Code != http.StatusOK { t.Fatalf("search status=%d body=%s", rr.Code, rr.Body.String()) }
	var payload struct{ Results []projectContentSearchResult `json:"results"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil { t.Fatal(err) }
	if len(payload.Results) != 2 { t.Fatalf("results=%+v", payload.Results) }
	paths := map[string]bool{}
	for _, row := range payload.Results { paths[row.Path] = true }
	if !paths["server.txt"] || !paths[workspaceVirtualPath(root.ID, "src/client.txt")] {
		t.Fatalf("paths=%+v", paths)
	}
}

func TestProjectContentSearchAttachedRootScope(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	if err := os.MkdirAll(primary, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(attached, "src"), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(primary, "primary.txt"), []byte("needle\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(attached, "src", "client.txt"), []byte("needle\n"), 0o644); err != nil { t.Fatal(err) }
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client"}`)
	if create.Code != http.StatusCreated { t.Fatal(create.Body.String()) }
	var root workspaceRootView
	if err := json.Unmarshal(create.Body.Bytes(), &root); err != nil { t.Fatal(err) }
	scope := workspaceVirtualPath(root.ID, "src")

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/content/search?q=needle&case=1&path="+urlQueryEscape(scope), nil))
	if rr.Code != http.StatusOK { t.Fatalf("search status=%d body=%s", rr.Code, rr.Body.String()) }
	var payload struct{ Results []projectContentSearchResult `json:"results"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil { t.Fatal(err) }
	if len(payload.Results) != 1 || payload.Results[0].Path != workspaceVirtualPath(root.ID, "src/client.txt") {
		t.Fatalf("results=%+v", payload.Results)
	}
}
