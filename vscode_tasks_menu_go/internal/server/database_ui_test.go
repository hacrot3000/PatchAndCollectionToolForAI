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
		"if(view.catalog.value)payload.catalog=view.catalog.value",
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
		"classList.add('db-null')",
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
		"const queryTab=createWorkbenchTab(view,firstKey",
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
		"Generate CREATE",
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
		"Discard unsaved changes in ",
		"database.queryHasPendingChanges?.(page.ctx)",
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
		"dataset.selectAll='1'",
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


func TestDatabaseContextSubmenuAllowsSlowPointerTransit(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const CONTEXT_SUBMENU_CLOSE_DELAY_MS=240",
		"const cancelClose=()=>",
		"const scheduleClose=()=>",
		"setTimeout(()=>{submenu.style.display='none';closeTimer=null;},CONTEXT_SUBMENU_CLOSE_DELAY_MS)",
		"function hideSiblingContextSubmenus(container,keepEntry)",
		"entry.onpointerenter=()=>{cancelClose();hideSiblingContextSubmenus(container,entry);if(!button.disabled)positionContextSubmenu(submenu,entry);}",
		"hideSiblingContextSubmenus(container,entry)",
		"entry.onpointerleave=scheduleClose",
		"submenu.onpointerenter=cancelClose",
		"submenu.onpointerleave=scheduleClose",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing delayed submenu hover behavior %q", want)
		}
	}
	if strings.Contains(js, "entry.onpointerleave=()=>{submenu.style.display='none';}") {
		t.Fatal("database submenu must not close immediately while the pointer crosses into the submenu")
	}
}


func TestDatabaseContextMenuUsesViewportHeightBeforeScrolling(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"max-height:90vh",
		"overflow-y:auto",
		"overscroll-behavior:contain",
		"scrollbar-gutter:stable",
		".db-context-submenu{position:fixed",
		"function positionContextSubmenu(submenu,entry)",
		"const anchor=entry.getBoundingClientRect()",
		"window.innerHeight-rect.height-4",
		"menu.addEventListener('scroll',()=>hideSiblingContextSubmenus(menu,null),{passive:true})",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing viewport-aware context menu behavior %q", want)
		}
	}
}


func TestDatabaseQueryResultsCollapseLongTextIntoPopup(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const LONG_TEXT_PREVIEW_LIMIT=160",
		"function isLongTextValue(value)",
		"value.length>LONG_TEXT_PREVIEW_LIMIT",
		"db-long-text-preview-text",
		"open.textContent='…'",
		"function openQueryValueViewer(titleText,value,{editable=false",
		"copy.textContent='Copy all'",
		"area.focus();area.select()",
		"area.setSelectionRange(0,area.value.length)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing long-text query result behavior %q", want)
		}
	}
}


func TestDatabaseWorkbenchEditsLongTextOnlyInPopup(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const LONG_TEXT_PREVIEW_LIMIT=160",
		"function isLongTextColumnType(columnType='')",
		"function shouldUseValuePopup(value,columnType='')",
		"type.includes('text')",
		"db-grid-value-preview-text",
		"open.textContent='…'",
		"const popupOnly=shouldUseValuePopup(value,column.type)",
		"if(popupOnly){",
		"td.classList.add('db-editable','db-popup-editable')",
		"renderGridValuePreview(td,value,{onOpen:openViewer})",
		"columnType:column.type||''",
		"copy.textContent='Copy all'",
		"area.focus();area.select()",
		"area.setSelectionRange(0,area.value.length)",
		"parseEditedValue(area.value,value,columnType)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing long-text popup editing behavior %q", want)
		}
	}
	longTextBranch := strings.Index(js, "const popupOnly=shouldUseValuePopup(value,column.type)")
	inlineEdit := strings.Index(js[longTextBranch:], "td.contentEditable='true'")
	if longTextBranch < 0 || inlineEdit < 0 {
		t.Fatal("database workbench long-text/inline edit branches unavailable")
	}
	blockEnd := strings.Index(js[longTextBranch:], "td.oncontextmenu=")
	if blockEnd < 0 {
		t.Fatal("database workbench cell rendering bounds unavailable")
	}
	cellBlock := js[longTextBranch : longTextBranch+blockEnd]
	if strings.Contains(cellBlock, "if(popupOnly){\n        td.contentEditable='true'") {
		t.Fatal("long-text popup branch must not enable inline contentEditable")
	}
}


