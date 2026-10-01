package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestFileTransferWorkspaceIsDualPaneWithHostAndLocalBrowser(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"ft-sites",
		"dual-pane transfer",
		"['host','Host']",
		"['local','Local browser']",
		"showDirectoryPicker",
		"indexedDB.open",
		"/api/project/tree?path=",
		"/api/file-transfer/host-to-remote",
		"/api/file-transfer/remote-to-host",
		"Transfer selected left file to remote",
		"Transfer selected remote file to left",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing dual-pane contract %q", want)
		}
	}
}

func TestFileTransferWorkspaceKeepsPerScopePathMemoryAndSorting(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"taskdeck:file-transfer:paths:v2:",
		"Favorites",
		"Recent",
		"toggleFavorite",
		"memoryScope",
		"remote:'+profile.id",
		"host:'+workspaceKey()",
		"local:'+panel.localRoot.id",
		"sort:{key:'type',direction:'asc'}",
		"dataset.sortKey",
		"case'size'",
		"case'modified'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing path/sort contract %q", want)
		}
	}
}

func TestFileTransferWorkspaceRowSelectionDoesNotReplaceDoubleClickTarget(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function selectTableEntry(panel,entry,row)",
		"tr.onclick=()=>selectTableEntry(panel,entry,tr)",
		"tr.ondblclick=event=>{event.preventDefault();onDoubleClick(entry);}",
		"tr.oncontextmenu=event=>{event.preventDefault();selectTableEntry(panel,entry,tr);onContextMenu(entry,event);}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing stable row event contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"tr.onclick=()=>{panel.selected=entry;renderTable(",
		"tr.oncontextmenu=event=>{event.preventDefault();panel.selected=entry;renderTable(",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("row selection must not rebuild the table before dblclick/contextmenu: %q", forbidden)
		}
	}
}

func TestFileTransferWorkspaceUsesStructuredTransferAndMutationAPIs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/file-transfer/list",
		"/api/file-transfer/mutate",
		"/api/file-transfer/upload",
		"/api/file-transfer/download-ticket",
		"/api/file-transfer/download?ticket=",
		"app.fetchWithLease('/api/file-transfer/upload'",
		"Directory removal is non-recursive",
		"New Folder",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing structured API contract %q", want)
		}
	}
}
