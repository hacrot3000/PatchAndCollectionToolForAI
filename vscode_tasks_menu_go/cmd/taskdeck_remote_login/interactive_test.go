package main

import (
 "bufio"
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "strings"
 "testing"
)

type fakeInteractiveTaskDeck struct{
 actions []string
 replacePath,replaceText,replaceSHA string
 previewText string
 denied bool
}
func (f *fakeInteractiveTaskDeck) record(action string)(string,error){
 f.actions=append(f.actions,action)
 if f.denied{return "",errors.New("TaskDeck revoked authenticated session")}
 return "ok",nil
}
func (f *fakeInteractiveTaskDeck) ReadFile(_ context.Context,p string)(string,error){
 f.actions=append(f.actions,"view:"+p)
 if f.denied{return "",errors.New("revoked")}
 return "before",nil
}
func (f *fakeInteractiveTaskDeck) ReadFilePreview(_ context.Context,p string)(string,error){
 f.actions=append(f.actions,"preview:"+p)
 if f.denied{return "",errors.New("revoked")}
 if f.previewText!=""{return f.previewText,nil}
 return "preview",nil
}
func (f *fakeInteractiveTaskDeck) ListDirectory(_ context.Context,p string)(string,error){
 return f.record("ls:"+p)
}
func (f *fakeInteractiveTaskDeck) GitStatus(_ context.Context)(string,error){
 return f.record("git:status")
}
func (f *fakeInteractiveTaskDeck) GitStatusFull(_ context.Context)(string,error){
 return f.record("git:status")
}
func (f *fakeInteractiveTaskDeck) GitLog(_ context.Context)(string,error){
 return f.record("git:log")
}
func (f *fakeInteractiveTaskDeck) GitBranches(_ context.Context)(string,error){
 return f.record("git:branches")
}
func (f *fakeInteractiveTaskDeck) GitDiff(_ context.Context,p string)(string,error){
 return f.record("git:diff:"+p)
}
func (f *fakeInteractiveTaskDeck) ReplaceFile(_ context.Context,p,text,sha string)(string,error){
 f.replacePath=p;f.replaceText=text;f.replaceSHA=sha
 return f.record("edit:"+p)
}
func (f *fakeInteractiveTaskDeck) ReplaceFileText(_ context.Context,p,text,sha string)(string,error){
 f.replacePath=p;f.replaceText=text;f.replaceSHA=sha
 return f.record("edit:"+p)
}
func (f *fakeInteractiveTaskDeck) ListApprovedTasks(_ context.Context)(string,error){
 return f.record("task:list")
}
func (f *fakeInteractiveTaskDeck) GitStagedDiff(_ context.Context,path string)(string,error){
 return f.record("git:staged:"+path)
}
func (f *fakeInteractiveTaskDeck) RunTaskText(_ context.Context,id int)(string,error){
 return f.record("task:"+string(rune(id+'0')))
}
func (f *fakeInteractiveTaskDeck) RunTask(_ context.Context,id int)(string,error){
 return f.record("task:"+string(rune(id+'0')))
}

