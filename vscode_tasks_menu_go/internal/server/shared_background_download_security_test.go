package server

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/config"
)

// Background FTP/SFTP downloads can be resumed from persisted jobs without
// calling the interactive host path resolver. Check both scan and execution.
func TestSharedBackgroundHostDownloadRejectsManagedDestinations(t *testing.T) {
    root:=t.TempDir()
    if err:=os.MkdirAll(filepath.Join(root,".vscode"),0o700);err!=nil{t.Fatal(err)}
    private:=filepath.Join(root,".vscode","vscode_tasks_menu.ini")
    if err:=os.WriteFile(private,[]byte("DO_NOT_OVERWRITE"),0o600);err!=nil{t.Fatal(err)}
    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    s:=&Server{Config:cfg,Workspace:root}

    for _,rel:=range []string{
        ".vscode/vscode_tasks_menu.ini",
        ".vscode/vscode_tasks_menu.new",
        "vscode_tasks_menu.filetransfer.jobs.json",
        ".vscode/vscode_tasks_menu.secret/private.txt",
    }{
        if _,err:=s.backgroundHostTarget(rel,true);err==nil{
            t.Errorf("executor accepted managed download target %q",rel)
        }
        if _,_,_,err:=s.backgroundHostExistingMeta(rel);err==nil{
            t.Errorf("scanner accepted managed download target %q",rel)
        }
    }
    if _,err:=s.ensureBackgroundHostDirectory(".vscode/vscode_tasks_menu.private");err==nil{
        t.Fatal("downloader created protected managed-state directory")
    }
    if _,err:=os.Stat(filepath.Join(root,".vscode","vscode_tasks_menu.private"));!os.IsNotExist(err){
        t.Fatalf("protected destination directory was created: %v",err)
    }
    file,err:=os.ReadFile(private)
    if err!=nil||string(file)!="DO_NOT_OVERWRITE" {
        t.Fatalf("protected file changed: %q %v",file,err)
    }

    // Normal source files inside .vscode are not TaskDeck managed state.
    if dir,err:=s.ensureBackgroundHostDirectory(".vscode");err!=nil||dir==""{
        t.Fatalf("ordinary .vscode directory blocked: %q %v",dir,err)
    }
    if path,err:=s.backgroundHostTarget(".vscode/tasks.json",false);err!=nil||
        !strings.HasSuffix(path,filepath.Join(".vscode","tasks.json")){
        t.Fatalf("safe download target rejected: %q %v",path,err)
    }

    cfg.SharedServerEnabled=false
    s.Config=cfg
    if _,err:=s.backgroundHostTarget(".vscode/vscode_tasks_menu.ini",true);err!=nil{
        t.Fatalf("local single-user download behavior regressed: %v",err)
    }
}
