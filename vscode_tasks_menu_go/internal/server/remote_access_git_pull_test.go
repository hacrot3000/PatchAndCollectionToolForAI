package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strconv"
 "strings"
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
 branchCommand:=exec.Command("git","-C",s.Workspace,"symbolic-ref","--short","HEAD")
 rawBranch,err:=branchCommand.Output()
 if err!=nil{t.Fatal(err)}
 branch:=strings.TrimSpace(string(rawBranch))
 run("remote","add","origin",filepath.Join(s.Workspace,".git"))
 run("config","branch."+branch+".remote","origin")
 run("config","branch."+branch+".merge","refs/heads/"+branch)
 run("update-ref","refs/remotes/origin/"+branch,"HEAD")
 if _,err:=s.executeRemoteGitPull(context.Background(),session);err==nil {
  t.Fatal("Git pull accepted a local filesystem remote")
 }
 run("remote","set-url","origin","ssh://git@example.invalid/team/repo.git")
 run("config","branch."+branch+".merge","refs/heads/different")
 if _,err:=s.executeRemoteGitPull(context.Background(),session);err==nil {
  t.Fatal("Git pull accepted a different upstream branch")
 }
 run("config","branch."+branch+".merge","refs/heads/"+branch)
 run("remote","set-url","--add","origin","ssh://git@other.invalid/team/repo.git")
 if _,err:=s.executeRemoteGitPull(context.Background(),session);err==nil {
  t.Fatal("Git pull accepted multiple fetch destinations")
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

func TestRemoteGitPullFastForwardFromSingleSSHUpstream(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only bounded Git pull")}
 t.Setenv("TASKDECK_REMOTE_GIT_PULL","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 gitPath,err:=exec.LookPath("git")
 if err!=nil{t.Fatal(err)}
 run:=func(dir string,args ...string)string{
  t.Helper()
  cmd:=exec.Command(gitPath,append([]string{"-C",dir},args...)...)
  result,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v: %v: %s",args,err,result)}
  return strings.TrimSpace(string(result))
 }
 run(s.Workspace,"config","user.name","Remote CI")
 run(s.Workspace,"config","user.email","remote@example.invalid")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"pull.txt"),[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run(s.Workspace,"add","-A")
 run(s.Workspace,"commit","-qm","Base")
 branch:=run(s.Workspace,"symbolic-ref","--short","HEAD")
 before:=run(s.Workspace,"rev-parse","HEAD")
 bare:=filepath.Join(t.TempDir(),"bare.git")
 run(s.Workspace,"init","--bare","-q",bare)
 run(s.Workspace,"push",bare,"HEAD:refs/heads/"+branch)
 run(s.Workspace,"--git-dir",bare,"symbolic-ref","HEAD","refs/heads/"+branch)
 other:=filepath.Join(t.TempDir(),"other")
 run(s.Workspace,"clone","-q",bare,other)
 run(other,"config","user.name","Remote author")
 run(other,"config","user.email","author@example.invalid")
 if err:=os.WriteFile(filepath.Join(other,"pull.txt"),[]byte("updated\n"),0600);err!=nil{t.Fatal(err)}
 run(other,"add","pull.txt")
 run(other,"commit","-qm","Update")
 run(other,"push","-q","origin","HEAD:refs/heads/"+branch)
 want:=run(other,"rev-parse","HEAD")
 run(s.Workspace,"remote","add","origin","ssh://git@example.invalid/repo.git")
 run(s.Workspace,"config","branch."+branch+".remote","origin")
 run(s.Workspace,"config","branch."+branch+".merge","refs/heads/"+branch)
 run(s.Workspace,"update-ref","refs/remotes/origin/"+branch,before)
 mockBin:=t.TempDir()
 script:="#!/bin/sh\nexec "+strconv.Quote(gitPath)+" upload-pack "+strconv.Quote(bare)+"\n"
 if err:=os.WriteFile(filepath.Join(mockBin,"ssh"),[]byte(script),0700);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",mockBin+string(os.PathListSeparator)+os.Getenv("PATH"))
 result,err:=s.executeRemoteGitPull(context.Background(),session)
 if err!=nil||!result.Updated||result.Before!=before||result.After!=want{
  t.Fatalf("fixed fast-forward pull failed: %+v %v, want %s",result,err,want)
 }
 if got:=run(s.Workspace,"rev-parse","HEAD");got!=want{
  t.Fatalf("pulled HEAD %s != %s",got,want)
 }
}

func TestRemoteGitPullRefusesExecutableFiltersBeforeGitStatus(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only remote Git process-group")}
 t.Setenv("TASKDECK_REMOTE_GIT_PULL","1")
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  command:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  output,err:=command.CombinedOutput()
  if err!=nil{t.Fatalf("git %v: %v: %s",args,err,output)}
  return strings.TrimSpace(string(output))
 }
 run("config","user.name","Remote test")
 run("config","user.email","remote@example.invalid")
 if err:=os.WriteFile(filepath.Join(s.Workspace,".gitattributes"),
  []byte("victim.txt filter=demo\n"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"victim.txt"),
  []byte("original\n"),0600);err!=nil{t.Fatal(err)}
 run("add","-A")
 run("commit","-qm","Baseline")
 branch:=run("symbolic-ref","--short","HEAD")
 before:=run("rev-parse","HEAD")
 run("remote","add","origin","ssh://git@example.invalid/team/repo.git")
 run("config","branch."+branch+".remote","origin")
 run("config","branch."+branch+".merge","refs/heads/"+branch)
 run("update-ref","refs/remotes/origin/"+branch,before)
 marker:=filepath.Join(t.TempDir(),"filter-ran")
 // Even a Git status requested by a pull preflight may run clean
 // drivers. The host must reject the repository's executable filter
 // configuration before Git can invoke any user-defined helper.
 for _,kind:=range []string{"clean","smudge","process"}{
  key:="filter.demo."+kind
  run("config",key,"sh -c 'echo EXECUTED > "+marker+"; cat'")
  if _,err:=s.executeRemoteGitPull(context.Background(),session);err==nil{
   t.Fatalf("Git pull admitted executable filter.%s",kind)
  }
  if _,err:=os.Stat(marker);err==nil{
   t.Fatalf("Git pull launched repository-defined filter.%s",kind)
  }else if !os.IsNotExist(err){t.Fatal(err)}
  if head:=run("rev-parse","HEAD");head!=before{
   t.Fatal("denied pull changed HEAD")
  }
  run("config","--unset",key)
 }
}