func TestInteractiveNativeConsoleRoutesOnlyTypedE2EEOperations(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 input:="ls\nview a.txt\ngit status\ngit diff a.txt\nterminal task 1\nexit\n"
 var output bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&output)
 if err!=nil{t.Fatal(err)}
 got:=strings.Join(f.actions,",")
 if got!="ls:.,preview:a.txt,git:status,git:diff:a.txt,task:1"{
  t.Fatalf("wrong typed actions %q",got)
 }
 if !strings.Contains(output.String(),"Terminal PTY/shell") {
  t.Fatal("console misrepresents itself as generic terminal")
 }
}
func TestInteractiveNativeConsoleEditUsesLocalSHAAndExplicitYES(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var output bytes.Buffer
 input:="edit a.txt\nafter\n.save-noeol\nNO\nedit a.txt\nafter\n.save-noeol\nYES\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&output)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256([]byte("preview"))
 if f.replacePath!="a.txt"||f.replaceText!="after"||
  f.replaceSHA!=hex.EncodeToString(sum[:]){
  t.Fatalf("edit data or optimistic SHA did not derive from authenticated view: path=%q data=%q sha=%s",f.replacePath,f.replaceText,f.replaceSHA)
 }
 if len(f.actions)!=3{
  t.Fatalf("cancel must not issue a file write: %+v",f.actions)
 }
}
func TestInteractiveConsoleRejectsShellGitFlagsTraversalAndStopsOnRevoke(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="terminal sh -c id\ngit push origin main\ngit diff ../private\nview /etc/passwd\nedit ../escape\nterminal task -1\nls\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=1||f.actions[0]!="ls:."{
  t.Fatalf("untrusted command was dispatched: %+v",f.actions)
 }
 f=&fakeInteractiveTaskDeck{denied:true}
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("git status\nls\n")),&out)
 if err==nil||len(f.actions)!=1{
  t.Fatal("revoked principal continued acting in a poisoned E2EE session")
 }
}
func TestInteractiveConsoleDoesNotBypassPerLoginSixActionQuota(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(strings.Repeat("git status\n",10))),&out)
 if err!=nil||len(f.actions)!=6{
  t.Fatalf("native console bypassed Relay quota: calls=%d err=%v",len(f.actions),err)
 }
}


func TestInteractiveConsoleWorkspaceNavigationAndSpaces(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="cd folder with spaces\npwd\nls\nview file with spaces.txt\ngit diff change with spaces.go\ncd ..\nls\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 want:="ls:folder with spaces,preview:folder with spaces/file with spaces.txt,git:diff:folder with spaces/change with spaces.go,ls:."
 if got:=strings.Join(f.actions,",");got!=want{t.Fatalf("unexpected rooted commands %q, want %q",got,want)}
 if !strings.Contains(out.String(),"folder with spaces"){
  t.Fatal("client-only cwd not displayed")
 }
}

func TestInteractiveConsoleNavigationNeverEscapesProjectRoot(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="cd ..\ncd ../escape\ncd nested\ncd ../../../../etc\nview ../../secret\ngit diff /etc/passwd\nterminal task 1;echo\nls\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="ls:nested"{
  t.Fatalf("client path traversal or command injection was forwarded: %q",got)
 }
 if !strings.Contains(out.String(),"workspace root"){
  t.Fatal("missing local root-escape rejection")
 }
}

func TestInteractiveConsoleBoundsTTYLinesBeforeRemoteDispatch(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("view "+strings.Repeat("x",300)+"\nls\n")),&out)
 if err==nil||len(f.actions)!=0{t.Fatalf("oversized TTY input reached remote: %v %+v",err,f.actions)}
 f=&fakeInteractiveTaskDeck{}
 err=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("edit a.txt\n"+strings.Repeat("y",5000)+"\nYES\n")),&out)
 if err==nil||len(f.actions)!=1||f.replacePath!=""{
  t.Fatalf("oversized edit buffer caused a remote write: %v %+v",err,f.actions)
 }
}

func TestInteractiveConsoleCleanExitAtEOF(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("pwd\nexit")),&out)
 if err!=nil||len(f.actions)!=0{t.Fatalf("EOF exit must be local: %v %+v",err,f.actions)}
}

func TestInteractiveGitLogAndBranchesUseOnlyFixedTypedOperations(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="git log\ngit branches\ngit log --oneline\ngit push\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="git:log,git:branches"{
  t.Fatalf("untyped Git arguments reached remote host: %q",got)
 }
}

func TestInteractiveMultilineEditPreservesNewlinesAndRequiresApproval(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:="edit dir/a.txt\nfirst\nsecond\n.save\nYES\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out)
 if err!=nil{t.Fatal(err)}
 if f.replacePath!="dir/a.txt"||f.replaceText!="first\nsecond\n"{
  t.Fatalf("multiline content malformed: %q %q",f.replacePath,f.replaceText)
 }
 if len(f.actions)!=2||f.actions[0]!="preview:dir/a.txt"||f.actions[1]!="edit:dir/a.txt"{
  t.Fatalf("edited without verified preview or after unauthorized action: %+v",f.actions)
 }
}
func TestInteractiveEditCancelNeverWrites(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:="edit a.txt\nsensitive draft\n.cancel\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out)
 if err!=nil||f.replacePath!=""||len(f.actions)!=1{
  t.Fatalf("canceled edit wrote: %v %+v",err,f.actions)
 }
}

