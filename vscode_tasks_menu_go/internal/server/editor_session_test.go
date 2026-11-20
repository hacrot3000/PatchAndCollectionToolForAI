package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func editorSessionSourceHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func putEditorSessionTest(t *testing.T, s *Server, state editorSessionState) editorSessionState {
	t.Helper()
	body, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/project/editor-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("editor session PUT status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got editorSessionState
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func getEditorSessionTest(t *testing.T, s *Server) editorSessionState {
	t.Helper()
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/editor-session", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("editor session GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got editorSessionState
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestEditorSessionPersistsDirtySwapAndRestoresCursor(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "src", "main.go")
	original := []byte("package main\nfunc main() {}\n")
	if err := os.WriteFile(source, original, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}
	dirty := "package main\nfunc main() {\n\tprintln(\"dirty\")\n}\n"

	putEditorSessionTest(t, s, editorSessionState{
		Version: editorSessionVersion,
		Active:  "src/main.go",
		Tabs: []editorSessionTab{{
			Path: "src/main.go", Dirty: true, Content: dirty,
			SourceSHA256: editorSessionSourceHash(original),
			Selection: editorSessionSelection{Anchor: 31, Head: 31},
			LineEnding: "lf", Encoding: "utf-8",
		}},
	})

	swapPath := source + editorSwapMarker
	info, err := os.Stat(swapPath)
	if err != nil {
		t.Fatalf("swap not created: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("swap mode=%o want=600", info.Mode().Perm())
	}
	swap, err := readEditorSwap(swapPath)
	if err != nil {
		t.Fatal(err)
	}
	if swap.Content != dirty || swap.Selection.Anchor != 31 || swap.Selection.Head != 31 {
		t.Fatalf("unexpected swap payload: %+v", swap)
	}

	exclude, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(exclude), editorSwapExcludeRule) {
		t.Fatalf("git exclude missing %q: %s", editorSwapExcludeRule, exclude)
	}

	restored := getEditorSessionTest(t, &Server{Workspace: root})
	if len(restored.Tabs) != 1 || !restored.Tabs[0].Dirty || restored.Tabs[0].Content != dirty {
		t.Fatalf("dirty session not restored: %+v", restored)
	}
	if restored.Tabs[0].Selection.Anchor != 31 || restored.Active != "src/main.go" {
		t.Fatalf("cursor/active editor not restored: %+v", restored)
	}

	if err := os.WriteFile(source, []byte("package main\n// changed outside TaskDeck\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed := getEditorSessionTest(t, &Server{Workspace: root})
	if len(changed.Tabs) != 1 || !changed.Tabs[0].ExternalChanged {
		t.Fatalf("external source change not detected: %+v", changed)
	}

	putEditorSessionTest(t, s, editorSessionState{Version: editorSessionVersion, Tabs: []editorSessionTab{}})
	if _, err := os.Stat(swapPath); !os.IsNotExist(err) {
		t.Fatalf("swap should be removed after dirty tab is discarded/closed: %v", err)
	}
}

func TestEditorSwapHiddenFromProjectTreeIndexAndDirectAPI(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	source := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(source, []byte("saved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	swapPath := source + editorSwapMarker
	if err := os.WriteFile(swapPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{Workspace: root}

	treeRR := httptest.NewRecorder()
	s.Handler().ServeHTTP(treeRR, httptest.NewRequest(http.MethodGet, "/api/project/tree", nil))
	if treeRR.Code != http.StatusOK {
		t.Fatalf("tree status=%d body=%s", treeRR.Code, treeRR.Body.String())
	}
	if strings.Contains(treeRR.Body.String(), editorSwapMarker) {
		t.Fatalf("project tree exposed editor swap: %s", treeRR.Body.String())
	}

	idx, err := buildProjectFileIndex(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for i := range idx.offsets {
		if isTaskDeckSwapName(filepath.Base(idx.pathAt(i))) {
			t.Fatalf("project file index exposed swap %q", idx.pathAt(i))
		}
	}

	swapRR := httptest.NewRecorder()
	swapURL := "/api/project/file?path=" + url.QueryEscape("sample.txt"+editorSwapMarker)
	s.Handler().ServeHTTP(swapRR, httptest.NewRequest(http.MethodGet, swapURL, nil))
	if swapRR.Code != http.StatusBadRequest {
		t.Fatalf("direct swap read status=%d want=%d body=%s", swapRR.Code, http.StatusBadRequest, swapRR.Body.String())
	}
}

func TestEditorSessionCacheStaysOutsideWorkspace(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	root := t.TempDir()
	path, err := editorSessionCacheFile(root, "local")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(filepath.Clean(path), filepath.Clean(root)+string(filepath.Separator)) {
		t.Fatalf("editor session metadata must stay outside workspace: %s", path)
	}
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(cache)+string(filepath.Separator)) {
		t.Fatalf("editor session cache not under user cache: %s", path)
	}
}