func TestDatabaseQueryEditorAppliesSafeEditableSelectChanges(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function queryCellValue(view,result,rowIndex,columnIndex)",
		"function setQueryDirtyCell(view,result,rowIndex,columnIndex,value)",
		"function applyQueryChanges(view,result)",
		"result?.edit?.editable",
		"result?.edit?.row_identities?.[rowIndex]",
		"mutations.push({action:'update',identity,values:Object.fromEntries(changes)})",
		"mutations.push({action:'insert',values:{...values}})",
		"function addQueryRow(view,result)",
		"db-query-new-row",
		"Add row",
		"'mutate_rows'",
		"catalog:edit.catalog||view.catalog.value||''",
		"name:edit.name",
		"Apply changes",
		"Revert",
		"await refreshQueryResult(view)",
		"Discard unsaved query result changes and run again?",
		"JSON.stringify(JSON.parse(text))",
		"db-query-dirty",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing editable query workflow %q", want)
		}
	}
	if strings.Contains(js, "action:'delete',identity") {
		t.Fatal("query editor must not expose delete mutations for arbitrary SELECT results")
	}
}


func TestDatabaseWorkbenchExportsSharedQueryCopyHelpers(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"globalThis.TaskMenuDatabaseWorkbench={",
		"showContextMenu,",
		"copyText,",
		"serializeClipboardData",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing shared query copy helper export %q", want)
		}
	}
}

func TestDatabaseQueryResultsSupportRowSelectionAndCopyMenus(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"querySelectedRows",
		"querySelectionAnchor",
		"function querySelectedRowIndexes(view,result=view.queryResult)",
		"function selectQueryRow(view,result,rowIndex,event={})",
		"event.ctrlKey||event.metaKey",
		"event.shiftKey&&Number.isInteger(view.querySelectionAnchor)",
		"function toggleSelectAllQueryRows(view,result)",
		"data-select-all",
		"Select all rows in current result",
		"Click to select row · Ctrl/Cmd-click multi-select · Shift-click range",
		"function queryCopyMenuItems(view,result)",
		"Copy TXT (without column header)",
		"Copy TXT (with column header)",
		"Copy CSV (without column header)",
		"Copy CSV (with column header)",
		"Copy as JSON",
		"Selected row (1)",
		"Selected rows (",
		"Current result",
		"Copy Value",
		"Copy Column Name",
		"helper.serializeClipboardData",
		"helper.showContextMenu",
		"db-selected",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing query row-selection/copy behavior %q", want)
		}
	}
}


func TestDatabaseQueryEditorUsesVendoredCodeMirrorAutocomplete(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const cmFactory=globalThis.cm6?.load?.()||null",
		"function relationalQueryEditor(view)",
		"view?.meta?.adapter_kind==='mysql'||view?.meta?.adapter_kind==='sqlite'",
		"function queryEditorExtensions(view)",
		"globalThis.cm6.sqlCompletion({dialect,...schema,upperCaseKeywords:true})",
		"function initQueryEditor(view)",
		"cmFactory.textarea(view.editor",
		"view.queryCM.contentDOM.addEventListener('keydown'",
		"event.key==='Enter'",
		"function queryEditorText(view)",
		"function queryEditorExecutionText(view)",
		"const script=queryEditorExecutionText(owner).trim()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing CodeMirror query autocomplete behavior %q", want)
		}
	}
}

