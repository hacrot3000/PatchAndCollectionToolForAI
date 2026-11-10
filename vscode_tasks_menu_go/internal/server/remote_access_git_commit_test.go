package server

import (
 "context"
 "errors"
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
 // This is otherwise a fully valid staged commit. A running job or PTY
 // must not allow Git to mutate HEAD before its host slot is released.
 session.longTaskMu.Lock()
 session.longTask=&remoteLongTaskJob{taskID:777,status:"running"}
 session.longTaskMu.Unlock()
 _,overlapErr:=s.executeRemoteGitCommit(context.Background(),session,"Unsafe overlap")
 if !errors.Is(overlapErr,remotetransport.ErrRemoteActionDenied)||
  run("rev-parse","HEAD")!=before{
  t.Fatalf("long task did not exclude Git commit: %v",overlapErr)
 }
 session.longTaskMu.Lock()
 session.longTask=nil
 session.longTaskMu.Unlock()
 session.terminalMu.Lock()
 session.terminal=&remoteTerminalJob{}
 session.terminalMu.Unlock()
 _,overlapErr=s.executeRemoteGitCommit(context.Background(),session,"Unsafe PTY overlap")
 if !errors.Is(overlapErr,remotetransport.ErrRemoteActionDenied)||
  run("rev-parse","HEAD")!=before{
  t.Fatalf("remote PTY did not exclude Git commit: %v",overlapErr)
 }
 session.terminalMu.Lock()
 session.terminal=nil
 session.terminalMu.Unlock()
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

func TestRemoteGitCommitRejectsExecutableFiltersBeforeCommit(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote Git execution")}
 t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  output,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v: %v: %s",args,err,output)}
  return strings.TrimSpace(string(output))
 }
 run("config","user.name","Remote test")
 run("config","user.email","remote@example.invalid")
 if err:=os.WriteFile(filepath.Join(s.Workspace,".gitattributes"),
  []byte("tracked.txt filter=demo\n"),0600);err!=nil{t.Fatal(err)}
 file:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(file,[]byte("initial\n"),0600);err!=nil{t.Fatal(err)}
 run("add","-A")
 run("commit","-qm","Base")
 before:=run("rev-parse","HEAD")
 if err:=os.WriteFile(file,[]byte("staged\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 marker:=filepath.Join(t.TempDir(),"executable-filter-ran")
 for _,kind:=range []string{"clean","smudge","process"}{
  key:="filter.demo."+kind
  run("config",key,"sh -c 'echo EXECUTED > "+marker+"; cat'")
  if _,err:=s.executeRemoteGitCommit(context.Background(),session,
   "Reject unsafe filter");err==nil{
   t.Fatalf("remote commit admitted filter.%s",kind)
  }
  if _,err:=os.Stat(marker);err==nil{
   t.Fatalf("remote commit executed configured filter.%s",kind)
  }else if !os.IsNotExist(err){t.Fatal(err)}
  if got:=run("rev-parse","HEAD");got!=before{
   t.Fatalf("denied filter.%s commit moved HEAD",kind)
  }
  run("config","--unset",key)
 }
}


func TestRemoteGitCommitRejectsNonBranchSymbolicHEAD(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only Git process-group execution")}
 t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  command:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  result,err:=command.CombinedOutput()
  if err!=nil{t.Fatalf("git fixture %v: %v %s",args,err,result)}
  return strings.TrimSpace(string(result))
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 tracked:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(tracked,[]byte("baseline\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-qm","baseline")
 head:=run("rev-parse","HEAD")
 // Deliberately make HEAD a valid symbolic ref outside refs/heads/.
 // Checking only git symbolic-ref -q would admit this state.
 run("update-ref","refs/taskdeck/unapproved",head)
 run("symbolic-ref","HEAD","refs/taskdeck/unapproved")
 if resolved:=run("symbolic-ref","-q","HEAD");
  resolved!="refs/taskdeck/unapproved"{
  t.Fatalf("malformed branch fixture: %q",resolved)
 }
 if err:=os.WriteFile(tracked,[]byte("changed\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 if _,err:=s.executeRemoteGitCommit(context.Background(),session,"Unsafe ref"); 
  !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("Remote Commit permitted a non-branch symbolic HEAD: %v",err)
 }
 if got:=run("rev-parse","refs/taskdeck/unapproved");got!=head{
  t.Fatalf("Remote Commit moved arbitrary refs namespace: %s != %s",got,head)
 }
 if got:=run("show","HEAD:tracked.txt");got!="baseline"{
  t.Fatalf("Remote Commit altered committed content: %q",got)
 }
}
