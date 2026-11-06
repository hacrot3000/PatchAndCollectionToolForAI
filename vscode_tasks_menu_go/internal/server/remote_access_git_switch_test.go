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

// This fixture commits the TaskDeck host-authored workflow definition so the
// index/worktree is genuinely clean before the fixed switch is attempted.
func setupRemoteGitSwitch(t *testing.T)(*Server,*remoteTaskDeckSession,func(...string)string,string,string,string){
 t.Helper()
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only same-HEAD switch")}
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  output,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v: %v %s",args,err,output)}
  return strings.TrimSpace(string(output))
 }
 run("config","user.name","TaskDeck Switch Test")
 run("config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(file,[]byte("original\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt",".vscode/tasks.json")
 run("commit","-m","initial")
 current:=run("symbolic-ref","--short","HEAD")
 head:=run("rev-parse","HEAD")
 run("branch","review")
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_NAME","review")
 return s,session,run,file,current,head
}
func TestRemoteGitSwitchSameHeadFixedTargetCleanAndAudit(t *testing.T){
 s,session,git,file,current,head:=setupRemoteGitSwitch(t)
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH","0")
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("branch switch bypassed host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH","1")
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_NAME","../invalid")
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("invalid host branch accepted")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_NAME","review")
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("mutation accepted without preceding host-audited preview")
 }
 if err:=os.WriteFile(file,[]byte("changed\n"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteGitSwitchReview(context.Background(),session);err==nil{
  t.Fatal("dirty tracked file allowed switch preview")
 }
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("dirty tracked file allowed branch switching")
 }
 if err:=os.WriteFile(file,[]byte("original\n"),0600);err!=nil{t.Fatal(err)}
 plan,err:=s.readRemoteGitSwitchReview(context.Background(),session)
 if err!=nil||plan.From!=current||plan.Branch!="review"||plan.Head!=head{
  t.Fatalf("host review mismatch: %+v %v",plan,err)
 }
 result,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session)
 if err!=nil||result.From!=current||result.Branch!="review"||result.Head!=head{
  t.Fatalf("fixed host switch refused: %+v %v",result,err)
 }
 if actual:=git("symbolic-ref","--short","HEAD");actual!="review"{
  t.Fatalf("branch not switched: %s",actual)
 }
 if actual:=git("rev-parse","HEAD");actual!=head{t.Fatal("switch changed the commit")}
 if git("status","--porcelain=v1","--untracked-files=all")!=""{
  t.Fatal("same-head switch changed working tree/index")
 }
 if data,err:=os.ReadFile(file);err!=nil||string(data)!="original\n"{
  t.Fatalf("branch switch changed file: %s %v",data,err)
 }
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("switching to current branch was accepted")
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_switch_same_head",Limit:5,
 })
 if err!=nil||len(events)!=1||events[0].Result!="attempted"||
  events[0].UserID==nil||*events[0].UserID!=session.userID||
  strings.Contains(events[0].Details,"review"){
  t.Fatalf("switch audit did not preserve privacy: %+v %v",events,err)
 }
}
func TestRemoteGitSwitchSameHeadRejectsMissingTargetAndDifferentTip(t *testing.T){
 s,session,git,_,current,_:=setupRemoteGitSwitch(t)
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH","1")
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_NAME","unknown")
 if _,err:=s.readRemoteGitSwitchReview(context.Background(),session);err==nil{
  t.Fatal("unconfigured missing branch allowed preview")
 }
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("unconfigured missing branch accepted")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_NAME","review")
 git("config","user.name","TaskDeck Switch Test")
 git("commit","--allow-empty","-m","new source HEAD")
 // Existing review branch still points to the older commit.
 if _,err:=s.readRemoteGitSwitchReview(context.Background(),session);err==nil{
  t.Fatal("different HEAD allowed preview")
 }
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("switch permitted target with a different tree or HEAD")
 }
 if got:=git("symbolic-ref","--short","HEAD");got!=current{
  t.Fatalf("different-HEAD refusal changed branch to %s",got)
 }
}
func TestRemoteGitSwitchSameHeadRejectsUntrackedOrNestedWorkspace(t *testing.T){
 s,session,git,_,current,_:=setupRemoteGitSwitch(t)
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH","1")
 file:=filepath.Join(s.Workspace,"untracked-private.txt")
 if err:=os.WriteFile(file,[]byte("secret"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteGitSwitchReview(context.Background(),session);err==nil{
  t.Fatal("untracked file was accepted in preview")
 }
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("switch silently discarded untracked file")
 }
 if got:=git("symbolic-ref","--short","HEAD");got!=current{
  t.Fatal("dirty refusal changed HEAD")
 }
 if err:=os.Remove(file);err!=nil{t.Fatal(err)}
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.Mkdir(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.readRemoteGitSwitchReview(context.Background(),session);err==nil{
  t.Fatal("nested workspace escaped to parent repository in review")
 }
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("nested workspace escaped to parent repository")
 }
}
func TestRemoteGitSwitchSameHeadViewerCannotEscalate(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only switch")}
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH","1")
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_NAME","review")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitSwitchSameHead,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err==nil||len(reply)!=0{t.Fatal("viewer escalated into git.write via branch switch")}
}

func TestRemoteGitSwitchSameHeadRejectsPreviewDrift(t *testing.T){
 s,session,git,_,current,head:=setupRemoteGitSwitch(t)
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH","1")
 plan,err:=s.readRemoteGitSwitchReview(context.Background(),session)
 if err!=nil||plan.Head!=head{t.Fatalf("expected initial preview: %+v %v",plan,err)}
 // Target changes after the native client saw the preview. The Agent must
 // NOT switch to the new host-configured target without a new preview.
 git("branch","other")
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_NAME","other")
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("unreviewed target branch substitution accepted")
 }
 if git("symbolic-ref","--short","HEAD")!=current{
  t.Fatal("mismatching preview changed HEAD")
 }
 if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
  t.Fatal("failed preview was reusable for mutation")
 }
 // Re-review this exact target and ensure the new plan is independently
 // checked, rather than trusting the old approval.
 next,err:=s.readRemoteGitSwitchReview(context.Background(),session)
 if err!=nil||next.Branch!="other"{t.Fatalf("new preview failed: %+v %v",next,err)}
 got,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session)
 if err!=nil||got.Branch!="other"||got.From!=current{t.Fatalf("reviewed target refused: %+v %v",got,err)}
}
func TestRemoteGitSwitchSameHeadViewerPreviewCannotMutate(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only branch switch")}
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 git:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v failed: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 git("config","user.name","Switch Review")
 git("config","user.email","test@example.invalid")
 git("add",".vscode/tasks.json")
 git("commit","-m","initial")
 git("branch","review")
 current:=git("symbolic-ref","--short","HEAD")
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH","1")
 t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_NAME","review")
 request:=func(op string)([]byte,error){
  t.Helper()
  frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:op,
  })
  if err!=nil{t.Fatal(err)}
  return s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 }
 sealed,err:=request(remotetransport.RemoteActionGitSwitchReview)
 if err!=nil{t.Fatalf("viewer could not read Git switch preview: %v",err)}
 plan,err:=remotetransport.OpenRemoteGitSwitchResult(client,sealed,remoteDispatchTestID)
 if err!=nil||plan.From!=current||plan.Branch!="review"{
  t.Fatalf("viewer received bad preview: %+v %v",plan,err)
 }
 sealed,err=request(remotetransport.RemoteActionGitSwitchSameHead)
 if err==nil||len(sealed)!=0{t.Fatal("read permission escalated into git.write")}
 if git("symbolic-ref","--short","HEAD")!=current{
  t.Fatal("viewer changed local Git branch")
 }
}