func TestDatabaseQueryAutocompleteLoadsTablesAndColumnsFromSchema(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const QUERY_SCHEMA_CONCURRENCY=4",
		"function queryCompletionSchema(view)",
		"view.querySchemaCache?.get(catalog+'\\u0000'+name)||[]",
		"defaultSchema:catalog",
		"const defaultTable=uniqueReferenced.length===1?uniqueReferenced[0]:''",
		"defaultTable",
		"function queryReferencedObjectNames(statement)",
		"function scheduleQuerySchemaReferences(view,text=queryEditorText(view))",
		"setTimeout(()=>loadQuerySchemaReferences(view,text)",
		"async function loadQuerySchemaReferences(view,text)",
		"object?.kind==='table'||object?.kind==='view'",
		"'describe_object'",
		"detail?.columns",
		"async function warmQuerySchema(view)",
		"warmQuerySchema(view).catch",
		"view.querySchemaCache?.clear?.()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing schema autocomplete behavior %q", want)
		}
	}
}

func TestDatabaseWorkbenchRoutesGeneratedSQLIntoQueryEditorAPI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"typeof database.setQueryText==='function'",
		"database.setQueryText(target,text,{focus:false})",
		"typeof database.focusQuery==='function'",
		"database.focusQuery(target)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing CodeMirror query routing %q", want)
		}
	}
}


func TestDatabaseSQLScriptOpenSaveAndImportFallback(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const SQL_SCRIPT_EDIT_LIMIT=2<<20",
		"function sqlScriptNameAllowed(name)",
		"function chooseScriptLocation(titleText)",
		"function chooseHostSQLScript()",
		"async function chooseClientSQLScript()",
		"showOpenFilePicker",
		"input.accept='.sql,.txt,text/plain'",
		"async function openSQLScript(view)",
		"source.size>SQL_SCRIPT_EDIT_LIMIT",
		"switched to streamed SQL import",
		"'/api/db/sessions/'+encodeURIComponent(view.meta.id)+'/import'",
		"function setQueryScriptIdentity(view,source)",
		"function saveSQLScriptToExistingSource(view)",
		"source?.kind==='host'&&source.path",
		"source?.kind==='client'&&source.handle?.createWritable",
		"function saveSQLScript(view)",
		"if(await saveSQLScriptToExistingSource(view))return",
		"function saveSQLScriptToHost(view)",
		"function saveSQLScriptToClient(view)",
		"browser?.chooseFile",
		"fileLabel:'SQL script file name:'",
		"globalThis.TaskMenuDirectoryBrowser",
		"showSaveFilePicker",
		"updateQueryTabIdentity",
		"openSQL.textContent='Open SQL'",
		"saveSQL.textContent='Save SQL'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing SQL script I/O behavior %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("database SQL script UI must remain DOM/textContent-only")
	}
	if strings.Contains(js, "prompt('SQL script file name:'") {
		t.Fatal("host SQL save must use the workspace file browser instead of prompt")
	}
}

func TestDatabaseQueryResultFilterOrderExportAndActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function normalizeQueryFilter(filter)",
		"function queryFilterMatchesRow(view,result,rowIndex,filter)",
		"normalized.logic==='or'?matches.some(Boolean):matches.every(Boolean)",
		"function queryDisplayRowIndexes(view,result=view.queryResult)",
		"function openQueryFilterDialog(view,result)",
		"Filter current query result",
		"Match all (AND)",
		"Match any (OR)",
		"Add condition",
		"view.queryFilter={logic:logic.value==='or'?'or':'and',conditions:filterConditions}",
		"function normalizeQueryOrder(order,columnCount=Number.MAX_SAFE_INTEGER)",
		"function openQueryOrderDialog(view,result)",
		"Order current query result",
		"Add order column",
		"view.queryOrder=orders.length?orders:null",
		"function toggleQueryHeaderOrder(view,result,columnIndex)",
		"view.queryOrder=[{columnIndex,direction:'asc'}]",
		"current.direction==='asc'",
		"function exportQueryData(view,result)",
		"Export query result",
		"function queryGridActionMenuItems(view,result)",
		"helper.gridActionMenuItems({",
		"exportData:()=>exportQueryData(view,result)",
		"refresh:()=>refreshQueryResult(view)",
		"filter:()=>openQueryFilterDialog(view,result)",
		"addRow:()=>addQueryRow(view,result)",
		"addDisabled:!result?.edit?.editable",
		"order:()=>openQueryOrderDialog(view,result)",
		"th.onclick=()=>toggleQueryHeaderOrder(view,result,columnIndex)",
		"helper.serializeClipboardData",
		"save.textContent='Choose save location…'",
		"await saveTextWithLocation('Export query result'",
		"hostFileLabel:'Export file name:'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing query result grid action %q", want)
		}
	}
}


