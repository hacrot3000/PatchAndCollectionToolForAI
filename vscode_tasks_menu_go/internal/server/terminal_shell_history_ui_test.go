package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestShellHistoryUIContracts(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/shellhistory.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"Terminal command history",
		"Copy command",
		"Copy output",
		"Rerun",
		"exitCode",
		"item.cwd",
		"taskmenu:shell-integration",
		"TaskDeckShellHistory",
		"view.ws.send(command+'\\r')",
	} {
		if !strings.Contains(js, want) { t.Fatalf("shellhistory.js missing %q", want) }
	}
	for _, forbidden := range []string{"innerHTML", "eval(", "new Function("} {
		if strings.Contains(js, forbidden) { t.Fatalf("shellhistory.js uses forbidden %q", forbidden) }
	}
}

func TestShellHistoryLoadsAfterShellIntegration(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	integration := strings.Index(js, "featuremods/shellintegration.js")
	history := strings.Index(js, "featuremods/shellhistory.js")
	if integration < 0 || history < 0 || history < integration {
		t.Fatalf("load order integration=%d history=%d", integration, history)
	}
}
