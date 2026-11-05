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

func TestRemoteGitIndexTrackedStageAndUnstagePreserveWorktree(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only Git mutation lifecycle")}
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  data,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v: %v %s",args,err,data)}
  return strings.TrimSpace(string(data))
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 tracked:=filepath.Join(s.Workspace,"tracked.txt")
 untracked:=filepath.Join(s.Workspace,"private-untracked.txt")
 if err:=os.WriteFile(tracked,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","Initial")
 head:=run("rev-parse","HEAD")
 if err:=os.WriteFile(tracked,[]byte("modified\n"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(untracked,[]byte("must stay untracked\n"),0600);err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE_TRACKED","0")
 if _,err:=s.executeRemoteGitIndex(context.Background(),session,
  remotetransport.RemoteActionGitStageTracked);err==nil{
  t.Fatal("stage operation ignored host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE_TRACKED","1")
 result,err:=s.executeRemoteGitIndex(context.Background(),session,
  remotetransport.RemoteActionGitStageTracked)
 if err!=nil||result.Operation!=remotetransport.RemoteActionGitStageTracked||
  result.Head!=head{t.Fatalf("tracked stage failed: %+v %v",result,err)}
 staged:=run("diff","--cached","--name-only")
 if staged!="tracked.txt"{t.Fatalf("unexpected staged paths: %q",staged)}
 if got:=run("status","--porcelain","--","private-untracked.txt");got!="?? private-untracked.txt"{
  t.Fatalf("untracked file accidentally staged: %q",got)
 }
 if data,err:=os.ReadFile(tracked);err!=nil||string(data)!="modified\n"{
  t.Fatalf("Git stage touched worktree: %q %v",data,err)
 }
 if _,err:=s.executeRemoteGitIndex(context.Background(),session,
  remotetransport.RemoteActionGitStageTracked);err==nil{
  t.Fatal("no-op stage was not denied")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_UNSTAGE_ALL","0")
 if _,err:=s.executeRemoteGitIndex(context.Background(),session,
  remotetransport.RemoteActionGitUnstageAll);err==nil{
  t.Fatal("unstage operation ignored host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_UNSTAGE_ALL","1")
 result,err=s.executeRemoteGitIndex(context.Background(),session,
  remotetransport.RemoteActionGitUnstageAll)
 if err!=nil||result.Operation!=remotetransport.RemoteActionGitUnstageAll||
  result.Head!=head{t.Fatalf("index unstage failed: %+v %v",result,err)}
 if got:=run("diff","--cached","--name-only");got!=""{
  t.Fatalf("staged content remains after unstage: %q",got)
 }
 if data,err:=os.ReadFile(tracked);err!=nil||string(data)!="modified\n"{
  t.Fatalf("Git unstage modified working file: %q %v",data,err)
 }
 if data,err:=os.ReadFile(untracked);err!=nil||string(data)!="must stay untracked\n"{
  t.Fatalf("Git unstage lost untracked file: %q %v",data,err)
 }
 if got:=run("rev-parse","HEAD");got!=head{
  t.Fatalf("Git stage changed HEAD %s != %s",got,head)
 }
 if _,err:=s.executeRemoteGitIndex(context.Background(),session,
  remotetransport.RemoteActionGitUnstageAll);err==nil{
  t.Fatal("no-op unstage was not denied")
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 for _,name:=range []string{"remote.action.git_stage_tracked","remote.action.git_unstage_all"}{
  events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
   ProjectID:project.ID,Action:name,Limit:10,
  })
  if err!=nil||len(events)!=1||events[0].Result!="attempted"||
   strings.Contains(events[0].Details,"tracked.txt"){
   t.Fatalf("invalid local audit for %s: %+v %v",name,events,err)
  }
 }
}

func TestRemoteGitIndexViewerAndParentRepoDenied(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE_TRACKED","1")
 t.Setenv("TASKDECK_REMOTE_GIT_UNSTAGE_ALL","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 for _,op:=range []string{
  remotetransport.RemoteActionGitStageTracked,
  remotetransport.RemoteActionGitUnstageAll,
 }{
  frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:op,
  })
  if err!=nil{t.Fatal(err)}
  reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
  if err==nil||len(reply)>0{
   t.Fatalf("Portal viewer escalated to Git index mutation %s",op)
  }
 }
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.executeRemoteGitIndex(context.Background(),session,
  remotetransport.RemoteActionGitStageTracked);err==nil{
  t.Fatal("Git stage escaped nested workspace into parent repository")
 }
}
