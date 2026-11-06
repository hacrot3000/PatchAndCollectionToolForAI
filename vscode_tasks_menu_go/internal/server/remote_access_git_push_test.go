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

func TestRemoteGitPushURLPolicy(t *testing.T){
 for _,u:=range []string{
  "git@github.com:owner/repo.git","ssh://git@github.com/owner/repo.git",
  "ssh://git@example.org:2222/team/repo.git",
  "https://github.com/owner/repo.git",
 }{
  if !validRemoteGitPushURL(u){t.Fatalf("valid network Git destination rejected: %q",u)}
 }
 for _,u:=range []string{"","/tmp/repo","./repo","file:///tmp/repo",
  "ext::sh -c echo","git://example.org/repo.git",
  "https://user:token@example.org/repo.git",
  "http://example.org/repo.git","ftp://example.org/repo",
  "https://example.org/repo.git?secret=123",
  "ssh://git@example.org/repo.git\nssh://other/repo.git",
  "git@github.com:../../sensitive",
  "git@github.com:owner/repo.git\n",
  "ssh://git@example.org",
 }{
  if validRemoteGitPushURL(u){t.Fatalf("unsafe Git destination admitted: %q",u)}
 }
}

func TestRemoteGitPushHostOptInAndUnsafeRemoteDenied(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only Git process-group execution")}
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  output,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("Git test setup %q: %v: %s",args,err,output)}
  return strings.TrimSpace(string(output))
 }
 run("config","user.name","TaskDeck test")
 run("config","user.email","test@example.invalid")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"tracked.txt"),[]byte("one\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("commit","-m","Initial")
 branch:=run("symbolic-ref","-q","--short","HEAD")
 run("remote","add","origin",filepath.Join(s.Workspace,".git"))
 run("config","branch."+branch+".remote","origin")
 run("config","branch."+branch+".merge","refs/heads/"+branch)
 run("update-ref","refs/remotes/origin/"+branch,"HEAD")
 t.Setenv("TASKDECK_REMOTE_GIT_PUSH","0")
 if _,err:=s.executeRemoteGitPush(context.Background(),session);err==nil{
  t.Fatal("remote Git push bypassed host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_PUSH","1")
 if _,err:=s.executeRemoteGitPush(context.Background(),session);err==nil{
  t.Fatal("Git push accepted a local filesystem remote")
 }
 run("remote","set-url","origin","ssh://git@example.org/team/repo.git")
 run("config","branch."+branch+".merge","refs/heads/other")
 if _,err:=s.executeRemoteGitPush(context.Background(),session);err==nil{
  t.Fatal("Git push silently redirected to different branch")
 }
 run("config","branch."+branch+".merge","refs/heads/"+branch)
 run("remote","set-url","--push","origin","ssh://git@example.org/team/repo.git")
 run("remote","set-url","--push","--add","origin","ssh://git@example.net/team/repo.git")
 if _,err:=s.executeRemoteGitPush(context.Background(),session);err==nil{
  t.Fatal("Git push accepted multiple destination push URLs")
 }
 // The remote push was never attempted: each test case must fail before
 // the durable write audit that precedes external side effects.
}

func TestRemoteGitPushViewerDeniedInAuthenticatedDispatcher(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PUSH","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitPush,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err==nil||len(reply)!=0{
  t.Fatal("Portal viewer gained TaskDeck git.write permission via GitPush")
 }
}

func TestRemoteGitPushRejectsParentRepository(t *testing.T){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PUSH","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.executeRemoteGitPush(context.Background(),session);err==nil{
  t.Fatal("Remote Git push escaped exact TaskDeck workspace root")
 }
}

func TestRemoteGitPushFastForwardAndRejectRemoteDivergence(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote Git execution")}
 t.Setenv("TASKDECK_REMOTE_GIT_PUSH","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 gitPath,err:=exec.LookPath("git")
 if err!=nil{t.Fatal(err)}
 run:=func(dir string,args ...string)string{
  t.Helper()
  command:=exec.Command(gitPath,append([]string{"-C",dir},args...)...)
  output,err:=command.CombinedOutput()
  if err!=nil{t.Fatalf("git in %s %+v: %v: %s",dir,args,err,output)}
  return strings.TrimSpace(string(output))
 }
 run(s.Workspace,"config","user.name","TaskDeck CI")
 run(s.Workspace,"config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"demo.txt")
 if err:=os.WriteFile(file,[]byte("first\n"),0600);err!=nil{t.Fatal(err)}
 run(s.Workspace,"add","demo.txt")
 run(s.Workspace,"commit","-m","Base")
 branch:=run(s.Workspace,"symbolic-ref","-q","--short","HEAD")
 bare:=filepath.Join(t.TempDir(),"bare.git")
 if err:=os.MkdirAll(bare,0700);err!=nil{t.Fatal(err)}
 run(s.Workspace,"init","--bare","-q",bare)
 run(s.Workspace,"push",bare,"HEAD:refs/heads/"+branch)
 run(s.Workspace,"remote","add","origin","ssh://git@example.invalid/team/repo.git")
 run(s.Workspace,"config","branch."+branch+".remote","origin")
 run(s.Workspace,"config","branch."+branch+".merge","refs/heads/"+branch)
 base:=run(s.Workspace,"rev-parse","HEAD")
 run(s.Workspace,"update-ref","refs/remotes/origin/"+branch,base)
 if err:=os.WriteFile(file,[]byte("second\n"),0600);err!=nil{t.Fatal(err)}
 run(s.Workspace,"add","demo.txt")
 run(s.Workspace,"commit","-m","Second")
 head:=run(s.Workspace,"rev-parse","HEAD")
 // Git invokes this test-only fake SSH binary as a deterministic transport
 // to a temporary bare repository. The production Agent never accepts
 // a local file URL, test script, remote command or SSH override from E2EE.
 mockBin:=t.TempDir()
 ssh:=filepath.Join(mockBin,"ssh")
 commandScript:="#!/bin/sh\nexec "+strconv.Quote(gitPath)+" receive-pack "+strconv.Quote(bare)+"\n"
 if err:=os.WriteFile(ssh,[]byte(commandScript),0700);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",mockBin+string(os.PathListSeparator)+os.Getenv("PATH"))
 result,err:=s.executeRemoteGitPush(context.Background(),session)
 if err!=nil||result.Head!=head{
  t.Fatalf("bounded fast-forward push failed: %+v %v",result,err)
 }
 got:=run(s.Workspace,"--git-dir",bare,"rev-parse","refs/heads/"+branch)
 if got!=head{t.Fatalf("pushed HEAD mismatch: %s != %s",got,head)}
 // Advance the server independently; a stale local tracking ref must
 // NEVER cause a force update when the server has progressed.
 run(s.Workspace,"--git-dir",bare,"symbolic-ref","HEAD","refs/heads/"+branch)
 other:=filepath.Join(t.TempDir(),"other")
 run(s.Workspace,"clone","-q",bare,other)
 run(other,"config","user.name","Another author")
 run(other,"config","user.email","other@example.invalid")
 if err:=os.WriteFile(filepath.Join(other,"demo.txt"),[]byte("remote ahead\n"),0600);err!=nil{t.Fatal(err)}
 run(other,"add","demo.txt")
 run(other,"commit","-m","Remote advanced")
 run(other,"push","origin","HEAD:refs/heads/"+branch)
 advanced:=run(s.Workspace,"--git-dir",bare,"rev-parse","refs/heads/"+branch)
 run(s.Workspace,"update-ref","refs/remotes/origin/"+branch,head)
 if _,err:=s.executeRemoteGitPush(context.Background(),session);err==nil{
  t.Fatal("remote server advanced but client Git push did not fail")
 }
 if after:=run(s.Workspace,"--git-dir",bare,"rev-parse","refs/heads/"+branch);after!=advanced{
  t.Fatal("remote non-fast-forward guard changed upstream branch")
 }
}