func TestInteractiveRemoteTaskCatalogueAndStagedDiffTypedOnly(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="terminal tasks\ngit diff --staged src/file name.go\nterminal task 1\n"+
  "terminal tasks --all\ngit diff --cached src/file name.go\nterminal sh -c id\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="task:list,git:staged:src/file name.go,task:1"{
  t.Fatalf("native console forwarded untyped Terminal/Git command: %q",got)
 }
}

func TestInteractiveReplaceUniqueMatchUsesAuthenticatedCAS(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"first\nkey=old\nlast\n"}
 var out bytes.Buffer
 script:="replace settings/app config.txt\nkey=old\nkey=new\nYES\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256([]byte(f.previewText))
 if f.replacePath!="settings/app config.txt"||
  f.replaceText!="first\nkey=new\nlast\n"||
  f.replaceSHA!=hex.EncodeToString(digest[:]){
  t.Fatalf("replace did not pin full authenticated preview and CAS: %+v",f)
 }
 if got:=strings.Join(f.actions,",");got!="preview:settings/app config.txt,edit:settings/app config.txt"{
  t.Fatalf("unexpected remote calls: %s",got)
 }
 if !strings.Contains(out.String(),"first\nkey=new\nlast\n"){
  t.Fatal("preview is no longer readable with real line breaks")
 }
}

func TestInteractiveReplaceRejectsMissingAndAmbiguousMatchWithoutWrite(t *testing.T){
 for _,tc:=range []struct{content,search string}{
  {"abc abc","abc"},
  {"abcdef","missing"},
 }{
  f:=&fakeInteractiveTaskDeck{previewText:tc.content}
  var out bytes.Buffer
  script:="replace a.txt\n"+tc.search+"\nnew\nexit\n"
  err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out)
  if err!=nil{t.Fatal(err)}
  if len(f.actions)!=1||f.replacePath!=""{
   t.Fatalf("ambiguous/missing search caused a write: %+v",f)
  }
 }
}

func TestInteractiveReplaceCancellationAndOversizeDoNotWrite(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"key"}
 var out bytes.Buffer
 script:="replace a.txt\nkey\nnew\nNO\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=1||f.replacePath!=""{t.Fatalf("NO did not cancel: %+v",f)}
 f=&fakeInteractiveTaskDeck{previewText:"key"+strings.Repeat("z",4090)}
 out.Reset()
 script="replace a.txt\nkey\n"+strings.Repeat("x",10)+"\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=1||f.replacePath!=""{t.Fatalf("oversized edit wrote: %+v",f)}
}

func TestInteractiveConsoleEscapesTerminalControlsButPreservesNewlines(t *testing.T){
 var out bytes.Buffer
 consoleDisplayText(&out,"one\n\033[31mred\r\n\u202eRIGHT\u2067X\u2069")
 got:=out.String()
 if strings.ContainsRune(got,'\x1b')||strings.ContainsRune(got,'\r')||
  strings.ContainsRune(got,'\u202e')||strings.ContainsRune(got,'\u2067'){
  t.Fatalf("terminal control escaped incorrectly: %q",got)
 }
 if !strings.Contains(got,"one\n")||!strings.Contains(got,"\\u001b")||
  !strings.Contains(got,"\\u000d")||!strings.Contains(got,"\\u202e"){
  t.Fatalf("remote text presentation lost line boundaries or sanitizer: %q",got)
 }
}

func TestInteractiveReplaceRespectsSessionQuotaAndRejectsTraversal(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:=strings.Repeat("git status\n",5)+"replace a.txt\npwd\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=5||f.replacePath!=""{
  t.Fatalf("replace issued unsafe first leg at quota boundary: %+v",f)
 }
 f=&fakeInteractiveTaskDeck{}
 out.Reset()
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("replace ../../etc/passwd\nexit\n")),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=0{t.Fatalf("traversal reached Agent: %+v",f.actions)}
}
