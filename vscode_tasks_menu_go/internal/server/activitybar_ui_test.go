package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestActivityBarAutoHideSidebarFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"task-activity-bar",
		"task-sidebar-auto-hide",
		"task-sidebar-panel-open",
		"sidebar-auto-hide-toggle",
		"sidebar-auto-hide:",
		"iconButton('tasks','Tasks'",
		"iconButton('explorer','Explorer'",
		"TaskMenuExplorer?.open()",
		"TaskMenuExplorer?.close()",
		"taskmenu:sidebar-mode-changed",
		"localStorage.setItem",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("activitybar.js missing %q", want)
		}
	}
}

func TestActivityBarIsDesktopOnlyAndLoadsBeforeMenus(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "if(app.layoutProfile!=='mobile')") {
		t.Fatal("activity bar must leave the existing mobile drawer behavior unchanged")
	}

	nextData, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	next := string(nextData)
	explorer := strings.Index(next, "explorer.js")
	activity := strings.Index(next, "activitybar.js")
	menus := strings.Index(next, "menus.js")
	if explorer < 0 || activity < 0 || menus < 0 || !(explorer < activity && activity < menus) {
		t.Fatal("Explorer must load before activity bar, and activity bar before grouped menus")
	}
}
