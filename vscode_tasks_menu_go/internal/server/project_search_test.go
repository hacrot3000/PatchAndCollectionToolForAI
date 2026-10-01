package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestSearchProjectFileIndexRanksBasenameAndBoundsResults(t *testing.T) {
	paths := []string{
		"docs/app_nfc_reader_notes.md",
		"main-esp32c3/main/app_nfc_reader.c",
		"main-esp32c3/main/app_runtime.c",
		"tools/read_nfc.py",
	}
	for i := 0; i < 100; i++ {
		paths = append(paths, "generated/file"+strconv.Itoa(i)+".txt")
	}
	idx, err := newProjectFileIndex(paths, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	results := searchProjectFileIndex(idx, "app nfc reader", 3)
	if len(results) == 0 {
		t.Fatal("search returned no results")
	}
	if results[0].Path != "main-esp32c3/main/app_nfc_reader.c" {
		t.Fatalf("top result=%#v", results[0])
	}
	if len(results) > 3 {
		t.Fatalf("result count=%d want <=3", len(results))
	}
}

func TestProjectFileSearchAPIUsesBoundedBackendResults(t *testing.T) {
	paths := make([]string, 0, 10000)
	for i := 0; i < 10000; i++ {
		paths = append(paths, "src/pkg"+strconv.Itoa(i%100)+"/feature_file_"+strconv.Itoa(i)+".go")
	}
	idx, err := newProjectFileIndex(paths, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: t.TempDir(), projectIndex: idx}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/files/search?q=feature&limit=500", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Results []projectFileSearchResult `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 50 {
		t.Fatalf("results=%d want capped 50", len(got.Results))
	}
}

func TestCurrentProjectFileIndexLoadsFreshCacheAndRecoversCorruptCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	cached, err := newProjectFileIndex([]string{"cached/only.go"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := saveProjectIndexCache(root, cached); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	idx, err := s.currentProjectFileIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.offsets) != 1 || idx.pathAt(0) != "cached/only.go" {
		t.Fatalf("cache index=%#v", idx)
	}

	cacheFile, err := projectIndexCacheFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real.go"), []byte("package real"), 0o644); err != nil {
		t.Fatal(err)
	}
	s = &Server{Workspace: root}
	idx, err = s.currentProjectFileIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.offsets) != 1 || idx.pathAt(0) != "real.go" {
		t.Fatalf("fallback index=%#v", idx)
	}
}


func TestSearchProjectFileIndexTopKPreservesRankingAndTieBreak(t *testing.T) {
	paths := make([]string, 0, 2000)
	for i := 1999; i >= 0; i-- {
		paths = append(paths, "src/feature_"+strconv.Itoa(i)+".go")
	}
	paths = append(paths, "feature.go", "aaa/feature.go", "zzz/feature.go")
	idx, err := newProjectFileIndex(paths, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	results := searchProjectFileIndex(idx, "feature", 3)
	if len(results) != 3 {
		t.Fatalf("results=%d want 3", len(results))
	}
	for i := 1; i < len(results); i++ {
		prev, current := results[i-1], results[i]
		if prev.Score < current.Score || prev.Score == current.Score && prev.Path > current.Path {
			t.Fatalf("results not sorted best-first: %#v", results)
		}
	}
	if results[0].Path != "feature.go" {
		t.Fatalf("top result=%#v want feature.go", results[0])
	}
}


func TestProjectFileSearchFuzzySubsequenceAndWildcardRanking(t *testing.T) {
	paths := []string{
		"src/abcdef.go",
		"src/abXXdef.go",
		"docs/notes_abcdef.txt",
		"src/other.go",
	}
	idx, err := newProjectFileIndex(paths, time.Now())
	if err != nil { t.Fatal(err) }

	results := searchProjectFileIndex(idx, "abdef", 10)
	if len(results) == 0 || results[0].Path != "src/abcdef.go" {
		t.Fatalf("fuzzy results=%+v, want src/abcdef.go first", results)
	}

	results = searchProjectFileIndex(idx, "ab*def.go", 10)
	if len(results) < 2 {
		t.Fatalf("wildcard results=%+v, want matching files", results)
	}
	if results[0].Path != "src/abcdef.go" {
		t.Fatalf("wildcard top=%+v, want compact basename first", results[0])
	}

	results = searchProjectFileIndex(idx, "ab?def.go", 10)
	if len(results) != 1 || results[0].Path != "src/abcdef.go" {
		t.Fatalf("single-char wildcard results=%+v", results)
	}

	results = searchProjectFileIndex(idx, "ab%def.go", 10)
	if len(results) < 2 {
		t.Fatalf("percent wildcard results=%+v", results)
	}
}

func TestProjectFileSearchRefreshFindsFileMissingFromStaleIndex(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	stale, err := newProjectFileIndex([]string{"old.txt"}, time.Now().Add(-time.Hour))
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "exact_new_file.txt"), []byte("new"), 0o644); err != nil { t.Fatal(err) }

	s := &Server{Workspace: root, projectIndex: stale}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/project/files/search?q=exact_new_file.txt&limit=50&refresh=1", nil)
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("refresh search status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Results []projectFileSearchResult `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if len(got.Results) == 0 || got.Results[0].Path != "exact_new_file.txt" {
		t.Fatalf("refresh results=%+v", got.Results)
	}
}


func TestProjectFileSearchRefreshFindsExactIgnoredFilename(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil { t.Fatal(err) }
	ignoredDir := filepath.Join(root, "ignored")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(ignoredDir, "exact_hidden.txt"), []byte("hidden"), 0o644); err != nil { t.Fatal(err) }

	stale, err := newProjectFileIndex([]string{"visible.txt"}, time.Now().Add(-time.Hour))
	if err != nil { t.Fatal(err) }
	s := &Server{Workspace: root, projectIndex: stale}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/project/files/search?q=exact_hidden.txt&limit=50&refresh=1", nil)
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ignored exact search status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Results []projectFileSearchResult `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if len(got.Results) == 0 || got.Results[0].Path != "ignored/exact_hidden.txt" {
		t.Fatalf("ignored exact results=%+v", got.Results)
	}
	if got.Results[0].Name != "exact_hidden.txt" {
		t.Fatalf("ignored exact top result=%+v, want exact filename", got.Results[0])
	}
}
