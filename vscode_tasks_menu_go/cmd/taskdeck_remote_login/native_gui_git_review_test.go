package main

import (
 "errors"
 "strings"
 "testing"
)

func TestNativeGUIGitTwoWayReviewRequiresTwoHostE2EEReads(t *testing.T){
 g,f:=newTestNativeGUI()
 code,res:=guiRequestTest(t,g,guiRequest{
  Operation:"git_review",Path:"src/main.go",
 },g.origin,g.key)
 if code!=200||!res.OK||res.Used!=2||len(f.calls)!=2||
  f.calls[0]!="git:diff:src/main.go"||
  f.calls[1]!="git:staged:src/main.go"||
  !strings.Contains(res.Text,"-- WORKTREE / UNSTAGED --\n")||
  !strings.Contains(res.Text,"-- INDEX / STAGED --\n")||
  !strings.Contains(res.Text,"HOST-E2EE: git:diff:src/main.go")||
  !strings.Contains(res.Text,"HOST-E2EE: git:staged:src/main.go"){
  t.Fatalf("two-way Git review did not produce both authenticated snapshots: %d %+v %v",code,res,f.calls)
 }
}

func TestNativeGUIGitTwoWayReviewNoPartialOutputWhenStagedDenied(t *testing.T){
 g,f:=newTestNativeGUI()
 f.stagedErr=errors.New("permission revoked on second Agent request")
 code,res:=guiRequestTest(t,g,guiRequest{Operation:"git_review",
  Path:"src/main.go"},g.origin,g.key)
 if code!=400||res.OK||res.Text!=""||res.Used!=2||len(f.calls)!=2{
  t.Fatalf("unauthorized partial Git review leaked: %d %+v %v",code,res,f.calls)
 }
}

func TestNativeGUIGitTwoWayReviewRejectsPathsAndExtraCommands(t *testing.T){
 g,f:=newTestNativeGUI()
 bad:=[]guiRequest{
  {Operation:"git_review",Path:"../secret"},
  {Operation:"git_review",Path:".git/config"},
  {Operation:"git_review",Path:"dir/.GIT/config"},
  {Operation:"git_review",Path:"--name-only"},
  {Operation:"git_review",Path:"src/*.go"},
  {Operation:"git_review",Path:"src/main.go",Mode:"commit"},
  {Operation:"git_review",Path:"src/main.go",TaskID:1},
  {Operation:"git_review",Path:"src/main.go",Confirmation:"YES"},
  {Operation:"git_review",Path:"src/main.go",Content:"git push"},
 }
 for _,req:=range bad{
  code,res:=guiRequestTest(t,g,req,g.origin,g.key)
  if code!=400||res.OK||res.Used!=0||len(f.calls)!=0{
   t.Fatalf("unsafe reviewed Git source crossed network: %+v %d %+v %v",req,code,res,f.calls)
  }
 }
 code,res:=guiRequestTest(t,g,guiRequest{
  Operation:"git_review",Path:"src/main.go",
 },"https://compromised-php.invalid",g.key)
 if code!=403||res.OK||len(f.calls)!=0{
  t.Fatal("compromised PHP was able to dispatch Git review")
 }
}

func TestNativeGUIGitTwoWayReviewReservesRoundBudget(t *testing.T){
 g,f:=newTestNativeGUI()
 for i:=0;i<5;i++{
  guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 }
 code,res:=guiRequestTest(t,g,guiRequest{Operation:"git_review",
  Path:"src/main.go"},g.origin,g.key)
 if code!=400||res.OK||res.Used!=5||len(f.calls)!=5{
  t.Fatal("Git two-way review left E2EE quota negative or displayed partial data")
 }
}
