package server

import (
    "strings"
    "testing"

    webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSharedTerminalBrowserStateIsScopedToAuthenticatedUser(t *testing.T) {
    files:=[]struct{path string;expected []string}{
        {"featuremods/split.js",[]string{
            "vscode-tasks-menu:split:",
            "app.sharedMode?':user:'+encodeURIComponent(app.currentUser?.user_id||'unknown')",
        }},
        {"featuremods/tabdrag.js",[]string{
            "vscode-tasks-menu:tab-order:",
            "app.sharedMode?':user:'+encodeURIComponent(app.currentUser?.user_id||'unknown')",
        }},
        {"featuremods/terminalrestore.js",[]string{
            "function layoutOwnsTerminal(meta)",
            "String(meta.owner_user_id||'')===String(app.currentUser?.user_id||'')",
            "function canPersistTerminalLayout()",
            "function canViewTerminalLayout()",
            "if(!canViewTerminalLayout())return;",
            "if(!canRestoreTerminalLayout())return;",
        }},
    }
    for _,file:=range files{
        data,err:=webassets.Files.ReadFile(file.path)
        if err!=nil{t.Fatal(err)}
        for _,want:=range file.expected{
            if !strings.Contains(string(data),want){t.Errorf("%s missing scoped state guard %q",file.path,want)}
        }
    }
}

func TestSharedTerminalStateAPIBypassesOnlyGlobalRouteMapping(t *testing.T) {
    data,err:=webassets.Files.ReadFile("featuremods/terminalrestore.js")
    if err!=nil{t.Fatal(err)}
    js:=string(data)
    if !strings.Contains(js,"scope=terminals&profile=") {
        t.Fatal("terminal UI must access dedicated owner-scoped state API")
    }
    // The regular shared authorization switch still rejects unknown API
    // routes; the terminal-state scope alone is handled specially and checks
    // ownership in s.sharedTerminalState.
    // Backend GET/PUT/POST rules and session ownership are tested in
    // shared_terminal_state_test.go. This check remains asset-only.

}
