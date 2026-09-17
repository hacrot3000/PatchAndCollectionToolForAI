package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestRestartAndClearConsoleControls(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/restartclear.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"session-force-restart",
		"restart.textContent='Restart'",
		"/api/sessions/force-kill",
		"waitUntilStopped",
		"will not be restarted to avoid overlapping processes",
		"session-clear-console",
		"🧹 Clear console",
		"window.confirm('Clear this console and server-side scrollback?",
		"/api/sessions/clear-console",
		"view.term.clear()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("restartclear.js missing behavior %q", want)
		}
	}
	if !strings.Contains(indexHTML, `/featuremods/next.js`) {
		t.Fatalf("index HTML must load feature module chain")
	}
}
