package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSelfUpdateBrowserWorkflow(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/selfupdate.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"/api/state/tasks?scope=self-update",
		"Update now",
		"TaskMenuTerminalRestore?.persistSnapshot",
		"action='+encodeURIComponent(action)",
		"postAction('ack',req.id)",
		"awaiting_confirmation",
		"completed",
		"failed",
		"target_url",
		"location.replace",
		"Đang chờ daemon mới khởi động",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("selfupdate.js missing %q", want)
		}
	}
	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(loader), "import '/featuremods/selfupdate.js';") {
		t.Fatal("next.js must load selfupdate.js")
	}
	terminalRestore, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(terminalRestore), "TaskMenuTerminalRestore={persistSnapshot,snapshotPayload}") {
		t.Fatal("terminal restore must expose immediate snapshot for self update")
	}
}
