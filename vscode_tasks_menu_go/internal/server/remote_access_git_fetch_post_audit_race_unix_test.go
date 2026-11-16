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
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Perform an unrelated HOST mutation during the durable attempted row.
// The remote client cannot choose the URL, Git config, branch or command.
type remoteGitFetchPostAuditMutation struct {
 identity.Store
 root,branch,mode string
 session *remoteTaskDeckSession
}
func(h remoteGitFetchPostAuditMutation)AppendAudit(
 ctx context.Context,event identity.AuditEvent)error{
 if err:=h.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action!="remote.action.git_fetch"||event.Result!="attempted"{
  return nil
 }
 run:=func(args ...string)error{
  cmd:=exec.Command("git",append([]string{"-C",h.root},args...)...)
  return cmd.Run()
 }
 switch h.mode{
 case "url":
  return run("remote","set-url","origin",
   "ssh://git@different.invalid/team/repo.git")
 case "upstream":
  return run("config","branch."+h.branch+".merge","refs/heads/other")
 case "head":
  return run("reset","--soft","HEAD^")
 case "tracking":
  // Point the tracking ref at an existing different commit.
  return run("update-ref","refs/remotes/origin/"+h.branch,"HEAD^")
 case "http":
  return run("config","http.sslVerify","false")
 case "opt_out":
  return os.Setenv("TASKDECK_REMOTE_GIT_FETCH","0")
 case "role":
  project,err:=h.Store.ProjectByKey(ctx,h.session.projectKey)
  if err!=nil{return err}
  member,err:=h.Store.ProjectMember(ctx,project.ID,h.session.userID)
  if err!=nil{return err}
  member.RoleID=identity.RestrictedDeveloperRoleID
  member.UpdatedAt=time.Now().UTC()
  return h.Store.UpsertProjectMember(ctx,member)
 default:
  return errors.New("unrecognized fetch regression mode")
 }
}

func TestRemoteGitFetchPostAuditRejectsDriftAndRevocation(t *testing.T){
 requireGit(t)
 for _,mode:=range []string{
  "url","upstream","head","tracking","http","opt_out","role",
 }{
  t.Run(mode,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_GIT_FETCH","1")
   s,session,_,_:=remoteTerminalAdminFixture(t)
   gitInitRemoteTest(t,s.Workspace)
   run:=func(args ...string)string{
    t.Helper()
    cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
    out,err:=cmd.CombinedOutput()
    if err!=nil{t.Fatalf("git %v: %v %s",args,err,out)}
    return strings.TrimSpace(string(out))
   }
   run("config","user.name","Fetch Guard")
   run("config","user.email","fetch@example.invalid")
   file:=filepath.Join(s.Workspace,"tracked.txt")
   if err:=os.WriteFile(file,[]byte("baseline\n"),0600);err!=nil{
    t.Fatal(err)
   }
   run("add","-A")
   run("commit","-qm","first")
   if err:=os.WriteFile(file,[]byte("second\n"),0600);err!=nil{
    t.Fatal(err)
   }
   run("add","tracked.txt")
   run("commit","-qm","second")
   head:=run("rev-parse","HEAD")
   branch:=run("symbolic-ref","--short","HEAD")
   run("remote","add","origin","ssh://git@original.invalid/team/repo.git")
   run("config","branch."+branch+".remote","origin")
   run("config","branch."+branch+".merge","refs/heads/"+branch)
   run("update-ref","refs/remotes/origin/"+branch,head)

   // A malicious network target must NEVER be contacted. This CI-only
   // fake SSH records a forbidden attempted connection and returns 1.
   mockBin:=t.TempDir()
   marker:=filepath.Join(mockBin,"network-invoked")
   script:="#!/bin/sh\nprintf forbidden > "+strconv.Quote(marker)+"\nexit 1\n"
   if err:=os.WriteFile(filepath.Join(mockBin,"ssh"),
    []byte(script),0700);err!=nil{t.Fatal(err)}
   t.Setenv("PATH",mockBin+string(os.PathListSeparator)+os.Getenv("PATH"))
   store:=s.Identity
   s.Identity=remoteGitFetchPostAuditMutation{
    Store:store,root:s.Workspace,branch:branch,mode:mode,session:session,
   }
   _,err:=s.executeRemoteGitFetch(context.Background(),session)
   if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
    t.Fatalf("Git Fetch accepted post-audit %s: %v",mode,err)
   }
   if _,err:=os.Stat(marker);!os.IsNotExist(err){
    t.Fatalf("Git Fetch contacted network after %s drift: %v",mode,err)
   }
   expected:=head
   if mode=="head"{expected=run("rev-parse","HEAD")}
   if got:=run("rev-parse","HEAD");got!=expected{
    t.Fatalf("Git Fetch unexpectedly moved local HEAD %s != %s",got,expected)
   }
   audit,err:=store.ListAudit(context.Background(),identity.AuditQuery{
    Action:"remote.action.git_fetch",Limit:5,
   })
   if err!=nil||len(audit)!=1||audit[0].Result!="attempted"||
    strings.Contains(audit[0].Details,"original.invalid")||
    strings.Contains(audit[0].Details,"different.invalid"){
    t.Fatalf("fetch audit missing or URL leaked: %+v %v",audit,err)
   }
  })
 }
}
