package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strconv"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitFetchOptInAndUnsafeDestinationsDenied(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote Git process groups")}
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %+v failed: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 run("config","user.name","TaskDeck Test")
 run("config","user.email","test@example.invalid")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"tracked.txt"),[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","initial")
 branch:=run("symbolic-ref","-q","--short","HEAD")
 run("remote","add","origin",filepath.Join(s.Workspace,".git"))
 run("config","branch."+branch+".remote","origin")
 run("config","branch."+branch+".merge","refs/heads/"+branch)
 run("update-ref","refs/remotes/origin/"+branch,"HEAD")
 t.Setenv("TASKDECK_REMOTE_GIT_FETCH","0")
 if _,err:=s.executeRemoteGitFetch(context.Background(),session);err==nil{
  t.Fatal("Git fetch bypassed the host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_FETCH","1")
 if _,err:=s.executeRemoteGitFetch(context.Background(),session);err==nil{
  t.Fatal("Git fetch accepted local file repository URL")
 }
 run("remote","set-url","origin","ssh://git@example.invalid/team/repo.git")
 run("remote","set-url","--add","origin","ssh://git@other.invalid/team/repo.git")
 if _,err:=s.executeRemoteGitFetch(context.Background(),session);err==nil{
  t.Fatal("Git fetch accepted multiple configured remote fetch URLs")
 }
 run("remote","set-url","--delete","origin","ssh://git@other.invalid/team/repo.git")
 run("config","branch."+branch+".merge","refs/heads/different")
 if _,err:=s.executeRemoteGitFetch(context.Background(),session);err==nil{
  t.Fatal("Git fetch targeted a branch other than current upstream")
 }
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.executeRemoteGitFetch(context.Background(),session);err==nil{
  t.Fatal("Git fetch escaped a nested project into parent Git repository")
 }
}

func TestRemoteGitFetchViewerCannotUseGitWrite(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_FETCH","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitFetch,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err==nil||len(reply)>0{t.Fatal("viewer escalated to remote git.fetch")}
}

func TestRemoteGitFetchUpstreamAdvancementWithoutChangingHEAD(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote Git process groups")}
 t.Setenv("TASKDECK_REMOTE_GIT_FETCH","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 gitPath,err:=exec.LookPath("git")
 if err!=nil{t.Fatal(err)}
 run:=func(dir string,args ...string)string{
  t.Helper()
  cmd:=exec.Command(gitPath,append([]string{"-C",dir},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %s %+v failed: %v %s",dir,args,err,out)}
  return strings.TrimSpace(string(out))
 }
 run(s.Workspace,"config","user.name","TaskDeck Test")
 run(s.Workspace,"config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"demo.txt")
 if err:=os.WriteFile(file,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run(s.Workspace,"add","demo.txt")
 run(s.Workspace,"commit","-m","base")
 branch:=run(s.Workspace,"symbolic-ref","-q","--short","HEAD")
 head:=run(s.Workspace,"rev-parse","HEAD")
 bare:=filepath.Join(t.TempDir(),"remote.git")
 run(s.Workspace,"init","--bare","-q",bare)
 run(s.Workspace,"push",bare,"HEAD:refs/heads/"+branch)
 // Host's upstream has one more commit than local HEAD.
 if err:=os.WriteFile(file,[]byte("remote ahead\n"),0600);err!=nil{t.Fatal(err)}
 run(s.Workspace,"add","demo.txt")
 run(s.Workspace,"commit","-m","remote-next")
 latest:=run(s.Workspace,"rev-parse","HEAD")
 run(s.Workspace,"push",bare,"HEAD:refs/heads/"+branch)
 run(s.Workspace,"reset","--hard",head)
 run(s.Workspace,"remote","add","origin","ssh://git@example.invalid/team/repo.git")
 run(s.Workspace,"config","branch."+branch+".remote","origin")
 run(s.Workspace,"config","branch."+branch+".merge","refs/heads/"+branch)
 run(s.Workspace,"update-ref","refs/remotes/origin/"+branch,head)

 mockBin:=t.TempDir()
 // This SSH binary is ONLY a CI fixture and always serves the temp bare
 // repository; production E2EE cannot select an SSH binary or transport.
 fakeSSH:="#!/bin/sh\nexec "+strconv.Quote(gitPath)+" upload-pack "+strconv.Quote(bare)+"\n"
 if err:=os.WriteFile(filepath.Join(mockBin,"ssh"),[]byte(fakeSSH),0700);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",mockBin+string(os.PathListSeparator)+os.Getenv("PATH"))
 result,err:=s.executeRemoteGitFetch(context.Background(),session)
 if err!=nil||!result.Updated||result.Head!=head||
  result.Before!=head||result.After!=latest{
  t.Fatalf("bounded fetch failed: %+v %v",result,err)
 }
 if after:=run(s.Workspace,"rev-parse","HEAD");after!=head{
  t.Fatalf("fetch moved local HEAD: %s != %s",after,head)
 }
 unchanged,err:=s.executeRemoteGitFetch(context.Background(),session)
 if err!=nil||unchanged.Updated||unchanged.After!=latest{
  t.Fatalf("repeat upstream fetch was not idempotent: %+v %v",unchanged,err)
 }
 // Rewind upstream while the host tracking ref stays advanced.
 run(s.Workspace,"--git-dir",bare,"update-ref","refs/heads/"+branch,head)
 if _,err:=s.executeRemoteGitFetch(context.Background(),session);err==nil{
  t.Fatal("fetch force-updated a rewritten upstream branch")
 }
 if track:=run(s.Workspace,"rev-parse","refs/remotes/origin/"+branch);track!=latest{
  t.Fatal("non-fast-forward fetch changed tracking ref")
 }
}