func TestDatabaseDataGridsShareActionMenuBuilder(t *testing.T) {
	workbenchData, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	workbench := string(workbenchData)
	for _, want := range []string{
		"function gridActionMenuItems(actions={})",
		"function gridContextMenuItems(...sections)",
		"{label:'Export data…'",
		"{label:'Refresh'",
		"{label:'Filter…'",
		"{label:'Add row'",
		"{label:'Order…'",
		"function tableGridActionMenuItems(view)",
		"exportData:()=>exportTableData(view)",
		"refresh:()=>{if(confirmDiscardChanges(view))loadData(view).catch(app.showError);}",
		"filter:()=>openFilterDialog(view)",
		"addRow:()=>addGridRow(view)",
		"order:()=>openDataOrderDialog(view)",
		"function exportTableData(view)",
		"function openDataOrderDialog(view)",
		"tableGridActionMenuItems(view)",
		"gridContextMenuItems(",
		"gridActionMenuItems,",
		"gridContextMenuItems,",
	} {
		if !strings.Contains(workbench, want) {
			t.Fatalf("database_workbench.js missing shared data-grid action behavior %q", want)
		}
	}

	queryData, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	query := string(queryData)
	if !strings.Contains(query, "helper.gridActionMenuItems({") {
		t.Fatal("query result must use the shared database grid action menu builder")
	}
	if !strings.Contains(query, "helper.gridContextMenuItems(") {
		t.Fatal("query result must use the shared database grid context menu composer")
	}
	for _, stale := range []string{
		"{label:'Export data…',action:()=>exportQueryData(view,result)}",
		"{label:'Refresh',action:()=>executeQuery(view)}",
		"{label:'Filter…',action:()=>openQueryFilterDialog(view,result)}",
		"{label:'Order…',action:()=>openQueryOrderDialog(view,result)}",
	} {
		if strings.Contains(query, stale) {
			t.Fatalf("database.js must not define a duplicate grid action menu item %q", stale)
		}
	}
}


func TestDatabaseQueryEditorSupportsMultipleStatementsAndResultTabs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function splitSQLStatements(script)",
		"segmentHasCode",
		"if(ch===';'){",
		"function createQueryResultContext(owner,statement,index)",
		"db-query-result-tabs",
		"db-query-result-tab",
		"tab.textContent='Result '+(index+1)",
		"function activateQueryResult(owner,index)",
		"for(let index=0;index<statements.length;index++)",
		"statement:statements[index]",
		"renderResult(ctx,result",
		"renderQueryResultError(ctx,error",
		"firstError=error",
		"if(firstError)throw firstError",
		"function refreshQueryResult(view)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing multi-statement query behavior %q", want)
		}
	}
	if strings.Contains(js, "statement:script,max_rows") {
		t.Fatal("multi-query execution must not send the entire SQL script as one execute request")
	}
}

func TestDatabaseQueryPanelOwnsItsControlsAfterRefactor(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)

	setupStart := strings.Index(js, "function setupQueryPanel(view")
	attachStart := strings.Index(js, "function attachDatabaseView(meta,activate)")
	openProfileOffset := strings.Index(js[attachStart:], "\nasync function openProfile")
	openProfileStart := -1
	if openProfileOffset >= 0 {
		openProfileStart = attachStart + openProfileOffset
	}
	if setupStart < 0 || attachStart < 0 || openProfileStart < 0 {
		t.Fatal("database.js missing query panel or database attach function")
	}
	setupEnd := strings.Index(js[setupStart:], "\nfunction createAdditionalQueryView")
	if setupEnd < 0 {
		t.Fatal("database.js missing setupQueryPanel end marker")
	}
	setup := js[setupStart : setupStart+setupEnd]
	for _, want := range []string{
		"run.onclick=()=>executeQuery(view).catch(app.showError)",
		"openSQL.onclick=()=>openSQLScript(view).catch(app.showError)",
		"saveSQL.onclick=()=>saveSQLScript(view).catch(app.showError)",
		"if(!view.queryCM){",
		"editor.addEventListener('keydown'",
	} {
		if !strings.Contains(setup, want) {
			t.Fatalf("setupQueryPanel missing reusable query control binding %q", want)
		}
	}

	attach := js[attachStart:openProfileStart]
	for _, stale := range []string{
		"run.onclick",
		"openSQL.onclick",
		"saveSQL.onclick",
		"editor.addEventListener",
	} {
		if strings.Contains(attach, stale) {
			t.Fatalf("attachDatabaseView must not reference moved query-local control %q", stale)
		}
	}
}

