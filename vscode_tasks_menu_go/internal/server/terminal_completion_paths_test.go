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
 gitQuickRun(t,root,"branch","develop/new_SNV")
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
 code,data=read("kind=branches&prefix=develop%2Fnew")
 if code!=200 {t.Fatalf("namespaced branch lookup status=%d",code)}
 if err:=json.Unmarshal(data["branches"],&branches);err!=nil {t.Fatal(err)}
 if len(branches)!=1 || branches[0]!="develop/new_SNV" {t.Fatalf("namespaced branch hints=%v",branches)}
 code,data=read("kind=branches&prefix=tes")
 if code!=200 {t.Fatalf("testing branch status=%d",code)}
 if err:=json.Unmarshal(data["branches"],&branches);err!=nil {t.Fatal(err)}
 if len(branches)!=1 || branches[0]!="testing" {t.Fatalf("branch hints=%v",branches)}
}

// Single-user terminals produced by workspaceTerminalExecution have Kind="".
// Rejecting them prevented both autocomplete and its diagnostic HTTP probe.
func TestTerminalCompletionAcceptsLegacyLocalSessionOnlyOutsideSharedMode(t *testing.T) {
 root:=t.TempDir()
 if err:=os.Mkdir(filepath.Join(root,"a_python_patch"),0755);err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(root,"a_python_patch_ok"),0755);err!=nil {t.Fatal(err)}
 manager:=session.NewManager(64<<10)
 defer manager.Shutdown(time.Second)
 spec,err:=workspaceTerminalExecutionAt(root,".")
 if err!=nil {t.Fatal(err)}
 spec.SessionKind=""
 spec.OwnerUserID=""
 spec.ProjectID=""
 meta,err:=manager.Start(spec)
 if err!=nil {t.Fatal(err)}
 if meta.Kind!="" || meta.TaskID!=0 {t.Fatalf("legacy terminal metadata unexpected: %+v",meta)}
 s:=&Server{Workspace:root,Sessions:manager}
 run:=func() (int,string) {
  req:=httptest.NewRequest(http.MethodGet,"/api/sessions/"+meta.ID+"/completion-paths?kind=paths&prefix=a_py",nil)
  rr:=httptest.NewRecorder()
  s.sessionCompletionPaths(rr,req,meta.ID)
  return rr.Code,rr.Body.String()
 }
 status,payload:=run()
 if status!=http.StatusOK {t.Fatalf("legacy local completion denied: %d %s",status,payload)}
 for _,want:=range []string{"a_python_patch","a_python_patch_ok"} {
  if !strings.Contains(payload,want){t.Fatalf("missing %q in completion: %s",want,payload)}
 }
 s.Config.SharedServerEnabled=true
 status,payload=run()
 if status!=http.StatusForbidden {t.Fatalf("legacy unowned terminal must be denied in shared-server mode: %d %s",status,payload)}
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

func TestTerminalBashStartupAliasesAreReadOnlyAndFiltered(t *testing.T) {
 home:=t.TempDir()
 t.Setenv("HOME",home)
 init:=strings.Join([]string{
  "alias gb='git branch'",
  "alias gc='gc-safe'",
  "alias gco=\"git checkout\"",
  "alias ll='ls -alF'",
  "alias ga='git add'",
  "alias gis='git status'",
  "#alias hidden='git push'",
  "alias dangerous='rm -rf /; echo danger'",
  "alias token='password=secret'",
 },"\n")+"\n"
 if err:=os.WriteFile(filepath.Join(home,".bashrc"),[]byte(init),0600);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(home,".bash_aliases"),[]byte("alias extra='git checkout'\n"),0600);err!=nil {t.Fatal(err)}
 found:=map[string]bool{}
 for _,entry:=range terminalBashStartupAliases(){found[entry]=true}
 for _,want:=range []string{"alias gb='git branch'","alias gc='gc-safe'","alias gco='git checkout'","alias ll='ls -alF'","alias ga='git add'","alias extra='git checkout'"} {
  if !found[want] {t.Fatalf("expected %s, got %v",want,found)}
 }
 for entry:=range found {if strings.Contains(entry,"hidden")||strings.Contains(entry,"dangerous")||strings.Contains(entry,"password") {t.Fatalf("unsafe alias leaked: %q",entry)}}
}

func TestTerminalCompletionLocalCWDOutsideSharedWorkspace(t *testing.T) {
 workspace:=t.TempDir()
 actualCWD:=t.TempDir()
 if err:=os.Mkdir(filepath.Join(actualCWD,"artifacts"),0755);err!=nil{t.Fatal(err)}
 manager:=session.NewManager(64<<10)
 defer manager.Shutdown(time.Second)
 spec,err:=workspaceTerminalExecutionAt(actualCWD,".")
 if err!=nil {t.Fatal(err)}
 // Shared-server sessions are owned and have explicit terminal kind.
 spec.SessionKind="terminal";spec.OwnerUserID="test-user";spec.ProjectID="test-project"
 meta,err:=manager.Start(spec)
 if err!=nil {t.Fatal(err)}
 s:=&Server{Workspace:workspace,Sessions:manager}
 s.Config.SharedServerEnabled=true
 run:=func(query string) (int,string){
  req:=httptest.NewRequest(http.MethodGet,"/api/sessions/"+meta.ID+"/completion-paths?"+query,nil)
  rr:=httptest.NewRecorder()
  // Shared session authorization is separately tested. This test exercises
  // scope validation for an already-authorized interactive session.
  s.sessionCompletionPaths(rr,req,meta.ID)
  return rr.Code,rr.Body.String()
 }
 code,body:=run("kind=paths&prefix=ar")
 if code!=http.StatusOK || !strings.Contains(body,"artifacts") {t.Fatalf("shared cwd completion: %d %s",code,body)}
 code,_=run("kind=paths&dir=..%2F&prefix=ar")
 if code!=http.StatusForbidden {t.Fatalf("parent directory must be rejected: %d",code)}
 code,_=run("kind=paths&dir=%2Fetc&prefix=ho")
 if code!=http.StatusForbidden {t.Fatalf("absolute path must be rejected: %d",code)}
}


func TestTerminalAliasLookupAPIUsesConfigWithoutShellExecution(t *testing.T) {
 home:=t.TempDir()
 t.Setenv("HOME",home)
 if err:=os.WriteFile(filepath.Join(home,".bash_aliases"),[]byte("alias gb='git branch'\nalias gc='gc-safe'\n"),0600);err!=nil{t.Fatal(err)}
 root:=t.TempDir()
 manager:=session.NewManager(64<<10)
 defer manager.Shutdown(time.Second)
 spec,err:=workspaceTerminalExecutionAt(root,".")
 if err!=nil{t.Fatal(err)}
 meta,err:=manager.Start(spec)
 if err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:root,Sessions:manager}
 request:=httptest.NewRequest(http.MethodGet,"/api/sessions/"+meta.ID+"/completion-paths?kind=aliases",nil)
 recorder:=httptest.NewRecorder()
 s.sessionItem(recorder,request)
 if recorder.Code!=http.StatusOK {t.Fatalf("alias lookup status %d: %s",recorder.Code,recorder.Body.String())}
 var payload struct{Aliases []string `json:"aliases"`}
 if err:=json.Unmarshal(recorder.Body.Bytes(),&payload);err!=nil{t.Fatal(err)}
 value:=strings.Join(payload.Aliases,"\n")
 if !strings.Contains(value,"alias gb='git branch'")||!strings.Contains(value,"alias gc='gc-safe'"){
  t.Fatalf("expected Bash config aliases, received %q",value)
 }
}
