package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestPopupTabTitleRename(t *testing.T) {
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
		"promptRename(view)",
		"window.prompt('Tab title:',before)",
		"answer===null",
		"await persistTitle(view,value&&value!==fallback?value:'')",
		"view.meta.title=String(meta?.title||'')",
		"migrateLegacyBrowserTitle(view)",
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
	for _, forbidden := range []string{
		"contentEditable='true'",
		"tab-title-editing",
		"tab-renaming",
		"label.onblur",
		"label.onkeydown",
		"beginInlineRename",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("popup rename must not retain inline editing behavior %q", forbidden)
		}
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
