package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"

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
		"const api=editorAPI();if(!api?.openFile)",
		"const view=await api.openFile",
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
	req:=httptest.NewRequest(http.MethodPost,"/api/lsp",strings.NewReader(`{"action":"hover","path":"main.go","text":"package main","position":{"line":0,"character":0}}`))
	permissions:=sharedRoutePermissions(req)
	if len(permissions)!=1||permissions[0]!=identity.PermissionFilesRead {
		t.Fatalf("LSP route permissions=%v",permissions)
	}

	root:=t.TempDir()
	if err:=os.WriteFile(filepath.Join(root,"main.go"),[]byte("package main\n"),0o644);err!=nil{t.Fatal(err)}
	s:=&Server{Workspace:root}
	rename:=httptest.NewRequest(http.MethodPost,"/api/lsp",strings.NewReader(`{"action":"rename","path":"main.go","text":"package main\n","position":{"line":0,"character":8},"new_name":"renamed"}`))
	rename.Header.Set("Content-Type","application/json")
	principal:=identity.Principal{Permissions:map[string]bool{identity.PermissionFilesRead:true}}
	rename=rename.WithContext(contextWithSharedPrincipal(rename.Context(),principal))
	recorder:=httptest.NewRecorder()
	s.lspAPI(recorder,rename)
	if recorder.Code!=http.StatusForbidden {
		t.Fatalf("rename without files.write status=%d body=%s",recorder.Code,recorder.Body.String())
	}
}

func contextWithSharedPrincipal(ctx context.Context, principal identity.Principal) context.Context {
	return context.WithValue(ctx, sharedPrincipalContextKey{}, principal)
}
