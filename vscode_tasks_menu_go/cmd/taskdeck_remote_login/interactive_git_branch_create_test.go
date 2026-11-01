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

func TestInteractiveGitBranchCreatePinsHeadAndRequiresExactConfirmation(t *testing.T){
 review:="HEAD "+strings.Repeat("a",40)+"\nBRANCH refs/heads/main\n"
 f:=&fakeInteractiveTaskDeck{branchReview:review}
 var out bytes.Buffer
 input:="git branch-create safe-feature1\n"+
  "CREATE BRANCH taskdeck/safe-feature1 aaaaaaaaaaaa\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="git:branch-review,git:branch-create"{
  t.Fatalf("Git branch create did not require two E2EE actions: %q",got)
 }
 sum:=sha256.Sum256([]byte(review))
 if f.replaceSHA!=hex.EncodeToString(sum[:])||f.branchSlug!="safe-feature1"{
  t.Fatalf("wrong branch review pin or ref: %s %q",f.replaceSHA,f.branchSlug)
 }
 if !strings.Contains(out.String(),"CREATE BRANCH taskdeck/safe-feature1 aaaaaaaaaaaa")||
  !strings.Contains(out.String(),"KHÔNG checkout"){
  t.Fatalf("missing branch/HEAD operator confirmation: %q",out.String())
 }
}

func TestInteractiveGitBranchCreateRejectsCancellationShellAndQuota(t *testing.T){
 for _,input:=range []string{
  "git branch-create ok-name\nNO\nexit\n",
  "git branch-create ok-name\nCREATE BRANCH taskdeck/other-name aaaaaaaaaaaa\nexit\n",
  "git branch-create ok-name\nCREATE BRANCH taskdeck/ok-name bbbbbbbbbbbb\nexit\n",
  "git branch-create ../refs\nexit\n",
  "git branch-create main/x\nexit\n",
  "git branch-create --force\nexit\n",
  "git branch-create ok-name; echo\nexit\n",
  "git branch-create Aname\nexit\n",
 }{
  f:=&fakeInteractiveTaskDeck{}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
  if strings.Contains(strings.Join(f.actions,","),"git:branch-create"){
   t.Fatalf("unconfirmed branch creation forwarded: %q %v",input,f.actions)
  }
 }
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:=strings.Repeat("git status\n",5)+"git branch-create safe-feature1\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=5||!strings.Contains(out.String(),"cần hai lượt E2EE"){
  t.Fatalf("last E2EE round started incomplete branch review: %+v %q",f.actions,out.String())
 }
}

func TestInteractiveGitBranchCreateRejectsMalformedReadOnlyReview(t *testing.T){
 for _,preview:=range []string{
  "bad","HEAD "+strings.Repeat("a",40)+"\n",
  "HEAD "+strings.Repeat("Z",40)+"\nBRANCH refs/heads/main\n",
  "HEAD "+strings.Repeat("a",40)+"\nBRANCH refs/heads/main\nSPOOF",
 }{
  f:=&fakeInteractiveTaskDeck{branchReview:preview}
  var out bytes.Buffer
  err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader("git branch-create safe-feature1\nexit\n")),&out)
  if err==nil||len(f.actions)!=1||f.actions[0]!="git:branch-review"{
   t.Fatalf("malformed Git review not rejected: %q actions=%v err=%v",preview,f.actions,err)
  }
 }
}
