package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTabDragOrderingFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabdrag.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"tab.draggable=true",
		"addEventListener('dragstart'",
		"addEventListener('dragover'",
		"addEventListener('drop'",
		"addEventListener('dragend'",
		"originalOrder=currentIDs()",
		"if(!committed)applyOrder(originalOrder)",
		"sessionStorage.setItem(storageKey()",
		"TaskMenuTerminalRestore?.ready",
		"TaskMenuTerminalRestore?.persistSnapshot?.()",
		"target?.closest('.close')?'0':'1'",
		"function workspaceTabs()",
		"node instanceof HTMLElement&&Boolean(node.dataset.id)",
		"function workspaceTabFromTarget(target)",
		"tab&&tab.parentElement===tabsHost?tab:null",
		"function workspaceTabByID(id)",
		"#tabs>[data-id][draggable=\"true\"] .close{cursor:pointer}",
		"const tabObserver=new MutationObserver(records=>",
		"node.parentElement!==tabsHost||!node.dataset.id",
		"const alreadyInstalled=node.dataset.dragOrderInstalled==='1'",
		"installTab(node)",
		"if(!alreadyInstalled)installedNewTab=true",
		"if(installedNewTab&&!dragged)",
		"tabObserver.observe(tabsHost,{childList:true})",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabdrag.js missing %q", want)
		}
	}

	restore, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restore), "ready:restoreReady") {
		t.Fatal("terminal restore API must expose readiness so tab order is applied after recovery")
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/tabdrag.js';") {
		t.Fatal("next.js must load tabdrag.js")
	}
}

func TestTabDragSupportsPatchFileTransferAndDatabaseWorkspaceTabs(t *testing.T) {
	dragData, err := webassets.Files.ReadFile("featuremods/tabdrag.js")
	if err != nil {
		t.Fatal(err)
	}
	dragJS := string(dragData)
	for _, want := range []string{
		"workspaceTabs().map(tab=>tab.dataset.id||'').filter(Boolean)",
		"const byID=new Map(current.map(tab=>[tab.dataset.id,tab]))",
		"for(const tab of workspaceTabs())installTab(tab)",
		"const direct=workspaceTabFromTarget(event.target)",
		"const tab=workspaceTabByID(id)",
	} {
		if !strings.Contains(dragJS, want) {
			t.Fatalf("tabdrag.js missing shared workspace-tab support %q", want)
		}
	}
	if strings.Contains(dragJS, "querySelectorAll('.tab[data-id]')") {
		t.Fatal("tab drag ordering must not exclude FTP/SFTP or database tabs by requiring the generic .tab class")
	}

	patchData, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patchData), "patchTab.dataset.id='patch'") {
		t.Fatal("Patch Tool tab must expose a stable data-id so shared drag ordering can track it")
	}

	transferData, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(transferData), "tab.dataset.id='file-transfer:'+id") {
		t.Fatal("FTP/SFTP tabs must expose their existing stable data-id to shared drag ordering")
	}

	databaseData, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(databaseData), "tab.className='db-tab';tab.dataset.id=meta.id") {
		t.Fatal("database tabs must remain compatible with shared data-id drag ordering")
	}
}


func TestTabDragObserverDoesNotLoopOnItsOwnReorder(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabdrag.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const current=workspaceTabs()",
		"ordered.every((tab,index)=>tab===current[index])",
		"return false",
		"const alreadyInstalled=node.dataset.dragOrderInstalled==='1'",
		"if(!alreadyInstalled)installedNewTab=true",
		"if(installedNewTab&&!dragged)",
		"genuinely new workspace tab should trigger saved-order restoration",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabdrag.js missing reorder-loop guard %q", want)
		}
	}
	if strings.Contains(js, "installTab(node);added=true") {
		t.Fatal("MutationObserver must not treat reorder append() operations as newly added tabs")
	}
}