func TestDatabaseWorkbenchSupportsMultipleQueryTabs(t *testing.T) {
	databaseData, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	databaseJS := string(databaseData)
	for _, want := range []string{
		"function setupQueryPanel(view",
		"function createAdditionalQueryView(root",
		"createQueryView:createAdditionalQueryView",
		"queryHasPendingChanges:queryViewHasPendingChanges",
		"function databaseQueryViews(view)",
		"maxRowsValue:maxRowsValue||root.maxRows?.value||'100'",
	} {
		if !strings.Contains(databaseJS, want) {
			t.Fatalf("database.js missing query-tab factory behavior %q", want)
		}
	}

	workbenchData, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	workbench := string(workbenchData)
	for _, want := range []string{
		"function createQueryPage(view,{",
		"addQuery.className='db-workbench-add-query'",
		".db-workbench-add-query{",
		"margin-left:12px;margin-right:10px",
		"addQuery.textContent='+ Query'",
		"addQuery.title='Create a new query tab'",
		"function isQueryPageKey(key)",
		"wb.tabsBar.insertBefore(button,wb.addQuery)",
		"else wb.tabsBar.append(button)",
		"button.dataset.pageKey=key",
		"querySelectorAll('.db-workbench-tab[data-page-key]')",
		"database.createQueryView(root",
		"panel.classList.add('db-workbench-panel','hidden')",
		"function activeQueryPage(view)",
		"if(active?.mode==='query')return active",
		"page.mode==='query'&&page.ctx&&database.queryHasPendingChanges?.(page.ctx)",
		"function firstQueryPage(view)",
		"function activateWorkbenchFallback(view,preferredIndex=0)",
		"page.ctx.queryCM?.destroy?.()",
	} {
		if !strings.Contains(workbench, want) {
			t.Fatalf("database_workbench.js missing multiple-query-tab behavior %q", want)
		}
	}
	for _, stale := range []string{
		"createWorkbenchTab(view,'query','Query 1',{closable:false})",
		"disabled:key==='query'",
		"if(!page||key==='query')return false",
		"filter(item=>item!=='query')",
		"position:sticky;right:0",
	} {
		if strings.Contains(workbench, stale) {
			t.Fatalf("database_workbench.js still special-cases Query 1 with stale behavior %q", stale)
		}
	}
}


