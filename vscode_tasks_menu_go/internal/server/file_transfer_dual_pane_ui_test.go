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
		"tr.ondblclick=event=>{event.preventDefault();if(panel.navigationDisabled)return;panel.goUp?.().catch(app.showError);}",
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
		"Queued '+counts.queued+' · Running '+counts.running+' · Conflict '+counts.conflict+' · Done '+counts.success+' · Skipped '+counts.skipped+' · Failed '+counts.failed",
		"function enqueueTransferTasks(view,tasks)",
		"async function processTransferQueue(view)",
		"item.status='running'",
		"item.status=result?.skipped?'skipped':'success'",
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
		"enqueueRemoteDownloadFile(view,remotePath,leftPath,entry.size,state,entry.modified)",
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
		"(queue.runningCount||0)===0&&queue.activeScans===0&&!hasAnyPendingTransfer(queue)",
		"await item.run(item)",
		"function nextPendingTransfer(queue)",
		"queue.items.push(item)",
		"if(item.status==='queued')queue.pending.push(item)",
		"pending:[],pendingHead:0",
		"item.status=result?.skipped?'skipped':'success';item.run=null",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing large streaming queue contract %q", want)
		}
	}
	if strings.Contains(js, "queue.items.find(candidate=>candidate.status==='queued')") {
		t.Fatal("large transfer queue must not linearly rescan all items for every dequeue")
	}
}


func TestFileTransferSuggestsCompressedFolderUploadWithoutRemovingNormalPath(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const compressedUploadScanFileThreshold=200;",
		"const compressedUploadScanDurationMS=2500;",
		"async function quickScanHostUploadSelection(view,entries)",
		"async function quickScanLocalUploadSelection(view,entries)",
		"/api/file-transfer/upload-scan",
		"function requestCompressedUploadDecision(view,entries,scan",
		"Upload normally",
		"Compress + upload",
		"async function buildLocalSelectionTarGz(view,entries)",
		"new CompressionStream('gzip')",
		"async function uploadCompressedSelection(view,entries)",
		"/api/file-transfer/archive-upload",
		"/api/file-transfer/archive-extract",
		"Archive uploaded — manual extraction required",
		"Copy POSIX command",
		"Copy PowerShell command",
		"const decision=await compressedUploadDecision(view,selected);",
		"if(decision==='archive')",
		"await createServerTransferJob(view,{kind:'host_upload'",
		"kind:'local_upload_scan'",
		"compressedUploadRootsClear(view,entries)",
		"canAutoExtractCompressedUpload(view)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("compressed folder upload UI missing %q", want)
		}
	}
	if !strings.Contains(js, "Upload the selection normally instead?") {
		t.Fatal("compressed upload failure must offer normal upload fallback")
	}
}

func TestFileTransferConnectionPoolUIAndParallelWorkers(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function remoteConnectionUsage(view)",
		"function remoteConnectionPoolFull(view)",
		"function transferWorkerLimit(view)",
		"serverActiveScans",
		"serverQueuedScans",
		"serverActiveTransfers",
		"Connections '+activeConnections+'/'+maxConnections",
		"Scan queued '+queuedScans",
		"async function runTransferQueueItem(view,item)",
		"queue.runningCount=(queue.runningCount||0)+1",
		"while((queue.runningCount||0)<limit)",
		"Waiting for an FTP/SFTP connection slot",
		"scheduleTransferPoolRetry(view)",
		"Max connections",
		"updateViewMaxConnections(view,connectionLimit.value)",
		"panel.pathInput.disabled=full",
		"panel.pathHistorySelect.disabled=full",
		"panel.pathGo.disabled=full",
		"panel.pathUp.disabled=full",
		"panel.refresh.disabled=full",
		"if(entryType(entry)==='directory'&&remoteConnectionPoolFull(view))",
		"disabled:remoteConnectionPoolFull(view)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer connection-pool UI missing %q", want)
		}
	}
}

func TestFileTransferRemoteDeleteUsesDaemonQueue(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function createServerTransferJob(view,payload)",
		"async function syncServerTransferQueue(view)",
		"startServerTransferQueuePolling(view)",
		"kind:'remote_delete'",
		"Background delete queued on TaskDeck daemon",
		"/api/file-transfer/jobs?profile_id=",
		"/api/file-transfer/jobs/control",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing daemon remote-delete queue contract %q", want)
		}
	}
	start := strings.Index(js, "async function streamRemoteDeleteEntries(view,entries)")
	if start < 0 {
		t.Fatal("cannot find remote delete UI block")
	}
	end := strings.Index(js[start:], "async function deleteRemoteEntries(view,entries)")
	if end < 0 {
		t.Fatal("cannot isolate remote delete UI block")
	}
	block := js[start : start+end]
	if strings.Contains(block, "runTransferScan(view,'Delete scan'") || strings.Contains(block, "upsertPersistentFileTransferJob") {
		t.Fatal("remote delete must be daemon-owned instead of browser scan/journal")
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
		"if(activate)activateView(id,{force:true})",
		"activeViewID=activeID&&views.has(activeID)?activeID:''",
		"if(saved?.active_profile_id&&views.has(saved.active_profile_id))activateView(saved.active_profile_id)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing non-stealing restore contract %q", want)
		}
	}
}


