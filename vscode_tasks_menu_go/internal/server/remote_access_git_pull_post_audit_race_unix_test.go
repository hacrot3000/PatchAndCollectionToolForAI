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

// Hook mutates host repository state AFTER the attempted audit row.
// This is not a client-controlled ref, Git option or URL.
type remoteGitPullPostAuditMutation struct{
 identity.Store
 root,branch,mode string
}
func(store remoteGitPullPostAuditMutation)AppendAudit(ctx context.Context,
 event identity.AuditEvent)error{
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action!="remote.action.git_pull"||event.Result!="attempted"{
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
  return run("remote","set-url","origin","ssh://git@different.invalid/team/repo.git")
 case "tracking":
  return run("config","branch."+store.branch+".merge","refs/heads/different")
 case "head":
  return run("reset","--soft","HEAD^")
 case "worktree":
  return os.WriteFile(filepath.Join(store.root,"unexpected.txt"),
   []byte("untracked during audit\n"),0600)
 case "filter":
  return run("config","filter.raced.clean","/bin/false")
 case "http":
  return run("config","http.sslVerify","false")
 default:return errors.New("unknown test-only pull mutation")
 }
}

func TestRemoteGitPullPostAuditRejectsRepositoryRefConfigAndWorktreeDrift(t *testing.T){
 requireGit(t)
 for _,mode:=range []string{"url","tracking","head","worktree","filter","http"}{
  t.Run(mode,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_GIT_PULL","1")
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
   tracked:=filepath.Join(s.Workspace,"tracked.txt")
   if err:=os.WriteFile(tracked,[]byte("baseline\n"),0600);err!=nil{t.Fatal(err)}
   run("add","tracked.txt")
   run("commit","-qm","First")
   if err:=os.WriteFile(tracked,[]byte("second commit\n"),0600);err!=nil{t.Fatal(err)}
   run("add","tracked.txt")
   run("commit","-qm","Second")
   originalHEAD:=run("rev-parse","HEAD")
   parentHEAD:=run("rev-parse","HEAD^")
   branch:=run("symbolic-ref","--short","HEAD")
   // Configure one syntactically approved SSH upstream, but do not
   // contact it. If the mutation guard fails, the network operation
   // would contact a host the user never approved.
   run("remote","add","origin","ssh://git@original.invalid/team/repo.git")
   run("config","branch."+branch+".remote","origin")
   run("config","branch."+branch+".merge","refs/heads/"+branch)
   run("update-ref","refs/remotes/origin/"+branch,originalHEAD)
   s.Identity=remoteGitPullPostAuditMutation{
    Store:s.Identity,root:s.Workspace,branch:branch,mode:mode,
   }
   _,err:=s.executeRemoteGitPull(context.Background(),session)
   if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
    t.Fatalf("Git Pull executed after %s drift during audit: %v",mode,err)
   }
   want:=originalHEAD
   if mode=="head"{want=parentHEAD}
   if got:=run("rev-parse","HEAD");got!=want{
    t.Fatalf("Git Pull mutated HEAD after audit: %s != %s",got,want)
   }
   project,e:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
   if e!=nil{t.Fatal(e)}
   audit,e:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
    ProjectID:project.ID,UserID:session.userID,
    Action:"remote.action.git_pull",Limit:5,
   })
   if e!=nil||len(audit)!=1||audit[0].Result!="attempted"||
    strings.Contains(audit[0].Details,"original.invalid")||
    strings.Contains(audit[0].Details,"different.invalid"){
    t.Fatalf("host-only drift audit missing or leaked URL: %+v %v",audit,e)
   }
  })
 }
}