func TestDatabaseQueryTabsPersistAcrossReload(t *testing.T) {
	workbenchData, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	workbench := string(workbenchData)
	for _, want := range []string{
		"const QUERY_TABS_STORAGE_VERSION=1",
		"const QUERY_TABS_SAVE_DELAY_MS=120",
		"function queryTabsStorageKey(view)",
		"'vscode-tasks-menu:db-query-tabs:v1:'",
		"function queryPageSnapshot(page)",
		"database.getQueryText?.(ctx)",
		"maxRows:String(ctx.maxRows?.value||'100')",
		"scriptSource:serializableQuerySource(ctx.scriptSource)",
		"function saveQueryTabsNow(view)",
		"localStorage.setItem(key,JSON.stringify({",
		"activeQueryKey",
		"function readSavedQueryTabs(view)",
		"localStorage.getItem(key)",
		"function applyQuerySnapshot(ctx,snapshot)",
		"database.setQueryText?.(ctx,snapshot.text,{focus:false})",
		"ctx.maxRows.value=String(snapshot.maxRows||'100')",
		"ctx.scriptSource=snapshot.scriptSource||null",
		"scriptSource:serializableQuerySource(item?.scriptSource)",
		"const saved=readSavedQueryTabs(view)",
		"for(const snapshot of snapshots.slice(1))",
		"const restoredActive=saved?.activeQueryKey",
		"scheduleQueryTabsSave(view)",
		"window.addEventListener('pagehide'",
	} {
		if !strings.Contains(workbench, want) {
			t.Fatalf("database_workbench.js missing query-tab persistence behavior %q", want)
		}
	}
	persistStart := strings.Index(workbench, "function queryPageSnapshot(page)")
	persistEnd := strings.Index(workbench, "function applyQuerySnapshot(ctx,snapshot)")
	if persistStart < 0 || persistEnd < 0 || persistEnd <= persistStart {
		t.Fatal("database_workbench.js persistence block bounds are unavailable")
	}
	persistBlock := workbench[persistStart:persistEnd]
	for _, stale := range []string{"queryResult:", "queryResultContexts", "result.rows", "queryDirtyRows", "queryNewRows", "querySelectedRows"} {
		if strings.Contains(persistBlock, stale) {
			t.Fatalf("query-tab persistence must not persist query result state %q", stale)
		}
	}

	databaseData, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	databaseJS := string(databaseData)
	for _, want := range []string{
		"function notifyQueryStateChanged(view)",
		"queryStateChanged?.(view)",
		"notifyQueryStateChanged(view)",
		"maxRows.addEventListener",
	} {
		if !strings.Contains(databaseJS, want) {
			t.Fatalf("database.js missing query persistence trigger %q", want)
		}
	}
}


func TestDatabaseQueryTabsShowOpenedFileIdentity(t *testing.T) {
	workbenchData, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	workbench := string(workbenchData)
	for _, want := range []string{
		"updateQueryTabIdentity(view,{label='',tooltip=''}={})",
		"const text=page.tab?.querySelector?.('.db-workbench-tab-label')",
		"if(text&&label)text.textContent=label",
		"if(page.tab)page.tab.title=tooltip||label||page.tab.title",
		"scheduleQueryTabsSave(root)",
	} {
		if !strings.Contains(workbench, want) {
			t.Fatalf("database_workbench.js missing query file identity behavior %q", want)
		}
	}

	databaseData, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	databaseJS := string(databaseData)
	for _, want := range []string{
		"setQueryScriptIdentity(view,{",
		"kind:source.kind",
		"name:source.name||'query.sql'",
		"path:source.path",
		"label:source.name",
		"tooltip:queryScriptTooltip(source)",
	} {
		if !strings.Contains(databaseJS, want) {
			t.Fatalf("database.js missing opened SQL tab identity behavior %q", want)
		}
	}
}

func TestDatabaseQueryEditorRunsOnlyPartialSelection(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function queryEditorExecutionText(view)",
		"if(to>from&&!(from===0&&to===full.length))return full.slice(from,to)",
		"const script=queryEditorExecutionText(owner).trim()",
		"getQueryExecutionText:queryEditorExecutionText",
		"Enter a database statement or select SQL to run",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing selected-query execution behavior %q", want)
		}
	}
	if strings.Contains(js, "const script=queryEditorText(owner).trim()") {
		t.Fatal("executeQuery must use the selected-query execution helper, not always the full editor text")
	}
}


func TestDatabaseObjectBrowserIsResizableAndWrapsWhenNarrow(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const DB_BROWSER_WIDTH_STORAGE_KEY='taskdeck:database-browser-width'",
		"const DB_BROWSER_DEFAULT_WIDTH=280",
		"const DB_BROWSER_MIN_WIDTH=160",
		"const DB_BROWSER_MAX_WIDTH=720",
		"const DB_BROWSER_NARROW_WIDTH=330",
		".db-pane-body.db-browser-resizable{grid-template-columns:var(--db-browser-width,280px) 6px minmax(0,1fr)}",
		".db-browser-resizer{width:6px",
		"cursor:col-resize",
		".db-browser.db-browser-narrow .db-browser-head{display:grid;grid-template-columns:minmax(0,1fr) auto",
		".db-browser.db-browser-narrow .db-browser-head>select{grid-column:1;grid-row:1",
		".db-browser.db-browser-narrow .db-browser-head>.db-browser-filter{grid-column:1;grid-row:2",
		"function installDatabaseBrowserResizer(view,body,main)",
		"localStorage.setItem(DB_BROWSER_WIDTH_STORAGE_KEY",
		"browser.classList.toggle('db-browser-narrow'",
		"resizer.addEventListener('pointerdown'",
		"resizer.addEventListener('pointermove'",
		"resizer.addEventListener('dblclick'",
		"installDatabaseBrowserResizer(view,body,main)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing resizable object browser behavior %q", want)
		}
	}
}


