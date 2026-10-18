package server

import (
 "archive/zip"
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
)

func TestSharedManagedProjectPathClassification(t *testing.T) {
 for _,path:=range []string{
  ".vscode/vscode_tasks_menu.ini",
  "vscode_tasks_menu.filetransfer.jobs.json",
  "subdir/vscode_tasks_menu.private.json",
  ".vscode/../.vscode/VSCODE_TASKS_MENU.ini",
  ".vscode\\vscode_tasks_menu.ini",
 } {
  if !isTaskDeckManagedProjectPath(path){t.Fatalf("managed state path not protected: %q",path)}
 }
 for _,path:=range []string{
  ".vscode/tasks.json",
  "src/main.go",
  "vscode_tasks_menu_go/main.go",
  "docs/vscode_tasks_menu",
 }{
  if isTaskDeckManagedProjectPath(path){t.Fatalf("ordinary source classified private: %q",path)}
 }
}

func TestSharedProjectRoutesCannotReadOrArchiveManagedState(t *testing.T) {
 root:=t.TempDir()
 private:=".vscode/vscode_tasks_menu.ini"
 if err:=os.MkdirAll(filepath.Join(root,".vscode"),0o755);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,filepath.FromSlash(private)),
  []byte("TASKDECK_MANAGED_TEST_SECRET=private\n"),0o600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,".vscode","tasks.json"),
  []byte(`{"version":"2.0.0","tasks":[]}`),0o644);err!=nil{t.Fatal(err)}
 cfg:=config.Default()
 cfg.SharedServerEnabled=true
 s:=&Server{Config:cfg,Workspace:root}
 if _,err:=s.resolveProjectPath(private,false,false);err==nil{
  t.Fatal("direct managed state read unexpectedly allowed")
 }
 if _,err:=s.resolveProjectPath(".vscode/tasks.json",false,false);err!=nil{
  t.Fatalf("ordinary VS Code tasks file unexpectedly blocked: %v",err)
 }
 alias:=filepath.Join(root,"safe-name.txt")
 if err:=os.Symlink(filepath.Join(root,filepath.FromSlash(private)),alias);err==nil{
  if _,err:=s.resolveProjectPath("safe-name.txt",false,false);err==nil{
   t.Fatal("symlink alias to managed state was permitted")
  }
 }
 tree:=httptest.NewRecorder()
 s.projectTree(tree,httptest.NewRequest(http.MethodGet,"/api/project/tree?path=.vscode",nil))
 if tree.Code!=http.StatusOK{t.Fatalf("Explorer: %d %s",tree.Code,tree.Body)}
 if strings.Contains(tree.Body.String(),"vscode_tasks_menu.ini")||
  !strings.Contains(tree.Body.String(),"tasks.json"){
  t.Fatalf("Explorer leaked private file or hid ordinary tasks.json: %s",tree.Body)
 }
 read:=httptest.NewRecorder()
 s.projectFileRead(read,httptest.NewRequest(http.MethodGet,
  "/api/project/file?path=.vscode/vscode_tasks_menu.ini",nil))
 if read.Code!=http.StatusNotFound{t.Fatalf("direct file read status=%d",read.Code)}
 chunk:=httptest.NewRecorder()
 s.projectBytes(chunk,httptest.NewRequest(http.MethodGet,
  "/api/project/bytes?path=.vscode/vscode_tasks_menu.ini",nil))
 if chunk.Code!=http.StatusNotFound{t.Fatalf("binary chunk status=%d",chunk.Code)}
 search:=httptest.NewRecorder()
 s.projectFileSearch(search,httptest.NewRequest(http.MethodGet,
  "/api/project/search?q=vscode_tasks_menu&refresh=1",nil))
 if search.Code!=http.StatusOK||strings.Contains(search.Body.String(),"vscode_tasks_menu.ini"){
  t.Fatalf("filename search returned private state: %d %s",search.Code,search.Body)
 }
 content:=httptest.NewRecorder()
 s.projectContentSearch(content,httptest.NewRequest(http.MethodGet,
  "/api/project/content-search?q=TASKDECK_MANAGED_TEST_SECRET&path=.vscode",nil))
 if content.Code!=http.StatusOK||strings.Contains(content.Body.String(),"TASKDECK_MANAGED_TEST_SECRET"){
  t.Fatalf("content search returned private state: %d %s",content.Code,content.Body)
 }
 request:=httptest.NewRequest(http.MethodPost,"/api/project/archive/download",
  strings.NewReader(`{"paths":[".vscode"],"name":"test.zip"}`))
 request.Header.Set("Content-Type","application/json")
 archive:=httptest.NewRecorder()
 s.projectArchiveDownload(archive,request)
 if archive.Code!=http.StatusOK{t.Fatalf("archive HTTP status=%d: %s",archive.Code,archive.Body)}
 archiveBytes:=archive.Body.Bytes()
 zipReader,err:=zip.NewReader(bytes.NewReader(archiveBytes),int64(len(archiveBytes)))
 if err!=nil{t.Fatal(err)}
 sawTasks:=false
 for _,entry:=range zipReader.File {
  if strings.Contains(entry.Name,"vscode_tasks_menu.ini"){t.Fatal("archive included managed state")}
  if strings.HasSuffix(entry.Name,"tasks.json"){sawTasks=true}
 }
 if !sawTasks{t.Fatal("archive unexpectedly excluded ordinary VS Code tasks.json")}
 direct:=httptest.NewRecorder()
 directReq:=httptest.NewRequest(http.MethodPost,"/api/project/archive/download",
  strings.NewReader(`{"paths":[".vscode/vscode_tasks_menu.ini"]}`))
 directReq.Header.Set("Content-Type","application/json")
 s.projectArchiveDownload(direct,directReq)
 if direct.Code==http.StatusOK{t.Fatal("direct archive export of managed state succeeded")}
}

func TestSharedManagedFileNotHostTransferable(t *testing.T) {
 root:=t.TempDir()
 if err:=os.MkdirAll(filepath.Join(root,".vscode"),0o755);err!=nil{t.Fatal(err)}
 private:=filepath.Join(root,".vscode","vscode_tasks_menu.ini")
 if err:=os.WriteFile(private,[]byte("private"),0o600);err!=nil{t.Fatal(err)}
 cfg:=config.Default()
 cfg.SharedServerEnabled=true
 s:=&Server{Config:cfg,Workspace:root}
 if _,_,_,err:=s.resolveHostWorkspaceEntry(".vscode/vscode_tasks_menu.ini");err==nil{
  t.Fatal("host transfer accepted managed state")
 }
 if _,_,_,err:=s.resolveHostWorkspaceEntry(".vscode");err==nil{
  t.Fatal("host transfer accepted entire .vscode directory")
 }
 if _,_,err:=s.resolveHostWorkspaceDestination(".vscode/vscode_tasks_menu.secret");err==nil{
  t.Fatal("host transfer accepted private managed destination")
 }
 cfg.SharedServerEnabled=false
 s.Config=cfg
 if _,err:=s.resolveProjectPath(".vscode/vscode_tasks_menu.ini",false,false);err!=nil{
  t.Fatalf("local single-user project access regressed: %v",err)
 }
}

func TestSharedManagedStateResponsesDoNotLeakJSONContent(t *testing.T) {
 // Browser serializers should not surface the internal-state identifier in
 // otherwise empty responses.
 payload,err:=json.Marshal(map[string]any{"results":[]string{}})
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(payload,[]byte("vscode_tasks_menu.ini")){t.Fatal("unexpected internal-state leak")}
}
