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
		"makeMenu('Settings'",
		"addSection(settings.pop,'PATCH TOOL',[patchUIControl])",
		"patchUISelect.id='patch-ui-mode'",
		"Native UI",
		"Terminal (legacy)",
		"vscode-tasks-menu:patch-ui-mode:",
		"taskmenu:patch-ui-mode",
		"TaskMenuPatchUISettings",
		"addSection(files.pop,'FILES',[document.querySelector('#upload-workspace')])",
		"addSection(settings.pop,'APPEARANCE',[document.querySelector('.appearance-controls'),document.querySelector('#self-update-check')])",
		"addSection(settings.pop,'WORKSPACE',[document.querySelector('#reload'),document.querySelector('#edit-title')])",
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
	if strings.Contains(js, "makeMenu('Workspace'") {
		t.Fatal("Workspace header menu must be removed")
	}
	if !strings.Contains(indexHTML, `/featuremods/next.js`) {
		t.Fatal("index must load featuremods/next.js")
	}
}


func TestGroupedMenusDefersWorkspaceSettingsUntilTasksLoaded(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const projectWorkspace=String(app.taskData?.workspace||'').trim();",
		"if(!projectWorkspace)return false;",
		"vscode-tasks-menu:patch-ui-mode:'+projectWorkspace",
		"if(!installHeaderMenus()){",
		"window.addEventListener('taskmenu:tasks',()=>installHeaderMenus(),{once:true})",
	} {
		if !strings.Contains(js,want) {
			t.Fatalf("menus bootstrap guard missing %q",want)
		}
	}
	if strings.Contains(js,"app.taskData.workspace") {
		t.Fatal("menus.js must not dereference taskData.workspace before /api/tasks has loaded")
	}
}
