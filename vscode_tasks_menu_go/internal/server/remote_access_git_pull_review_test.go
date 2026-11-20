package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func setupRemoteGitPullReview(t *testing.T)(*Server,*remoteTaskDeckSession,*remotetransport.Channel,*remotetransport.Channel,func(...string)string,string,string){
 t.Helper()
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only trusted Git review")}
 s,session,client,agent:=remoteDispatchFixture(t)
 // The TaskDeck login fixture may have untracked DB/config files in the
 // original workspace. Use a fresh host-selected root so the Git preview's
 // strict clean-worktree precondition can be tested without committing them.
 s.Workspace=t.TempDir()
 // Client approvals are keyed by the absolute TaskDeck workspace path,
 // even though the registry itself lives outside the Git repository.
 // Rebind the test client key after choosing a new clean Git root.
 bindTestRemoteOwnerClient(t,s,session)
 gitInitRemoteTest(t,s.Workspace)
 git:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v failed: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 git("-c","user.name=Test","-c","user.email=test@example.invalid",
  "commit","--allow-empty","-qm","initial")
 branch:=git("symbolic-ref","--short","HEAD")
 head:=git("rev-parse","HEAD")
 // Remote URL never leaves the repository; no command below contacts it.
 git("remote","add","origin","https://user:SECRET_DO_NOT_LEAK@example.invalid/repo.git")
 git("update-ref","refs/remotes/origin/"+branch,head)
 git("config","branch."+branch+".remote","origin")
 git("config","branch."+branch+".merge","refs/heads/"+branch)
 return s,session,client,agent,git,branch,head
}
func TestRemoteGitPullReviewOfflineUpToDateAndFastForward(t *testing.T){
 s,_,_,_,git,branch,head:=setupRemoteGitPullReview(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_REVIEW","0")
 if _,err:=s.readRemoteGitPullReview(context.Background());err==nil{
  t.Fatal("Git Pull Review bypassed its host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_REVIEW","1")
 result,err:=s.readRemoteGitPullReview(context.Background())
 if err!=nil||result.Status!="up-to-date"||
  result.Head!=head||result.UpstreamHead!=head{
  t.Fatalf("up-to-date local snapshot: %+v %v",result,err)
 }
 // Create a genuine child commit in the Git object database without
 // changing HEAD, working tree or contacting the remote.
 child:=git("-c","user.name=Test","-c","user.email=test@example.invalid",
  "commit-tree","HEAD^{tree}","-p",head,"-m","upstream change")
 git("update-ref","refs/remotes/origin/"+branch,child)
 result,err=s.readRemoteGitPullReview(context.Background())
 if err!=nil||result.Status!="fast-forward"||
  result.Head!=head||result.UpstreamHead!=child{
  t.Fatalf("offline fast-forward preflight: %+v %v",result,err)
 }
 if got:=git("rev-parse","HEAD");got!=head{
  t.Fatal("read-only preflight unexpectedly modified local HEAD")
 }
 if got:=git("rev-parse","refs/remotes/origin/"+branch);got!=child{
  t.Fatal("read-only preflight unexpectedly modified upstream ref")
 }
 if err:=os.WriteFile(filepath.Join(s.Workspace,"dirty-untracked"),[]byte("private"),0600);err!=nil{
  t.Fatal(err)
 }
 if _,err:=s.readRemoteGitPullReview(context.Background());err==nil{
  t.Fatal("preflight recommended pull with dirty or untracked worktree")
 }
}
func TestRemoteGitPullReviewDivergedOrUnconfiguredRejected(t *testing.T){
 s,_,_,_,git,branch,head:=setupRemoteGitPullReview(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_REVIEW","1")
 // Only previously fetched local Git refs count; a missing upstream is
 // never invented from a remote URL or user-provided branch name.
 git("config","--unset","branch."+branch+".merge")
 if _,err:=s.readRemoteGitPullReview(context.Background());err==nil{
  t.Fatal("missing upstream accepted as a pull preview")
 }
 git("config","branch."+branch+".merge","refs/heads/"+branch)
 child:=git("-c","user.name=Test","-c","user.email=test@example.invalid",
  "commit-tree","HEAD^{tree}","-p",head,"-m","upstream change")
 git("update-ref","refs/remotes/origin/"+branch,child)
 git("-c","user.name=Test","-c","user.email=test@example.invalid",
  "commit","--allow-empty","-qm","local divergent commit")
 if _,err:=s.readRemoteGitPullReview(context.Background());err==nil{
  t.Fatal("diverged histories incorrectly accepted for fast-forward")
 }
}
func TestRemoteGitPullReviewE2EEViewerAndAudit(t *testing.T){
 s,session,client,agent,_,_,head:=setupRemoteGitPullReview(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_REVIEW","1")
 direct,checkErr:=s.readRemoteGitPullReview(context.Background())
 if checkErr!=nil||direct.Head!=head||direct.Status!="up-to-date"{
  t.Fatalf("host-only read preflight failed before authenticated action: %+v %v",direct,checkErr)
 }
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitPullReview,
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);err==nil||len(reply)!=0{
  t.Fatal("PHP-only principal authorized a pull review")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 result,err:=remotetransport.OpenRemoteGitPullReviewResult(client,reply,remoteDispatchTestID)
 if err!=nil||result.Status!="up-to-date"||result.Head!=head||
  result.UpstreamHead!=head{
  t.Fatalf("incorrect authenticated pull preflight: %+v %v",result,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 rows,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_pull_review",Limit:5,
 })
 if err!=nil||len(rows)==0||rows[0].UserID==nil||
  *rows[0].UserID!=session.userID||rows[0].ResourceType!="git"{
  t.Fatalf("Git pull review audit missing verified local user: %+v %v",rows,err)
 }
}
