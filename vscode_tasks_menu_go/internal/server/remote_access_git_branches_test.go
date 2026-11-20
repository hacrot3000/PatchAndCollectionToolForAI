package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitBranchesFixedReadAndOptIn(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only fixed native Git")}
 s,_,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v failed: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(file,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","Initial")
 current:=run("symbolic-ref","--quiet","--short","HEAD")
 for _,name:=range []string{"feature/a","feature/b","feature/c","feature/d","feature/e"}{
  run("branch",name)
 }
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCHES","0")
 if _,err:=s.readRemoteGitBranches(context.Background());err==nil{
  t.Fatal("Git branches ignored default-off host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCHES","1")
 summary,err:=s.readRemoteGitBranches(context.Background())
 if err!=nil||!strings.Contains(summary,"Current: "+current)||
  !strings.Contains(summary,"Local branches (first 4):")||
  !strings.Contains(summary,"... more")||
  !remotetransport.ValidRemoteGitLogText(summary){
  t.Fatalf("invalid bounded branch summary: %q %v",summary,err)
 }
 if got:=run("symbolic-ref","--quiet","--short","HEAD");got!=current{
  t.Fatal("read-only GitBranches switched checked-out branch")
 }
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.readRemoteGitBranches(context.Background());err==nil{
  t.Fatal("GitBranches exposed parent repository outside exact workspace")
 }
}

func TestRemoteGitBranchesDetachedHeadDenied(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only fixed native Git")}
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCHES","1")
 s,_,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string){
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  if out,err:=cmd.CombinedOutput();err!=nil{t.Fatalf("git %v: %v %s",args,err,out)}
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"tracked.txt"),[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","Initial")
 run("checkout","--detach","HEAD")
 if _,err:=s.readRemoteGitBranches(context.Background());err==nil{
  t.Fatal("Git branches returned misleading current branch in detached HEAD")
 }
}

func TestRemoteGitBranchesViewerRequiresGitStatus(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCHES","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitBranches,
 })
 if err!=nil{t.Fatal(err)}
 // Viewer is eligible for GitStatus but a valid Git repository is mandatory.
 // Here there is no committed HEAD: response must not disclose a branch.
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err==nil||len(reply)!=0{t.Fatal("unborn repository was exposed via GitBranches")}
}
