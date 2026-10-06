package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestConnectionGraphRuntimeMetadataContract(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/connection_graph.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"CONNECTION / TUNNEL GRAPH",
		"Browser",
		"TaskDeck",
		"SSH host",
		"Tunnel",
		"Database",
		"Tunnel PID",
		"Tunnel local port",
		"Tunnel remote",
		"Tunnel started",
		"database.reconnect?.(item.view)",
		"connections.testSSHProfile?.(String(item.profile.id))",
		"transfers?.testProfile?.(String(item.profile.id))",
		"transfers?.openProfile?.(String(item.profile.id))",
		"setInterval(()=>{if(backdrop.classList.contains('visible'))refresh();},2000)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("connection_graph.js missing %q", want)
		}
	}
	for _, forbidden := range []string{"password", "secret_ref", "private_key"} {
		if strings.Contains(strings.ToLower(js), forbidden) {
			t.Fatalf("connection graph must not expose credential material marker %q", forbidden)
		}
	}
}

func TestConnectionGraphLauncherAndLoadOrder(t *testing.T) {
	connectionsData, err := webassets.Files.ReadFile("featuremods/connections.js")
	if err != nil {
		t.Fatal(err)
	}
	connectionsJS := string(connectionsData)
	for _, want := range []string{
		"graph.textContent='Graph'",
		"TaskMenuConnectionGraph?.open?.()",
		"async testSSHProfile(id)",
	} {
		if !strings.Contains(connectionsJS, want) {
			t.Fatalf("connections.js missing graph bridge %q", want)
		}
	}

	nextData, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	next := string(nextData)
	fileTransfer := strings.Index(next, "filetransfer.js")
	connections := strings.Index(next, "connections.js")
	graph := strings.Index(next, "connection_graph.js")
	if fileTransfer < 0 || connections < 0 || graph < 0 || graph < fileTransfer || graph < connections {
		t.Fatalf("connection_graph.js must load after File Transfer and Connections: %q", next)
	}
}

func TestDatabaseReconnectProtectsPendingAndTransactionState(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function reconnectDatabaseView(viewOrID)",
		"Reconnect will discard local Data Grid / query-result edits",
		"Reconnect will close the active database transaction",
		"await closeDatabaseView(String(view.meta.id))",
		"return openProfile(profileID)",
		"reconnect:reconnectDatabaseView",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database reconnect safety contract missing %q", want)
		}
	}
}
