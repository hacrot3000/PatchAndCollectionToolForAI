package server

import (
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/config"
)

func TestSharedFilesUploadCannotOverwriteManagedState(t *testing.T) {
    root:=t.TempDir()
    if err:=os.MkdirAll(filepath.Join(root,".vscode"),0o700);err!=nil{t.Fatal(err)}
    private:=filepath.Join(root,".vscode","vscode_tasks_menu.ini")
    if err:=os.WriteFile(private,[]byte("PRIVATE_TASKDECK_CONFIG=keep\n"),0o600);err!=nil{t.Fatal(err)}
    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    s:=&Server{Config:cfg,Workspace:root}
    upload:=http.HandlerFunc(s.fileUpload)

    for _,tc:=range []struct{dir,name string;overwrite bool}{
        {".vscode","vscode_tasks_menu.ini",true},
        {".vscode","vscode_tasks_menu.new.json",false},
        {"","vscode_tasks_menu.filetransfer.jobs.json",false},
    } {
        result:=uploadRequest(t,upload,tc.dir,tc.name,"ATTACKER_REPLACEMENT",tc.overwrite)
        if result.Code!=http.StatusForbidden{
            t.Fatalf("managed upload allowed at %q/%q: %d %s",tc.dir,tc.name,result.Code,result.Body)
        }
    }
    original,err:=os.ReadFile(private)
    if err!=nil||string(original)!="PRIVATE_TASKDECK_CONFIG=keep\n" {
        t.Fatalf("private config was overwritten: %q %v",original,err)
    }
    if _,err:=os.Stat(filepath.Join(root,".vscode","vscode_tasks_menu.new.json"));!os.IsNotExist(err){
        t.Fatalf("protected file unexpectedly created: %v",err)
    }
    alias:=filepath.Join(root,"innocuous-folder")
    if err:=os.Symlink(filepath.Join(root,".vscode"),alias);err==nil {
        result:=uploadRequest(t,upload,"innocuous-folder","vscode_tasks_menu.ini","ATTACKER_REPLACEMENT",true)
        if result.Code!=http.StatusForbidden {
            t.Fatalf("symlink alias enabled private config overwrite: %d %s",result.Code,result.Body)
        }
    }
    resolvedDir,err:=s.resolveUploadDir(".vscode")
    if err!=nil{t.Fatalf("ordinary .vscode directory unexpectedly restricted: %v",err)}
    canonicalRoot,err:=s.projectRoot()
    if err!=nil{t.Fatal(err)}
    if sharedManagedResolvedPath(canonicalRoot,filepath.Join(resolvedDir,"tasks.json")) {
        t.Fatal("ordinary .vscode/tasks.json should remain uploadable")
    }

    // Single-user mode is intentionally backward compatible.
    cfg.SharedServerEnabled=false
    s.Config=cfg
    result:=uploadRequest(t,upload,".vscode","vscode_tasks_menu.ini","LOCAL_REPLACEMENT",true)
    if result.Code!=http.StatusCreated {
        t.Fatalf("local managed upload unexpectedly blocked: %d %s",result.Code,result.Body)
    }
    after,err:=os.ReadFile(private)
    if err!=nil||!strings.Contains(string(after),"LOCAL_REPLACEMENT"){
        t.Fatalf("local upload did not preserve historical behavior: %q %v",after,err)
    }
}
