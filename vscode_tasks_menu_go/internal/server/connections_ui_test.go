package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestConnectionsPanelUsesExistingTerminalSessionsAndSafeProfileAPI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/connections.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"task-connections-panel",
		"app.jsonFetch('/api/ssh/profiles')",
		"app.jsonFetch('/api/config/terminal-cwds')",
		"body:JSON.stringify({kind:'terminal',...payload})",
		"app.materializeSession(meta,true)",
		"ssh_profile_id:profile.id",
		"cwd",
		"Custom remote home directory",
		"Preset commands",
		"ProxyJump",
		"Password / passphrase",
		"payload.secret=secret.input.value",
		"has_secret",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("connections.js missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"profile.secret_ref",
		"profile.secret",
		"innerHTML=profile",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("connections.js must not expose stored SSH secrets through %q", forbidden)
		}
	}
}

func TestConnectionsPanelIsLoadedBeforeActivityBar(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	connections := strings.Index(js, "connections.js")
	activity := strings.Index(js, "activitybar.js")
	if connections < 0 || activity < 0 || connections > activity {
		t.Fatalf("Connections must load before Activity Bar: connections=%d activity=%d", connections, activity)
	}
}

func TestActivityBarIntegratesConnectionsView(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"makeButton('connections','Connections'",
		"function showConnections()",
		"globalThis.TaskMenuConnections?.open()",
		"globalThis.TaskMenuConnections?.close()",
		"activeView==='connections'",
		"TaskMenuConnections?.panel?.contains(target)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("activitybar.js missing Connections integration %q", want)
		}
	}
}

func TestTerminalCWDInjectionPreservesExplicitConnectionTarget(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalcwd.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"!payload.ssh_profile_id",
		"!Object.prototype.hasOwnProperty.call(payload,'cwd')",
		"cwd:selectedCWD()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminalcwd.js missing explicit-target protection %q", want)
		}
	}
}
