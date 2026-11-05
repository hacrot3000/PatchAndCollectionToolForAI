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

func TestRemoteGitBranchCreateExactHostNameAndAudit(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only Git job lifecycle")}
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  output,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v: %v %s",args,err,output)}
  return strings.TrimSpace(string(output))
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(file,[]byte("first\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","Initial")
 head:=run("rev-parse","HEAD")
 current:=run("symbolic-ref","--short","HEAD")
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE_NAME","remote-topic")
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE","0")
 if _,err:=s.executeRemoteGitBranchCreate(context.Background(),session);err==nil{
  t.Fatal("branch creation ignored separate host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE","1")
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE_NAME","../invalid")
 if _,err:=s.executeRemoteGitBranchCreate(context.Background(),session);err==nil{
  t.Fatal("unsafe configured ref accepted")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE_NAME","remote-topic")
 if err:=os.WriteFile(file,[]byte("dirty\n"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.executeRemoteGitBranchCreate(context.Background(),session);err==nil{
  t.Fatal("dirty worktree allowed branch creation")
 }
 if err:=os.WriteFile(file,[]byte("first\n"),0600);err!=nil{t.Fatal(err)}
 result,err:=s.executeRemoteGitBranchCreate(context.Background(),session)
 if err!=nil||result.Head!=head||result.Branch!="remote-topic"{
  t.Fatalf("branch creation refused: %+v %v",result,err)
 }
 if got:=run("rev-parse","refs/heads/remote-topic");got!=head{
  t.Fatalf("new branch not pinned to host HEAD: %s",got)
 }
 if got:=run("rev-parse","HEAD");got!=head{t.Fatal("branch create moved HEAD")}
 if got:=run("symbolic-ref","--short","HEAD");got!=current{
  t.Fatal("branch create checked out newly created branch")
 }
 if data,err:=os.ReadFile(file);err!=nil||string(data)!="first\n"{
  t.Fatalf("branch create touched file: %q %v",data,err)
 }
 if _,err:=s.executeRemoteGitBranchCreate(context.Background(),session);err==nil{
  t.Fatal("duplicate branch creation allowed")
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_branch_create",Limit:10,
 })
 if err!=nil||len(events)!=1||events[0].Result!="attempted"||
  strings.Contains(events[0].Details,"remote-topic"){
  t.Fatalf("audit missing or disclosed configured ref: %+v %v",events,err)
 }
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE_NAME","another-topic")
 if _,err:=s.executeRemoteGitBranchCreate(context.Background(),session);err==nil{
  t.Fatal("nested workspace escaped into parent repository")
 }
}

func TestRemoteGitBranchCreateViewerCannotMutate(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE","1")
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE_NAME","remote-topic")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitBranchCreate,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err==nil||len(reply)!=0{
  t.Fatal("Portal viewer escalated to Git branch mutation")
 }
}
