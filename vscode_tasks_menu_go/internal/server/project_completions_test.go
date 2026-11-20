package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCompletionFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil { t.Fatal(err) }
}

func postProjectCompletion(t *testing.T, s *Server, req projectCompletionRequest) projectCompletionResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil { t.Fatal(err) }
	httpReq := httptest.NewRequest(http.MethodPost, "/api/project/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httpReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("completion status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got projectCompletionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	return got
}

func completionItemByLabel(items []projectCompletionItem, label string) (projectCompletionItem, bool) {
	for _, item := range items {
		if item.Label == label { return item, true }
	}
	return projectCompletionItem{}, false
}

func TestProjectCompletionMergesIndexedAndUnsavedSymbols(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	root := t.TempDir()
	writeCompletionFixture(t, root, "internal/user/service.go", "package user\ntype UserService struct {}\nfunc NewUserService() *UserService { return nil }\n")
	writeCompletionFixture(t, root, "src/current.go", "package main\nfunc main() {}\n")
	s := &Server{Workspace: root}

	got := postProjectCompletion(t, s, projectCompletionRequest{
		Path: "src/current.go", Language: "go", Prefix: "User", LexicalMode: "code",
		Text: "package main\ntype UserLocal struct {}\nfunc main() { User }\n",
		LinePrefix: "func main() { User",
		Position: lspPosition{Line: 2, Character: 18},
		Limit: 40,
	})
	project, ok := completionItemByLabel(got.Items, "UserService")
	if !ok || project.Source != "project" {
		t.Fatalf("project symbol missing: %+v", got.Items)
	}
	local, ok := completionItemByLabel(got.Items, "UserLocal")
	if !ok || local.Source != "current-file" {
		t.Fatalf("unsaved current-file symbol missing: %+v", got.Items)
	}
	if local.Score <= project.Score {
		t.Fatalf("current-file completion should outrank project completion: local=%+v project=%+v", local, project)
	}
	if got.IndexedFiles == 0 || got.IndexedBytes == 0 {
		t.Fatalf("index accounting missing: %+v", got)
	}
}

func TestProjectCompletionSuggestsIncludePaths(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	root := t.TempDir()
	writeCompletionFixture(t, root, "include/utils.h", "#pragma once\n")
	writeCompletionFixture(t, root, "src/main.cpp", "#include \"include/ut\"\nint main() { return 0; }\n")
	s := &Server{Workspace: root}

	got := postProjectCompletion(t, s, projectCompletionRequest{
		Path: "src/main.cpp", Language: "cpp", Prefix: "ut", LexicalMode: "string",
		Text: "#include \"include/ut", LinePrefix: "#include \"include/ut",
		Position: lspPosition{Line: 0, Character: 20}, Limit: 30,
	})
	item, ok := completionItemByLabel(got.Items, "include/utils.h")
	if !ok {
		t.Fatalf("include suggestion missing: %+v", got.Items)
	}
	if item.InsertText != "include/utils.h" || item.ReplacePrefix != "include/ut" || item.Source != "project-path" {
		t.Fatalf("unexpected include completion: %+v", item)
	}
}

func TestProjectSymbolIndexPersistsOutsideWorkspace(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	root := t.TempDir()
	writeCompletionFixture(t, root, "src/store.go", "package src\ntype PersistentStore struct {}\n")
	s := &Server{Workspace: root}

	idx, err := s.currentProjectSymbolIndex(context.Background())
	if err != nil { t.Fatal(err) }
	if len(idx.Symbols) == 0 { t.Fatal("symbol index unexpectedly empty") }
	cacheFile, err := projectSymbolIndexCacheFile(root)
	if err != nil { t.Fatal(err) }
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("persisted symbol cache missing: %v", err)
	}
	if strings.HasPrefix(cacheFile, root+string(filepath.Separator)) {
		t.Fatalf("symbol cache must not dirty workspace: cache=%s workspace=%s", cacheFile, root)
	}

	reloaded, err := (&Server{Workspace: root}).currentProjectSymbolIndex(context.Background())
	if err != nil { t.Fatal(err) }
	if len(reloaded.Symbols) != len(idx.Symbols) {
		t.Fatalf("reloaded symbols=%d want=%d", len(reloaded.Symbols), len(idx.Symbols))
	}
}

