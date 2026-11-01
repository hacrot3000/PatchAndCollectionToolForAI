package main

import (
 "bufio"
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "strings"
 "testing"
)

func TestInteractiveGitSameHeadSwitchConfirmsReviewedTarget(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:="git switch taskdeck/safe-target\nSWITCH taskdeck/safe-target aaaaaaaaaaaa\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 want:="git:switch-review:safe-target,git:switch"
 if got:=strings.Join(f.actions,",");got!=want{
  t.Fatalf("wrong reviewed E2EE operations: %s",got)
 }
 review:="HEAD "+strings.Repeat("a",40)+
  "\nBRANCH refs/heads/main\nTARGET refs/heads/taskdeck/safe-target"+
  "\nTARGET_HEAD "+strings.Repeat("a",40)+
  "\nINDEX_SHA256 "+strings.Repeat("b",64)+"\nCLEAN yes\n"
 sum:=sha256.Sum256([]byte(review))
 if f.switchSlug!="safe-target"||f.replaceSHA!=hex.EncodeToString(sum[:]){
  t.Fatal("wrong SHA pin or target sent to host")
 }
 if !strings.Contains(out.String(),"KHÔNG checkout"){t.Fatal("missing no-checkout scope")}
}
func TestInteractiveGitSameHeadSwitchRejectsCancelInjectionAndQuota(t *testing.T){
 for _,in:=range []string{
  "git switch taskdeck/safe-target\nNO\nexit\n",
  "git switch taskdeck/safe-target\nSWITCH taskdeck/other-name aaaaaaaaaaaa\nexit\n",
  "git switch taskdeck/../main\nexit\n",
  "git switch taskdeck/A-upper\nexit\n",
  "git switch taskdeck/safe-target --force\nexit\n",
  "git switch --force taskdeck/safe-target\nexit\n",
 }{
  f:=&fakeInteractiveTaskDeck{}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(in)),&out);err!=nil{t.Fatal(err)}
  if strings.Contains(strings.Join(f.actions,","),"git:switch"){
   t.Fatalf("unconfirmed or injected switch invoked host: %v",f.actions)
  }
 }
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:=strings.Repeat("git status\n",5)+"git switch taskdeck/safe-target\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=5||!strings.Contains(out.String(),"cần hai lượt E2EE"){
  t.Fatalf("switch bypassed E2EE quota: %v %q",f.actions,out.String())
 }
}
func TestInteractiveGitSameHeadSwitchRejectsForgedReview(t *testing.T){
 good:="HEAD "+strings.Repeat("a",40)+"\nBRANCH refs/heads/main"+
  "\nTARGET refs/heads/taskdeck/safe-target\nTARGET_HEAD "+strings.Repeat("a",40)+
  "\nINDEX_SHA256 "+strings.Repeat("b",64)+"\nCLEAN yes\n"
 for _,bad:=range []string{
  "garbage",
  strings.Replace(good,"TARGET_HEAD "+strings.Repeat("a",40),
   "TARGET_HEAD "+strings.Repeat("c",40),1),
  strings.Replace(good,"CLEAN yes","CLEAN no",1),
  strings.Replace(good,"taskdeck/safe-target","taskdeck/other",1),
  strings.TrimSuffix(good,"\n"),
 }{
  f:=&fakeInteractiveTaskDeck{switchReview:bad}
  var out bytes.Buffer
  err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader("git switch taskdeck/safe-target\nexit\n")),&out)
  if err==nil||len(f.actions)!=1||f.actions[0]!="git:switch-review:safe-target"{
   t.Fatalf("forged switch review accepted: %v %q",f.actions,err)
  }
 }
}
