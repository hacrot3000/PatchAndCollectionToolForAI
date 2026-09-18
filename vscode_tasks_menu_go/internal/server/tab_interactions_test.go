package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTabDragOrderingFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabdrag.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"tab.draggable=true",
		"addEventListener('dragstart'",
		"addEventListener('dragover'",
		"addEventListener('drop'",
		"addEventListener('dragend'",
		"originalOrder=currentIDs()",
		"if(!committed)applyOrder(originalOrder)",
		"sessionStorage.setItem(storageKey()",
		"TaskMenuTerminalRestore?.ready",
		"TaskMenuTerminalRestore?.persistSnapshot?.()",
		"target?.closest('.close')?'0':'1'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabdrag.js missing %q", want)
		}
	}

	restore, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restore), "ready:restoreReady") {
		t.Fatal("terminal restore API must expose readiness so tab order is applied after recovery")
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/tabdrag.js';") {
		t.Fatal("next.js must load tabdrag.js")
	}
}

func TestTabContextMenuReusesSessionAndConsoleActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"addEventListener('contextmenu'",
		"sourceActions(view,'Session')",
		"sourceActions(view,'Console')",
		"app.activateView(targetView.meta.id)",
		"source.click()",
		"event.preventDefault()",
		"context-copy",
		"context-find",
		"context-save",
		"context-danger",
		"session-clear-console",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabcontext.js missing %q", want)
		}
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	next := string(loader)
	menus := strings.Index(next, "import '/featuremods/menus.js';")
	context := strings.Index(next, "import '/featuremods/tabcontext.js';")
	if menus < 0 || context < 0 || context < menus {
		t.Fatal("tabcontext.js must load after menus.js so Session/Console source actions already exist")
	}
}


func TestEditorTabContextMenuUsesEditorOnlyActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function openEditorContextMenu(view,x,y)",
		"globalThis.TaskMenuEditor?.editors?.get(id)",
		"{label:'Save'",
		"{label:'Reload'",
		"{label:'Go to line…'",
		"{label:'Close'",
		"editor?.activateEditor?.(view.id)",
		"if(editorView)openEditorContextMenu",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabcontext.js missing editor-only context behavior %q", want)
		}
	}
}
