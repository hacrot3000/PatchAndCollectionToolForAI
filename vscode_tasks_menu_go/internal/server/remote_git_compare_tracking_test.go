package server

import (
 "context"
 "os/exec"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func runGitCompareFixture(t *testing.T,root string,args ...string)string{
 t.Helper()
 cmd:=exec.Command("git",append([]string{"-C",root},args...)...)
 out,err:=cmd.CombinedOutput()
 if err!=nil{t.Fatalf("git %v: %v %s",args,err,out)}
 return strings.TrimSpace(string(out))
}
func TestRemoteGitCompareTrackingIsOfflineAndCommitPinned(t *testing.T){
 root:=initRemoteGitTestRepo(t)
 runGitCompareFixture(t,root,"-c","user.name=CI","-c","user.email=ci@example.invalid",
  "commit","--allow-empty","-qm","base")
 base:=runGitCompareFixture(t,root,"rev-parse","HEAD")
 tree:=runGitCompareFixture(t,root,"rev-parse","HEAD^{tree}")
 remote:=runGitCompareFixture(t,root,"-c","user.name=CI","-c","user.email=ci@example.invalid",
  "commit-tree",tree,"-p",base,"-m","remote")
 runGitCompareFixture(t,root,"update-ref","refs/remotes/origin/main",remote)
 runGitCompareFixture(t,root,"-c","user.name=CI","-c","user.email=ci@example.invalid",
  "commit","--allow-empty","-qm","local")
 head:=runGitCompareFixture(t,root,"rev-parse","HEAD")
 got,err:=remoteGitCompareTracking(context.Background(),root,"origin/main")
 if err!=nil{t.Fatal(err)}
 want:="GIT_COMPARE_V1\nREF origin/main\nHEAD "+head+
  "\nTRACKING "+remote+"\nAHEAD 1\nBEHIND 1\n"
 if string(got)!=want{t.Fatalf("compare result %q want %q",got,want)}
 if !remotetransport.ValidRemoteGitCompareTrackingReport(string(got)){
  t.Fatal("host produced invalid E2EE comparison response")
 }
 if actual:=runGitCompareFixture(t,root,"rev-parse","HEAD");actual!=head{
  t.Fatal("offline compare changed HEAD")
 }
 if actual:=runGitCompareFixture(t,root,"rev-parse","refs/remotes/origin/main");actual!=remote{
  t.Fatal("offline compare changed remote-tracking ref")
 }
}
func TestRemoteGitCompareTrackingRejectsUnknownAndMaliciousRefInput(t *testing.T){
 root:=initRemoteGitTestRepo(t)
 runGitCompareFixture(t,root,"-c","user.name=CI","-c","user.email=ci@example.invalid",
  "commit","--allow-empty","-qm","base")
 for _,ref:=range []string{
  "","origin/HEAD","origin/../main","--all","-origin/main","origin/main\ngit fetch",
  "https://example.invalid/repo","origin/main","refs/remotes/origin/main",
  "origin/.hidden","origin/main.lock","origin/ü",
 }{
  if result,err:=remoteGitCompareTracking(context.Background(),root,ref);
   err==nil||len(result)!=0{
   t.Fatalf("invalid/unknown ref accepted: %q %q",ref,result)
  }
 }
}
func TestRemoteGitCompareTrackingRejectsExecutableFilterConfig(t *testing.T){
 root:=initRemoteGitTestRepo(t)
 runGitCompareFixture(t,root,"-c","user.name=CI","-c","user.email=ci@example.invalid",
  "commit","--allow-empty","-qm","base")
 head:=runGitCompareFixture(t,root,"rev-parse","HEAD")
 runGitCompareFixture(t,root,"update-ref","refs/remotes/origin/main",head)
 runGitCompareFixture(t,root,"config","filter.evil.clean","sh -c 'touch SHOULD_NOT_EXECUTE'")
 got,err:=remoteGitCompareTracking(context.Background(),root,"origin/main")
 if err==nil||len(got)!=0{t.Fatal("compare allowed unsafe Git filter configuration")}
}
