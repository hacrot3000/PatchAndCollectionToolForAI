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

func TestRemoteGitPatchSnapshotPagesAndStaleDenial(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only fixed Git diff")}
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
 if err:=os.WriteFile(file,[]byte("initial\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","Initial")
 head:=run("rev-parse","HEAD")
 content:=strings.Repeat("modified tracked file line 12345\n",48)
 if err:=os.WriteFile(file,[]byte(content),0600);err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_GIT_PATCH","0")
 if _,err:=s.readRemoteGitPatchPage(context.Background(),session,true,remoteDispatchTestID);err==nil{
  t.Fatal("diff patch bypassed host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_PATCH","1")
 if _,err:=s.readRemoteGitPatchPage(context.Background(),session,false,remoteDispatchTestID);err==nil{
  t.Fatal("uninitialized Git patch cursor was accepted")
 }
 first,err:=s.readRemoteGitPatchPage(context.Background(),session,true,remoteDispatchTestID)
 if err!=nil||first.Page!=1||!first.More||!strings.Contains(first.Patch,"diff --git"){
  t.Fatalf("invalid first Git patch: %+v %v",first,err)
 }
 var combined strings.Builder
 combined.WriteString(first.Patch)
 page:=first
 for page.More{
  page,err=s.readRemoteGitPatchPage(context.Background(),session,false,remoteDispatchTestID)
  if err!=nil||page.Page<2||!remotetransport.ValidRemoteGitPatchPage(page){
   t.Fatalf("invalid subsequent page: %+v %v",page,err)
  }
  combined.WriteString(page.Patch)
  if page.Page>30{t.Fatal("Git patch pages did not terminate")}
 }
 if !strings.Contains(combined.String(),"modified tracked file line")||
  !strings.Contains(combined.String(),"tracked.txt"){
  t.Fatalf("Git diff output incomplete: %s",combined.String())
 }
 if _,err:=s.readRemoteGitPatchPage(context.Background(),session,false,remoteDispatchTestID);err==nil{
  t.Fatal("Git patch cursor replayed after last page")
 }
 if got:=run("rev-parse","HEAD");got!=head{t.Fatal("read-only patch changed HEAD")}
 if data,err:=os.ReadFile(file);err!=nil||string(data)!=content{
  t.Fatal("read-only patch changed worktree")
 }
 first,err=s.readRemoteGitPatchPage(context.Background(),session,true,remoteDispatchTestID)
 if err!=nil||!first.More{t.Fatalf("start failed after completion: %+v %v",first,err)}
 if err:=os.WriteFile(file,[]byte("new concurrent content\n"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteGitPatchPage(context.Background(),session,false,remoteDispatchTestID);err==nil{
  t.Fatal("Git patch accepted a stale SHA-256 snapshot after file mutation")
 }
 if _,err:=s.readRemoteGitPatchPage(context.Background(),session,false,remoteDispatchTestID);err==nil{
  t.Fatal("stale patch cursor remained available after denial")
 }
 root:=s.Workspace
 nested:=filepath.Join(root,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.readRemoteGitPatchPage(context.Background(),session,true,remoteDispatchTestID);err==nil{
  t.Fatal("Git patch escaped from nested workspace into parent repo")
 }
}

func TestRemoteGitPatchCleanAndOversize(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only fixed Git diff")}
 t.Setenv("TASKDECK_REMOTE_GIT_PATCH","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string){
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  if output,err:=cmd.CombinedOutput();err!=nil{
   t.Fatalf("git %v: %v %s",args,err,output)
  }
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(file,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","Initial")
 clean,err:=s.readRemoteGitPatchPage(context.Background(),session,true,remoteDispatchTestID)
 if err!=nil||clean.More||!strings.Contains(clean.Patch,"no tracked changes"){
  t.Fatalf("clean patch invalid: %+v %v",clean,err)
 }
 // Large tracked content must fail closed, not expose a partial patch as
 // if the complete diff were available.
 if err:=os.WriteFile(file,[]byte(strings.Repeat("large change line 1234567890\n",400)),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteGitPatchPage(context.Background(),session,true,remoteDispatchTestID);err==nil{
  t.Fatal("oversize diff was truncated into a misleading partial response")
 }
}
