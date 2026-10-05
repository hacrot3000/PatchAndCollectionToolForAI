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