func TestProjectSymbolIndexUpdatesSingleSavedFile(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	root := t.TempDir()
	writeCompletionFixture(t, root, "src/value.go", "package src\ntype BeforeSave struct {}\n")
	s := &Server{Workspace: root}
	idx, err := s.currentProjectSymbolIndex(context.Background())
	if err != nil { t.Fatal(err) }
	if len(searchProjectSymbolIndex(idx, "BeforeSave", 10)) != 1 {
		t.Fatal("initial symbol missing")
	}

	s.updateProjectSymbolIndexFile("src/value.go", "package src\ntype AfterSave struct {}\n")
	if len(searchProjectSymbolIndex(idx, "BeforeSave", 10)) != 0 {
		t.Fatal("stale symbol remained after incremental update")
	}
	after := searchProjectSymbolIndex(idx, "AfterSave", 10)
	if len(after) != 1 || after[0].Path != "src/value.go" {
		t.Fatalf("updated symbol missing: %+v", after)
	}
}


func TestProjectCompletionSuggestsImportsWithoutTypedPrefix(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	root := t.TempDir()
	writeCompletionFixture(t, root, "pkg/alpha.go", "package pkg\n")
	writeCompletionFixture(t, root, "pkg/beta.go", "package pkg\n")
	writeCompletionFixture(t, root, "src/main.go", "package main\n")
	s := &Server{Workspace: root}

	got := postProjectCompletion(t, s, projectCompletionRequest{
		Path: "src/main.go", Language: "go", Prefix: "", LexicalMode: "string",
		Text: "package main\nimport \"", LinePrefix: "import \"",
		Position: lspPosition{Line: 1, Character: 8}, Limit: 20,
	})
	if len(got.Items) == 0 {
		t.Fatalf("expected bounded import suggestions for empty prefix")
	}
	for _, item := range got.Items {
		if item.Source != "project-path" {
			t.Fatalf("unexpected non-path completion in import context: %+v", item)
		}
		if item.InsertText == "" {
			t.Fatalf("empty import insert text: %+v", item)
		}
	}
}

func TestProjectCompletionRequestBoundsLimitAndDocumentSize(t *testing.T) {
	req, err := normalizeProjectCompletionRequest(projectCompletionRequest{
		Path: "main.go", Language: "go", Prefix: strings.Repeat("x", 300),
		Text: "package main", Limit: 1000,
		Position: lspPosition{Line: 0, Character: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Prefix) != 256 {
		t.Fatalf("prefix length=%d want=256", len(req.Prefix))
	}
	if req.Limit != projectCompletionMaxItems {
		t.Fatalf("limit=%d want=%d", req.Limit, projectCompletionMaxItems)
	}

	_, err = normalizeProjectCompletionRequest(projectCompletionRequest{
		Path: "main.go", Language: "go",
		Text: strings.Repeat("x", projectCompletionMaxText+1),
		Position: lspPosition{Line: 0, Character: 0},
	})
	if err != errProjectCompletionDocumentTooLarge {
		t.Fatalf("oversized document err=%v", err)
	}
}


func TestProjectSymbolIndexRecoversFromCorruptPersistedCache(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	root := t.TempDir()
	writeCompletionFixture(t, root, "src/recover.go", "package src\ntype RecoveredSymbol struct {}\n")

	cacheFile, err := projectSymbolIndexCacheFile(root)
	if err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(cacheFile, []byte("not-a-valid-gzip-index"), 0o600); err != nil { t.Fatal(err) }

	s := &Server{Workspace: root}
	idx, err := s.currentProjectSymbolIndex(context.Background())
	if err != nil {
		t.Fatalf("corrupt cache should rebuild: %v", err)
	}
	results := searchProjectSymbolIndex(idx, "RecoveredSymbol", 10)
	if len(results) != 1 || results[0].Path != "src/recover.go" {
		t.Fatalf("rebuilt symbol missing: %+v", results)
	}
	if _, err := loadProjectSymbolIndexCache(root, idx.Fingerprint); err != nil {
		t.Fatalf("rebuilt cache was not persisted as valid gzip index: %v", err)
	}
}
