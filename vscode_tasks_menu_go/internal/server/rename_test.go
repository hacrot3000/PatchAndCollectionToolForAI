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
		"beginInlineRename(view)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("rename module missing %q", want)
		}
	}
}
