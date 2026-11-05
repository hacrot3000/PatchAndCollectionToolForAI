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

func TestRemoteGitCommitOnlyAlreadyStagedAndNoHooks(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote process groups")}
 t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  command:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  command.Env=append(os.Environ(),"LC_ALL=C")
  out,err:=command.CombinedOutput()
  if err!=nil{t.Fatalf("git %q: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(file,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","Initial")
 if err:=os.WriteFile(file,[]byte("staged\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 if err:=os.WriteFile(file,[]byte("unstaged\n"),0600);err!=nil{t.Fatal(err)}
 hooks:=filepath.Join(s.Workspace,".git","hooks")
 if err:=os.MkdirAll(hooks,0700);err!=nil{t.Fatal(err)}
 marker:=filepath.Join(s.Workspace,"hook-ran")
 if err:=os.WriteFile(filepath.Join(hooks,"pre-commit"),
  []byte("#!/bin/sh\nprintf yes > hook-ran\nexit 8\n"),0700);err!=nil{t.Fatal(err)}
 before:=run("rev-parse","HEAD")
 result,err:=s.executeRemoteGitCommit(context.Background(),session,"Sửa lỗi staging")
 if err!=nil{t.Fatalf("staged commit failed: %v",err)}
 if result.Before!=before||result.After==before||result.After!=run("rev-parse","HEAD"){
  t.Fatalf("unexpected commit change: %+v",result)
 }
 if got:=run("show","HEAD:tracked.txt");got!="staged"{
  t.Fatalf("unstaged data accidentally committed: %q",got)
 }
 disk,err:=os.ReadFile(file)
 if err!=nil||string(disk)!="unstaged\n"{
  t.Fatalf("working tree changed by commit: %q %v",disk,err)
 }
 if _,err:=os.Stat(marker);!os.IsNotExist(err){t.Fatalf("untrusted Git hook executed: %v",err)}
 if _,err:=s.executeRemoteGitCommit(context.Background(),session,"No staged changes");err==nil{
  t.Fatal("commit accepted with no staged changes")
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_commit",Limit:10,
 })
 if err!=nil||len(events)!=1||events[0].Result!="attempted"||
  strings.Contains(events[0].Details,"Sửa lỗi") {
  t.Fatalf("audited message disclosure or missing attempted audit: %+v %v",events,err)
 }
}

func TestRemoteGitCommitDefaultOffAndViewerDenied(t *testing.T){
 requireGit(t)
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","0")
 if _,err:=s.executeRemoteGitCommit(context.Background(),session,"Fix test");err==nil{
  t.Fatal("host opt-in bypassed")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","1")
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitCommit,
  Message:"Fix test",
 })
 if err!=nil{t.Fatal(err)}
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err==nil||len(response)>0{
  t.Fatal("viewer was able to perform authenticated git.write mutation")
 }
}

func TestRemoteGitCommitRejectsParentRepositoryWorkspace(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.Mkdir(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.executeRemoteGitCommit(context.Background(),session,"Fix test");err==nil{
  t.Fatal("commit escaped a nested workspace into parent repository")
 }
}