func TestTabContextMenuReusesSessionAndConsoleActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"addEventListener('contextmenu'",
		"sourceActions(view,'Session')",
		"sourceActions(view,'Console')",
		"app.activateView(targetView.meta.id,{force:true})",
		"source.click()",
		"event.preventDefault()",
		".tab-context-menu button,.tab-context-submenu button{display:block;width:100%;text-align:left;margin:0;border:0;background:transparent",
		".tab-context-menu button:hover:not(:disabled),.tab-context-submenu button:hover:not(:disabled){background:#2b3440}",
		"function semanticClass(button,label)",
		"if(label==='Console')return ''",
		"if(button.classList.contains('stop'))return 'context-danger'",
		"sourceActions(view,'Console')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabcontext.js missing %q", want)
		}
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	next := string(loader)
	menus := strings.Index(next, "import '/featuremods/menus.js';")
	context := strings.Index(next, "import '/featuremods/tabcontext.js';")
	if menus < 0 || context < 0 || context < menus {
		t.Fatal("tabcontext.js must load after menus.js so Session/Console source actions already exist")
	}
}


func TestTerminalPaneHasContextMenuTrigger(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function decorateTerminalPane(view)",
		"terminal-context-trigger",
		"trigger.textContent='⋮'",
		"head.prepend(trigger)",
		"openContextMenu(view,rect.left,rect.bottom+3)",
		"if(descriptor.kind==='terminal')decorateTerminalPane(descriptor.view)",
		"aria-haspopup','menu'",
		"aria-expanded','false'",
		"document.querySelectorAll('.terminal-context-trigger[aria-expanded=\"true\"]')",
		"decorateTerminalPane,",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabcontext.js missing terminal pane context trigger behavior %q", want)
		}
	}
}

func TestEditorTabContextMenuUsesEditorOnlyActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function openEditorContextMenu(view,x,y)",
		"globalThis.TaskMenuEditor?.editors?.get?.(id)",
		"{label:'Save'",
		"{label:'Reload'",
		"{label:'Go to line…'",
		"{label:'Split vertical…'",
		"{label:'Split horizontal…'",
		"{label:'Swap split panes'",
		"{label:'Unsplit'",
		"editor?.getSplitParent?.(view)",
		"editor?.splitEditor?.(view,'vertical')",
		"editor?.splitEditor?.(view,'horizontal')",
		"editor?.swapEditorSplit?.(view)",
		"editor?.unsplitEditor?.(view)",
		"{label:'Close'",
		"globalThis.TaskMenuEditor?.activateEditor?.(targetView.id)",
		"if(descriptor.kind==='editor')openEditorContextMenu",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabcontext.js missing editor-only context behavior %q", want)
		}
	}
}


func TestTabDragInstallsForExternalEditorTabs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabdrag.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"window.addEventListener('taskmenu:view-activated'",
		"event.detail?.kind!=='external'",
		"const tab=workspaceTabByID(id)",
		"if(tab)installTab(tab)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabdrag.js missing external/editor tab install behavior %q", want)
		}
	}
}

func TestTabOrderIsScopedByLayoutProfile(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabdrag.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	if !strings.Contains(js, "'vscode-tasks-menu:tab-order:'+(app.layoutProfile||'desktop')") {
		t.Fatal("tab order storage must be scoped by desktop/mobile layout profile")
	}
}


