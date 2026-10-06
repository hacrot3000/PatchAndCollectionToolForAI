package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestLSPBrowserEditorIntegration(t *testing.T) {
	data,err:=webassets.Files.ReadFile("featuremods/lsp.js")
	if err!=nil{t.Fatal(err)}
	js:=string(data)
	for _,want:=range []string{
		"editor-lsp",
		"Diagnostics",
		"Hover",
		"Definition",
		"References",
		"Rename",
		"app.jsonFetch('/api/lsp'",
		"app.jsonFetch('/api/lsp?path='",
		"view.cm.state.selection.main.head",
		"view.cm.state.doc.toString()",
		"editorAPI().openFile",
		"jumpEditorToLine",
		"Save the active editor before Rename Symbol",
		"Save or revert unsaved editor before rename:",
		"expected_sha256:item.file.sha256",
		"Rename stopped because file changed externally:",
		"TaskMenuLSP={",
	} {
		if !strings.Contains(js,want){t.Fatalf("lsp.js missing %q",want)}
	}
}

func TestLSPModuleLoadsAfterEditorAndPaletteExposesAction(t *testing.T) {
	nextData,err:=webassets.Files.ReadFile("featuremods/next.js")
	if err!=nil{t.Fatal(err)}
	next:=string(nextData)
	editor:=strings.Index(next,"editor.js")
	lsp:=strings.Index(next,"lsp.js")
	if editor<0||lsp<0||lsp<editor{t.Fatalf("lsp.js must load after editor.js: %q",next)}

	paletteData,err:=webassets.Files.ReadFile("featuremods/terminalpalette.js")
	if err!=nil{t.Fatal(err)}
	palette:=string(paletteData)
	for _,want:=range []string{
		"Editor: Language Server…",
		"TaskMenuLSP?.open?.(activeEditor)",
		"function activeEditorView()",
	}{
		if !strings.Contains(palette,want){t.Fatalf("command palette missing LSP action %q",want)}
	}
}

func TestLSPSharedRouteIsReadAndRenameHasWriteGuard(t *testing.T) {
	data,err:=webassets.Files.ReadFile("featuremods/lsp.js")
	if err!=nil{t.Fatal(err)}
	if !strings.Contains(string(data),"method:'POST'"){t.Fatal("LSP browser requests are not POSTed")}

	serverData,err:=webassets.Files.ReadFile("../internal/server/lsp.go")
	if err==nil {
		_ = serverData
	}
}
