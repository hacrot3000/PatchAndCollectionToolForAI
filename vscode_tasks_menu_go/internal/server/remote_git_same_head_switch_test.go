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

func cleanSwitchFixture(t *testing.T,root string)string{
 t.Helper()
 _,head,_:=pinnedCommitFixture(t,root)
 pinnedCommitGit(t,root,"reset","--hard","HEAD")
 // The authenticated TaskDeck fixture owns a visible .vscode/tasks.json.
 // Commit it as fixture data so the test actually exercises a clean
 // workspace; never weaken host dirty-tree refusal to make tests pass.
 if _,err:=os.Stat(filepath.Join(root,".vscode","tasks.json"));err==nil{
  pinnedCommitGit(t,root,"add","--",".vscode/tasks.json")
  pinnedCommitGit(t,root,"-c","user.name=TaskDeck CI",
   "-c","user.email=ci@example.invalid","commit","-qm","test TaskDeck workspace")
  head=string(pinnedCommitGit(t,root,"rev-parse","HEAD"))
 }
 pinnedCommitGit(t,root,"branch","taskdeck/safe-target")
 return head
}
func switchReviewDigest(t *testing.T,root string)string{
 t.Helper()
 b,err:=remoteGitSameHeadSwitchReview(context.Background(),root,"safe-target")
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(b)
 return hex.EncodeToString(sum[:])
}
func TestGitSameHeadSwitchOnlyMutatesSymbolicHEADAndNoHooks(t *testing.T){
 root:=t.TempDir()
 oid:=cleanSwitchFixture(t,root)
 marker:=filepath.Join(root,"UNTRUSTED_HOOK")
 for _,name:=range []string{"post-checkout","reference-transaction"}{
  if err:=os.WriteFile(filepath.Join(root,".git","hooks",name),
   []byte("#!/bin/sh\ntouch '"+marker+"'\n"),0700);err!=nil{t.Fatal(err)}
 }
 indexBefore,err:=os.ReadFile(filepath.Join(root,".git","index"))
 if err!=nil{t.Fatal(err)}
 dataBefore,err:=os.ReadFile(filepath.Join(root,"code.txt"))
 if err!=nil{t.Fatal(err)}
 targetBefore,err:=os.ReadFile(filepath.Join(root,".git","refs","heads","taskdeck","safe-target"))
 if err!=nil{t.Fatal(err)}
 hash:=switchReviewDigest(t,root)
 result,err:=remoteGitSwitchSameHead(context.Background(),root,"safe-target",hash)
 if err!=nil||result!=oid{t.Fatalf("safe switch denied %q: %v",result,err)}
 afterBranch:=string(pinnedCommitGit(t,root,"symbolic-ref","HEAD"))
 if afterBranch!="refs/heads/taskdeck/safe-target"{
  t.Fatalf("wrong HEAD symbolic-ref: %s",afterBranch)
 }
 if got:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"));got!=oid{
  t.Fatal("switch altered commit tree")
 }
 indexAfter,_:=os.ReadFile(filepath.Join(root,".git","index"))
 fileAfter,_:=os.ReadFile(filepath.Join(root,"code.txt"))
 targetAfter,_:=os.ReadFile(filepath.Join(root,".git","refs","heads","taskdeck","safe-target"))
 if !bytes.Equal(indexBefore,indexAfter)||!bytes.Equal(dataBefore,fileAfter)||
  !bytes.Equal(targetBefore,targetAfter){t.Fatal("index, source or target ref mutated")}
 if _,err:=os.Stat(marker);!os.IsNotExist(err){t.Fatal("untrusted Git hook executed")}
 for _,lock:=range []string{"HEAD.lock","refs/heads/taskdeck/safe-target.lock"}{
  if _,err:=os.Stat(filepath.Join(root,".git",lock));!os.IsNotExist(err){
   t.Fatalf("left behind %s",lock)
  }
 }
 if _,err:=remoteGitSwitchSameHead(context.Background(),root,"safe-target",hash);err==nil{
  t.Fatal("stale review reused after branch switch")
 }
}
func TestGitSameHeadSwitchRejectsDirtyOrDifferentTree(t *testing.T){
 for _,kind:=range []string{"staged","working-tree","untracked","different-target","missing-target"}{
  t.Run(kind,func(t *testing.T){
   root:=t.TempDir()
   old:=cleanSwitchFixture(t,root)
   switch kind{
   case "staged":
    if err:=os.WriteFile(filepath.Join(root,"code.txt"),[]byte("staged\n"),0600);err!=nil{t.Fatal(err)}
    pinnedCommitGit(t,root,"add","code.txt")
   case "working-tree":
    if err:=os.WriteFile(filepath.Join(root,"code.txt"),[]byte("dirty\n"),0600);err!=nil{t.Fatal(err)}
   case "untracked":
    if err:=os.WriteFile(filepath.Join(root,"untracked.txt"),[]byte("private\n"),0600);err!=nil{t.Fatal(err)}
   case "different-target":
    pinnedCommitGit(t,root,"-c","user.name=TaskDeck CI","-c","user.email=ci@example.invalid",
     "commit","--allow-empty","-qm","other commit")
    pinnedCommitGit(t,root,"branch","-f","taskdeck/safe-target","HEAD")
    pinnedCommitGit(t,root,"reset","--hard",old)
   case "missing-target":
    pinnedCommitGit(t,root,"branch","-D","taskdeck/safe-target")
   }
   if _,err:=remoteGitSameHeadSwitchReview(context.Background(),root,"safe-target");err==nil{
    t.Fatalf("unsafe %s state accepted as clean same-HEAD",kind)
   }
   if got:=string(pinnedCommitGit(t,root,"symbolic-ref","HEAD"));strings.HasPrefix(got,"refs/heads/taskdeck/"){
    t.Fatal("unsafe switch changed branch")
   }
  })
 }
}
func TestGitSameHeadSwitchRejectsLocksAndSymlinks(t *testing.T){
 for _,kind:=range []string{"head.lock","target.lock","target.symlink"}{
  t.Run(kind,func(t *testing.T){
   root:=t.TempDir()
   cleanSwitchFixture(t,root)
   hash:=switchReviewDigest(t,root)
   var lock string
   switch kind{
   case "head.lock":
    lock=filepath.Join(root,".git","HEAD.lock")
   case "target.lock":
    lock=filepath.Join(root,".git","refs","heads","taskdeck","safe-target.lock")
   case "target.symlink":
    target:=filepath.Join(root,".git","refs","heads","taskdeck","safe-target")
    if err:=os.Remove(target);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(filepath.Join(t.TempDir(),"external"),target);err!=nil{t.Fatal(err)}
   }
   if lock!=""{
    if err:=os.WriteFile(lock,[]byte("external writer"),0600);err!=nil{t.Fatal(err)}
   }
   if _,err:=remoteGitSwitchSameHead(context.Background(),root,"safe-target",hash);err==nil{
    t.Fatal("unsafe Git lock/ref accepted")
   }
   if lock!=""{
    data,err:=os.ReadFile(lock)
    if err!=nil||string(data)!="external writer"{t.Fatal("overwrote or removed outside lock")}
   }
  })
 }
}
func TestGitSameHeadSwitchNeedsHostGatesPrincipalAndRBAC(t *testing.T){
 for _,kind:=range []string{"off","gitwrite-off","viewer","php-alone","admin"}{
  t.Run(kind,func(t *testing.T){
   role:=remotetransport.RoleAdmin
   if kind=="viewer"{role=remotetransport.RoleViewer}
   s,session,client,host:=remoteDispatchFixtureWithRole(t,role)
   oid:=cleanSwitchFixture(t,s.Workspace)
   hash:=switchReviewDigest(t,s.Workspace)
   t.Setenv("TASKDECK_REMOTE_GIT_WRITE","1")
   t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_SAME_HEAD","1")
   if kind=="off"{t.Setenv("TASKDECK_REMOTE_GIT_SWITCH_SAME_HEAD","")}
   if kind=="gitwrite-off"{t.Setenv("TASKDECK_REMOTE_GIT_WRITE","")}
   if kind=="php-alone"{session=nil}
   req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitSwitchSameHead,
    Path:"safe-target",ExpectedSHA256:hash})
   if err!=nil{t.Fatal(err)}
   reply,err:=s.executeRemoteE2EEPing(context.Background(),session,host,req)
   if kind=="admin"{
    if err!=nil{t.Fatal(err)}
    result,err:=remotetransport.OpenRemoteGitSwitchResult(client,reply,remoteDispatchTestID)
    if err!=nil||result!=oid{t.Fatalf("bad authenticated switch result %q %v",result,err)}
   }else{
    if err==nil||len(reply)>0{t.Fatalf("%s bypassed host gates",kind)}
    if got:=string(pinnedCommitGit(t,s.Workspace,"symbolic-ref","HEAD"));strings.HasPrefix(got,"refs/heads/taskdeck/"){
     t.Fatal("unauthorized request changed HEAD")
    }
   }
  })
 }
}
