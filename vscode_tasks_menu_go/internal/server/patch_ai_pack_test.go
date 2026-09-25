package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestPatchAIPackEndpointBuildsAndThenReusesCache(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	runtimeRoot := filepath.Join(root, "patchtool")
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "_patch_lib", "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(runtimeRoot, "python_patch_entry.py")
	if err := os.WriteFile(entry, []byte("# test\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "_patch_lib", "docs", "AI_USAGE_CONTRACT.md"), []byte("current docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prompts, err := json.MarshalIndent(map[string]any{
		"version": 1,
		"vi": "Read current docs before work.",
		"en": "Read current docs before work.",
		"ru": "Read current docs before work.",
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	prompts = append(prompts, '\n')
	if err := os.WriteFile(filepath.Join(runtimeRoot, "_patch_lib", "docs", "AI_STANDARD_PROMPTS.json"), prompts, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)

	s := &Server{Workspace: workspace}
	call := func() patchtool.AIPackResult {
		req := httptest.NewRequest(http.MethodPost, "/api/patch/ai-pack", nil)
		res := httptest.NewRecorder()
		s.patchAIPack(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
		var got patchtool.AIPackResult
		if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	first := call()
	if first.Cached || first.Prompt != "Read current docs before work." || first.DocCount != 2 {
		t.Fatalf("unexpected first AI Pack response: %+v", first)
	}
	second := call()
	if !second.Cached || second.Fingerprint != first.Fingerprint || second.Path != first.Path {
		t.Fatalf("second AI Pack request should reuse cache: first=%+v second=%+v", first, second)
	}
}

func TestPatchAIPackEndpointIsPostOnly(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/patch/ai-pack", nil)
	res := httptest.NewRecorder()
	s.patchAIPack(res, req)
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status=%d want %d", res.Code, http.StatusMethodNotAllowed)
	}
}


func TestPatchAIPackRouteIsRegistered(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `mux.HandleFunc("/api/patch/ai-pack", s.patchAIPack)`) {
		t.Fatal("Patch AI Pack API route is not registered")
	}
}
