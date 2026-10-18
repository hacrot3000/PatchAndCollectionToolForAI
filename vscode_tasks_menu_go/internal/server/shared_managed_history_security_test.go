package server

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/config"
)

// The legacy local-history store can hold an old managed-state snapshot even
// after the live source file is deleted or moved to private server storage.
// A shared files.read user must not retrieve its index or historical contents.
func TestSharedManagedStateHistoryRejectsOldPrivateVersions(t *testing.T) {
    workspace:=t.TempDir()
    private:=".vscode/vscode_tasks_menu.ini"
    secret:="TASKDECK_HISTORY_PRIVATE_SECRET=never-expose\n"
    if err:=recordProjectFileHistory(workspace,private,[]byte(secret));err!=nil{t.Fatal(err)}
    idx,err:=readProjectFileHistoryIndex(workspace,private)
    if err!=nil||len(idx.Entries)!=1{t.Fatalf("private history setup: %v, %+v",err,idx)}
    if err:=recordProjectFileHistory(workspace,"notes.txt",[]byte("regular source history\n"));err!=nil{t.Fatal(err)}
    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    s:=&Server{Workspace:workspace,Config:cfg}
    get:=func(path string)*httptest.ResponseRecorder{
        t.Helper()
        r:=httptest.NewRecorder()
        s.projectFileHistory(r,httptest.NewRequest(http.MethodGet,path,nil))
        return r
    }
    for _,path:=range []string{
        "/api/project/file-history?path="+urlQueryEscape(private),
        "/api/project/file-history?path="+urlQueryEscape(private)+"&id="+idx.Entries[0].ID,
    }{
        rr:=get(path)
        if rr.Code!=http.StatusNotFound||strings.Contains(rr.Body.String(),secret){
            t.Fatalf("managed history leak: %s status=%d response=%s",path,rr.Code,rr.Body.String())
        }
    }
    regular:=get("/api/project/file-history?path=notes.txt")
    if regular.Code!=http.StatusOK||!strings.Contains(regular.Body.String(),"notes.txt"){
        t.Fatalf("ordinary file history unexpectedly blocked: %d %s",regular.Code,regular.Body.String())
    }
    cfg.SharedServerEnabled=false
    s.Config=cfg
    local:=get("/api/project/file-history?path="+urlQueryEscape(private)+"&id="+idx.Entries[0].ID)
    if local.Code!=http.StatusOK||!strings.Contains(local.Body.String(),"TASKDECK_HISTORY_PRIVATE_SECRET"){
        t.Fatalf("single-user history behavior regressed: %d %s",local.Code,local.Body.String())
    }
}