func TestFeatureTabsSupportPersistentReadOnlyMode(t *testing.T) {
	contextData, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	contextJS := string(contextData)
	for _, want := range []string{
		"taskdeck:tab-readonly:",
		"checkbox.type='checkbox'",
		"label.textContent='Read only'",
		".tab[data-id],.db-tab[data-id],.task-patch-tab",
		"function descriptorForTab(tab)",
		"function setDescriptorReadOnly(descriptor,enabled)",
		"taskmenu:tab-readonly-changed",
		"data-taskdeck-readonly",
		"descriptor.pane.dataset.taskdeckReadonlyKind=descriptor.kind",
		"[data-taskdeck-readonly=\"1\"]:not([data-taskdeck-readonly-kind=\"terminal\"])",
		"function readonlyMutationPaneFromTarget(target)",
		"if(!pane||pane.dataset.taskdeckReadonlyKind==='terminal')return null",
		"document.addEventListener('beforeinput',stopReadonlyMutation,true)",
		"document.addEventListener('click',event=>",
		"document.addEventListener('contextmenu',event=>",
		"event.stopImmediatePropagation()",
		"new MutationObserver(records=>",
		"registerTab(tab)",
	} {
		if !strings.Contains(contextJS, want) {
			t.Fatalf("tabcontext.js missing shared read-only behavior %q", want)
		}
	}

	for _, want := range []string{
		"function setViewReadOnly(viewOrID,enabled)",
		"view.tabReadOnly=Boolean(enabled)",
		"view.term.options.disableStdin=!view.canControl||view.tabReadOnly",
		"view.canControl&&!view.tabReadOnly&&!browserLeaseLost",
		"view.pane.dataset.taskdeckReadonlyKind='terminal'",
		"if(view.stop)view.stop.disabled=view.meta?.status!=='running'",
		"view.stop.hidden=!view.canControl;view.stop.disabled=meta.status!=='running'",
		"materializeSession,setViewReadOnly,addOutputFilter",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("core terminal UI missing tab read-only behavior %q", want)
		}
	}
	if strings.Contains(appJS, "view.stop.disabled=Boolean(view.tabReadOnly)") || strings.Contains(appJS, "view.stop.disabled=view.tabReadOnly") {
		t.Fatal("terminal read-only must not disable Stop or other utility controls")
	}

	editorData, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	editorJS := string(editorData)
	for _, want := range []string{
		"function editorReadOnly(view)",
		"view?.file?.read_only||view?.tabReadOnly",
		"function setTabReadOnly(viewOrID,enabled)",
		"view.readonlyBadge.textContent=view.file.read_only?'READ-ONLY':'TAB READ-ONLY'",
		"setTabReadOnly,",
	} {
		if !strings.Contains(editorJS, want) {
			t.Fatalf("editor.js missing tab read-only behavior %q", want)
		}
	}

	databaseData, err := webassets.Files.ReadFile("featuremods/database.js")
	if err != nil {
		t.Fatal(err)
	}
	databaseJS := string(databaseData)
	for _, want := range []string{
		"function setDatabaseTabReadOnly(viewOrID,enabled)",
		"for(const queryView of databaseQueryViews(view))",
		"content.setAttribute('contenteditable',readonly?'false':'true')",
		"function setQueryWorkbenchReadOnly(queryView,enabled)",
		"setQueryTabReadOnly:setQueryWorkbenchReadOnly",
		"setTabReadOnly:setDatabaseTabReadOnly",
	} {
		if !strings.Contains(databaseJS, want) {
			t.Fatalf("database.js missing tab read-only behavior %q", want)
		}
	}

	patchData, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	patchJS := string(patchData)
	for _, want := range []string{
		"let tabReadOnly=false",
		"function setReadOnly(enabled)",
		"panel.dataset.taskdeckReadonly='1'",
		"setReadOnly,isReadOnly:()=>tabReadOnly",
		"TaskMenuTabContext?.registerTab?.(patchTab)",
	} {
		if !strings.Contains(patchJS, want) {
			t.Fatalf("patchpanel.js missing tab read-only behavior %q", want)
		}
	}
}


func TestTabContextMenuKeepsLongMenusOpenWhileScrolling(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"overscroll-behavior:contain",
		"scrollbar-gutter:stable",
		"menu.addEventListener('scroll',()=>closeSubmenu(),{passive:true})",
		"menu.addEventListener('wheel',event=>event.stopPropagation(),{passive:true})",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabcontext.js missing long-menu scroll behavior %q", want)
		}
	}
	if strings.Contains(js, "window.addEventListener('scroll',closeContextMenu,true)") {
		t.Fatal("tab context menu must not close itself when its own scroll container moves")
	}
}

