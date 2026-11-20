package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func setupRemoteGitBranchDelete(t *testing.T)(*Server,*remoteTaskDeckSession,func(...string)string,string,string){
 t.Helper()
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only safe branch delete")}
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 git:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 git("config","user.name","TaskDeck Test")
 git("config","user.email","test@example.invalid")
 file:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(file,[]byte("committed\n"),0600);err!=nil{t.Fatal(err)}
 git("add","tracked.txt",".vscode/tasks.json")
 git("commit","-m","Initial")
 current:=git("symbolic-ref","--short","HEAD")
 head:=git("rev-parse","HEAD")
 git("branch","taskdeck/safe-target")
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE_NAME","safe-target")
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE","1")
 return s,session,git,current,head
}
func TestRemoteGitBranchDeleteRequiresPreviewAndAudit(t *testing.T){
 s,session,git,current,head:=setupRemoteGitBranchDelete(t)
 leaf:=filepath.Join(s.Workspace,".git","refs","heads","taskdeck","safe-target")
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE","0")
 if _,err:=s.readRemoteGitBranchDeleteReview(context.Background(),session);err==nil{
  t.Fatal("read-only deletion preview bypassed opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE","1")
 if _,err:=s.executeRemoteGitBranchDelete(context.Background(),session);err==nil{
  t.Fatal("unreviewed deletion accepted")
 }
 preview,err:=s.readRemoteGitBranchDeleteReview(context.Background(),session)
 if err!=nil||preview.Current!=current||preview.Head!=head||
  preview.Branch!="taskdeck/safe-target"{
  t.Fatalf("unexpected read-only review: %+v %v",preview,err)
 }
 result,err:=s.executeRemoteGitBranchDelete(context.Background(),session)
 if err!=nil||result!=preview{t.Fatalf("reviewed deletion refused: %+v %v",result,err)}
 if _,err:=os.Lstat(leaf);!os.IsNotExist(err){
  t.Fatal("reviewed ref was not deleted")
 }
 if got:=git("rev-parse","HEAD");got!=head{t.Fatal("delete altered HEAD commit")}
 if got:=git("symbolic-ref","--short","HEAD");got!=current{
  t.Fatal("delete changed checked out branch")
 }
 if body,err:=os.ReadFile(filepath.Join(s.Workspace,"tracked.txt"));err!=nil||
  string(body)!="committed\n"{t.Fatalf("delete altered working tree: %q %v",body,err)}
 if _,err:=s.executeRemoteGitBranchDelete(context.Background(),session);err==nil{
  t.Fatal("deleted target replayed without a new review")
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_branch_delete",Limit:10,
 })
 if err!=nil||len(events)!=1||events[0].UserID==nil||
  *events[0].UserID!=session.userID||events[0].Result!="attempted"||
  strings.Contains(events[0].Details,"safe-target"){
  t.Fatalf("private Git delete audit: %+v %v",events,err)
 }
}
func TestRemoteGitBranchDeleteRefusesUnsafeStates(t *testing.T){
 for _,kind:=range []string{
  "untracked","staged","other-tip","current","linked-worktree","head-lock",
  "reflog-symlink","ref-parent-symlink","reflog-parent-symlink",
  "target-lock","changed-host-target",
 }{
  t.Run(kind,func(t *testing.T){
   s,session,git,current,head:=setupRemoteGitBranchDelete(t)
   _,err:=s.readRemoteGitBranchDeleteReview(context.Background(),session)
   if err!=nil{t.Fatal(err)}
   gitDir:=filepath.Join(s.Workspace,".git")
   switch kind{
   case "untracked":
    if err:=os.WriteFile(filepath.Join(s.Workspace,"untracked"),[]byte("x"),0600);err!=nil{t.Fatal(err)}
   case "staged":
    if err:=os.WriteFile(filepath.Join(s.Workspace,"tracked.txt"),[]byte("stage"),0600);err!=nil{t.Fatal(err)}
    git("add","tracked.txt")
   case "other-tip":
    git("commit","--allow-empty","-m","HEAD moved")
   case "current":
    git("switch","taskdeck/safe-target")
   case "linked-worktree":
    if err:=os.Mkdir(filepath.Join(gitDir,"worktrees"),0700);err!=nil{t.Fatal(err)}
   case "head-lock":
    if err:=os.WriteFile(filepath.Join(gitDir,"HEAD.lock"),[]byte("busy"),0600);err!=nil{t.Fatal(err)}
   case "reflog-symlink":
    leaf:=filepath.Join(gitDir,"logs","refs","heads","taskdeck","safe-target")
    _=os.Remove(leaf)
    if err:=os.Symlink(filepath.Join(t.TempDir(),"outside"),leaf);err!=nil{t.Fatal(err)}
   case "ref-parent-symlink":
    dir:=filepath.Join(gitDir,"refs","heads","taskdeck")
    moved:=filepath.Join(gitDir,"refs","heads","taskdeck-original")
    if err:=os.Rename(dir,moved);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(moved,dir);err!=nil{t.Fatal(err)}
   case "reflog-parent-symlink":
    dir:=filepath.Join(gitDir,"logs","refs","heads","taskdeck")
    moved:=filepath.Join(gitDir,"logs","refs","heads","taskdeck-original")
    if err:=os.Rename(dir,moved);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(moved,dir);err!=nil{t.Fatal(err)}
   case "target-lock":
    if err:=os.WriteFile(filepath.Join(gitDir,"refs","heads","taskdeck","safe-target.lock"),
      []byte("busy"),0600);err!=nil{t.Fatal(err)}
   case "changed-host-target":
    git("branch","taskdeck/other")
    t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE_NAME","other")
   }
   if _,err:=s.executeRemoteGitBranchDelete(context.Background(),session);err==nil{
    t.Fatalf("unsafe %s state deleted branch",kind)
   }
   if _,err:=os.Lstat(filepath.Join(gitDir,"refs","heads","taskdeck","safe-target"));err!=nil{
    t.Fatalf("unsafe %s state removed original target: %v",kind,err)
   }
   if kind!="current"&&kind!="other-tip"&&kind!="head-lock"{
    if git("rev-parse","HEAD")!=head||git("symbolic-ref","--short","HEAD")!=current{
     t.Fatalf("unsafe %s changed HEAD",kind)
    }
   }
  })
 }
}
func TestRemoteGitBranchDeleteViewerCannotMutate(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only safe delete")}
 gitInitRemoteTest(t,s.Workspace)
 git:=func(args ...string){
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v: %v %s",args,err,out)}
 }
 git("config","user.name","TaskDeck Viewer")
 git("config","user.email","test@example.invalid")
 git("add",".vscode/tasks.json")
 git("commit","-m","initial")
 git("branch","taskdeck/safe-target")
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE","1")
 t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE_NAME","safe-target")
 preview,err:=s.readRemoteGitBranchDeleteReview(context.Background(),session)
 if err!=nil||preview.Branch!="taskdeck/safe-target"{
  t.Fatalf("viewer could not read authorized preview: %+v %v",preview,err)
 }
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitBranchDelete,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err==nil||len(reply)!=0{t.Fatal("viewer escalated to destructive git.write")}
 leaf:=filepath.Join(s.Workspace,".git","refs","heads","taskdeck","safe-target")
 if _,err:=os.Lstat(leaf);err!=nil{t.Fatalf("viewer removed existing reviewed ref: %v",err)}
}

func TestRemoteGitBranchDeletePinsGitWorktreeConfig(t *testing.T){
 s,session,git,_,_:=setupRemoteGitBranchDelete(t)
 if _,err:=s.readRemoteGitBranchDeleteReview(context.Background(),session);err!=nil{
  t.Fatalf("valid before config change: %v",err)
 }
 // A malicious checked-in or externally edited repository config may
 // redirect git's worktree even when TaskDeck root has an ordinary .git.
 // The Agent must refuse, never operate in the redirected worktree.
 outside:=t.TempDir()
 git("config","core.worktree",outside)
 if _,err:=s.executeRemoteGitBranchDelete(context.Background(),session);err==nil{
  t.Fatal("ref deletion followed configured worktree outside workspace")
 }
 leaf:=filepath.Join(s.Workspace,".git","refs","heads","taskdeck","safe-target")
 if _,err:=os.Lstat(leaf);err!=nil{
  t.Fatalf("changed core.worktree deleted ref: %v",err)
 }
}
