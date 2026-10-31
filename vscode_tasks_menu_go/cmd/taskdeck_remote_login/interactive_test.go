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
 return "preview",nil
}
func (f *fakeInteractiveTaskDeck) ListDirectory(_ context.Context,p string)(string,error){
 return f.record("ls:"+p)
}
func (f *fakeInteractiveTaskDeck) GitStatus(_ context.Context)(string,error){
 return f.record("git:status")
}
func (f *fakeInteractiveTaskDeck) GitDiff(_ context.Context,p string)(string,error){
 return f.record("git:diff:"+p)
}
func (f *fakeInteractiveTaskDeck) ReplaceFile(_ context.Context,p,text,sha string)(string,error){
 f.replacePath=p;f.replaceText=text;f.replaceSHA=sha
 return f.record("edit:"+p)
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
 input:="edit a.txt\nafter\nNO\nedit a.txt\nafter\nYES\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&output)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256([]byte("before"))
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
