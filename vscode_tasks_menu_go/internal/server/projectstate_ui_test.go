package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestQuickTaskStateUsesProjectPersistence(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/all.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/state/tasks",
		"function loadProjectState()",
		"function queueProjectStateSave()",
		"favorites:normalizeIDs(raw?.favorites,20)",
		"recent:normalizeIDs(raw?.recent,10)",
		"history:normalizeHistory(raw?.history)",
		"projectState.history=normalizeHistory(items).slice(0,10)",
		"const items=readHistory().slice(0,10)",
		"legacyRead('favorites',[])",
		"legacyRead('recent',[])",
		"legacyRead('history',[])",
		"const key=legacyStorageKey(name);if(key)localStorage.removeItem(key)",
		"const taskDataReady=app.taskData?.workspace?Promise.resolve():new Promise",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("project state UI missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"localStorage.setItem(storageKey(",
		"JSON.stringify(items.slice(0,80))",
		"readHistory().slice(0,15)",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("project state UI still contains legacy behavior %q", forbidden)
		}
	}
}

func TestProjectTaskStateFileName(t *testing.T) {
	if projectTaskStateFile != "vscode_tasks_menu.state.json" {
		t.Fatalf("state file = %q", projectTaskStateFile)
	}
}
