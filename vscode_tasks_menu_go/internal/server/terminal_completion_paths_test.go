package server

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/session"
)

func TestTerminalCompletionPathsAndGitBranchesUseSessionCWD(t *testing.T) {
 root, _, _ := setupGitQuickRepo(t)
 for _,dir := range []string{"Client","Client/assets","Client/My Folder"} {
  if err:=os.MkdirAll(filepath.Join(root,dir),0755);err!=nil {t.Fatal(err)}
 }
 if err:=os.WriteFile(filepath.Join(root,"Client","aside.txt"),[]byte("ok"),0644);err!=nil {t.Fatal(err)}
 gitQuickRun(t,root,"branch","testing")
 gitQuickRun(t,root,"branch","ss20-dev-hp64")
 manager:=session.NewManager(1<<20)
 defer manager.Shutdown(time.Second)
 spec,err:=workspaceTerminalExecutionAt(root,".")
 if err!=nil {t.Fatal(err)}
 meta,err:=manager.Start(spec)
 if err!=nil {t.Fatal(err)}
 s:=&Server{Workspace:root,Sessions:manager}
 read:=func(query string) (int,map[string]json.RawMessage){
  t.Helper()
  req:=httptest.NewRequest(http.MethodGet,"/api/sessions/"+meta.ID+"/completion-paths?"+query,nil)
  rr:=httptest.NewRecorder()
  s.sessionItem(rr,req)
  var response map[string]json.RawMessage
  if rr.Code==http.StatusOK {
   if err:=json.Unmarshal(rr.Body.Bytes(),&response);err!=nil {t.Fatal(err)}
  }
  return rr.Code,response
 }
 code,data:=read("prefix=Cli")
 if code!=200 {t.Fatalf("root suggestions status=%d",code)}
 var entries []struct{Name string `json:"name"`;Dir bool `json:"dir"`}
 if err:=json.Unmarshal(data["entries"],&entries);err!=nil {t.Fatal(err)}
 if len(entries)!=1 || entries[0].Name!="Client" || !entries[0].Dir {t.Fatalf("root entries=%+v",entries)}
 code,data=read("dir=Client%2F&prefix=as")
 if code!=200 {t.Fatalf("nested suggestions status=%d",code)}
 if err:=json.Unmarshal(data["entries"],&entries);err!=nil {t.Fatal(err)}
 names:=map[string]bool{}
 for _,entry:=range entries {names[entry.Name]=entry.Dir}
 if !names["assets"] {t.Fatalf("nested entries=%+v",entries)}
 if _,ok:=names["aside.txt"];!ok {t.Fatalf("missing matching file: %+v",entries)}
 code,data=read("kind=branches&prefix=ss20")
 if code!=200 {t.Fatalf("branch completion status=%d",code)}
 var branches []string
 if err:=json.Unmarshal(data["branches"],&branches);err!=nil {t.Fatal(err)}
 if len(branches)!=1 || branches[0]!="ss20-dev-hp64" {t.Fatalf("branch hints=%v",branches)}
 code,data=read("kind=branches&prefix=tes")
 if code!=200 {t.Fatalf("testing branch status=%d",code)}
 if err:=json.Unmarshal(data["branches"],&branches);err!=nil {t.Fatal(err)}
 if len(branches)!=1 || branches[0]!="testing" {t.Fatalf("branch hints=%v",branches)}
}

