package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func branchReviewSHA(t *testing.T,root string)string{
 t.Helper()
 preview,err:=remoteGitBranchReview(context.Background(),root)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(preview)
 return hex.EncodeToString(sum[:])
}

func TestGitBranchCreateOnlyCreatesNewNamespacedRefNoHooks(t *testing.T){
 root:=t.TempDir()
 file,head,index:=pinnedCommitFixture(t,root)
 marker:=filepath.Join(root,"HOOK_EXECUTED")
 for _,name:=range []string{"reference-transaction","post-checkout","post-commit"}{
  path:=filepath.Join(root,".git","hooks",name)
  if err:=os.WriteFile(path,[]byte("#!/bin/sh\ntouch '"+marker+"'\n"),0700);err!=nil{t.Fatal(err)}
 }
 review:=branchReviewSHA(t,root)
 got,err:=remoteGitBranchCreate(context.Background(),root,"safe-feature1",review)
 if err!=nil||got!=head{t.Fatalf("branch create %q %v",got,err)}
 branch:=string(pinnedCommitGit(t,root,"rev-parse","refs/heads/taskdeck/safe-feature1"))
 if branch!=head{t.Fatalf("branch ref points elsewhere: %s",branch)}
 if actual:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"));actual!=head{
  t.Fatal("current HEAD changed during branch creation")
 }
 if current,err:=os.ReadFile(filepath.Join(root,".git","index"));err!=nil||!bytes.Equal(current,index){
  t.Fatal("Git index was modified")
 }
 if contents,err:=os.ReadFile(file);err!=nil||string(contents)!="working tree\n"{
  t.Fatal("working tree was changed")
 }
 if _,err:=os.Stat(marker);!os.IsNotExist(err){t.Fatal("untrusted Git hook executed")}
 if _,err:=remoteGitBranchCreate(context.Background(),root,"safe-feature1",review);err==nil{
  t.Fatal("existing branch overwritten")
 }
 if actual:=string(pinnedCommitGit(t,root,"rev-parse","refs/heads/taskdeck/safe-feature1"));actual!=head{
  t.Fatal("branch was overwritten after denial")
 }
}

func TestGitBranchCreateRejectsStaleReviewAndUnsafeRefParents(t *testing.T){
 t.Run("stale HEAD",func(t *testing.T){
  root:=t.TempDir()
  _,_,_=pinnedCommitFixture(t,root)
  review:=branchReviewSHA(t,root)
  pinnedCommitGit(t,root,"-c","user.name=TaskDeck CI",
   "-c","user.email=ci@example.invalid","commit","-am","next HEAD")
  if _,err:=remoteGitBranchCreate(context.Background(),root,"stale-name",review);err==nil{
   t.Fatal("stale reviewed HEAD accepted")
  }
  if _,err:=os.Stat(filepath.Join(root,".git","refs","heads","taskdeck","stale-name"));!os.IsNotExist(err){
   t.Fatal("stale request created branch")
  }
 })
 for _,kind:=range []string{"refs-taskdeck-symlink","logs-symlink","alternates","packed-refs-symlink"}{
  t.Run(kind,func(t *testing.T){
   root:=t.TempDir()
   _,old,_:=pinnedCommitFixture(t,root)
   review:=branchReviewSHA(t,root)
   gitDir:=filepath.Join(root,".git")
   switch kind {
   case "refs-taskdeck-symlink":
    if err:=os.Symlink(t.TempDir(),filepath.Join(gitDir,"refs","heads","taskdeck"));err!=nil{t.Fatal(err)}
   case "logs-symlink":
    orig:=filepath.Join(gitDir,"logs")
    if err:=os.RemoveAll(orig);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(t.TempDir(),orig);err!=nil{t.Fatal(err)}
   case "alternates":
    p:=filepath.Join(gitDir,"objects","info","alternates")
    if err:=os.MkdirAll(filepath.Dir(p),0700);err!=nil{t.Fatal(err)}
    if err:=os.WriteFile(p,[]byte(t.TempDir()+"\n"),0600);err!=nil{t.Fatal(err)}
   case "packed-refs-symlink":
    p:=filepath.Join(gitDir,"packed-refs")
    if err:=os.Symlink(filepath.Join(t.TempDir(),"pack"),p);err!=nil{t.Fatal(err)}
   }
   if _,err:=remoteGitBranchCreate(context.Background(),root,"safe-create",review);err==nil{
    t.Fatal("unsafe Git ref/log/object path accepted")
   }
   if string(pinnedCommitGit(t,root,"rev-parse","HEAD"))!=old{t.Fatal("HEAD unexpectedly changed")}
  })
 }
}

