package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitPullRequiresHostOptInAndConfiguredUpstream(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote execution")}
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string){
  t.Helper()
  command:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  if out,err:=command.CombinedOutput();err!=nil{t.Fatalf("git %+v: %v: %s",args,err,out)}
 }
 run("config","user.name","Test Author")
 run("config","user.email","test@example.invalid")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"marker.txt"),[]byte("one\n"),0600);err!=nil{t.Fatal(err)}
 run("add","marker.txt")
 run("commit","-qm","initial")
 t.Setenv("TASKDECK_REMOTE_GIT_PULL","0")
 if _,err:=s.executeRemoteGitPull(context.Background(),session);err==nil {
  t.Fatal("Git pull bypassed host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_PULL","1")
 if _,err:=s.executeRemoteGitPull(context.Background(),session);err==nil {
  t.Fatal("Git pull without configured upstream was accepted")
 }
 // A user-controlled project directory cannot make a parent Git repository
 // a valid TaskDeck workspace repository. No attempted pull is allowed.
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.Mkdir(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.executeRemoteGitPull(context.Background(),session);err==nil {
  t.Fatal("Git pull walked outside the explicit TaskDeck workspace root")
 }
}

func TestRemoteGitPullRejectsViewerAtE2EEAdmission(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PULL","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitPull,
 })
 if err!=nil{t.Fatal(err)}
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err==nil||len(response)>0{
  t.Fatal("Portal viewer became authorized Git writer by presenting a GitPull AEAD action")
 }
}
