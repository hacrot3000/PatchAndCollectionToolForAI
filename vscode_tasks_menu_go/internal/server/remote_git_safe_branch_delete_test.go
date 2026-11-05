package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func safeDeleteReviewDigest(t *testing.T,root string)string{
 t.Helper()
 review,err:=remoteGitSafeBranchDeleteReview(context.Background(),root,"safe-target")
 if err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(review)
 return hex.EncodeToString(digest[:])
}

func TestGitSafeBranchDeleteKeepsHEADIndexAndWorktree(t *testing.T){
 root:=t.TempDir()
 oid:=cleanSwitchFixture(t,root)
 indexBefore,err:=os.ReadFile(filepath.Join(root,".git","index"))
 if err!=nil{t.Fatal(err)}
 workBefore,err:=os.ReadFile(filepath.Join(root,"code.txt"))
 if err!=nil{t.Fatal(err)}
 marker:=filepath.Join(root,"HOOK_MARKER")
 if err:=os.WriteFile(filepath.Join(root,".git","hooks","reference-transaction"),
  []byte("#!/bin/sh\ntouch '"+marker+"'\n"),0700);err!=nil{t.Fatal(err)}
 hash:=safeDeleteReviewDigest(t,root)
 result,err:=remoteGitSafeBranchDelete(context.Background(),root,"safe-target",hash)
 if err!=nil||result!="deleted"{t.Fatalf("authenticated same-HEAD delete: %q %v",result,err)}
 if _,err:=os.Lstat(filepath.Join(root,".git","refs","heads","taskdeck","safe-target"));!os.IsNotExist(err){
  t.Fatal("target ref survived delete")
 }
 if got:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"));got!=oid{
  t.Fatal("HEAD commit changed")
 }
 if b,_:=os.ReadFile(filepath.Join(root,".git","index"));!bytes.Equal(b,indexBefore){
  t.Fatal("Git index mutated")
 }
 if b,_:=os.ReadFile(filepath.Join(root,"code.txt"));!bytes.Equal(b,workBefore){
  t.Fatal("source mutated")
 }
 if _,err:=os.Stat(marker);!os.IsNotExist(err){t.Fatal("untrusted Git hook executed")}
 if _,err:=os.Lstat(filepath.Join(root,".git","HEAD.lock"));!os.IsNotExist(err){
  t.Fatal("HEAD.lock leaked")
 }
 if _,err:=remoteGitSafeBranchDelete(context.Background(),root,"safe-target",hash);err==nil{
  t.Fatal("replayed delete accepted")
 }
}

func TestGitSafeBranchDeleteRefusesUnsafeRepositoryStates(t *testing.T){
 for _,kind:=range []string{"dirty","staged","missing","different-commit","current","linked-worktree","head-lock","ref-lock","reflog-symlink","stale-review"}{
  t.Run(kind,func(t *testing.T){
   root:=t.TempDir()
   cleanSwitchFixture(t,root)
   hash:=safeDeleteReviewDigest(t,root)
   gitDir:=filepath.Join(root,".git")
   switch kind{
   case "dirty":
    if err:=os.WriteFile(filepath.Join(root,"code.txt"),[]byte("dirty\n"),0600);err!=nil{t.Fatal(err)}
   case "staged":
    if err:=os.WriteFile(filepath.Join(root,"code.txt"),[]byte("dirty\n"),0600);err!=nil{t.Fatal(err)}
    pinnedCommitGit(t,root,"add","code.txt")
   case "missing":
    pinnedCommitGit(t,root,"branch","-D","taskdeck/safe-target")
   case "different-commit":
    pinnedCommitGit(t,root,"-c","user.name=TaskDeck CI","-c","user.email=ci@example.invalid",
     "commit","--allow-empty","-qm","other commit")
   case "current":
    pinnedCommitGit(t,root,"checkout","taskdeck/safe-target")
   case "linked-worktree":
    if err:=os.Mkdir(filepath.Join(gitDir,"worktrees"),0700);err!=nil{t.Fatal(err)}
   case "head-lock":
    if err:=os.WriteFile(filepath.Join(gitDir,"HEAD.lock"),[]byte("external"),0600);err!=nil{t.Fatal(err)}
   case "ref-lock":
    if err:=os.WriteFile(filepath.Join(gitDir,"refs","heads","taskdeck","safe-target.lock"),[]byte("external"),0600);err!=nil{t.Fatal(err)}
   case "reflog-symlink":
    leaf:=filepath.Join(gitDir,"logs","refs","heads","taskdeck","safe-target")
    if err:=os.MkdirAll(filepath.Dir(leaf),0700);err!=nil{t.Fatal(err)}
    _=os.Remove(leaf)
    if err:=os.Symlink(filepath.Join(t.TempDir(),"outside"),leaf);err!=nil{t.Fatal(err)}
   case "stale-review":
    hash=strings.Repeat("f",64)
   }
   if _,err:=remoteGitSafeBranchDelete(context.Background(),root,"safe-target",hash);err==nil{
    t.Fatalf("unsafe %s state deleted branch",kind)
   }
   if kind!="missing"{
    if _,err:=os.Lstat(filepath.Join(gitDir,"refs","heads","taskdeck","safe-target"));err!=nil{
     t.Fatalf("%s removed target despite denial: %v",kind,err)
    }
   }
  })
 }
}

func TestGitSafeBranchDeleteEnforcesHostGatesAndTaskDeckPrincipal(t *testing.T){
 for _,kind:=range []string{"flag-off","write-off","viewer","php-only","admin"}{
  t.Run(kind,func(t *testing.T){
   role:=remotetransport.RoleAdmin
   if kind=="viewer"{role=remotetransport.RoleViewer}
   s,session,client,host:=remoteDispatchFixtureWithRole(t,role)
   cleanSwitchFixture(t,s.Workspace)
   reviewSHA:=safeDeleteReviewDigest(t,s.Workspace)
   t.Setenv("TASKDECK_REMOTE_GIT_WRITE","1")
   t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE","1")
   if kind=="flag-off"{t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_DELETE","")}
   if kind=="write-off"{t.Setenv("TASKDECK_REMOTE_GIT_WRITE","")}
   if kind=="php-only"{session=nil}
   request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitBranchDelete,
    Path:"safe-target",ExpectedSHA256:reviewSHA})
   if err!=nil{t.Fatal(err)}
   reply,err:=s.executeRemoteE2EEPing(context.Background(),session,host,request)
   if kind=="admin"{
    if err!=nil{t.Fatal(err)}
    result,err:=remotetransport.OpenRemoteGitBranchDeleteResult(client,reply,remoteDispatchTestID)
    if err!=nil||result!="deleted"{t.Fatalf("wrong reply: %q %v",result,err)}
   }else{
    if err==nil||len(reply)>0{t.Fatal("unauthorized delete accepted")}
    if _,err:=os.Lstat(filepath.Join(s.Workspace,".git","refs","heads","taskdeck","safe-target"));err!=nil{
     t.Fatal("unprivileged principal mutated branch")
    }
   }
  })
 }
}
