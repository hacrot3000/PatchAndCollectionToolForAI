package patchtool

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAIPackFixture(t *testing.T, root, prompt string) string {
	t.Helper()
	entry := filepath.Join(root, "patchtool", "python_patch_entry.py")
	if err := os.MkdirAll(filepath.Join(root, "patchtool", "_patch_lib", "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("# test\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "patchtool", "_patch_lib", "docs", "AI_USAGE_CONTRACT.md"), []byte("contract-v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "patchtool", "_patch_lib", "docs", "nested.json"), []byte("{\"ok\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prompts := map[string]any{
		"version": 1,
		"vi": prompt,
		"en": "English prompt",
		"ru": "Russian prompt",
	}
	raw, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(root, "patchtool", "_patch_lib", "docs", aiPackPromptDocName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestBuildAIPackBuildsOnDemandThenReusesMatchingCache(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := writeAIPackFixture(t, root, "Prompt version 1")
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)

	first, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached || first.DocCount != 3 || first.Prompt != "Prompt version 1" || first.Fingerprint == "" {
		t.Fatalf("unexpected first AI Pack result: %+v", first)
	}
	if first.Path != "artifacts/patch_tool/ai_pack/"+aiPackZipName {
		t.Fatalf("unexpected AI Pack path: %q", first.Path)
	}
	packPath := filepath.Join(workspace, filepath.FromSlash(first.Path))
	zr, err := zip.OpenReader(packPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, file := range zr.File {
		names[file.Name] = true
	}
	for _, want := range []string{
		"PATCH_TOOL_AI_PACK/START_HERE.md",
		"PATCH_TOOL_AI_PACK/PROMPT_VI.txt",
		"PATCH_TOOL_AI_PACK/PROMPT_EN.txt",
		"PATCH_TOOL_AI_PACK/PROMPT_RU.txt",
		"PATCH_TOOL_AI_PACK/docs/AI_USAGE_CONTRACT.md",
		"PATCH_TOOL_AI_PACK/docs/" + aiPackPromptDocName,
		"PATCH_TOOL_AI_PACK/docs/nested.json",
		"PATCH_TOOL_AI_PACK/MANIFEST.json",
	} {
		if !names[want] {
			t.Fatalf("AI Pack missing %q", want)
		}
	}

	second, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached || second.Fingerprint != first.Fingerprint || second.GeneratedAt != first.GeneratedAt {
		t.Fatalf("matching docs should reuse cache: first=%+v second=%+v", first, second)
	}
}

func TestBuildAIPackRebuildsOnlyAfterDocsOrPromptChange(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := writeAIPackFixture(t, root, "Prompt version 1")
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)

	first, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}

	doc := filepath.Join(root, "patchtool", "_patch_lib", "docs", "AI_USAGE_CONTRACT.md")
	if err := os.WriteFile(doc, []byte("contract-v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Cached || second.Fingerprint == first.Fingerprint {
		t.Fatalf("changed docs must rebuild AI Pack: first=%+v second=%+v", first, second)
	}

	promptPath := filepath.Join(root, "patchtool", "_patch_lib", "docs", aiPackPromptDocName)
	raw, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), "Prompt version 1", "Prompt version 2", 1)
	if err := os.WriteFile(promptPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if third.Cached || third.Fingerprint == second.Fingerprint || third.Prompt != "Prompt version 2" {
		t.Fatalf("changed standard prompt must rebuild AI Pack: second=%+v third=%+v", second, third)
	}
}

func TestBuildAIPackRejectsMissingStandardPromptDoc(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := writeAIPackFixture(t, root, "Prompt version 1")
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	if err := os.Remove(filepath.Join(root, "patchtool", "_patch_lib", "docs", aiPackPromptDocName)); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck")); err == nil || !strings.Contains(err.Error(), "missing from the active Patch Tool docs") {
		t.Fatalf("missing standard prompt doc should fail closed, err=%v", err)
	}
}
