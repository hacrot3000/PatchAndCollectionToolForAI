package server

import (
    "net/http"
    "net/http/httptest"
    "net/url"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/config"
)

// Git reads the repository object database, not the live source file.
// Historical internal-state blobs must not bypass the shared source guard.
func TestSharedManagedGitHistoryAndBlobReadsFailClosed(t *testing.T) {
    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    server:=&Server{Config:cfg}
    sha:=strings.Repeat("a",40)
    for _,name:=range []string{
        ".vscode/vscode_tasks_menu.ini",
        "vscode_tasks_menu.filetransfer.jobs.json",
        "nested/VSCODE_TASKS_MENU.private.json",
    }{
        t.Run(name,func(t *testing.T){
            file:=url.QueryEscape(name)
            for _,tc:=range []struct{
                label,path string
                handler func(http.ResponseWriter,*http.Request)
            }{
                {"git history","/api/git/status?view=file-history&path="+file,server.gitFileHistory},
                {"git diff","/api/git/status?view=diff&path="+file+"&mode=unstaged",server.gitDiff},
                {"git blame","/api/git/status?view=blame&path="+file,server.gitFileBlame},
                {"historical blob","/api/git/status?view=file-content&path="+file+"&ref="+sha,server.gitFileContent},
                {"index blob","/api/git/status?view=file-content&path="+file+"&state=index",server.gitFileContent},
            }{
                rr:=httptest.NewRecorder()
                tc.handler(rr,httptest.NewRequest(http.MethodGet,tc.path,nil))
                if rr.Code!=http.StatusNotFound{
                    t.Fatalf("%s exposed internal file %q: status=%d response=%s",tc.label,name,rr.Code,rr.Body.String())
                }
            }
        })
    }
    normal:=httptest.NewRecorder()
    server.gitFileContent(normal,httptest.NewRequest(http.MethodGet,
        "/api/git/status?view=file-content&path=src/main.go&ref="+sha,nil))
    if normal.Code==http.StatusNotFound{
        t.Fatal("ordinary Git file content should not be denied as managed state")
    }
}

func TestSharedGitConflictStagesCannotAccessManagedState(t *testing.T) {
    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    server:=&Server{Config:cfg}
    private:=".vscode/vscode_tasks_menu.ini"
    req:=httptest.NewRequest(http.MethodGet,
        "/api/git/conflict-file?path="+url.QueryEscape(private),nil)
    read:=httptest.NewRecorder()
    server.gitConflictFileRead(read,req,gitRepository{})
    if read.Code!=http.StatusNotFound {
        t.Fatalf("Git index stage could expose protected content: %d %s",read.Code,read.Body.String())
    }
    writeReq:=httptest.NewRequest(http.MethodPut,"/api/git/conflict-file",
        strings.NewReader(`{"path":".vscode/vscode_tasks_menu.ini","content":"unsafe"}`))
    writeReq.Header.Set("Content-Type","application/json")
    written:=httptest.NewRecorder()
    server.gitConflictFileWrite(written,writeReq,gitRepository{})
    if written.Code!=http.StatusNotFound {
        t.Fatalf("Git conflict write accepted a protected path: %d %s",written.Code,written.Body.String())
    }
}