func TestFileTransferRestoreCannotEraseSnapshotBeforeItReadsIt(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"let sessionPersistenceReady=false",
		"if(restoringSession||!sessionPersistenceReady)return",
		"restoringSession=false;sessionPersistenceReady=true",
		"await scheduleFileTransferSessionRestore()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing restore snapshot guard %q", want)
		}
	}
}

func TestDaemonTransferQueueSurvivesBrowserReload(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function startServerTransferQueuePolling(view)",
		"poll();view.serverQueueTimer=setInterval(poll,700)",
		"const serverItems=(Array.isArray(snapshot?.items)?snapshot.items:[]).map",
		"id:'server:'+item.id",
		"serverID:String(item.id||'')",
		"queue.serverActiveScans=Number(snapshot?.active_scans)||0",
		"startServerTransferQueuePolling(view)",
		"for(const view of views.values())syncServerTransferQueue(view).catch(()=>{})",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing daemon queue reload contract %q", want)
		}
	}
}

func TestHostUploadAndDownloadUseDaemonJobs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"if(source==='host')",
		"kind:'host_upload'",
		"host_paths:hostPaths",
		"Background upload queued on TaskDeck daemon",
		"if(view.left.source==='host')",
		"kind:'host_download'",
		"remote_targets:remoteTargets",
		"Background download queued on TaskDeck daemon",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing daemon host-transfer contract %q", want)
		}
	}
}



func TestLocalBrowserTransferQueueRehydratesAfterReload(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"taskdeck:file-transfer:local-queue:",
		"function persistLocalTransferQueue(view)",
		"status:item.status==='running'?'queued':item.status",
		"async function persistentLocalTransferRun(view,spec,item=null)",
		"function restorePersistentLocalTransferQueue(view)",
		"persistSpec:spec",
		"kind:'local_upload'",
		"kind:'local_download'",
		"restorePersistentLocalTransferQueue(view)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing Local queue reload recovery contract %q", want)
		}
	}
}

func TestLocalBrowserScannerJournalResumesAndDeduplicatesAfterReload(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"taskdeck:file-transfer:local-scans:",
		"function upsertPersistentLocalScan(scan)",
		"function removePersistentLocalScan(scanID)",
		"async function runPersistentLocalUploadScan(view,scan)",
		"async function runPersistentLocalDownloadScan(view,scan)",
		"async function resumePersistentLocalScans()",
		"kind:'local_upload_scan'",
		"kind:'local_download_scan'",
		"const existingPersistent=new Set(queue.items.map(item=>localPersistentKey(item.persistSpec)).filter(Boolean))",
		"if(persistentKey&&existingPersistent.has(persistentKey))continue",
		"resumePersistentLocalScans()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing Local scanner reload recovery contract %q", want)
		}
	}
}


func TestFileTransferDuplicateFilesUseSharedConflictPolicyDialog(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function requestTransferConflictDecision(conflict)",
		"Folder collisions are reused automatically.",
		"Overwrite destination",
		"Skip source file",
		"Overwrite only if size differs",
		"Overwrite only if source Modified time is newer",
		"Overwrite only if SHA-256 differs",
		"This file only",
		"This transfer only",
		"All '+word+' in this TaskDeck session",
		"Always for '+word+' (remember)",
		"/api/file-transfer/hash",
		"/api/file-transfer/hash-upload",
		"function evaluateBrowserConflictPolicy(view,policy,conflict,sourceHash,targetHash)",
		"effectiveDirectionConflictPolicy(view,'upload')",
		"effectiveDirectionConflictPolicy(view,'download')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing duplicate conflict policy contract %q", want)
		}
	}
	if strings.Contains(js, "already exists locally. Overwrite it?") {
		t.Fatal("Local duplicate handling must use shared conflict policy dialog instead of legacy confirm")
	}
}

func TestFileTransferDaemonConflictsRemainQueueItemsUntilResolved(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"case'conflict':return 'Conflict'",
		"case'skipped':return 'Skipped'",
		"status:String(item.status||'queued')",
		"conflict:item.conflict||null",
		"async function resolveServerConflictItems(view,items)",
		"async function maybePromptServerConflict(view)",
		"serverTransferQueueControl(view,'resolve_conflict'",
		"conflict_policy:decision.policy",
		"conflict_scope:decision.scope",
		"Resolve conflict…",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing daemon conflict queue contract %q", want)
		}
	}
}

