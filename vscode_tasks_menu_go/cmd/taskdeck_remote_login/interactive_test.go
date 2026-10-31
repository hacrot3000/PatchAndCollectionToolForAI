package main

import (
 "bufio"
 "bytes"
 "context"
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
 if got!="ls:.,view:a.txt,git:status,git:diff:a.txt,task:1"{
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
 if f.replacePath!="a.txt"||f.replaceText!="after"||
  f.replaceSHA!="6dbd01abc247ba1d9fa01ac0b13e4c2d5bb12ba3b6a1488a095bb1b9d44efca9"{
  // The exact digest must derive solely from the authenticated read, not
  // from a Portal-provided field or user-entered expected hash.
  t.Logf("edit hash: %s",f.replaceSHA)
  if f.replacePath!="a.txt"||f.replaceText!="after"{t.Fatal("edit confirmation failed")}
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
