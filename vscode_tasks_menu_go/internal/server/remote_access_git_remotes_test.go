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

func TestRemoteGitRemotesNamesOnlyNeverLeaksURL(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("native remote Git on Unix")}
 s,_,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 t.Setenv("TASKDECK_REMOTE_GIT_REMOTES","0")
 if _,err:=s.readRemoteGitRemotes(context.Background());err==nil{
  t.Fatal("Git remote metadata did not require explicit host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_REMOTES","1")
 value,err:=s.readRemoteGitRemotes(context.Background())
 if err!=nil||value!="GIT_REMOTES_V1\n"{t.Fatalf("empty metadata: %q %v",value,err)}
 token:="SECRET_TOKEN_NEVER_SENT"
 git:=func(args ...string){
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  if out,err:=cmd.CombinedOutput();err!=nil{
   t.Fatalf("git fixture: %v %s",err,out)
  }
 }
 git("remote","add","upstream","ssh://git@example.invalid/another.git")
 git("remote","add","origin","https://user:"+token+"@example.invalid/private.git")
 value,err=s.readRemoteGitRemotes(context.Background())
 if err!=nil||value!="GIT_REMOTES_V1\norigin\nupstream\n"{
  t.Fatalf("unexpected names-only metadata: %q %v",value,err)
 }
 for _,secret:=range []string{token,"https://","ssh://","user:","example.invalid"}{
  if strings.Contains(value,secret){t.Fatalf("credential leaked through Git remote name list: %q",secret)}
 }
}
func TestRemoteGitRemotesRefusesMoreThan16Names(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_REMOTES","1")
 s,_,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 for i:=0;i<17;i++{
  name:="r"+string(rune('a'+i))
  cmd:=exec.Command("git","-C",s.Workspace,"remote","add",name,"https://example.invalid/foo.git")
  if out,err:=cmd.CombinedOutput();err!=nil{t.Fatalf("create %s: %s %v",name,out,err)}
 }
 if text,err:=s.readRemoteGitRemotes(context.Background());err==nil||text!=""{
  t.Fatalf("more than 16 names leaked a partial list: %q %v",text,err)
 }
}
func TestRemoteGitRemotesAuthenticatedE2EEAndAudit(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_REMOTES","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 token:="SECRET_NOT_FOR_PHP"
 cmd:=exec.Command("git","-C",s.Workspace,"remote","add",
  "origin","https://user:"+token+"@example.invalid/private.git")
 if out,err:=cmd.CombinedOutput();err!=nil{t.Fatalf("remote fixture: %s %v",out,err)}
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitRemotes,
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,frame);err==nil||len(reply)!=0{
  t.Fatal("PHP-only principal read Git metadata")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{t.Fatal(err)}
 nameList,err:=remotetransport.OpenRemoteGitRemotesResult(client,reply,remoteDispatchTestID)
 if err!=nil||nameList!="GIT_REMOTES_V1\norigin\n"||strings.Contains(nameList,token){
  t.Fatalf("leaky or invalid Git names result: %q %v",nameList,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 rows,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_remotes",Limit:5,
 })
 if err!=nil||len(rows)==0||rows[0].UserID==nil||*rows[0].UserID!=session.userID{
  t.Fatalf("missing verified local audit: %v",err)
 }
}
