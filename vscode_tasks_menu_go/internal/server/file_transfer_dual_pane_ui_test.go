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
		"Upload selected left item(s) to remote FTP/SFTP",
		"Download selected remote item(s) to left",
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
		"Upload selected items to remote FTP/SFTP →",
		"Download selected items to left ←",
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

func TestFileTransferLeftContextMenuHasCommonFileManagerActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/file-transfer/host-mutate",
		"function renameHostEntry(view,entry)",
		"function deleteHostEntries(view,entries)",
		"function newHostFolder(view)",
		"function renameLocalEntry(view,entry)",
		"dir.removeEntry(entry.name,{recursive:false})",
		"getDirectoryHandle(name,{create:true})",
		"function renameLeftEntry(view,entry)",
		"function deleteLeftEntries(view,entries)",
		"function newLeftFolder(view)",
		"label:'Rename'",
		"label:'New folder'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing left context action %q", want)
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


func TestFileTransferWorkspaceHasParentRowsAndBackgroundOnlySelection(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function canGoParentPath(value,remote=false)",
		"tr.className='ft-parent-row'",
		"label.textContent='..'",
		"tr.onclick=event=>event.preventDefault();",
		"tr.ondblclick=event=>{event.preventDefault();panel.goUp?.().catch(app.showError);}",
		"outline:none!important;box-shadow:none!important",
		"user-select:none;-moz-user-select:none;-webkit-user-select:none",
		".ft-table tbody tr.selected{background:#29384b}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing parent/selection visual contract %q", want)
		}
	}
	if strings.Contains(js, "tr.onclick=event=>{event.preventDefault();panel.goUp?.().catch(app.showError);}") {
		t.Fatal("parent row must not navigate on single click")
	}
}

func TestFileTransferPathHistoryIsAutomaticCompactDropdown(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const maxRecentPaths=50;",
		"history.className='ft-path-history-select'",
		"history.setAttribute('aria-label','Visited paths')",
		"addGroup('Recent paths',memory.recent)",
		"rememberPath(scope,path)",
		"markPathLoaded(panel,path)",
		"history.onchange=()=>{if(history.value){input.value=history.value;onLoad(history.value).catch(app.showError);}history.value='';}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing automatic path history contract %q", want)
		}
	}
	if strings.Contains(js, "ft-favorite-select") {
		t.Fatal("legacy wide saved-path select should not remain")
	}
}


func TestFileTransferRemoteFolderCacheIsSessionScopedAndRefreshable(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"remoteCache:new Map()",
		"function remoteCacheGet(view,path)",
		"function remoteCacheSet(view,path,data)",
		"function invalidateRemoteCache(view,path,recursive=false)",
		"async function fetchRemoteDirectory(view,path,{force=false}={})",
		"if(cached)return {...cached,fromCache:true};",
		"onRefresh:()=>loadRemoteDirectory(view,remote.currentPath,{force:true})",
		"cached?'Opening cached '+path+'…':'Loading '+path+'…'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing remote cache contract %q", want)
		}
	}
}

func TestFileTransferQueueTracksPerFileLifecycle(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Transfer Queue",
		"Queued '+counts.queued+' · Running '+counts.running+' · Done '+counts.success+' · Failed '+counts.failed",
		"function enqueueTransferTasks(view,tasks)",
		"async function processTransferQueue(view)",
		"item.status='running'",
		"item.status='success'",
		"item.status='failed'",
		"Retry failed",
		"Clear done",
		"ft-queue-error",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing transfer queue contract %q", want)
		}
	}
}

func TestFileTransferFolderTransfersAreRecursiveBothDirections(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function collectHostUploadEntry(view,parent,entry,remoteParent,plan)",
		"async function collectLocalUploadHandle(handle,targetPath,sourceLabel,plan)",
		"async function collectRemoteDownloadEntry(view,remoteParent,entry,leftParent,plan)",
		"async function prepareRemoteDirectories(view,directories)",
		"async function prepareLeftDirectories(view,directories)",
		"Upload to remote FTP/SFTP →",
		"Download to left ←",
		"transferLeftEntriesToRemote(view,selected)",
		"transferRemoteEntriesToLeft(view,selected)",
		"view.toRemote.disabled=selectedEntries(view.left).length===0",
		"view.toLeft.disabled=selectedEntries(view.remote).length===0",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing recursive folder transfer contract %q", want)
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
