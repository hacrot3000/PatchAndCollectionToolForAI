package patchtool

import (
	"archive/zip"
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
	guide := `<html><body>
<h2>5. Prompt mẫu chuẩn</h2>
<pre id="prompt-vi">` + prompt + `</pre>
<pre id="prompt-en">English prompt</pre>
<pre id="prompt-ru">Russian prompt</pre>
</body></html>`
	if err := os.WriteFile(filepath.Join(root, "patchtool", aiPackGuideName), []byte(guide), 0o644); err != nil {
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
	if first.Cached || first.DocCount != 2 || first.Prompt != "Prompt version 1" || first.Fingerprint == "" {
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
		"PATCH_TOOL_AI_PACK/" + aiPackGuideName,
		"PATCH_TOOL_AI_PACK/docs/AI_USAGE_CONTRACT.md",
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

	guidePath := filepath.Join(root, "patchtool", aiPackGuideName)
	raw, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), "Prompt version 1", "Prompt version 2", 1)
	if err := os.WriteFile(guidePath, []byte(updated), 0o644); err != nil {
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

func TestBuildAIPackRejectsMissingGuideInsteadOfUsingStaleEmbeddedPrompt(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := writeAIPackFixture(t, root, "Prompt version 1")
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	if err := os.Remove(filepath.Join(root, "patchtool", aiPackGuideName)); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck")); err == nil || !strings.Contains(err.Error(), "missing from the active Patch Tool runtime") {
		t.Fatalf("missing guide should fail closed, err=%v", err)
	}
}


func TestTaskDeckInstallerBundlesGuideNeededByAIPack(t *testing.T) {
	raw, err := os.ReadFile("../../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	for _, want := range []string{
		`[[ -f "$release/patchtool/HUONG_DAN_PYTHON_PATCH_TOOL.html" ]] || return 1`,
		`"$SOURCE_ROOT/HUONG_DAN_PYTHON_PATCH_TOOL.html"`,
		`cp "$SOURCE_ROOT/HUONG_DAN_PYTHON_PATCH_TOOL.html" "$STAGED_RELEASE/patchtool/HUONG_DAN_PYTHON_PATCH_TOOL.html"`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("TaskDeck installer must bundle current Patch Tool guide: missing %q", want)
		}
	}
}
