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
		"scope=terminals&profile=",
		"encodeURIComponent(app.layoutProfile||'desktop')",
		"app.layoutProfile==='mobile'",
		"clearPresentation",
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
		"split?.suspendFocusTracking?.()",
		"app.activateView(activeID,{focus:false})",
		"split?.resumeFocusTracking?.(activeID)",
		"app.focusView?.(activeID)",
		"const introduced=new Set()",
		"introduced.has(right)",
		"introduced.has(second)",
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
	if strings.Contains(js, "used.has(left)") || strings.Contains(js, "used.has(group.first)") {
		t.Fatal("terminal restore must allow an existing split leaf to be split again")
	}
	if strings.Contains(js, "app.activateView(activeID);") {
		t.Fatal("terminal restore must not focus during intermediate layout activation")
	}
	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/terminalrestore.js';") {
		t.Fatal("next.js must load terminalrestore.js")
	}
}
