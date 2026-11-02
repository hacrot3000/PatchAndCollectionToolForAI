package main

import (
 "crypto/sha256"
 "encoding/hex"
 "strings"
 "testing"
 "time"
)

func gitReviewDigest(s string)string{
 sum:=sha256.Sum256([]byte(s))
 return hex.EncodeToString(sum[:])
}

func TestNativeGUIGitStageReviewExactConfirmationAndCAS(t *testing.T){
 g,f:=newTestNativeGUI()
 req:=guiRequest{Operation:"git_write_review",Mode:"stage",Path:"src/main.go"}
 status,rev:=guiRequestTest(t,g,req,g.origin,g.key)
 if status!=200||!rev.OK||rev.Confirm!="STAGE src/main.go"||rev.Used!=1||
  rev.Text!="HOST-E2EE: view:src/main.go"{
  t.Fatalf("missing authenticated file preview: %d %+v",status,rev)
 }
 expected:="git:stage:src/main.go:"+gitReviewDigest(rev.Text)
 write:=guiRequest{Operation:"git_write_execute",Mode:"stage",Path:req.Path,Confirmation:rev.Confirm}
 status,result:=guiRequestTest(t,g,write,g.origin,g.key)
 if status!=200||!result.OK||result.Used!=2||
  len(f.calls)!=2||f.calls[1]!=expected{
  t.Fatalf("stage did not bind preview to host CAS: %d %+v %+v",status,result,f.calls)
 }
 status,result=guiRequestTest(t,g,write,g.origin,g.key)
 if status!=400||result.OK||len(f.calls)!=2{
  t.Fatal("replayed Git confirmation repeated an index mutation")
 }
}

func TestNativeGUIGitUnstageUsesStagedDiffDigest(t *testing.T){
 g,f:=newTestNativeGUI()
 status,rev:=guiRequestTest(t,g,guiRequest{
  Operation:"git_write_review",Mode:"unstage",Path:"notes.txt",
 },g.origin,g.key)
 if status!=200||rev.Confirm!="UNSTAGE notes.txt"||rev.Used!=1{
  t.Fatalf("missing staged review: %d %+v",status,rev)
 }
 status,result:=guiRequestTest(t,g,guiRequest{
  Operation:"git_write_execute",Mode:"unstage",Path:"notes.txt",Confirmation:rev.Confirm,
 },g.origin,g.key)
 expected:="git:unstage:notes.txt:"+gitReviewDigest(rev.Text)
 if status!=200||!result.OK||len(f.calls)!=2||f.calls[1]!=expected{
  t.Fatalf("unstage did not bind exact staged diff: %d %+v %+v",status,result,f.calls)
 }
}

func TestNativeGUIGitCommitUsesFullReviewAndSafeMessage(t *testing.T){
 g,f:=newTestNativeGUI()
 status,rev:=guiRequestTest(t,g,guiRequest{
  Operation:"git_write_review",Mode:"commit",
 },g.origin,g.key)
 if status!=200||!rev.OK||rev.Confirm!="COMMIT "+strings.Repeat("a",12)||
  !strings.Contains(rev.Text,"-- COMPLETE STAGED DIFF --"){
  t.Fatalf("commit did not authenticate staged snapshot: %d %+v",status,rev)
 }
 status,result:=guiRequestTest(t,g,guiRequest{
  Operation:"git_write_execute",Mode:"commit",Confirmation:rev.Confirm,
  Message:"reviewed staged change",
 },g.origin,g.key)
 expected:="git:commit:reviewed staged change:"+gitReviewDigest(rev.Text)
 if status!=200||!result.OK||len(f.calls)!=2||f.calls[1]!=expected{
  t.Fatalf("commit did not pin reviewed HEAD/index: %d %+v %+v",status,result,f.calls)
 }
}

func TestNativeGUIGitWriteDenyWrongReviewAndCompromisedPortal(t *testing.T){
 g,f:=newTestNativeGUI()
 deny:=[]guiRequest{
  {Operation:"git_write_execute",Mode:"stage",Path:"file.txt",Confirmation:"STAGE file.txt"},
  {Operation:"git_write_review",Mode:"stage",Path:".git/config"},
  {Operation:"git_write_review",Mode:"stage",Path:"../secret.txt"},
  {Operation:"git_write_review",Mode:"stage",Path:"--flag"},
  {Operation:"git_write_review",Mode:"stage",Path:"src/*.go"},
  {Operation:"git_write_review",Mode:"unstage",Path:"src/../file.go"},
  {Operation:"git_write_review",Mode:"commit",Path:"file.go"},
  {Operation:"git_write_review",Mode:"commit",Message:"unreviewed"},
  {Operation:"git_write_review",Mode:"push",Path:"origin"},
 }
 for _,req:=range deny{
  status,res:=guiRequestTest(t,g,req,g.origin,g.key)
  if status!=400||res.OK||res.Used!=0||len(f.calls)!=0{
   t.Fatalf("unreviewed write crossed E2EE: %+v %d %+v %+v",req,status,res,f.calls)
  }
 }
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"git_write_review",
  Mode:"stage",Path:"file.txt"},"https://php-attacker.invalid",g.key)
 if status!=403||res.OK||len(f.calls)!=0{
  t.Fatal("compromised PHP origin sent reviewed Git operation")
 }
 _,rev:=guiRequestTest(t,g,guiRequest{Operation:"git_write_review",
  Mode:"stage",Path:"file.txt"},g.origin,g.key)
 status,res=guiRequestTest(t,g,guiRequest{Operation:"git_write_execute",
  Mode:"stage",Path:"file.txt",Confirmation:"STAGE another.txt"},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=1{t.Fatal("wrong typed confirmation mutated Git")}
 status,res=guiRequestTest(t,g,guiRequest{Operation:"git_write_execute",
  Mode:"stage",Path:"file.txt",Confirmation:rev.Confirm},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=1{t.Fatal("invalid confirmation did not consume review")}
}

func TestNativeGUIGitWriteExpiredAndFiniteQuota(t *testing.T){
 g,f:=newTestNativeGUI()
 _,rev:=guiRequestTest(t,g,guiRequest{Operation:"git_write_review",
  Mode:"stage",Path:"file.txt"},g.origin,g.key)
 g.mu.Lock()
 g.pendingGit.expiry=time.Now().Add(-time.Second)
 g.mu.Unlock()
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"git_write_execute",
  Mode:"stage",Path:"file.txt",Confirmation:rev.Confirm},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=1{t.Fatal("expired review mutated Git")}
 for i:=1;i<6;i++{
  guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 }
 status,res=guiRequestTest(t,g,guiRequest{Operation:"git_write_review",
  Mode:"stage",Path:"file.txt"},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=6{
  t.Fatal("Git review exceeded E2EE mailbox quota")
 }
}
