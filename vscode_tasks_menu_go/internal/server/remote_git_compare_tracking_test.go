package server

import (
 "context"
 "os/exec"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/identity"
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


func TestRemoteGitCompareTrackingE2EEPermissionAndHostAudit(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 if out,err:=exec.Command("git","init","-q",s.Workspace).CombinedOutput();err!=nil{
  t.Skipf("git unavailable: %v %s",err,out)
 }
 runGitCompareFixture(t,s.Workspace,"-c","user.name=CI","-c","user.email=ci@example.invalid",
  "commit","--allow-empty","-qm","base")
 oid:=runGitCompareFixture(t,s.Workspace,"rev-parse","HEAD")
 runGitCompareFixture(t,s.Workspace,"update-ref","refs/remotes/origin/main",oid)
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitCompareTracking,
  Path:"origin/main",
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,req);
  err==nil||len(response)>0{t.Fatal("PHP-only identity accessed local Git compare")}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 output,err:=remotetransport.OpenRemoteGitCompareTrackingResult(client,reply,remoteDispatchTestID)
 if err!=nil||output!="GIT_COMPARE_V1\nREF origin/main\nHEAD "+oid+
  "\nTRACKING "+oid+"\nAHEAD 0\nBEHIND 0\n"{
  t.Fatalf("unexpected authenticated Git compare result %q %v",output,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_compare_tracking",Limit:5,
 })
 if err!=nil||len(audit)==0||audit[0].UserID==nil||*audit[0].UserID!=session.userID{
  t.Fatalf("missing TaskDeck-local read audit for Git comparison: %v",err)
 }
}
