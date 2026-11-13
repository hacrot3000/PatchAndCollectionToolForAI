//go:build linux || darwin

package server

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "strconv"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// These are HOST-LOCAL mutations made after the durable attempted audit.
// Neither the browser nor PHP supplies a Git command, ref, URL or directory.
type remoteGitPushPostAuditMutation struct{
 identity.Store
 root,branch,mode,parent string
}
func (store remoteGitPushPostAuditMutation)AppendAudit(ctx context.Context,
 event identity.AuditEvent)error{
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action!="remote.action.git_push"||event.Result!="attempted"{
  return nil
 }
 run:=func(args ...string)error{
  cmd:=exec.Command("git",append([]string{"-C",store.root},args...)...)
  cmd.Stdout=os.Stdout
  cmd.Stderr=os.Stderr
  return cmd.Run()
 }
 switch store.mode{
 case "url":
  return run("remote","set-url","--push","origin",
   "ssh://git@changed.invalid/team/repo.git")
 case "branch":
  if err:=run("branch","other-branch");err!=nil{return err}
  return run("symbolic-ref","HEAD","refs/heads/other-branch")
 case "head":
  return run("reset","--soft","HEAD^")
 case "merge":
  return run("config","branch."+store.branch+".merge","refs/heads/different")
 case "remote":
  return run("config","branch."+store.branch+".remote","unexpected")
 case "tracking":
  return run("update-ref","refs/remotes/origin/"+store.branch,store.parent)
 case "http":
  return run("config","http.sslVerify","false")
 default:return errors.New("unknown audit-time push fixture")
 }
}

func TestRemoteGitPushPostAuditRejectsChangedURLBranchHeadTrackingAndHTTP(t *testing.T){
 requireGit(t)
 for _,mode:=range []string{
  "url","branch","head","merge","remote","tracking","http",
 }{
  t.Run(mode,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_GIT_PUSH","1")
   s,session,_,_:=remoteTerminalAdminFixture(t)
   gitInitRemoteTest(t,s.Workspace)
   run:=func(args ...string)string{
    t.Helper()
    cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
    output,err:=cmd.CombinedOutput()
    if err!=nil{t.Fatalf("git %v: %v %s",args,err,output)}
    return strings.TrimSpace(string(output))
   }
   run("config","user.name","TaskDeck Regression")
   run("config","user.email","test@example.invalid")
   file:=filepath.Join(s.Workspace,"tracked.txt")
   if err:=os.WriteFile(file,[]byte("first\n"),0600);err!=nil{t.Fatal(err)}
   run("add","tracked.txt")
   run("commit","-qm","First")
   parent:=run("rev-parse","HEAD")
   if err:=os.WriteFile(file,[]byte("second\n"),0600);err!=nil{t.Fatal(err)}
   run("add","tracked.txt")
   run("commit","-qm","Second")
   before:=run("rev-parse","HEAD")
   branch:=run("symbolic-ref","--short","HEAD")
   run("remote","add","origin","ssh://git@initial.invalid/team/repo.git")
   run("config","branch."+branch+".remote","origin")
   run("config","branch."+branch+".merge","refs/heads/"+branch)
   run("update-ref","refs/remotes/origin/"+branch,before)
   // If any of these cases unexpectedly starts a network push, this
   // SSH shim creates a marker. It never connects to the network.
   fakeBin:=t.TempDir()
   marker:=filepath.Join(fakeBin,"SSH-MUST-NOT-RUN")
   script:="#!/bin/sh\nprintf invoked > "+strconv.Quote(marker)+"\nexit 93\n"
   if err:=os.WriteFile(filepath.Join(fakeBin,"ssh"),[]byte(script),0700);err!=nil{
    t.Fatal(err)
   }
   t.Setenv("PATH",fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
   originalStore:=s.Identity
   s.Identity=remoteGitPushPostAuditMutation{
    Store:originalStore,root:s.Workspace,branch:branch,mode:mode,parent:parent,
   }
   _,err:=s.executeRemoteGitPush(context.Background(),session)
   if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
    t.Fatalf("push accepted host %s mutation after attempted audit: %v",mode,err)
   }
   expected:=before
   if mode=="head"{expected=parent}
   if got:=run("rev-parse","HEAD");got!=expected{
    t.Fatalf("unexpected HEAD mutation by remote push: got %s want %s",got,expected)
   }
   if _,err:=os.Stat(marker);!os.IsNotExist(err){
    t.Fatalf("Git Push invoked network SSH after audit-time %s drift: %v",mode,err)
   }
   project,err:=originalStore.ProjectByKey(context.Background(),session.projectKey)
   if err!=nil{t.Fatal(err)}
   audits,err:=originalStore.ListAudit(context.Background(),identity.AuditQuery{
    ProjectID:project.ID,UserID:session.userID,
    Action:"remote.action.git_push",Limit:6,
   })
   if err!=nil||len(audits)!=1||audits[0].Result!="attempted"||
    strings.Contains(audits[0].Details,"initial.invalid")||
    strings.Contains(audits[0].Details,"changed.invalid"){
    t.Fatalf("did not reach attempted audit or leaked Git URL: %+v %v",
     audits,err)
   }
  })
 }
}
