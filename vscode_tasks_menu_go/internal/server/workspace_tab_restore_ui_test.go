package server

import (
    "strings"
    "testing"

    webassets "bletonfc/vscode_tasks_menu/web"
)

func readWorkspaceTabFeature(t *testing.T, path string) string {
    t.Helper()
    data, err := webassets.Files.ReadFile("featuremods/" + path)
    if err != nil { t.Fatal(err) }
    return string(data)
}

func requireWorkspaceTabSource(t *testing.T, js string, wants ...string) {
    t.Helper()
    for _, want := range wants {
        if !strings.Contains(js, want) { t.Fatalf("workspace tab code missing %q",want) }
    }
}

func TestWorkspaceTabsRestoreAllFeatureKinds(t *testing.T) {
    loader := readWorkspaceTabFeature(t, "next.js")
    requireWorkspaceTabSource(t, loader,
        "import '/featuremods/workspacetabs.js';",
        "import '/featuremods/tabcolors.js';",
    )
    js := readWorkspaceTabFeature(t, "workspacetabs.js")
    requireWorkspaceTabSource(t,js,
        "function key()",
        "app.currentUser?.user_id||'local'",
        "function descriptor(tab,ordinals)",
        "function terminalOrdinalMap()",
        "function applySemanticOrder(order)",
        "function restoreActive(wanted)",
        "async function restoreStartup()",
        "Promise.allSettled([",
        "TaskMenuTerminalRestore?.ready",
        "TaskMenuEditor?.ready",
        "TaskMenuDatabase?.ready",
        "TaskMenuFileTransfer?.ready",
        "TaskMenuEditor?.restoreRemoteState?.(saved.remoteEditors)",
        "TaskMenuHexViewer?.restoreState?.(saved.hex)",
        "TaskMenuFileCompare?.restoreState?.(saved.compare)",
        "TaskMenuPatchPanel?.open?.()",
        "if(saved?.order)applySemanticOrder(saved.order)",
        "if(saved?.active)await restoreActive(saved.active)",
        "window.addEventListener('pagehide',()=>saveNow())",
        "window.addEventListener('taskmenu:patch-panel-visible',()=>scheduleSave())",
    )
    for _, kind := range []string{"terminal","session","editor","compare","patch","hex","transfer","database"} {
        if !strings.Contains(js,"kind:'"+kind+"'") {
            t.Fatalf("workspace tab restore does not identify %s tabs",kind)
        }
    }
}

func TestWorkspaceTabDragPersistsSemanticOrder(t *testing.T) {
    js:=readWorkspaceTabFeature(t,"tabdrag.js")
    requireWorkspaceTabSource(t,js,
        "TaskMenuWorkspaceTabs?.saveNow?.()",
        "const manager=globalThis.TaskMenuWorkspaceTabs",
        "if(manager){if(!manager.restoring)manager.scheduleSave?.();return;}",
        "function applyOrder(ids)",
    )
}

func TestWorkspaceTabFeatureRecoveryAndStartupReady(t *testing.T) {
    compare:=readWorkspaceTabFeature(t,"filecompare.js")
    requireWorkspaceTabSource(t,compare,
        "function snapshotCompareState()",
        "async function restoreCompareState(saved)",
        "function compareSourceDescriptor(source)",
        "function compareSourceFromDescriptor(spec)",
        "function storeCompareDraft(left,right)",
        "sessionStorage.setItem(key,JSON.stringify(draft))",
        "snapshotState:snapshotCompareState,restoreState:restoreCompareState",
        "taskmenu:workspace-tab-changed",
    )
    hex:=readWorkspaceTabFeature(t,"hexviewer.js")
    requireWorkspaceTabSource(t,hex,
        "function snapshotHexState()",
        "async function restoreHexState(items)",
        "snapshotState:snapshotHexState,restoreState:restoreHexState",
    )
    editor:=readWorkspaceTabFeature(t,"editor.js")
    requireWorkspaceTabSource(t,editor,
        "snapshotRemoteState:snapshotRemoteEditorTabs",
        "restoreRemoteState:restoreRemoteEditorTabs",
        "function remoteEditorSessionKey()",
        "get ready(){return editorSessionReady;}",
    )
    database:=readWorkspaceTabFeature(t,"database.js")
    requireWorkspaceTabSource(t,database,"get ready(){return databaseSessionsReady;}")
    transfer:=readWorkspaceTabFeature(t,"filetransfer.js")
    requireWorkspaceTabSource(t,transfer,"get ready(){return fileTransferSessionReady;}")
    patch:=readWorkspaceTabFeature(t,"patchpanel.js")
    requireWorkspaceTabSource(t,patch,
        "patchTab.hidden=true;",
        "window.dispatchEvent(new Event('taskmenu:workspace-tab-changed'))",
    )
    selfUpdate:=readWorkspaceTabFeature(t,"selfupdate.js")
    requireWorkspaceTabSource(t,selfUpdate,"TaskMenuWorkspaceTabs?.saveNow?.()")
}

func TestWorkspaceTabTypeColorsCannotMaskBroadcastGroups(t *testing.T) {
    js:=readWorkspaceTabFeature(t,"tabcolors.js")
    requireWorkspaceTabSource(t,js,
        "data-taskdeck-tab-type",
        "#tabs>[data-taskdeck-tab-type]::before",
        ":not(.broadcast-grouped)",
        "--taskdeck-type-accent",
        "function tabType(tab)",
        "window.addEventListener('taskmenu:session'",
    )
    for _, kind:=range []string{"terminal","ssh","task","editor","diff","patch","transfer","ftp","sftp","database","hex"} {
        requireWorkspaceTabSource(t,js,"data-taskdeck-tab-type=\""+kind+"\"")
    }
    if strings.Contains(js, "color:var(--taskdeck-type-accent)!important") {
        t.Fatal("tab category colors must not replace broadcast group text color")
    }
}
