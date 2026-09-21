package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGroupedMenusFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"header-action-menus",
		"makeMenu('Files'",
		"Quick Open…  Ctrl+P",
		"explorer.textContent='Explorer'",
		"TaskMenuExplorer?.open()",
		"makeMenu('Terminal'",
		"makeMenu('Workspace'",
		"makeMenu('Settings'",
		"addSection(settings.pop,'APPEARANCE',[document.querySelector('.appearance-controls'),document.querySelector('#self-update-check')])",
		".git-status-pill",
		"pane-action-menus",
		"paneMenu(view,'Session'",
		"paneMenu(view,'Console'",
		".session-force-restart",
		"['SPLIT'",
		".session-split-vertical",
		".session-split-horizontal",
		".session-merge-vertical",
		".session-merge-horizontal",
		".session-unsplit",
		".console-search-btn",
		".copy-console",
		".taskmenu-menu-popover>.copy-console{margin-left:0;max-width:100%}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("menus.js missing grouped control behavior %q", want)
		}
	}
	if !strings.Contains(indexHTML, `/featuremods/next.js`) {
		t.Fatal("index must load featuremods/next.js")
	}
}
