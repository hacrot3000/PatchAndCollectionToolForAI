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


func TestMarkdownEditorPreviewUsesCurrentBuffer(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function editorIsMarkdown(pathValue)",
		"['md','markdown','mdown','mkd','mkdn'].includes",
		"function openEditorMarkdownPreview(view)",
		"view.cm.state.doc.toString()",
		"preview.open({kind:'markdown',path:view.file.path,content,size:new Blob([content]).size})",
		"markdownPreview.textContent='Preview'",
		"markdownPreview.hidden=!editorIsMarkdown(file.path)",
		"save,autoSave,reload,markdownPreview,find",
		"markdownPreview.onclick=()=>",
		"openEditorMarkdownPreview(view)",
		"function syncEditorMarkdownPreview(view)",
		"syncEditorMarkdownPreview(view);",
		"view.file.path=nextPath;",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor Markdown Preview missing %q", want)
		}
	}
}

func TestMarkdownExplorerContextHasDirectPreview(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectfileactions.js")
	if err != nil { t.Fatal(err) }
	actions := string(data)
	for _, want := range []string{
		"if(type==='file'&&/\\.(?:md|markdown|mdown|mkd|mkdn)$/i.test(pathValue))",
		"{label:'Preview Markdown',run:()=>openProjectMarkdownPreview(pathValue)}",
		"info=info||await projectPreviewInfo(pathValue)",
		"if(info?.kind!=='markdown')",
		"preview.open(info)",
	} {
		if !strings.Contains(actions, want) {
			t.Fatalf("project Markdown context preview missing %q", want)
		}
	}
	data, err = webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil { t.Fatal(err) }
	explorer := string(data)
	if !strings.Contains(explorer, "appendSharedProjectActions(pathValue,type)") ||
		!strings.Contains(explorer, "TaskMenuProjectFileActions?.standardActions?.(pathValue,type)") {
		t.Fatal("Explorer context menu must reuse the shared project Markdown preview action")
	}
}
