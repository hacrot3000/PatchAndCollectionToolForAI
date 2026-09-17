package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalRenameFeatureModule(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/rename.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"terminal-title:",
		"tab-title:",
		"Double-click",
		"task_id===0",
		"localStorage.setItem",
		"promptRename(view)",
		"window.prompt('Tab title:',before)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("rename module missing %q", want)
		}
	}
	if strings.Contains(js, "beginInlineRename") || strings.Contains(js, "contentEditable='true'") {
		t.Fatal("rename feature must use popup prompt instead of inline editing")
	}
}
