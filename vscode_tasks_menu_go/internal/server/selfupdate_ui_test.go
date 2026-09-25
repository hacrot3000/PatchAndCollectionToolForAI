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
		"persistSnapshot",
		"resumeAfterSelfUpdate",
		"nonblocking",
		"Close",
		"postAction('ack',id)",
		"action='+encodeURIComponent(action)",
		"postAction('ack',req.id)",
		"awaiting_confirmation",
		"completed",
		"failed",
		"target_url",
		"location.replace",
		"Waiting for the new daemon to start",
		"Update completed. The new daemon is ready.",
		"self-update-check",
		"Check update",
		"endpoint+'&action=check'",
		"endpoint+'&action=start'",
		"checkAndStartUpdate",
		"freezeForSelfUpdate",
		"startingFromSettings",
		"Automatic self-update did not start.",
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
	restoreJS := string(terminalRestore)
	for _, want := range []string{"TaskMenuTerminalRestore=", "persistSnapshot", "snapshotPayload", "freezeForSelfUpdate", "resumeAfterSelfUpdate"} {
		if !strings.Contains(restoreJS, want) {
			t.Fatalf("terminal restore API missing %q", want)
		}
	}
}

func TestSelfUpdateRemoteBrowserKeepsReachableOrigin(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/selfupdate.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function isLoopbackHostname(hostname)",
		"function redirectTargetForUpdate(req)",
		"host.startsWith('127.')",
		"if(isLoopbackHostname(target.hostname)&&!isLoopbackHostname(here.hostname))return here;",
		"const url=redirectTargetForUpdate(req);",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("self-update remote redirect protection missing %q", want)
		}
	}
	if strings.Contains(js, "const target=String(req.target_url||location.origin)") {
		t.Fatal("self-update must not blindly replace the browser origin with daemon target_url")
	}
}