func TestTerminalCompletionPathsRejectsUnsafeInputs(t *testing.T) {
 root:=t.TempDir()
 manager:=session.NewManager(64<<10)
 defer manager.Shutdown(time.Second)
 spec,err:=workspaceTerminalExecutionAt(root,".")
 if err!=nil {t.Fatal(err)}
 meta,err:=manager.Start(spec)
 if err!=nil {t.Fatal(err)}
 s:=&Server{Workspace:root,Sessions:manager}
 for _,query:=range []string{
  "prefix=ab%2Fcd",
  "prefix=hello%0Athere",
  "prefix=%2A",
  "kind=invalid",
  "kind=paths&dir=%5B",
 }{
  req:=httptest.NewRequest(http.MethodGet,"/api/sessions/"+meta.ID+"/completion-paths?"+query,nil)
  rr:=httptest.NewRecorder();s.sessionItem(rr,req)
  if rr.Code!=http.StatusBadRequest {t.Fatalf("query=%s status=%d: %s",query,rr.Code,rr.Body.String())}
 }
 req:=httptest.NewRequest(http.MethodPost,"/api/sessions/"+meta.ID+"/completion-paths",strings.NewReader(""))
 rr:=httptest.NewRecorder();s.sessionItem(rr,req)
 if rr.Code!=http.StatusMethodNotAllowed {t.Fatalf("POST completion status=%d",rr.Code)}
}


func TestTerminalExecutableNamesFromPATHWithoutHardcodedCommands(t *testing.T) {
 dir:=t.TempDir()
 execFile:=filepath.Join(dir,"telnet")
 if err:=os.WriteFile(execFile,[]byte("#!/bin/sh\nexit 0\n"),0755);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"telemetry"),[]byte("not executable"),0644);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"TelDemo"),[]byte("#!/bin/sh\nexit 0\n"),0755);err!=nil {t.Fatal(err)}
 results:=terminalExecutableNames(dir,"tel")
 found:=map[string]bool{}
 for _,name:=range results {found[name]=true}
 if !found["telnet"] || !found["TelDemo"] {t.Fatalf("PATH discovery missed executable: %v",results)}
 if found["telemetry"] {t.Fatalf("non-executable leaked into command suggestions: %v",results)}
 if got:=terminalExecutableNames(dir,"");len(got)!=0 {t.Fatalf("empty prefix should not scan PATH: %v",got)}
 if got:=terminalExecutableNames(dir,"te/");len(got)!=0 {t.Fatalf("path prefix must be rejected: %v",got)}
}

func TestTerminalCommandCompletionUsesPATHAndDoesNotRunCommand(t *testing.T) {
 root:=t.TempDir()
 execDir:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(execDir,"telnet"),[]byte("#!/bin/sh\nexit 0\n"),0755);err!=nil {t.Fatal(err)}
 manager:=session.NewManager(64<<10)
 defer manager.Shutdown(time.Second)
 spec,err:=workspaceTerminalExecutionAt(root,".")
 if err!=nil {t.Fatal(err)}
 meta,err:=manager.Start(spec)
 if err!=nil {t.Fatal(err)}
 s:=&Server{Workspace:root,Sessions:manager}
 t.Setenv("PATH",execDir)
 get:=func(query string) (int,map[string]json.RawMessage) {
  t.Helper()
  req:=httptest.NewRequest(http.MethodGet,"/api/sessions/"+meta.ID+"/completion-paths?"+query,nil)
  rr:=httptest.NewRecorder();s.sessionItem(rr,req)
  var data map[string]json.RawMessage
  if rr.Code==http.StatusOK {if err:=json.Unmarshal(rr.Body.Bytes(),&data);err!=nil{t.Fatal(err)}}
  return rr.Code,data
 }
 code,body:=get("kind=commands&prefix=tel")
 if code!=http.StatusOK {t.Fatalf("command completion status %d",code)}
 var commands []string
 if err:=json.Unmarshal(body["commands"],&commands);err!=nil{t.Fatal(err)}
 if len(commands)!=1 || commands[0]!="telnet" {t.Fatalf("expected PATH telnet, got %v",commands)}
 code,_=get("kind=commands&dir=private&prefix=tel")
 if code!=http.StatusBadRequest {t.Fatalf("commands must not accept arbitrary dirs, got %d",code)}
 // The endpoint only reads PATH directories; no command is ever executed.
}