func TestDatabaseObjectDetailRendersReadableStructureText(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function objectDetailScalar(value)",
		"function objectDetailSection(container,titleText)",
		"function objectDetailLine(section,text",
		"function objectDetailGeneric(section,key,value,depth=0)",
		"function renderObjectDetail(view,object,detail)",
		"'Columns ('+columns.length+')'",
		"'Indexes / Keys ('+indexes.length+')'",
		"'Definition'",
		"'Additional info'",
		"'Database: '+location",
		"parts.push(nullable?'NULL':'NOT NULL')",
		"parts.push('DEFAULT '+objectDetailScalar(column.default))",
		"flags.add('PK')",
		"flags.add('UNIQUE')",
		"db-object-detail-sql",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing readable object-detail rendering %q", want)
		}
	}
	if strings.Contains(js, "view.detail.textContent=JSON.stringify(detail,null,2)") {
		t.Fatal("database object detail must not render raw JSON")
	}
}


func TestDatabaseOpenSQLPreservesEditedTabsAndGeneratedSQLAppends(t *testing.T) {
	databaseData, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	databaseJS := string(databaseData)
	for _, want := range []string{
		"async function chooseClientSQLScript()",
		"function queryEditorCanReplace(view)",
		"return current===''||current===defaultText",
		"function appendQueryEditorText(view,text",
		"const separator=current.endsWith('\\n')?'\\n':'\\n\\n'",
		"const target=globalThis.TaskMenuDatabaseWorkbench?.queryTargetForOpen?.(view)||view",
		"setQueryEditorText(target,text)",
		"setQueryScriptIdentity(target,{",
		"queryCanReplace:queryEditorCanReplace",
		"appendQueryText:appendQueryEditorText",
	} {
		if !strings.Contains(databaseJS, want) {
			t.Fatalf("database.js missing safe Open SQL/query append behavior %q", want)
		}
	}

	workbenchData, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	workbench := string(workbenchData)
	for _, want := range []string{
		"function queryTargetForOpen(view)",
		"if(database.queryCanReplace?.(view))",
		"createQueryPage(root,{initialText:'',activate:true})",
		"typeof database.appendQueryText==='function'",
		"database.appendQueryText(target,text,{focus:false})",
		"case 'create': {",
		"const ddl=String(detail?.sql||'').trim()",
		"action==='insert'||action==='update'||action==='create'",
		"Generate CREATE",
		"generatedQuery(view,object,'create')",
		"queryTargetForOpen,",
	} {
		if !strings.Contains(workbench, want) {
			t.Fatalf("database_workbench.js missing generated/open SQL behavior %q", want)
		}
	}
}


