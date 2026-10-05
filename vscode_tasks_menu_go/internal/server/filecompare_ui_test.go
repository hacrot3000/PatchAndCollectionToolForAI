package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGenericFileCompareFeatureLoads(t *testing.T) {
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(next)
	if !strings.Contains(js, "import '/featuremods/filecompare.js';") {
		t.Fatal("generic file compare feature is not loaded")
	}
}

func TestGenericFileCompareSupportsCoreSourcesAndViews(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filecompare.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function buildCompareModel(leftText,rightText)",
		"function diffOperations(leftText,rightText)",
		"function lcsBlock(a,b)",
		"Side by side",
		"Inline",
		"Left → Right",
		"Right → Left",
		"function replaceLineRange(text,start,count,replacementLines)",
		"async function copyHunk(hunk,direction)",
		"function projectSource(pathValue",
		"expected_sha256:sha",
		"function editorSource(view",
		"function savedEditorSource(view)",
		"function clipboardSource(",
		"async function openProjectFiles(leftPath,rightPath)",
		"async function openEditorSaved(view)",
		"async function openEditorClipboard(view)",
		"async function promptProjectCompare(pathValue)",
		"globalThis.TaskMenuFileCompare={",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("generic file compare missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("file compare must not render source text through innerHTML")
	}
}

func TestProjectAndEditorMenusExposeGenericCompare(t *testing.T) {
	shared, err := webassets.Files.ReadFile("featuremods/projectfileactions.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Compare with another file…",
		"TaskMenuFileCompare",
		"promptProjectCompare(pathValue)",
	} {
		if !strings.Contains(string(shared), want) {
			t.Fatalf("shared project file actions missing compare contract %q", want)
		}
	}

	tabContext, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Compare current ↔ saved",
		"TaskMenuFileCompare?.openEditorSaved?.(view)",
		"Compare current ↔ clipboard",
		"TaskMenuFileCompare?.openEditorClipboard?.(view)",
	} {
		if !strings.Contains(string(tabContext), want) {
			t.Fatalf("editor context missing compare action %q", want)
		}
	}
}
