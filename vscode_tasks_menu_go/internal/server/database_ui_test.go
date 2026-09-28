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
		"app.jsonFetch('/api/db/test'",
		"async function testDraft(profileID,profile)",
		"testDraft,",
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
		"createWorkbenchTab(view,'query','Query',{closable:false})",
		"object.name+' - Data'",
		"object.name+' - Structure'",
		"ensureDataPage",
		"ensureStructurePage",
		"closeWorkbenchPage",
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
		"Copy as JSON",
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
		"const typed=prompt('Type \"'+object.name+'\" to confirm '+verb",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing inline-edit workflow %q", want)
		}
	}
}


func TestDatabaseWorkbenchExposesMongoCollectionActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"kind==='mongo'",
		"Count Documents",
		"Clear Collection",
		"Drop Collection",
		"Generate Find",
		"collection:object.name",
		"filter:{}",
		"limit:100",
		"All documents will be permanently removed",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing MongoDB UI workflow %q", want)
		}
	}
}


func TestDatabaseWorkbenchProvidesServerSideRelationalFilters(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function openFilterDialog(view)",
		"filters:state.filters",
		"Up to 16 conditions are combined with AND",
		"contains",
		"starts_with",
		"is_null",
		"not_null",
		"state.filters=filters.slice(0,16)",
		"kind==='mysql'||kind==='sqlite'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing relational filter workflow %q", want)
		}
	}
}


func TestDatabaseWorkbenchRendersInspectorColumnsAndIndexes(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Indexes / Keys",
		"Collection Info",
		"column?.primary_key?'PK':''",
		"typeof column?.not_null==='boolean'?!column.not_null:true",
		"index?.column_name??index?.key??''",
		"detail?.detail?.indexes",
		"Definition",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing inspector workflow %q", want)
		}
	}
}


func TestDatabaseWorkbenchOpensRedisKeysInTypeAwareViewer(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"['table','view','collection','key']",
		"View Value",
		"Generate Read Command",
		"GET '+key",
		"HGETALL '+key",
		"LRANGE '+key+' 0 99",
		"SMEMBERS '+key",
		"ZRANGE '+key+' 0 99 WITHSCORES",
		"Count Entries",
		"Clear Key",
		"Delete Key",
		"isRedisKey||!writable||isView",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing Redis viewer workflow %q", want)
		}
	}
}


func TestDatabaseWorkbenchPersistsPageSizeAndProvidesGridShortcuts(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"taskdeck.db.pageSize.",
		"localStorage.getItem",
		"localStorage.setItem",
		"state.limit=storedPageSize(root,object)",
		"storePageSize(view,state.object,state.limit)",
		"event.key.toLowerCase()==='s'",
		"applyGridChanges(view)",
		"event.altKey&&(event.key==='Insert'||event.key.toLowerCase()==='n')",
		"event.key==='Enter'&&!event.shiftKey",
		"event.key==='Escape'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing grid preference/shortcut workflow %q", want)
		}
	}
}


func TestDatabaseWorkbenchLoadsTotalCountOnDemand(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function loadTotalCount(view)",
		"action:'count_rows'",
		"state.result.total_rows=count",
		"count.textContent='Count'",
		"count.disabled=!supports(view,'object_actions')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing on-demand count workflow %q", want)
		}
	}
}


func TestDatabaseWorkbenchHandlesCapabilityLoadingAndEmptyStates(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function loadData(view",
		"await ensureAdapters()",
		"refreshWorkbenchCapabilities(view)",
		"status.dataset.base='Loading…'",
		"status.dataset.base='Error · '",
		"No rows match the current filters.",
		"This object contains no rows/documents/entries.",
		"busy:false",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing loading/capability state %q", want)
		}
	}
}


func TestDatabaseWorkbenchRejectsIncompleteMutationResponses(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"items.length!==mutations.length",
		"Database adapter returned an incomplete mutation result; data was reloaded",
		"await loadData(view)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing mutation-response guard %q", want)
		}
	}
}


func TestDatabaseWorkspaceConstrainsScrollableObjectListAndDataGrid(t *testing.T) {
	coreData, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	core := string(coreData)
	for _, want := range []string{
		".db-pane-body{flex:1;min-height:0;",
		".db-browser{min-width:0;min-height:0;overflow:hidden;",
		".db-browser-objects{flex:1;min-height:0;overflow:auto;",
	} {
		if !strings.Contains(core, want) {
			t.Fatalf("database.js missing scroll containment %q", want)
		}
	}

	workbenchData, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	workbench := string(workbenchData)
	for _, want := range []string{
		".db-main{min-width:0;min-height:0;overflow:hidden;",
		".db-data{display:flex;flex-direction:column;min-width:0;min-height:0;overflow:hidden}",
		".db-data-grid-wrap{flex:1;min-width:0;min-height:0;overflow:auto;scrollbar-gutter:stable}",
	} {
		if !strings.Contains(workbench, want) {
			t.Fatalf("database_workbench.js missing data-grid scroll containment %q", want)
		}
	}
}


func TestDatabaseWorkbenchKeepsIndependentObjectTabs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"pages:new Map()",
		"workbenchObjectPageKey('data',object)",
		"workbenchObjectPageKey('structure',object)",
		"const ctx=createWorkbenchChildView(root,'data')",
		"const page={key,mode:'data',object,ctx,tab,panel}",
		"const page={key,mode:'structure',object,ctx,tab,panel}",
		"const page=ensureDataPage(root,object)",
		"activatePanel(root,page.key)",
		"if(state.result||state.busy)return",
		"page.mode==='data'&&page.ctx",
		"overflow-x:auto;overflow-y:hidden",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing independent object-tab behavior %q", want)
		}
	}
}


func TestDatabaseWorkbenchTabContextMenuCloseActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"button.oncontextmenu=event=>",
		"showWorkbenchTabMenu",
		"Close this",
		"Close all but this",
		"Close all right tabs",
		"Close all left tabs",
		"closeWorkbenchPages",
		"Discard unsaved database grid changes in ",
		"page.mode==='query'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing tab context close action %q", want)
		}
	}
}

func TestDatabaseWorkbenchRowSelectionAndCopyDataOptions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"selectedRows:new Set()",
		"selectionAnchor:null",
		"function selectGridRow",
		"event.ctrlKey||event.metaKey",
		"event.shiftKey&&Number.isInteger(state.selectionAnchor)",
		"function toggleSelectAllPage",
		"data-select-all",
		"Select all rows on this page",
		"db-selected",
		"function appendContextMenuItems",
		"item.submenu",
		"has-submenu",
		"Copy TXT (without column header)",
		"Copy TXT (with column header)",
		"Copy CSV (without column header)",
		"Copy CSV (with column header)",
		"Copy as JSON",
		"Selected row (1)",
		"Selected rows (",
		"Current page",
		"All pages",
		"copyScopeMenuItems(view,'json',true)",
		"const MAX_COPY_ALL_ROWS=100000",
		"limit:1000",
		"sort:state.sort",
		"filters:state.filters",
		"Copy all pages uses saved database values and excludes unsaved grid changes.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing row-selection/copy behavior %q", want)
		}
	}
	for _, obsolete := range []string{
		"Copy Data…",
		"openCopyDataDialog",
		"Copy Row as JSON",
	} {
		if strings.Contains(js, obsolete) {
			t.Fatalf("database_workbench.js still contains obsolete copy UI %q", obsolete)
		}
	}
}
