package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestProjectSymbolSearchUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectsymbols.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"project-symbol-backdrop",
		"project-symbol-input",
		"function canRead()",
		"function select(index)",
		"async function choose(index=selected)",
		"function schedule()",
		"async function runSearch(query,currentSeq)",
		"new AbortController()",
		"controller.abort()",
		"new URLSearchParams({q:query,limit:'100'})",
		"'/api/project/symbols?'",
		"data?.truncated?' · bounded scan truncated':''",
		"editor.openFile(item.path)",
		"editor.jumpEditorToLine(view,item.line)",
		"event.key==='ArrowDown'",
		"event.key==='ArrowUp'",
		"event.key==='Enter'",
		"event.key==='Escape'",
		"globalThis.TaskMenuProjectSymbols={open,close",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("project symbol search UI missing %q", want)
		}
	}

	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(next), "import '/featuremods/projectsymbols.js';") {
		t.Fatal("project symbol search module is not loaded")
	}

	menus, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil { t.Fatal(err) }
	menuJS := string(menus)
	for _, want := range []string{
		"Project Symbols…",
		"globalThis.TaskMenuProjectSymbols?.open()",
		"app.hasPermission?.('files.read')",
		"addSection(files.pop,'OPEN',[quickOpen,searchFiles,symbols,explorer])",
	} {
		if !strings.Contains(menuJS, want) {
			t.Fatalf("Files menu missing project symbol search contract %q", want)
		}
	}
}

func TestEditorOutlineShortcut(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"if(key==='o'&&event.shiftKey)",
		"showEditorOutline(view)",
		"event.preventDefault()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor outline shortcut missing %q", want)
		}
	}
}
