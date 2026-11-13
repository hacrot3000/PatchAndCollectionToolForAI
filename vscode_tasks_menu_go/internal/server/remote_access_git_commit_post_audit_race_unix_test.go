//go:build linux || darwin

package server

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Mutations are injected by the HOST after the mandatory attempted audit.
// The E2EE client can only provide a bounded commit message, never these
// refs, paths, environment variables, commands or test-only hooks.
type remoteGitCommitPostAuditMutation struct {
 identity.Store
 root,mode string
}

func (store remoteGitCommitPostAuditMutation)AppendAudit(ctx context.Context,
 event identity.AuditEvent)error{
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action!="remote.action.git_commit"||event.Result!="attempted"{
  return nil
 }
 run:=func(args ...string)error{
  command:=exec.Command("git",append([]string{"-C",store.root},args...)...)
  command.Stdout=os.Stdout
  command.Stderr=os.Stderr
  return command.Run()
 }
 switch store.mode{
 case "index":
  file:=filepath.Join(store.root,"tracked.txt")
  if err:=os.WriteFile(file,[]byte("changed during audit\n"),0600);err!=nil{
   return err
  }
  return run("add","tracked.txt")
 case "branch":
  if err:=run("branch","other-branch");err!=nil{return err}
  return run("symbolic-ref","HEAD","refs/heads/other-branch")
 case "head":
  return run("reset","--soft","HEAD^")
 case "filter":
  return run("config","filter.raced.clean","/bin/false")
 }
 return errors.New("unknown audit mutation")
}

func TestRemoteGitCommitPostAuditRejectsIndexBranchHEADAndFilterDrift(t *testing.T){
 requireGit(t)
 for _,mode:=range []string{"index","branch","head","filter"}{
  t.Run(mode,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","1")
   s,session,_,_:=remoteTerminalAdminFixture(t)
   gitInitRemoteTest(t,s.Workspace)
   run:=func(args ...string)string{
    t.Helper()
    command:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
    output,err:=command.CombinedOutput()
    if err!=nil{t.Fatalf("git %v: %v %s",args,err,output)}
    return strings.TrimSpace(string(output))
   }
   run("config","user.name","TaskDeck Regression")
   run("config","user.email","test@example.invalid")
   file:=filepath.Join(s.Workspace,"tracked.txt")
   if err:=os.WriteFile(file,[]byte("initial\n"),0600);err!=nil{t.Fatal(err)}
   run("add","tracked.txt")
   run("commit","-qm","First")
   if err:=os.WriteFile(file,[]byte("second baseline\n"),0600);err!=nil{
    t.Fatal(err)
   }
   run("add","tracked.txt")
   run("commit","-qm","Second")
   initialHead:=run("rev-parse","HEAD")
   originalBranch:=run("symbolic-ref","-q","HEAD")
   parent:=run("rev-parse","HEAD^")
   if err:=os.WriteFile(file,[]byte("staged before audit\n"),0600);err!=nil{
    t.Fatal(err)
   }
   run("add","tracked.txt")
   s.Identity=remoteGitCommitPostAuditMutation{
    Store:s.Identity,root:s.Workspace,mode:mode,
   }
   _,err:=s.executeRemoteGitCommit(context.Background(),session,
    "MUST NOT COMMIT THIS RACE")
   if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
    t.Fatalf("post-audit %s drift admitted a Git commit: %v",mode,err)
   }
   wantHead:=initialHead
   if mode=="head"{wantHead=parent}
   if head:=run("rev-parse","HEAD");head!=wantHead{
    t.Fatalf("Remote Git Commit mutated changed HEAD: want %s, got %s",wantHead,head)
   }
   wantBranch:=originalBranch
   if mode=="branch"{wantBranch="refs/heads/other-branch"}
   if branch:=run("symbolic-ref","-q","HEAD");branch!=wantBranch{
    t.Fatalf("Remote Git Commit changed branch again: want %q, got %q",
     wantBranch,branch)
   }
   if latest:=run("log","-1","--format=%s");
    latest=="MUST NOT COMMIT THIS RACE"{
    t.Fatal("Git commit was executed after post-audit local drift")
   }
   project,e:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
   if e!=nil{t.Fatal(e)}
   events,e:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
    ProjectID:project.ID,UserID:session.userID,
    Action:"remote.action.git_commit",Limit:10,
   })
   if e!=nil||len(events)!=1||events[0].Result!="attempted"||
    strings.Contains(events[0].Details,"tracked.txt")||
    strings.Contains(events[0].Details,"MUST NOT"){
    t.Fatalf("post-audit rejection leaked staged data or missed audit: %+v %v",
     events,e)
   }
  })
 }
}
