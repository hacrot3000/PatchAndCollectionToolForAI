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

func TestRemoteGitDiffFixedStatRootAndOptIn(t *testing.T){
 requireGit(t)
 s,_,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string){
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  if out,err:=cmd.CombinedOutput();err!=nil{t.Fatalf("git %v: %v %s",args,err,out)}
 }
 run("config","user.name","Test")
 run("config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(file,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","base")
 t.Setenv("TASKDECK_REMOTE_GIT_DIFF","0")
 if _,err:=s.readRemoteGitDiff(context.Background());err==nil{
  t.Fatal("GitDiff bypassed host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_DIFF","1")
 clean,err:=s.readRemoteGitDiff(context.Background())
 if err!=nil||clean!="clean"{t.Fatalf("clean Git diff: %q %v",clean,err)}
 if err:=os.WriteFile(file,[]byte("base\nsecond\n"),0600);err!=nil{t.Fatal(err)}
 summary,err:=s.readRemoteGitDiff(context.Background())
 if err!=nil||!strings.Contains(summary,"tracked.txt")||
  !remotetransport.ValidRemoteGitLogText(summary){
  t.Fatalf("expected bounded diff stat: %q %v",summary,err)
 }
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.readRemoteGitDiff(context.Background());err==nil{
  t.Fatal("GitDiff exposed parent repository outside exact workspace")
 }
}