func TestGitBranchCreateRequiresHostGatesTaskDeckRBACAndNotPHPAlone(t *testing.T){
 for _,tc:=range []string{"disabled","write_gate_off","viewer","php_alone","admin"}{
  t.Run(tc,func(t *testing.T){
   role:=remotetransport.RoleAdmin
   if tc=="viewer"{role=remotetransport.RoleViewer}
   s,session,client,agent:=remoteDispatchFixtureWithRole(t,role)
   _,old,_:=pinnedCommitFixture(t,s.Workspace)
   t.Setenv("TASKDECK_REMOTE_GIT_WRITE","1")
   t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE","1")
   if tc=="disabled"{t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE","")}
   if tc=="write_gate_off"{t.Setenv("TASKDECK_REMOTE_GIT_WRITE","")}
   review:=branchReviewSHA(t,s.Workspace)
   request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitBranchCreate,
    Path:"safe-new",ExpectedSHA256:review,
   })
   if err!=nil{t.Fatal(err)}
   if tc=="php_alone"{session=nil}
   reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
   if tc=="admin"{
    if err!=nil{t.Fatalf("host-authorized branch denied: %v",err)}
    got,err:=remotetransport.OpenRemoteGitBranchCreateResult(client,reply,remoteDispatchTestID)
    if err!=nil||got!=old{t.Fatalf("wrong authenticated branch OID: %q %v",got,err)}
    if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil{
     t.Fatal("replayed action accepted")
    }
   }else{
    if err==nil||len(reply)!=0{t.Fatalf("%s bypassed Git gates",tc)}
    if _,err:=os.Stat(filepath.Join(s.Workspace,".git","refs","heads","taskdeck","safe-new"));!os.IsNotExist(err){
     t.Fatalf("%s unexpectedly created Git ref",tc)
    }
   }
  })
 }
}

func TestGitBranchCreateRejectsSymbolicAndPackedExistingRefs(t *testing.T){
 for _,kind:=range []string{"loose-symlink","symbolic-ref","packed-ref"}{
  t.Run(kind,func(t *testing.T){
   root:=t.TempDir()
   _,old,_:=pinnedCommitFixture(t,root)
   approved:=branchReviewSHA(t,root)
   ref:="refs/heads/taskdeck/blocked-name"
   leaf:=filepath.Join(root,".git","refs","heads","taskdeck","blocked-name")
   if err:=os.MkdirAll(filepath.Dir(leaf),0700);err!=nil{t.Fatal(err)}
   switch kind{
   case "loose-symlink":
    if err:=os.Symlink(filepath.Join(t.TempDir(),"outside"),leaf);err!=nil{t.Fatal(err)}
   case "symbolic-ref":
    pinnedCommitGit(t,root,"symbolic-ref",ref,"refs/heads/main")
   case "packed-ref":
    pinnedCommitGit(t,root,"update-ref",ref,old)
    pinnedCommitGit(t,root,"pack-refs","--all")
    if _,err:=os.Lstat(leaf);err==nil{t.Fatal("fixture not packed")}
   }
   if _,err:=remoteGitBranchCreate(context.Background(),root,"blocked-name",approved);err==nil{
    t.Fatalf("existing %s branch target was accepted",kind)
   }
   if head:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"));head!=old{
    t.Fatal("untrusted existing ref redirected HEAD")
   }
  })
 }
}
