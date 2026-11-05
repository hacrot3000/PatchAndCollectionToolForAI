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
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func remoteFileTestRow(t *testing.T,status,path string)int{
 t.Helper()
 for i,line:=range strings.Split(status,"\n"){
  if strings.HasSuffix(line," "+path)&&i>0{return i}
 }
 t.Fatalf("status did not contain %s: %q",path,status)
 return 0
}

func TestRemoteGitFileStageUnstageOnlySelectedTrackedFile(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote process groups")}
 t.Setenv("TASKDECK_REMOTE_GIT_STATUS","1")
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE_FILE","1")
 t.Setenv("TASKDECK_REMOTE_GIT_UNSTAGE_FILE","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %q: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 for _,name:=range []string{"a.txt","b.txt"}{
  if err:=os.WriteFile(filepath.Join(s.Workspace,name),[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 }
 run("add","a.txt","b.txt")
 run("commit","-m","Initial")
 head:=run("rev-parse","HEAD")
 for _,name:=range []string{"a.txt","b.txt"}{
  if err:=os.WriteFile(filepath.Join(s.Workspace,name),[]byte("changed\n"),0600);err!=nil{t.Fatal(err)}
 }
 if err:=os.WriteFile(filepath.Join(s.Workspace,"new.txt"),[]byte("secret\n"),0600);err!=nil{t.Fatal(err)}
 snapshot,err:=s.readRemoteGitStatus(context.Background())
 if err!=nil{t.Fatal(err)}
 index:=remoteFileTestRow(t,snapshot,"a.txt")
 if path,ok:=remoteGitSelectedTrackedFile(snapshot,index,remotetransport.RemoteActionGitStageFile);!ok||path!="a.txt"{
  t.Fatalf("selected row parsing failed: index=%d status=%q path=%q",index,snapshot,path)
 }
 principal,checkErr:=s.verifyRemoteSessionLocalPrincipal(context.Background(),session)
 if checkErr!=nil{t.Fatalf("local principal invalid before selection: %v",checkErr)}
 if checkErr=remotetransport.AuthorizeAction(true,session.role,
  &principal,session.projectKey,identity.PermissionGitStatus);checkErr!=nil{
  t.Fatalf("GitStatus prerequisite denied: %v",checkErr)
 }
 if tracked:=run("ls-files","--cached","--full-name","--","a.txt");tracked!="a.txt"{
  t.Fatalf("git did not resolve exact index path: %q",tracked)
 }
 result,err:=s.executeRemoteGitFileIndex(context.Background(),session,
  remotetransport.RemoteActionGitStageFile,index,snapshot)
 if err!=nil||result.Operation!=remotetransport.RemoteActionGitStageFile||result.Head!=head{
  t.Fatalf("stage selected file failed: %+v %v",result,err)
 }
 if staged:=run("diff","--cached","--name-only");staged!="a.txt"{
  t.Fatalf("stage touched files other than selected path: %q",staged)
 }
 if got:=run("status","--porcelain","--","new.txt");got!="?? new.txt"{
  t.Fatalf("stage accidentally added untracked file: %q",got)
 }
 if data,err:=os.ReadFile(filepath.Join(s.Workspace,"b.txt"));err!=nil||string(data)!="changed\n"{
  t.Fatalf("stage modified other working file: %q %v",data,err)
 }
 if _,err:=s.executeRemoteGitFileIndex(context.Background(),session,
  remotetransport.RemoteActionGitStageFile,index,snapshot);err==nil{
  t.Fatal("stale Git Status snapshot was accepted twice")
 }
 fresh,err:=s.readRemoteGitStatus(context.Background())
 if err!=nil{t.Fatal(err)}
 result,err=s.executeRemoteGitFileIndex(context.Background(),session,
  remotetransport.RemoteActionGitUnstageFile,remoteFileTestRow(t,fresh,"a.txt"),fresh)
 if err!=nil||result.Operation!=remotetransport.RemoteActionGitUnstageFile{
  t.Fatalf("unstage selected file failed: %+v %v",result,err)
 }
 if staged:=run("diff","--cached","--name-only");staged!=""{
  t.Fatalf("unstage left staged content: %q",staged)
 }
 if got:=run("rev-parse","HEAD");got!=head{t.Fatal("index operation changed HEAD")}
 for _,name:=range []string{"a.txt","b.txt"}{
  data,err:=os.ReadFile(filepath.Join(s.Workspace,name))
  if err!=nil||string(data)!="changed\n"{t.Fatalf("index mutation altered worktree %s: %q %v",name,data,err)}
 }
}

func TestRemoteGitFileSelectionRejectsUnsafeNames(t *testing.T){
 for _,status:=range []string{
  "## main\n M ../escape.txt\n",
  "## main\n M dir/../escape.txt\n",
  "## main\n M dir/file with space.txt\n",
  "## main\n M \"quoted.txt\"\n",
  "## main\nR  old.txt -> new.txt\n",
  "## main\n?? newly-created.txt\n",
  "## main\nUU conflict.txt\n",
  "## main\n M .git/config\n",
 }{
  for _,op:=range []string{remotetransport.RemoteActionGitStageFile,
   remotetransport.RemoteActionGitUnstageFile}{
   if _,ok:=remoteGitSelectedTrackedFile(status,1,op);ok{
    t.Fatalf("unsafe file index admitted: %q %s",status,op)
   }
  }
 }
}

func TestRemoteGitFileDefaultOffFilterAndParentRepoDenied(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote process groups")}
 t.Setenv("TASKDECK_REMOTE_GIT_STATUS","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string){
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %q: %v %s",args,err,out)}
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 path:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(path,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","Initial")
 if err:=os.WriteFile(path,[]byte("changed\n"),0600);err!=nil{t.Fatal(err)}
 snapshot,err:=s.readRemoteGitStatus(context.Background())
 if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE_FILE","0")
 if _,err:=s.executeRemoteGitFileIndex(context.Background(),session,
  remotetransport.RemoteActionGitStageFile,1,snapshot);err==nil{
  t.Fatal("stage file ignored separate host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE_FILE","1")
 run("config","filter.risky.clean","sh -c 'touch should-not-run'")
 if _,err:=s.executeRemoteGitFileIndex(context.Background(),session,
  remotetransport.RemoteActionGitStageFile,1,snapshot);err==nil{
  t.Fatal("stage file admitted an external Git clean filter")
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"should-not-run"));!os.IsNotExist(err){
  t.Fatal("Git clean filter executed")
 }
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.executeRemoteGitFileIndex(context.Background(),session,
  remotetransport.RemoteActionGitStageFile,1,snapshot);err==nil{
  t.Fatal("stage file escaped exact workspace root")
 }
}

func TestRemoteGitFileViewerCannotMutateIndex(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_STATUS","1")
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE_FILE","1")
 t.Setenv("TASKDECK_REMOTE_GIT_UNSTAGE_FILE","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 for _,op:=range []string{
  remotetransport.RemoteActionGitStageFile,
  remotetransport.RemoteActionGitUnstageFile,
 }{
  frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:op,FileIndex:1,
   Snapshot:"## main\n M tracked.txt\n",
  })
  if err!=nil{t.Fatal(err)}
  response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
  if err==nil||len(response)!=0{
   t.Fatalf("Portal viewer escalated to %s",op)
  }
 }
}
