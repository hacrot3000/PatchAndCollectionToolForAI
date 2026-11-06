package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitLogExactWorkspaceAndBoundedOutput(t *testing.T){
 requireGit(t)
 s,_,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %+v: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 run("config","user.name","Test")
 run("config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"tracked.txt")
 for _,message:=range []string{"Alpha","Beta","Gamma"}{
  if err:=os.WriteFile(file,[]byte(message+"\n"),0600);err!=nil{t.Fatal(err)}
  run("add","tracked.txt")
  run("commit","-m",message)
 }
 t.Setenv("TASKDECK_REMOTE_GIT_LOG","0")
 if _,err:=s.readRemoteGitLog(context.Background());err==nil{
  t.Fatal("host GitLog opt-in bypassed")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_LOG","1")
 log,err:=s.readRemoteGitLog(context.Background())
 if err!=nil||!strings.Contains(log,"Gamma")||
  !strings.Contains(log,"Beta")||!strings.Contains(log,"Alpha")||
  !remotetransport.ValidRemoteGitLogText(log){
  t.Fatalf("invalid host GitLog: %q %v",log,err)
 }
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.readRemoteGitLog(context.Background());err==nil{
  t.Fatal("GitLog escaped from selected project directory into parent repository")
 }
}
