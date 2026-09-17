package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestInlineTabTitleRename(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/rename.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Double-click to rename this tab",
		"tabsHost.addEventListener('dblclick'",
		"target?.closest('.tab[data-id]')",
		"target?.closest('.close')",
		"app.views.get(tab.dataset.id||'')",
		"beginInlineRename(view)",
		"contentEditable='true'",
		"tab-renaming",
		".tab.tab-renaming .status{display:none}",
		"view.tab.classList.add('tab-renaming')",
		"view.tab.classList.remove('tab-renaming')",
		"e.key==='Enter'",
		"e.key==='Escape'",
		"label.onblur=()=>finish(true)",
		"vscode-tasks-menu:tab-title:",
		"vscode-tasks-menu:terminal-title:",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("rename.js missing %q", want)
		}
	}
	if strings.Contains(js, "target?.closest('.status,.close')") {
		t.Fatal("running status must remain a valid double-click rename target")
	}
	if strings.Contains(js, "window.prompt(") {
		t.Fatal("tab rename should use inline editing instead of window.prompt")
	}
	if strings.Contains(js, "label.ondblclick") {
		t.Fatal("tab rename should use delegated dblclick so restored tabs cannot lose the handler")
	}
	// The dblclick handler is intentionally available for every tab. Only the
	// legacy toolbar Rename button remains terminal-only.
	if strings.Contains(js, "if(view.meta.task_id!==0)return;apply(view)") {
		t.Fatal("double-click rename must not be restricted to terminal tabs")
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/rename.js';") {
		t.Fatal("next.js must load rename.js")
	}
}
