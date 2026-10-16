package server

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/identity"
    "bletonfc/vscode_tasks_menu/internal/session"
    "bletonfc/vscode_tasks_menu/internal/tasks"
)

func sharedTerminalStatePrincipal(user, project string, permissions ...string) identity.Principal {
    rights:=make(map[string]bool,len(permissions))
    for _,permission:=range permissions {rights[permission]=true}
    return identity.Principal{UserID:identity.UserID(user),ProjectID:identity.ProjectID(project),Permissions:rights}
}

func sharedTerminalStateRequest(t *testing.T,s *Server,principal identity.Principal,method,body string) *httptest.ResponseRecorder {
    t.Helper()
    rr:=httptest.NewRecorder()
    s.terminalState(rr,sharedSessionRequest(method,"/api/state/tasks?scope=terminals&profile=desktop",body,principal))
    return rr
}

func TestSharedTerminalStateIsPrivatePerUserAndProject(t *testing.T) {
    private:=t.TempDir()
    t.Setenv("XDG_CONFIG_HOME",private)
    t.Setenv("APPDATA",private)
    workspace:=t.TempDir()
    service:=&ownershipTestService{supported:true,items:[]session.Metadata{
        {ID:"alice-shell",Kind:tasks.SessionKindTerminal,OwnerUserID:"alice",ProjectID:"p1",Cwd:workspace,Status:"running"},
        {ID:"bob-shell",Kind:tasks.SessionKindTerminal,OwnerUserID:"bob",ProjectID:"p1",Cwd:workspace,Status:"running"},
        {ID:"alice-other",Kind:tasks.SessionKindTerminal,OwnerUserID:"alice",ProjectID:"p2",Cwd:workspace,Status:"running"},
    }}
    s:=sharedSessionTestServer(t,service)
    s.Workspace=workspace
    alice:=sharedTerminalStatePrincipal("alice","p1",identity.PermissionTerminalViewOwn,identity.PermissionTerminalControlOwn)
    bob:=sharedTerminalStatePrincipal("bob","p1",identity.PermissionTerminalViewOwn,identity.PermissionTerminalControlOwn)
    otherProject:=sharedTerminalStatePrincipal("alice","p2",identity.PermissionTerminalViewOwn,identity.PermissionTerminalControlOwn)

    rr:=sharedTerminalStateRequest(t,s,alice,http.MethodPut,`{"session_ids":["alice-shell"],"active_session_id":"alice-shell"}`)
    if rr.Code!=http.StatusOK{t.Fatalf("Alice save status=%d body=%s",rr.Code,rr.Body.String())}
    var saved projectTerminalState
    if err:=json.Unmarshal(rr.Body.Bytes(),&saved);err!=nil{t.Fatal(err)}
    if len(saved.Terminals)!=1||saved.Terminals[0].SessionID!="alice-shell"{
        t.Fatalf("Alice's private snapshot was not preserved: %+v",saved)
    }
    for _,other:=range []identity.Principal{bob,otherProject}{
        rr=sharedTerminalStateRequest(t,s,other,http.MethodGet,"")
        if rr.Code!=http.StatusOK{t.Fatalf("other reader status=%d",rr.Code)}
        var empty projectTerminalState
        if err:=json.Unmarshal(rr.Body.Bytes(),&empty);err!=nil{t.Fatal(err)}
        if len(empty.Terminals)!=0{t.Fatalf("cross-user/project terminal leak: %+v",empty)}
    }
    rr=sharedTerminalStateRequest(t,s,bob,http.MethodPut,`{"session_ids":["bob-shell"]}`)
    if rr.Code!=http.StatusOK{t.Fatalf("Bob save status=%d body=%s",rr.Code,rr.Body.String())}
    rr=sharedTerminalStateRequest(t,s,alice,http.MethodGet,"")
    if !strings.Contains(rr.Body.String(),"alice-shell")||strings.Contains(rr.Body.String(),"bob-shell"){
        t.Fatalf("Bob's terminal replaced Alice's snapshot: %s",rr.Body.String())
    }
    alicePath,err:=s.sharedTerminalStatePath(alice,"desktop")
    if err!=nil{t.Fatal(err)}
    bobPath,err:=s.sharedTerminalStatePath(bob,"desktop")
    if err!=nil{t.Fatal(err)}
    if alicePath==bobPath||strings.HasPrefix(alicePath,workspace+string(filepath.Separator)){
        t.Fatalf("snapshots are not independent/private: %q vs %q",alicePath,bobPath)
    }
    file,err:=os.Stat(alicePath)
    if err!=nil{t.Fatal(err)}
    if file.Mode().Perm()!=0600{t.Fatalf("snapshot file permissions: %v",file.Mode())}
}