func TestFileTransferServerConflictPromptDoesNotQueueStaleDialogs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const activeServerConflictViews=new Set()",
		"if(typeof conflict?.stillCurrent==='function'&&!conflict.stillCurrent()){resolve(null);return;}",
		"function serverConflictViewKey(view)",
		"function liveServerConflictItem(view,serverID)",
		"if(activeServerConflictViews.has(viewKey))return",
		"activeServerConflictViews.add(viewKey)",
		"stillCurrent:()=>Boolean(liveServerConflictItem(view,serverID))",
		"if(!decision)return",
		"activeServerConflictViews.delete(viewKey)",
		"setTimeout(()=>maybePromptServerConflict(view),0)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing serialized/stale-safe server conflict prompt contract %q", want)
		}
	}
	if strings.Contains(js, "activeServerConflictPrompts") {
		t.Fatal("server conflict auto-prompt must lock per view, not independently per file")
	}
}

func TestFileTransferConflictDefaultsSupportJobSessionAndRememberedDirection(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"taskdeck:file-transfer:conflict-default:",
		"taskdeck:file-transfer:conflict-jobs:",
		"function setLocalJobConflictPolicy(jobID,policy)",
		"function effectiveLocalConflictPolicy(view,direction,item=null)",
		"decision.scope==='job'",
		"decision.scope==='direction_session'||decision.scope==='direction_always'",
		"if(decision.scope==='direction_always')rememberConflictPolicy(view,direction,decision.policy)",
		"conflict_policy:effectiveDirectionConflictPolicy(view,'upload')",
		"conflict_policy:effectiveDirectionConflictPolicy(view,'download')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing conflict scope persistence contract %q", want)
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

func TestFileTransferFolderSyncUsesDryRunBeforeSyncOrMirror(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Folder Sync / Mirror dry run",
		"async function configureAndCompareFolders(view)",
		"function openSyncSetupDialog(view)",
		"async function compareFoldersDryRun(view,options={})",
		"async function collectHostSyncTree(base,options={})",
		"async function collectLocalSyncTree(view,base,options={})",
		"async function collectRemoteSyncTree(view,base,options={})",
		"async function compareSyncTrees(view,left,remote,options={})",
		"async function syncLeftFileSHA256(view,relativePath,options={})",
		"async function syncRemoteFileSHA256(view,relativePath,options={})",
		"Folder Sync / Mirror — Dry run",
		"Recursive SHA-256 comparison for matching-size files",
		"Left only ",
		"Remote only ",
		"Different ",
		"Conflict ",
		"Type mismatch ",
		"Run: '+syncDirectionLabel(options.direction)",
		"Bidirectional Sync",
		"Mirror delete is disabled.",
		"Allow destination deletes",
		"maxSyncPlanEntries=10000",
		"maxSyncPlanDepth=64",
		"kind:'host_upload'",
		"kind:'host_download'",
		"kind:'remote_delete'",
		"dir.removeEntry(name,{recursive:false})",
		"exclude:options.exclude",
		"options.compare_mode==='checksum'",
		"hostFileSHA256(fullPath)",
		"remoteFileSHA256(view,joinPath(remoteBase,relativePath,true))",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing folder sync dry-run contract %q", want)
		}
	}
	compare := strings.Index(js, "async function compareFoldersDryRun(view,options={})")
	dialog := strings.Index(js, "function openFolderSyncDryRun(view,rows,options={})")
	if compare < 0 || dialog < 0 {
		t.Fatal("folder sync compare/dialog missing")
	}
	end := compare + 3200
	if end > len(js) {
		end = len(js)
	}
	if strings.Contains(js[compare:end], "syncPlanToRemote(view,rows)") {
		t.Fatal("dry-run compare must not start transfers before explicit dialog action")
	}
}

func TestFileTransferSyncProfilesAndBidirectionalConflictsAreFailSafe(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"taskdeck:file-transfer:sync-profiles:",
		"function normalizeSyncExclude(value)",
		"function syncPathExcluded(path,patterns)",
		"compare_mode:compareMode,direction,exclude:normalizeSyncExclude(raw.exclude||[])",
		"allow_delete:Boolean(raw.allow_delete)",
		"if(options.direction==='bidirectional')rows=rows.map(row=>row.status==='different'?{...row,status:'conflict'}:row);",
		"const safeRows=rows.map(row=>row.status==='different'?{...row,status:'conflict'}:row);",
		"if(!options.allow_delete)throw new Error('Mirror delete is disabled.",
		"destination deletion remains disabled unless explicitly enabled",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing fail-safe sync contract %q", want)
		}
	}
}

func TestFileTransferHostArchiveCanUploadAndExtractThroughSFTP(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function transferArchiveKind(name)",
		"async function uploadAndExtractRemoteArchive(view,entry)",
		"/api/file-transfer/archive-upload-extract",
		"Remote archive extraction requires an SFTP profile linked to SSH.",
		"Upload + extract archive on remote…",
		"String(view.profile?.protocol||'').toLowerCase()==='sftp'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("file transfer remote archive workflow missing %q", want)
		}
	}
}
