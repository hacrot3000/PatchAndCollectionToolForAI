package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestDatabaseWorkspaceUsesGenericSessionAPIs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"app.jsonFetch('/api/db/profiles')",
		"app.jsonFetch('/api/db/sessions'",
		"'/request'",
		"'list_catalogs'",
		"'list_objects'",
		"'describe_object'",
		"'execute'",
		"max_rows:maxRows",
		"Math.min(1000",
		"app.activateExternalView('database:'",
		"DB · ",
		"event.ctrlKey",
		"meta.adapter_kind==='redis'",
		"meta.adapter_kind==='mongo'",
		"\"op\": \"find\"",
		"\"collection\": \"users\"",
		"view.meta.adapter_kind==='redis'?'No keys'",
		"view.meta.adapter_kind==='mongo'?'No collections':'No tables or views'",
		"TaskMenuDatabaseWorkbench?.bindObject?.(view,object,button)",
		"TaskMenuDatabaseWorkbench?.enhanceView?.(view)",
		"request:sessionRequest",
		"getProfile:profileFor",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing %q", want)
		}
	}
}

func TestDatabaseWorkspaceRendersResultsWithoutInnerHTML(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"document.createElement('table')",
		"textContent=String(value)",
		"JSON.stringify(value)",
		"className='db-null'",
		"result?.truncated",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing safe result rendering %q", want)
		}
	}
	if strings.Contains(js, "innerHTML=") {
		t.Fatal("database workspace must not render database values through innerHTML")
	}
}

func TestDatabaseWorkspaceTestConnectionAlwaysClosesTemporarySession(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function testProfile",
		"await sessionRequest(meta.id,'ping')",
		"finally",
		"method:'DELETE'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database connection test cleanup missing %q", want)
		}
	}
}

func TestDatabaseFeatureLoadsBeforeConnections(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	database := strings.Index(js, "database.js")
	connections := strings.Index(js, "connections.js")
	if database < 0 || connections < 0 || database > connections {
		t.Fatalf("database feature must load before Connections: database=%d connections=%d", database, connections)
	}
}
