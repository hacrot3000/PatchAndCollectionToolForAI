package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestAppearanceControlsFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/appearance.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"appearance-controls",
		"Dark",
		"Light",
		"Default Mono",
		"DejaVu Mono",
		"Liberation Mono",
		"terminal-font-size",
		"view.term.options.fontSize",
		"view.term.options.fontFamily",
		"view.term.options.theme",
		"localStorage.setItem",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("appearance.js missing behavior %q", want)
		}
	}
}

func TestAppearancePreferencesAreScopedByLayoutProfile(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/appearance.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	if !strings.Contains(js, "(app.layoutProfile||'desktop')") {
		t.Fatal("appearance storage must be scoped by desktop/mobile layout profile")
	}
}


func TestDesktopAppearanceMigratesLegacyStorageWithoutFeedingMobile(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/appearance.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"legacyWorkspaceKey",
		"if(app.layoutProfile==='desktop')",
		"localStorage.getItem(legacyWorkspaceKey(name))",
		"localStorage.setItem(key,legacy)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("appearance migration missing %q", want)
		}
	}
}


func TestAppearanceDefersWorkspaceStorageUntilTasksLoaded(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/appearance.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function workspaceName(){return String(app.taskData?.workspace||'').trim();}",
		"if(!key)return fallback",
		"if(app.taskData?.workspace)reloadWorkspaceAppearance()",
		"window.addEventListener('taskmenu:tasks',reloadWorkspaceAppearance,{once:true})",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("appearance bootstrap guard missing %q", want)
		}
	}
	if strings.Contains(js, "app.taskData.workspace") {
		t.Fatal("appearance.js must not dereference taskData.workspace before /api/tasks has loaded")
	}
}
