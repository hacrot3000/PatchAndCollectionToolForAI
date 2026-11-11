//go:build linux || darwin

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
)

// A configured submodule.*.ignore=all can make a dirty nested worktree
// invisible to ordinary Git status. Pull must explicitly override this
// setting BEFORE an SSH fetch or any remote mutation is attempted.
func TestRemoteGitPullRejectsDirtyIgnoredSubmoduleBeforeSSH(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only Git process cleanup")}
 t.Setenv("TASKDECK_REMOTE_GIT_PULL","1")
 s,session,_,_:=remoteTerminalAdminFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 gitPath,err:=exec.LookPath("git")
 if err!=nil{t.Fatal(err)}
 run:=func(dir string,args ...string)string{
  t.Helper()
  cmd:=exec.Command(gitPath,append([]string{"-C",dir},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %+v in %s: %v: %s",args,dir,err,out)}
  return strings.TrimSpace(string(out))
 }
 for _,dir:=range []string{s.Workspace}{
  run(dir,"config","user.name","TaskDeck safety test")
  run(dir,"config","user.email","test@example.invalid")
 }
 if err:=os.WriteFile(filepath.Join(s.Workspace,"tracked.txt"),
  []byte("base\n"),0600);err!=nil{t.Fatal(err)}
 run(s.Workspace,"add","tracked.txt")
 run(s.Workspace,"commit","-qm","Base")
 child:=filepath.Join(t.TempDir(),"nested-repo")
 if err:=os.Mkdir(child,0700);err!=nil{t.Fatal(err)}
 run(child,"init","-q")
 run(child,"config","user.name","Nested fixture")
 run(child,"config","user.email","nested@example.invalid")
 if err:=os.WriteFile(filepath.Join(child,"file.txt"),
  []byte("committed\n"),0600);err!=nil{t.Fatal(err)}
 run(child,"add","file.txt")
 run(child,"commit","-qm","Submodule base")
 run(s.Workspace,"-c","protocol.file.allow=always","submodule","add",
  child,"vendor/module")
 run(s.Workspace,"add","-A")
 run(s.Workspace,"commit","-qm","Add submodule")
 branch:=run(s.Workspace,"symbolic-ref","--short","HEAD")
 run(s.Workspace,"remote","add","origin","ssh://git@example.invalid/team/repo.git")
 run(s.Workspace,"config","branch."+branch+".remote","origin")
 run(s.Workspace,"config","branch."+branch+".merge","refs/heads/"+branch)
 run(s.Workspace,"update-ref","refs/remotes/origin/"+branch,"HEAD")
 // Configured ignore=all is a legitimate local setting but must never
 // weaken the Remote Pull requirement that the entire tree be clean.
 run(s.Workspace,"config","submodule.vendor/module.ignore","all")
 nestedUntracked:=filepath.Join(s.Workspace,"vendor","module","unsaved.txt")
 if err:=os.WriteFile(nestedUntracked,[]byte("not staged\n"),0600);err!=nil{
  t.Fatal(err)
 }
 visible:=run(s.Workspace,"status","--porcelain=v1","--untracked-files=all",
  "--ignore-submodules=none")
 if visible==""{
  t.Fatal("fixture failed to expose dirty submodule with strict status")
 }
 mock:=t.TempDir()
 marker:=filepath.Join(mock,"ssh-launched")
 script:="#!/bin/sh\nprintf invoked > "+strconv.Quote(marker)+"\nexit 77\n"
 if err:=os.WriteFile(filepath.Join(mock,"ssh"),[]byte(script),0700);err!=nil{
  t.Fatal(err)
 }
 t.Setenv("PATH",mock+string(os.PathListSeparator)+os.Getenv("PATH"))
 if _,err:=s.executeRemoteGitPull(context.Background(),session);err==nil{
  t.Fatal("Remote Pull admitted an ignored-but-dirty submodule")
 }
 if _,err:=os.Stat(marker);!os.IsNotExist(err){
  t.Fatalf("Remote Pull contacted SSH before rejecting dirty submodule: %v",err)
 }
 if _,err:=os.Stat(nestedUntracked);err!=nil{
  t.Fatal("denied pull discarded a nested working-tree change")
 }
}
