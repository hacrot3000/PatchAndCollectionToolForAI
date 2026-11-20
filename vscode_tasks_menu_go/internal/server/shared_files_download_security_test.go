package server

import (
    "net/http"
    "net/http/httptest"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/config"
)

func TestSharedFilesDownloadAndSelectionHideManagedState(t *testing.T) {
    root:=t.TempDir()
    if err:=os.MkdirAll(filepath.Join(root,".vscode"),0o700);err!=nil{t.Fatal(err)}
    private:=filepath.Join(root,".vscode","vscode_tasks_menu.ini")
    if err:=os.WriteFile(private,[]byte("PRIVATE_TASKDECK_CREDENTIAL=secret\n"),0o600);err!=nil{t.Fatal(err)}
    ordinary:=filepath.Join(root,"ordinary.txt")
    if err:=os.WriteFile(ordinary,[]byte("ordinary content\n"),0o644);err!=nil{t.Fatal(err)}
    alias:=filepath.Join(root,"harmless-alias.txt")
    symlinkSupported:=os.Symlink(private,alias)==nil

    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    s:=&Server{Config:cfg,Workspace:root}

    for _,candidate:=range []string{".vscode/vscode_tasks_menu.ini",private}{
        if got,err:=s.resolveDownloadPath(candidate);err==nil {
            t.Fatalf("managed file downloadable via %q at %q",candidate,got)
        }
    }
    if symlinkSupported {
        if _,err:=s.resolveDownloadPath("harmless-alias.txt");err==nil {
            t.Fatal("managed-state symlink alias was downloadable")
        }
    }

    for _,candidate:=range []string{".vscode/vscode_tasks_menu.ini",private}{
        rr:=httptest.NewRecorder()
        req:=httptest.NewRequest(http.MethodGet,"/api/files/download?path="+url.QueryEscape(candidate),nil)
        s.fileDownload(rr,req)
        if rr.Code!=http.StatusNotFound||strings.Contains(rr.Body.String(),"PRIVATE_TASKDECK_CREDENTIAL"){
            t.Fatalf("download exposed %q: %d %s",candidate,rr.Code,rr.Body.String())
        }
    }
    if symlinkSupported {
        rr:=httptest.NewRecorder()
        s.fileDownload(rr,httptest.NewRequest(http.MethodGet,
            "/api/files/download?path="+url.QueryEscape(alias),nil))
        if rr.Code!=http.StatusNotFound {
            t.Fatalf("alias download status=%d",rr.Code)
        }
    }

    for _,name:=range []string{".vscode/vscode_tasks_menu.ini",private} {
        if list:=s.downloadableFilesFromSelectionAt(name,"","");len(list)!=0 {
            t.Fatalf("file selection offered a private download: %+v",list)
        }
    }
    if symlinkSupported {
        if list:=s.downloadableFilesFromSelectionAt(alias,"","");len(list)!=0 {
            t.Fatalf("file selection offered a private alias: %+v",list)
        }
    }
    rr:=httptest.NewRecorder()
    s.fileDownload(rr,httptest.NewRequest(http.MethodGet,
        "/api/files/download?path="+url.QueryEscape(ordinary),nil))
    if rr.Code!=http.StatusOK||!strings.Contains(rr.Body.String(),"ordinary content") {
        t.Fatalf("ordinary shared file download regressed: %d %s",rr.Code,rr.Body.String())
    }

    // Only the shared browser mode gets this restriction. A local owner
    // retains the existing ability to inspect their own TaskDeck state.
    cfg.SharedServerEnabled=false
    s.Config=cfg
    if _,err:=s.resolveDownloadPath(private);err!=nil {
        t.Fatalf("single-user managed file unexpectedly blocked: %v",err)
    }
}