func TestSharedTerminalStateRejectsForeignAndUnauthorizedSaves(t *testing.T) {
    private:=t.TempDir()
    t.Setenv("XDG_CONFIG_HOME",private)
    t.Setenv("APPDATA",private)
    workspace:=t.TempDir()
    service:=&ownershipTestService{supported:true,items:[]session.Metadata{
        {ID:"alice-shell",Kind:tasks.SessionKindTerminal,OwnerUserID:"alice",ProjectID:"p1",Cwd:workspace,Status:"running"},
        {ID:"bob-shell",Kind:tasks.SessionKindTerminal,OwnerUserID:"bob",ProjectID:"p1",Cwd:workspace,Status:"running"},
    }}
    s:=sharedSessionTestServer(t,service)
    s.Workspace=workspace
    alice:=sharedTerminalStatePrincipal("alice","p1",identity.PermissionTerminalViewOwn,identity.PermissionTerminalControlOwn)
    rr:=sharedTerminalStateRequest(t,s,alice,http.MethodPut,`{"session_ids":["bob-shell"]}`)
    if rr.Code!=http.StatusForbidden{t.Fatalf("foreign terminal save status=%d body=%s",rr.Code,rr.Body.String())}
    rr=sharedTerminalStateRequest(t,s,alice,http.MethodPut,`{"session_ids":["alice-shell","bob-shell"]}`)
    if rr.Code!=http.StatusForbidden{t.Fatalf("mixed foreign terminal save status=%d",rr.Code)}
    viewer:=sharedTerminalStatePrincipal("alice","p1",identity.PermissionTerminalViewOwn)
    rr=sharedTerminalStateRequest(t,s,viewer,http.MethodPut,`{"session_ids":["alice-shell"]}`)
    if rr.Code!=http.StatusForbidden{t.Fatalf("read-only account saved terminal state: %d",rr.Code)}
    rr=sharedTerminalStateRequest(t,s,viewer,http.MethodGet,"")
    if rr.Code!=http.StatusOK{t.Fatalf("own read failed: %d",rr.Code)}
    none:=sharedTerminalStatePrincipal("alice","p1")
    rr=sharedTerminalStateRequest(t,s,none,http.MethodGet,"")
    if rr.Code!=http.StatusForbidden{t.Fatalf("unpermitted terminal read status=%d",rr.Code)}
    rr=sharedTerminalStateRequest(t,s,viewer,http.MethodPost,"")
    if rr.Code!=http.StatusCreated {t.Fatalf("empty layout should not attempt unauthorized shell creation, status=%d",rr.Code)}
    if len(service.started)!=0{t.Fatal("unauthorized restore started a terminal")}
}

func TestSharedTerminalStateDoesNotImportLegacyProjectSnapshot(t *testing.T) {
    private:=t.TempDir()
    t.Setenv("XDG_CONFIG_HOME",private)
    t.Setenv("APPDATA",private)
    workspace:=t.TempDir()
    service:=&ownershipTestService{supported:true}
    s:=sharedSessionTestServer(t,service)
    s.Workspace=workspace
    legacy:=projectTerminalState{Terminals:[]terminalStateItem{{Cwd:workspace,SessionID:"legacy-unowned"}}}
    if err:=writeProjectTerminalState(workspace,legacy);err!=nil{t.Fatal(err)}
    principal:=sharedTerminalStatePrincipal("alice","p1",identity.PermissionTerminalViewOwn)
    rr:=sharedTerminalStateRequest(t,s,principal,http.MethodGet,"")
    if rr.Code!=http.StatusOK||strings.Contains(rr.Body.String(),"legacy-unowned"){
        t.Fatalf("legacy global snapshot leaked to shared account: %d %s",rr.Code,rr.Body.String())
    }
    count,warnings,err:=s.RestoreProjectTerminalsForStartup()
    if err!=nil||count!=0||len(warnings)!=0||len(service.started)!=0{
        t.Fatalf("shared self-update restored ownerless legacy terminal: count=%d warnings=%v err=%v",count,warnings,err)
    }
}
