package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestWorkspaceSnapshotFrontendStateAdapters(t *testing.T) {
	tests := []struct{ file string; required []string }{
		{file:"featuremods/editor.js",required:[]string{"snapshotState(){","async restoreState(state)","return {files,active:","await openFile(pathValue)"}},
		{file:"featuremods/explorer.js",required:[]string{"snapshotState(){return {expanded:[...expanded]};}","async restoreState(state)","expanded.clear()","await restoreExpandedDirectories()"}},
		{file:"featuremods/gitstatus.js",required:[]string{"snapshotState(){","repository_id:String(activeRepoID||'')","async restoreState(state)","await selectRepository(id,{reload:false,persist:true})"}},
		{file:"featuremods/tabdrag.js",required:[]string{"TaskMenuTabOrder={saveOrder,restoreSavedOrder,currentIDs,applyOrder}"}},
		{file:"featuremods/database_workbench.js",required:[]string{"function workspaceQuerySnapshot(view)","function restoreWorkspaceQuerySnapshot(view,snapshot)","snapshotQueries:workspaceQuerySnapshot","restoreQueries:restoreWorkspaceQuerySnapshot"}},
		{file:"featuremods/database.js",required:[]string{"snapshotState(){","active_query_key:String(state.activeQueryKey||'')","async restoreState(items)","restoreQueries?.(view"}},
		{file:"featuremods/filetransfer.js",required:[]string{"snapshotState(){","local_mode:String(view.left?.source||'host')","async restoreState(items)","await loadRemoteDirectory(view,remotePath)","get views(){return views;}"}} ,
	}
	for _, tc := range tests {
		data, err := webassets.Files.ReadFile(tc.file)
		if err != nil { t.Fatal(err) }
		js := string(data)
		for _, want := range tc.required {
			if !strings.Contains(js,want) { t.Fatalf("%s missing snapshot adapter %q",tc.file,want) }
		}
	}
}

func TestWorkspaceSnapshotManagerUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/workspacesnapshots.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"Workspace snapshots",
		"Save current",
		"async function captureState()",
		"TaskMenuTerminalRestore?.persistSnapshot?.()",
		"semanticTabState()",
		"terminal:",
		"editorToken(view?.file?.path)",
		"databaseToken(profile,index)",
		"transferToken(profile)",
		"TaskMenuDatabase?.snapshotState?.()",
		"TaskMenuFileTransfer?.snapshotState?.()",
		"TaskMenuGitFiles?.snapshotState?.()",
		"async function saveSnapshot(id='',existingName='')",
		"method:id?'PUT':'POST'",
		"async function deleteSnapshot(item)",
		"async function restoreSnapshot(item)",
		"TaskMenuEditor?.restoreState?.",
		"TaskMenuExplorer?.restoreState?.",
		"TaskMenuDatabase?.restoreState?.",
		"TaskMenuFileTransfer?.restoreState?.",
		"TaskMenuGitFiles?.restoreState?.",
		"TaskMenuTabOrder?.applyOrder?.",
		"Ctrl+Alt+S",
		"TaskMenuWorkspaceSnapshots=",
	} {
		if !strings.Contains(js,want) { t.Fatalf("workspacesnapshots.js missing %q",want) }
	}
	if strings.Contains(js,"innerHTML") || strings.Contains(js,"eval(") || strings.Contains(js,"new Function(") {
		t.Fatal("workspace snapshot UI must not use dynamic HTML or eval")
	}
}

func TestWorkspaceSnapshotModuleLoadsAfterActivityBar(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	activity := strings.Index(js,"featuremods/activitybar.js")
	snapshot := strings.Index(js,"featuremods/workspacesnapshots.js")
	if activity < 0 || snapshot < 0 || snapshot < activity {
		t.Fatalf("load order activity=%d snapshot=%d",activity,snapshot)
	}
}

func TestWorkspaceSnapshotTerminalRestoreContracts(t *testing.T) {
	terminalData, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil { t.Fatal(err) }
	snapshotData, err := webassets.Files.ReadFile("featuremods/workspacesnapshots.js")
	if err != nil { t.Fatal(err) }
	terminalJS := string(terminalData)
	for _, want := range []string{
		"function snapshotWorkspaceRelativeCwd(value)",
		"async function restoreWorkspaceSnapshotTerminals(saved)",
		"body:JSON.stringify({kind:'terminal',cwd})",
		"idsBySnapshot=Array(items.length).fill('')",
		"restoredItems=items.map(item=>({...item,session_id:''}))",
		"TaskMenuBroadcast.assign(view,groupID)",
		"applySavedLayout(restored,ids,{clearMissing:true})",
		"restoreSnapshotState:restoreWorkspaceSnapshotTerminals",
	} {
		if !strings.Contains(terminalJS,want) { t.Fatalf("terminalrestore.js missing snapshot restore contract %q",want) }
	}
	snapshotJS := string(snapshotData)
	for _, want := range []string{
		"TaskMenuTerminalRestore?.restoreSnapshotState?.(item?.terminal)",
		"const terminalIDs=Array.isArray(terminalRestore?.session_ids)?terminalRestore.session_ids:[]",
		"resolvedTabIDs(state,terminalIDs)",
		"restoreActiveTab(state.active_tab,terminalIDs)",
		"Saved local terminals are recreated with their CWD/title/split layout.",
	} {
		if !strings.Contains(snapshotJS,want) { t.Fatalf("workspacesnapshots.js missing terminal restore integration %q",want) }
	}
}
