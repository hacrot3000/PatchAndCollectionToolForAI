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