func TestDatabaseQueryResultStickyHeadersUseSolidBackgrounds(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		".db-query{min-width:0;display:flex;flex-direction:column;background:#090c10}",
		".db-query-tools{display:flex;align-items:center;gap:6px;padding:7px;border-bottom:1px solid #30343b;background:#11151b}",
		".db-query .codemirror .cm-editor{height:100%;font-size:13px;background:#090c10}",
		".db-query .codemirror .cm-scroller{overflow:auto;font-family:ui-monospace,SFMono-Regular,Consolas,\"Liberation Mono\",monospace;background:#090c10}",
		".db-query .codemirror .cm-gutters{background:#090c10}",
		".db-result-wrap{--db-result-tabs-height:0px;--db-result-status-height:28px;",
		".db-result-wrap.has-result-tabs{--db-result-tabs-height:30px}",
		".db-query-result-tabs{display:flex;align-items:end;gap:2px;height:30px;box-sizing:border-box;",
		".db-result-status{position:sticky;top:var(--db-result-tabs-height);z-index:9;height:var(--db-result-status-height);",
		".db-result-table{border-collapse:separate;border-spacing:0;",
		".db-result-table th{position:sticky;top:calc(var(--db-result-tabs-height) + var(--db-result-status-height));",
		".db-result-edit-tools{position:relative;z-index:1;",
		".db-result-table th.db-row-number{z-index:11;",
		"owner.result.classList.add('has-result-tabs')",
		"owner.result.classList.remove('has-result-tabs')",
		"html[data-taskmenu-theme=\"light\"] .db-result-wrap{background:#fff}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing solid/sticky query result styling %q", want)
		}
	}
	for _, stale := range []string{
		".db-result-edit-tools{position:sticky",
		".has-edit-tools .db-result-table th{top:58px}",
		".db-query-result-panel .db-result-edit-tools{top:57px}",
		".db-query-result-panel.has-edit-tools .db-result-table th{top:88px}",
		".db-result-table{border-collapse:collapse",
		"opacity:.55;background:#11151b",
	} {
		if strings.Contains(js, stale) {
			t.Fatalf("database.js must not retain stale overlapping sticky styling %q", stale)
		}
	}
}


func TestDatabaseQueryResultOrdersByMultipleColumns(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const orders=normalizeQueryOrder(view.queryOrder,columns.length)",
		"for(const order of orders)",
		"if(compared!==0)return compared*(order.direction==='desc'?-1:1)",
		"const rows=[]",
		"const addOrder=(order={})=>",
		"add.textContent='Add order column'",
		"rows.map(row=>({columnIndex:Number(row.column.value),direction:row.direction.value==='desc'?'desc':'asc'}))",
		"const activeOrders=normalizeQueryOrder(view.queryOrder,columns.length)",
		"const orderIndex=activeOrders.findIndex(order=>order.columnIndex===columnIndex)",
		"(activeOrders.length>1?String(orderIndex+1):'')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing multi-column query ordering behavior %q", want)
		}
	}
}


func TestDatabaseWorkbenchTabsSupportReadOnlyToggle(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database_workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"typeof item.checked==='boolean'",
		"checkbox.type='checkbox'",
		"function setWorkbenchPageReadOnly",
		"{label:'Read only',checked:Boolean(page.readOnly)",
		"page.panel.dataset.taskdeckReadonly='1'",
		"database.setQueryTabReadOnly?.(page.ctx,enabled)",
		"readOnly:Boolean(page.readOnly)",
		"readOnly:Boolean(item?.readOnly)",
		"readOnly:snapshot.readOnly",
		"setPageReadOnly:setWorkbenchPageReadOnly",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database_workbench.js missing workbench-tab read-only behavior %q", want)
		}
	}
}

func TestDatabaseQueryCancelAndExplainUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function sessionRequest(id,operation,payload,options={})",
		"if(options?.signal)requestOptions.signal=options.signal",
		"const controller=new AbortController()",
		"owner.queryAbortController=controller",
		"supports(owner,'cancel')",
		"owner.queryAbortController.abort()",
		"function renderQueryCanceled(view,elapsed)",
		"status.textContent='CANCELED'",
		"function explainStatementFor(view,statement,analyze)",
		"'EXPLAIN QUERY PLAN '+statement",
		"(analyze?'EXPLAIN ANALYZE ':'EXPLAIN ')+statement",
		"Explain Analyze is limited to SELECT/WITH statements",
		"async function explainQuery(view,analyze=false)",
		"explain.textContent=view.meta.adapter_kind==='sqlite'?'Explain Plan':'Explain'",
		"explainAnalyze.textContent='Explain Analyze'",
		"explainAnalyze.hidden=view.meta.adapter_kind!=='mysql'",
		"renderQueryCanceled(ctx,elapsed)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("database.js missing query cancel/explain behavior %q", want)
		}
	}
	if strings.Contains(js, "owner.run.disabled=true;owner.run.textContent='Running") {
		t.Fatal("query execution must allow the Run button to become Cancel when adapter supports cancellation")
	}
}
