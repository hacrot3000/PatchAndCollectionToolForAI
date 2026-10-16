package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalRestoreFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalrestore.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"scope=terminals&profile=",
		"encodeURIComponent(app.layoutProfile||'desktop')",
		"app.layoutProfile==='mobile'",
		"clearPresentation",
		"liveTerminalIDsInTabOrder",
		"terminalIDsInTabOrder",
		"liveSessionIDs.filter(id=>app.views.get(id)?.meta?.task_id===0)",
		"active_session_id",
		"left_session_id",
		"right_session_id",
		"orientation",
		"splits",
		"live_splits",
		"app.views.has(first)",
		"app.views.has(second)",
		"getGroups",
		"restoreTabOrder(ids)",
		"restoreProjectGroups",
		"restoredGroups(restored,ids)",
		"split?.suspendFocusTracking?.()",
		"app.activateView(activeID,{focus:false})",
		"split?.resumeFocusTracking?.(activeID)",
		"app.focusView?.(activeID)",
		"const introduced=new Set()",
		"liveSessionIDs.includes(left)",
		"liveSessionIDs.includes(right)",
		"introduced.has(right)",
		"introduced.has(second)",
		"if(view?.meta&&layoutOwnsTerminal(view.meta))ids.push(id)",
		"await app.syncSessions?.()",
		"const liveSessions=live.sessions||[]",
		"const existing=liveSessions.filter(meta=>meta.task_id===0&&meta.status==='running'&&layoutOwnsTerminal(meta))",
		"const hasLiveSplitViews=",
		"if(existing.length||hasLiveSplitViews)",
		"await waitForViews(ids)",
		"method:'POST'",
		"method:'PUT'",
		"pagehide",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminalrestore.js missing %q", want)
		}
	}
	if strings.Contains(js, "consoleText") || strings.Contains(js, "scrollback") {
		t.Fatal("terminal restore must not persist console/scrollback")
	}
	if strings.Contains(js, "used.has(left)") || strings.Contains(js, "used.has(group.first)") {
		t.Fatal("terminal restore must allow an existing split leaf to be split again")
	}
	if strings.Contains(js, "app.activateView(activeID);") {
		t.Fatal("terminal restore must not focus during intermediate layout activation")
	}
	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/terminalrestore.js';") {
		t.Fatal("next.js must load terminalrestore.js")
	}
}

func TestSharedTerminalRestoreNeverPersistsOtherUsersOrRepeatsForbiddenWrites(t *testing.T) {
    data,err:=webassets.Files.ReadFile("featuremods/terminalrestore.js")
    if err!=nil{t.Fatal(err)}
    js:=string(data)
    for _,want:=range []string{
        "function canPersistTerminalLayout()",
        "app.hasPermission?.('terminal.control_own')",
        "app.hasPermission?.('terminal.control_all')",
        "app.hasPermission?.('sessions.manage')",
        "function canRestoreTerminalLayout()",
        "app.hasPermission?.('terminal.create')",
        "function layoutOwnsTerminal(meta)",
        "String(meta.owner_user_id||'')===String(app.currentUser?.user_id||'')",
        "if(view?.meta&&layoutOwnsTerminal(view.meta))ids.push(id)",
        "if(restoring||persistenceFrozen||!canPersistTerminalLayout())return Promise.resolve()",
        "if(canPersistTerminalLayout())await app.jsonFetch(endpoint,",
        "if(!canRestoreTerminalLayout())return",
        "if(!persistenceFrozen&&canPersistTerminalLayout())scheduleSave(500)",
    }{
        if !strings.Contains(js,want){t.Errorf("shared terminal restore protection missing %q",want)}
    }
}
