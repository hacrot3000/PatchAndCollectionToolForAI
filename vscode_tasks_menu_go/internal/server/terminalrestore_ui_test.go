package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalRestoreFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/state/tasks?scope=terminals",
		"terminalIDsInTabOrder",
		"active_session_id",
		"left_session_id",
		"right_session_id",
		"orientation",
		"splits",
		"getGroups",
		"restoreTabOrder(ids)",
		"restoreProjectGroups",
		"restoredGroups(restored,ids)",
		"await waitForViews(ids)",
		"method:'POST'",
		"method:'PUT'",
		"pagehide",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminalrestore.js missing %q", want)
		}
	}
	if strings.Contains(js, "consoleText") || strings.Contains(js, "scrollback") {
		t.Fatal("terminal restore must not persist console/scrollback")
	}
	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/terminalrestore.js';") {
		t.Fatal("next.js must load terminalrestore.js")
	}
}
