package server

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitPullChangesHostOfflineAndClean(t *testing.T){
 s,_,_,_,git,branch,head:=setupRemoteGitPullReview(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_REVIEW","1")
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_CHANGES","0")
 if _,err:=s.readRemoteGitPullChanges(context.Background());err==nil{
  t.Fatal("changed-file review ran without host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_CHANGES","1")
 fresh,err:=s.readRemoteGitPullChanges(context.Background())
 if err!=nil||fresh.Head!=head||fresh.UpstreamHead!=head||len(fresh.Files)!=0{
  t.Fatalf("up-to-date tree should have no changes: %+v %v",fresh,err)
 }
 if err:=os.MkdirAll(filepath.Join(s.Workspace,"src"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"src","hello.txt"),[]byte("new remote commit\n"),0600);err!=nil{t.Fatal(err)}
 git("add","src/hello.txt")
 git("-c","user.name=Test","-c","user.email=test@example.invalid","commit","-m","upstream file")
 target:=git("rev-parse","HEAD")
 git("reset","--hard",head)
 git("update-ref","refs/remotes/origin/"+branch,target)
 got,err:=s.readRemoteGitPullChanges(context.Background())
 if err!=nil||got.Head!=head||got.UpstreamHead!=target||len(got.Files)!=1||
  got.Files[0]!="A:src/hello.txt"{
  t.Fatalf("offline file inventory inaccurate: %+v %v",got,err)
 }
 if result:=git("rev-parse","HEAD");result!=head{t.Fatal("changed-files review wrote HEAD")}
 if result:=git("rev-parse","refs/remotes/origin/"+branch);result!=target{
  t.Fatal("changed-files review changed upstream")
 }
 // The workspace must still be clean; an untracked file denies review.
 if err:=os.WriteFile(filepath.Join(s.Workspace,"private-note"),[]byte("secret"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteGitPullChanges(context.Background());err==nil{
  t.Fatal("review allowed untracked worktree")
 }
}

func TestRemoteGitPullChangesRejectsOversizeAndUnsafePaths(t *testing.T){
 s,_,_,_,git,branch,head:=setupRemoteGitPullReview(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_REVIEW","1")
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_CHANGES","1")
 // More than six changed paths is a hard denial, never partial disclosure.
 for i:=0;i<7;i++{
  name:="file"+string(rune('a'+i))
  if err:=os.WriteFile(filepath.Join(s.Workspace,name),[]byte("x"),0600);err!=nil{t.Fatal(err)}
 }
 git("add",".")
 git("-c","user.name=Test","-c","user.email=test@example.invalid","commit","-m","too many")
 big:=git("rev-parse","HEAD")
 git("reset","--hard",head)
 git("update-ref","refs/remotes/origin/"+branch,big)
 if _,err:=s.readRemoteGitPullChanges(context.Background());err==nil{
  t.Fatal("incomplete inventory was represented as complete")
 }
 git("update-ref","refs/remotes/origin/"+branch,head)
 // A path that cannot be represented unambiguously is rejected.
 if err:=os.WriteFile(filepath.Join(s.Workspace,"unsafe name.txt"),[]byte("x"),0600);err!=nil{t.Fatal(err)}
 git("add",".")
 git("-c","user.name=Test","-c","user.email=test@example.invalid","commit","-m","unsafe path")
 bad:=git("rev-parse","HEAD")
 git("reset","--hard",head)
 git("update-ref","refs/remotes/origin/"+branch,bad)
 if _,err:=s.readRemoteGitPullChanges(context.Background());err==nil{
  t.Fatal("unsafe path accepted in read-only inventory")
 }
 if strings.Contains(git("status","--short"),"unsafe name.txt"){
  t.Fatal("Git preview unexpectedly modified the worktree")
 }
}

func TestRemoteGitPullChangesE2EEPermissionAndAudit(t *testing.T){
 s,session,client,agent,_,_,head:=setupRemoteGitPullReview(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_REVIEW","1")
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_CHANGES","1")
 action,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitPullChanges,
 })
 if err!=nil{t.Fatal(err)}
 if body,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,action);
  err==nil||len(body)!=0{t.Fatal("unbound PHP session was admitted")}
 encrypted,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,action)
 if err!=nil{t.Fatalf("E2EE read refused: %v",err)}
 result,err:=remotetransport.OpenRemoteGitPullChanges(client,encrypted,remoteDispatchTestID)
 if err!=nil||result.Head!=head||result.UpstreamHead!=head||len(result.Files)!=0{
  t.Fatalf("authenticated file inventory mismatch: %+v %v",result,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_pull_changes",Limit:5,
 })
 if err!=nil||len(audits)==0||audits[0].UserID==nil||
  *audits[0].UserID!=session.userID||audits[0].ResourceType!="git"{
  t.Fatalf("missing TaskDeck-local principal/audit: %+v %v",audits,err)
 }
}
