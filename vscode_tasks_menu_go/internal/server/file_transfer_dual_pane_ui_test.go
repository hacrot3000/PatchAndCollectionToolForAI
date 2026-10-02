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

func TestFileTransferFolderTransfersStreamWhileScanningBothDirections(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function runTransferScan(view,label,scanner)",
		"activeScans",
		"async function scanHostUploadEntry(view,parent,entry,remoteParent,state)",
		"async function scanLocalUploadHandle(view,handle,targetPath,sourcePath,state)",
		"enqueueHostUploadFile(view,sourcePath,targetPath,entry.size,state)",
		"enqueueLocalUploadHandle(view,handle,sourcePath,targetPath,state)",
		"async function scanRemoteDownloadEntry(view,remoteParent,entry,leftParent,state)",
		"enqueueRemoteDownloadFile(view,remotePath,leftPath,entry.size,state)",
		"async function streamLeftEntriesToRemote(view,entries)",
		"async function streamRemoteEntriesToLeft(view,entries)",
		"Upload to remote FTP/SFTP →",
		"Download to left ←",
		"view.toRemote.disabled=selectedEntries(view.left).length===0",
		"view.toLeft.disabled=selectedEntries(view.remote).length===0",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing streaming folder transfer contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"const plan=await buildLeftUploadPlan",
		"const plan=await buildRemoteDownloadPlan",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("folder transfer must not pre-scan the entire tree before transfer: %q", forbidden)
		}
	}
}

func TestFileTransferQueueScalesForLargeStreamingScans(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const maxRenderedTransferRows=2000;",
		"function scheduleTransferQueueRender(view)",
		"setTimeout(()=>{queue.renderTimer=0;renderTransferQueue(view);},80)",
		"document.createDocumentFragment()",
		"if(queue.activeScans===0&&!hasAnyPendingTransfer(queue))await afterTransferQueueIdle(view)",
		"await item.run(item)",
		"function nextPendingTransfer(queue)",
		"queue.items.push(item);queue.pending.push(item)",
		"pending:[],pendingHead:0",
		"item.status='success';item.run=null",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing large streaming queue contract %q", want)
		}
	}
	if strings.Contains(js, "queue.items.find(candidate=>candidate.status==='queued')") {
		t.Fatal("large transfer queue must not linearly rescan all items for every dequeue")
	}
}


func TestFileTransferRemoteDeleteScansAndQueuesPostOrder(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function enqueueRemoteDeleteItem(view,path,directory,state)",
		"async function scanRemoteDeleteEntry(view,remoteParent,entry,state)",
		"const listing=await fetchRemoteDirectory(view,remotePath,{force:true})",
		"for(const child of listing.entries)await scanRemoteDeleteEntry(view,remotePath,child,state);",
		"enqueueRemoteDeleteItem(view,remotePath,true,state);",
		"kind:'Delete'",
		"runTransferScan(view,'Delete scan'",
		"deleted recursively through Transfer Queue",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing recursive delete queue contract %q", want)
		}
	}
	start := strings.Index(js, "async function streamRemoteDeleteEntries(view,entries)")
	if start < 0 {
		t.Fatal("cannot find remote delete UI block")
	}
	end := strings.Index(js[start:], "async function copyText(value)")
	if end < 0 {
		t.Fatal("cannot isolate remote delete UI block")
	}
	if strings.Contains(js[start:start+end], "non-recursive") {
		t.Fatal("remote delete UI must no longer describe folder delete as non-recursive")
	}
}

func TestFileTransferQueueSupportsPauseSelectionAndKind(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"paused:false",
		"selectedIDs:new Set()",
		"priorityPending:[]",
		"function pauseTransferQueue(view)",
		"function resumeTransferQueue(view)",
		"function resumeSelectedTransfers(view)",
		"function removeSelectedTransfers(view)",
		"function queueContextMenu(view,event,item=null,visible=[])",
		"queue.paused?'Resume queue':'Pause queue'",
		"Resume selected",
		"Remove selected",
		"if(queue.paused)return null;",
		"const priority=nextQueuedFrom(queue.priorityPending,'priorityHead',queue);",
		"queue.priorityPending.push(item)",
		"activeScans",
		"['','', 'Kind','Source','Target','Size','Status','Error']",
		"kind:task.kind||'Transfer'",
		"kind:'Upload'",
		"kind:'Download'",
		"kind:'Delete'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing pause/selection/kind queue contract %q", want)
		}
	}
}

func TestFileTransferQueuePauseDoesNotStopScanners(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	pause := strings.Index(js, "function pauseTransferQueue(view)")
	scan := strings.Index(js, "function runTransferScan(view,label,scanner)")
	if pause < 0 || scan < 0 {
		t.Fatal("pause/scanner functions missing")
	}
	scanEnd := strings.Index(js[scan:], "function pauseTransferQueue(view)")
	if scanEnd < 0 {
		t.Fatal("cannot isolate runTransferScan")
	}
	scanBody := js[scan : scan+scanEnd]
	if strings.Contains(scanBody, "queue.paused") {
		t.Fatal("background scanner must continue while transfer queue is paused")
	}
	if !strings.Contains(js, "scheduleTransferQueueRender(view);if(!queue.paused)processTransferQueue(view);") {
		t.Fatal("paused queue must not auto-start worker for newly scanned items")
	}
	if !strings.Contains(js, "scheduleTransferQueueRender(view);processTransferQueue(view);") {
		t.Fatal("resume-selected path must be able to wake the worker while global queue is paused")
	}
}


func TestFileTransferQueueIsVerticallyResizableAndPersistent(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"ft-queue-resizer",
		"Drag to resize Transfer Queue · double-click to reset",
		"function installQueueResizer(view)",
		"taskdeck:file-transfer:queue-height:",
		"height=startHeight-(event.clientY-startY)",
		"root.style.height=height+'px'",
		"root.style.flex='0 0 '+height+'px'",
		"safeStorageSet(storageKey,String(height))",
		"resizer.ondblclick=()=>{height=defaultHeight;apply();safeStorageSet(storageKey,String(height));}",
		"installQueueResizer(view)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing queue resize contract %q", want)
		}
	}
}


func TestFileTransferSessionRestoresOpenProfilesAndPathsAfterReload(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"taskdeck:file-transfer:session:",
		"function readFileTransferSession()",
		"function persistFileTransferSession()",
		"function restoreFileTransferSession()",
		"active_profile_id",
		"profile_id:id",
		"left_path:normalizeRelativePath(view.left?.currentPath||'.')",
		"remote_path:normalizeRemotePath(view.remote?.currentPath||view.profile?.initial_path||'.')",
		"attachView(profile,{activate:false,session:item})",
		"window.addEventListener('taskmenu:tasks',scheduleFileTransferSessionRestore)",
		"setTimeout(scheduleFileTransferSessionRestore,0)",
		"await globalThis.TaskMenuTerminalRestore?.ready",
		"restoreSession:restoreFileTransferSession",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing reload session persistence contract %q", want)
		}
	}
}

func TestFileTransferSessionRestoreDoesNotStealFocusFromOtherViews(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function attachView(profile,{activate=true,session=null}={})",
		"if(activate)activateView(id)",
		"activeViewID=activeID&&views.has(activeID)?activeID:''",
		"if(saved.active_profile_id&&views.has(saved.active_profile_id))activateView(saved.active_profile_id)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing non-stealing restore contract %q", want)
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
		"New Folder",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing structured API contract %q", want)
		}
	}
}
