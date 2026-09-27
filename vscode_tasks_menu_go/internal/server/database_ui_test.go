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


func TestDatabaseWorkbenchProvidesNavigatorContextMenuAndPagedGrid(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"db-workbench-tabs",
		"makeTab('data','Data')",
		"makeTab('structure','Structure')",
		"makeTab('query','Query')",
		"Filter objects…",
		"button.addEventListener('dblclick'",
		"button.addEventListener('contextmenu'",
		"View Data",
		"Inspect / Structure",
		"Copy Qualified Name",
		"Generate SELECT",
		"Generate INSERT",
		"Generate UPDATE",
		"Generate DELETE",
		"Count Rows",
		"'browse_rows'",
		"offset:state.offset",
		"limit:state.limit",
		"sort:state.sort",
		"[25,50,100,250,500,1000]",
		"result.has_more",
		"Open Value in Editor",
		"Copy Row as JSON",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML=") {
		t.Fatal("database workbench must not render database content through innerHTML")
	}
}

func TestDatabaseWorkbenchLoadsImmediatelyAfterDatabaseCore(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	core := strings.Index(js, "database.js")
	workbench := strings.Index(js, "database_workbench.js")
	connections := strings.Index(js, "connections.js")
	if core < 0 || workbench < 0 || connections < 0 || !(core < workbench && workbench < connections) {
		t.Fatalf("unexpected database feature load order: core=%d workbench=%d connections=%d", core, workbench, connections)
	}
}


func TestDatabaseWorkbenchSupportsInlineGridEditing(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"dirtyRows:new Map()",
		"deletedRows:new Set()",
		"newRows:[]",
		"contentEditable='true'",
		"Apply changes",
		"Revert",
		"+ Row",
		"'mutate_rows'",
		"action:'insert'",
		"action:'update'",
		"action:'delete'",
		"Set NULL",
		"Delete Row",
		"Discard unsaved database grid changes?",
		"Truncate Table",
		"Type \"'+object.name+'\" to confirm dropping",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing inline-edit workflow %q", want)
		}
	}
}
