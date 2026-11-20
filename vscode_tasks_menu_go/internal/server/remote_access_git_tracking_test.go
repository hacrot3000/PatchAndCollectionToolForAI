package server

import(
 "context"
 "os/exec"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitTrackingOfflineSnapshotHostOptIn(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix remote Git only")}
 s,_,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 t.Setenv("TASKDECK_REMOTE_GIT_TRACKING_REFS","0")
 if _,_,err:=s.readRemoteGitTrackingSnapshot(context.Background());err==nil{
  t.Fatal("tracking refs available without opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_TRACKING_REFS","1")
 snapshot,root,err:=s.readRemoteGitTrackingSnapshot(context.Background())
 if err!=nil||root==""||snapshot!="GIT_TRACKING_V1\n"{
  t.Fatalf("empty tracking snapshot: %q %v",snapshot,err)
 }
 git:=func(args ...string)string{
  t.Helper()
  command:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=command.CombinedOutput()
  if err!=nil{t.Fatalf("git fixture: %v %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 git("-c","user.name=Test","-c","user.email=test@example.invalid","commit","--allow-empty","-qm","First")
 oid:=git("rev-parse","HEAD")
 for _,ref:=range []string{"origin/main","origin/feature/test","backup/release"}{
  git("update-ref","refs/remotes/"+ref,oid)
 }
 snapshot,_,err=s.readRemoteGitTrackingSnapshot(context.Background())
 want:="GIT_TRACKING_V1\nbackup/release "+oid+"\norigin/feature/test "+oid+"\norigin/main "+oid+"\n"
 if err!=nil||snapshot!=want||!remotetransport.ValidRemoteGitTrackingSnapshot(snapshot){
  t.Fatalf("unexpected offline tracking refs: %q %v",snapshot,err)
 }
}
func TestRemoteGitTrackingE2EEAuditAndSnapshotChange(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix remote Git only")}
 t.Setenv("TASKDECK_REMOTE_GIT_TRACKING_REFS","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 command:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 command("-c","user.name=Test","-c","user.email=test@example.invalid","commit","--allow-empty","-qm","First")
 oid:=command("rev-parse","HEAD")
 for i:=0;i<12;i++{
  ref:="refs/remotes/origin/branch"+string(rune('a'+i))
  command("update-ref",ref,oid)
 }
 // A PHP-only session cannot authorize the exact same AEAD ciphertext.
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitTrackingStart,
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);err==nil||len(reply)>0{
  t.Fatal("PHP-only principal accessed offline refs")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 page,err:=remotetransport.OpenRemoteGitTrackingPage(client,reply,remoteDispatchTestID)
 if err!=nil||!page.More||page.Page!=1{t.Fatalf("bad initial tracking page: %+v %v",page,err)}
 proj,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:proj.ID,Action:"remote.action.git_tracking_start",Limit:5,
 })
 if err!=nil||len(audits)==0||audits[0].UserID==nil||*audits[0].UserID!=session.userID{
  t.Fatalf("missing local E2EE audit: %v",err)
 }
 // The next page must fail instead of joining two different Git snapshots.
 command("-c","user.name=Test","-c","user.email=test@example.invalid","commit","--allow-empty","-qm","Changed")
 command("update-ref","refs/remotes/origin/brancha",command("rev-parse","HEAD"))
 next,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:"abcdefabcdefabcdefabcdefabcdefab",Operation:remotetransport.RemoteActionGitTrackingNext,
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,next);err==nil||len(response)>0{
  t.Fatal("tracking page mixed refs from changed snapshot")
 }
}
