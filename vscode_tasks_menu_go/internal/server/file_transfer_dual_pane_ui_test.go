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
		"function selectTableEntry(panel,entry,event={},mode='click')",
		"tr.onclick=event=>selectTableEntry(panel,entry,event)",
		"tr.ondblclick=event=>{event.preventDefault();onDoubleClick(entry);}",
		"tr.oncontextmenu=event=>{event.preventDefault();event.stopPropagation();selectTableEntry(panel,entry,event,'context');onContextMenu(entry,event);}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing stable row event contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"tr.onclick=()=>{panel.selected=entry;renderTable(",
		"tr.oncontextmenu=event=>{event.preventDefault();panel.selected=entry;renderTable(",
		"tr.onclick=event=>{panel.selected=entry;renderTable(",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("row selection must not rebuild the table before dblclick/contextmenu: %q", forbidden)
		}
	}
}


func TestFileTransferWorkspaceSupportsMultiSelectAndContextActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"selectedKeys:new Set()",
		"event.ctrlKey||event.metaKey",
		"event.shiftKey&&panel.selectionAnchor",
		"selectAll.type='checkbox'",
		"selectAllEntries(panel)",
		"tr.oncontextmenu=event=>{event.preventDefault();event.stopPropagation();selectTableEntry(panel,entry,event,'context');onContextMenu(entry,event);}",
		"if(!keys.has(key)){keys.clear();keys.add(key);}",
		"Upload '+(files.length>1?files.length+' selected files':'to remote')+' →",
		"Transfer '+(files.length>1?files.length+' selected files':'to left')+' ←",
		"Delete '+(selected.length>1?selected.length+' selected items':'item')",
		"Open folder",
		"Copy selected paths",
		"Select all",
		"Clear selection",
		"New remote folder",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing multi-select/context-menu contract %q", want)
		}
	}
}

func TestFileTransferContextMenuAlwaysHasItemActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function showContextMenu(items,x,y,title='')",
		"menu.setAttribute('role','menu')",
		"button.setAttribute('role','menuitem')",
		"function leftContext(view,entry,event)",
		"function remoteContext(view,entry,event)",
		"showContextMenu(items,event.clientX,event.clientY,contextTitle(panel,entry))",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing context menu contract %q", want)
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
