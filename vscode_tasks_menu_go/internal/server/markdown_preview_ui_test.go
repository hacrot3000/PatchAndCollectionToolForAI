package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestMarkdownSelectedFileHasSeparateViewAndPreviewActions(t *testing.T) {
	for _, want := range []string{
		"markdownPreview.textContent='Preview'",
		"file.preview_kind==='markdown'",
		"previewSelectedMarkdown(view)",
		"taskmenu:file-markdown-preview",
		"info.kind==='text'||info.kind==='markdown'",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing Markdown selected-file behavior %q", want)
		}
	}
}

func TestMarkdownPreviewRendererIsVendoredAndDoesNotUseHTMLInjection(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/markdownpreview.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function renderMarkdown(content)",
		"taskmenu:file-markdown-preview",
		"document.createElement('h'",
		"document.createElement('table')",
		"document.createElement('pre')",
		"textContent=codeLines.join",
		"TaskMenuMarkdownPreview",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("markdownpreview.js missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML=") {
		t.Fatal("Markdown preview must not inject source through innerHTML")
	}
	data, err = webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(data), "markdownpreview.js") {
		t.Fatal("next.js must load markdownpreview.js")
	}
}