func TestTabContextGroupsFrequentActionsIntoSubmenus(t *testing.T) {
    data,err:=webassets.Files.ReadFile("featuremods/tabcontext.js")
    if err!=nil {t.Fatal(err)}
    js:=string(data)
    start:=strings.Index(js,"function openContextMenu(view,x,y){")
    end:=strings.Index(js,"function openFeatureContextMenu(",)
    if start<0||end<=start {t.Fatal("terminal context menu handler not found")}
    block:=js[start:end]
    for _,want:=range []string{
        "const grouped=groupSessionSources(view,session)",
        "const consoleItems=consoleActions.map(source=>sourceAction(view,source,'Console'))",
        "{label:'Console',children:consoleItems}",
        "{label:'Session',children:grouped.session}",
        "{label:'Split',children:grouped.split}",
        "{label:'Lifecycle',children:lifecycle}",
        "{label:'Broadcast group',children:broadcastActions}",
        "...presetActions",
        "addCustomActions('',groups)",
        "const lifecycle=[closeTabAction(view),...grouped.lifecycle]",
    } {
        if !strings.Contains(block,want){t.Errorf("grouped terminal context action missing %q",want)}
    }
    console:=strings.Index(block,"{label:'Console',children:consoleItems}")
    session:=strings.Index(block,"{label:'Session',children:grouped.session}")
    split:=strings.Index(block,"{label:'Split',children:grouped.split}")
    lifecycle:=strings.Index(block,"{label:'Lifecycle',children:lifecycle}")
    if console<0||session<=console||split<=session||lifecycle<=split {
        t.Fatalf("expected Console above Session, followed by Split and Lifecycle; offsets: %d %d %d %d",console,session,split,lifecycle)
    }
    for _,want:=range []string{
        "source.matches('.session-split-vertical,.session-split-horizontal",
        "source.matches('.stop,.session-terminate,.session-force-kill')",
        "if(!view.closed&&source.isConnected&&!source.disabled)source.click()",
        "disabled:source.disabled",
        "disabled:!close",
        "if(close?.isConnected)close.click()",
        "if(button.classList.contains('stop'))return 'context-danger'",
        "if(button.matches('.session-terminate,.session-force-kill'))return 'context-danger'",
    } {
        if !strings.Contains(js,want){t.Errorf("submenu dispatch or destructive-action safety missing %q",want)}
    }
    if strings.Contains(block,"addActions(view,'Session',session)")||strings.Contains(block,"addActions(view,'Console',consoleActions)"){
        t.Fatal("terminal context actions should not be displayed as flat lists")
    }
}

func TestEditorTabContextKeepsQuickActionsAndGroupsAdvancedCommands(t *testing.T){
    data,err:=webassets.Files.ReadFile("featuremods/tabcontext.js")
    if err!=nil{t.Fatal(err)}
    js:=string(data)
    start:=strings.Index(js,"function openEditorContextMenu(view,x,y){")
    end:=strings.Index(js,"function openContextMenu(view,x,y){")
    if start<0||end<=start{t.Fatal("editor tab context handler missing")}
    block:=js[start:end]
    for _,want:=range []string{
        "contextView=view",
        "addCustomActions('',[actions[0],actions[1],actions[6]])",
        "{label:'Compare',children:actions.slice(2,6)}",
        "{label:'Split',children:actions.slice(7,11)}",
        "{label:'Project',children:projectActions}",
        "{label:'Lifecycle',children:[actions[11]]}",
        ".filter(action=>action.label!=='Open'&&!action.separator)",
        "disabled:!splitParent",
    }{
        if !strings.Contains(block,want){t.Errorf("editor context submenu missing %q",want)}
    }
    if !strings.Contains(js,"if(targetView.tab?.dataset?.viewKind==='editor')globalThis.TaskMenuEditor?.activateEditor?.(targetView.id)") {
        t.Fatal("Editor submenu must activate its editor, not treat the editor as a PTY session")
    }
}
