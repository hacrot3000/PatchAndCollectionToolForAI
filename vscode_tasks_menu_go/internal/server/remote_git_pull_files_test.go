package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func pullFilesReviewDigest(t *testing.T,root string)string{
 t.Helper()
 reviewed,err:=remoteGitPullChangesReview(context.Background(),root)
 if err!=nil||!remotetransport.ValidRemoteGitPullChangesReport(string(reviewed)){
  t.Fatalf("file-changing FF review failed: %q %v",reviewed,err)
 }
 digest:=sha256.Sum256(reviewed)
 return hex.EncodeToString(digest[:])
}

func TestRemoteGitPullFilesFFUpdatesOnlyApprovedRegularFiles(t *testing.T){
 root,old,target:=remotePullChangesFixture(t)
 expected:=pullFilesReviewDigest(t,root)
 marker:=filepath.Join(root,"MALICIOUS_HOOK_EXECUTED")
 if err:=os.WriteFile(filepath.Join(root,".git","hooks","post-merge"),
  []byte("#!/bin/sh\ntouch '"+marker+"'\n"),0700);err!=nil{t.Fatal(err)}
 updated,err:=remoteGitPullApplyFiles(context.Background(),root,expected)
 if err!=nil||updated!=target{t.Fatalf("clean file FF denied: %q %v",updated,err)}
 if head:=remotePullTestGit(t,root,"rev-parse","HEAD");head!=target||head==old{
  t.Fatal("approved file FF failed to advance expected branch")
 }
 changed,err:=os.ReadFile(filepath.Join(root,"a.txt"))
 if err!=nil||string(changed)!="second\n"{t.Fatal("tracked file did not update")}
 added,err:=os.ReadFile(filepath.Join(root,"b.txt"))
 if err!=nil||string(added)!="new\n"{t.Fatal("new tracked file was not created")}
 if status:=remotePullTestGit(t,root,"status","--porcelain=v1","--untracked-files=all");
  status!=""{t.Fatalf("fast-forward left an unexpected dirty tree: %q",status)}
 if _,err:=os.Stat(marker);!os.IsNotExist(err){t.Fatal("post-merge hook executed")}
 if _,err:=remoteGitPullApplyFiles(context.Background(),root,expected);err==nil{
  t.Fatal("reusing approved review digest re-applied a file-changing Pull")
 }
}

func TestRemoteGitPullFilesFFRefusesDirtyStaleAndForeignLocks(t *testing.T){
 for _,kind:=range []string{"dirty","stale","index-lock","head-lock","wrong-digest"}{
  t.Run(kind,func(t *testing.T){
   root,old,target:=remotePullChangesFixture(t)
   digest:=pullFilesReviewDigest(t,root)
   var lockPath string
   switch kind{
   case "dirty":
    if err:=os.WriteFile(filepath.Join(root,"a.txt"),[]byte("local edit\n"),0600);err!=nil{t.Fatal(err)}
   case "stale":
    remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",old,target)
   case "index-lock","head-lock":
    lockPath=filepath.Join(root,".git",map[string]string{
     "index-lock":"index.lock","head-lock":"HEAD.lock",
    }[kind])
    if err:=os.WriteFile(lockPath,[]byte("foreign writer"),0600);err!=nil{t.Fatal(err)}
   case "wrong-digest":
    digest=strings.Repeat("a",64)
   }
   changed,err:=remoteGitPullApplyFiles(context.Background(),root,digest)
   if err==nil||changed!=""{t.Fatal("unsafe file-changing FF unexpectedly succeeded")}
   if head:=remotePullTestGit(t,root,"rev-parse","HEAD");head!=old{
    t.Fatal("denied Pull moved current HEAD")
   }
   if lockPath!=""{
    data,e:=os.ReadFile(lockPath)
    if e!=nil||!bytes.Equal(data,[]byte("foreign writer")){
     t.Fatal("foreign Git writer lock was removed or modified")
    }
   }
  })
 }
}

func TestRemoteGitPullFilesFFRefusesSymlinkedMetadataAndUnsafeFile(t *testing.T){
 for _,kind:=range []string{"orig-head","branch-log","worktree-symlink"}{
  t.Run(kind,func(t *testing.T){
   root,old,_:=remotePullChangesFixture(t)
   digest:=pullFilesReviewDigest(t,root)
   outside:=filepath.Join(t.TempDir(),"external")
   if err:=os.WriteFile(outside,[]byte("untouched"),0600);err!=nil{t.Fatal(err)}
   if kind=="worktree-symlink"{
    if err:=os.Remove(filepath.Join(root,"a.txt"));err!=nil{t.Fatal(err)}
    if err:=os.Symlink(outside,filepath.Join(root,"a.txt"));err!=nil{t.Fatal(err)}
   }else{
    leaf:=filepath.Join(root,".git","ORIG_HEAD")
    if kind=="branch-log"{leaf=filepath.Join(root,".git","logs","refs","heads","main")}
    if _,err:=os.Lstat(leaf);err==nil{
     if err:=os.Remove(leaf);err!=nil{t.Fatal(err)}
    }else if !os.IsNotExist(err){t.Fatal(err)}
    if err:=os.Symlink(outside,leaf);err!=nil{t.Fatal(err)}
   }
   updated,err:=remoteGitPullApplyFiles(context.Background(),root,digest)
   if err==nil||updated!=""{t.Fatal("Pull followed a symlinked write destination")}
   if content,err:=os.ReadFile(outside);err!=nil||string(content)!="untouched"{
    t.Fatal("Pull wrote outside the host-approved worktree")
   }
   if head:=remotePullTestGit(t,root,"rev-parse","HEAD");head!=old{
    t.Fatal("unsafe Pull advanced HEAD")
   }
  })
 }
}

func TestRemoteGitPullFilesFFRefusesExecutableFilters(t *testing.T){
 root,old,_:=remotePullChangesFixture(t)
 digest:=pullFilesReviewDigest(t,root)
 remotePullTestGit(t,root,"config","filter.malicious.clean","sh -c 'exit 99'")
 if changed,err:=remoteGitPullApplyFiles(context.Background(),root,digest);
  err==nil||changed!=""{t.Fatal("untrusted Git filter was permitted")}
 if current:=remotePullTestGit(t,root,"rev-parse","HEAD");current!=old{
  t.Fatal("unsafe Git filter nevertheless advanced branch")
 }
}

func TestRemoteGitPullFilesHostGateE2EEAndNoPrincipalBypass(t *testing.T){
 for _,tc:=range []struct{name,gitFlag,fileFlag string;allow bool}{
  {"off","","",false},
  {"git-write-only","1","",false},
  {"file-pull-only","","1",false},
  {"both-flags","1","1",true},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_GIT_WRITE",tc.gitFlag)
   t.Setenv("TASKDECK_REMOTE_GIT_PULL_FILES",tc.fileFlag)
   s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
   root:=s.Workspace
   init:=exec.Command("git","init","-q","-b","main",root)
   if out,err:=init.CombinedOutput();err!=nil{t.Fatalf("git init: %v %s",err,out)}
   remotePullTestGit(t,root,"add",".vscode/tasks.json")
   remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
    "-c","user.email=test@localhost.invalid","commit","-q","-m","baseline")
   old:=remotePullTestGit(t,root,"rev-parse","HEAD")
   remotePullTestGit(t,root,"remote","add","origin","https://unreachable.invalid/repository.git")
   remotePullTestGit(t,root,"config","branch.main.remote","origin")
   remotePullTestGit(t,root,"config","branch.main.merge","refs/heads/main")
   if err:=os.WriteFile(filepath.Join(root,"added.txt"),[]byte("upstream file"),0600);err!=nil{t.Fatal(err)}
   remotePullTestGit(t,root,"add","added.txt")
   remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
    "-c","user.email=test@localhost.invalid","commit","-q","-m","new upstream")
   target:=remotePullTestGit(t,root,"rev-parse","HEAD")
   remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",target)
   remotePullTestGit(t,root,"reset","--hard",old)
   readID:="0123456789abcdef0123456789abcdef"
   sealed,e:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:readID,Operation:remotetransport.RemoteActionGitPullChangesReview,
   })
   if e!=nil{t.Fatal(e)}
   if result,e:=s.executeRemoteE2EEPing(context.Background(),nil,agent,sealed);
    e==nil||len(result)!=0{t.Fatal("untrusted PHP bypassed TaskDeck principal")}
   preview,e:=s.executeRemoteE2EEPing(context.Background(),owner,agent,sealed)
   if e!=nil{t.Fatalf("valid authenticated preview denied: %v",e)}
   report,e:=remotetransport.OpenRemoteGitPullChangesResult(client,preview,readID)
   if e!=nil{t.Fatal(e)}
   h:=sha256.Sum256([]byte(report))
   writeID:="1123456789abcdef0123456789abcdef"
   request,e:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:writeID,Operation:remotetransport.RemoteActionGitPullApplyFiles,
    ExpectedSHA256:hex.EncodeToString(h[:]),
   })
   if e!=nil{t.Fatal(e)}
   reply,e:=s.executeRemoteE2EEPing(context.Background(),owner,agent,request)
   if !tc.allow{
    if e==nil||len(reply)!=0{t.Fatal("disabled host flag permitted tree-changing Pull")}
    if got:=remotePullTestGit(t,root,"rev-parse","HEAD");got!=old{
     t.Fatal("host-disabled Pull still moved HEAD")
    }
    return
   }
   if e!=nil{t.Fatalf("host-gated reviewed Pull failed: %v",e)}
   actual,e:=remotetransport.OpenRemoteGitPullApplyFilesResult(client,reply,writeID)
   if e!=nil||actual!=target{t.Fatalf("wrong authenticated target: %q %v",actual,e)}
   if content,err:=os.ReadFile(filepath.Join(root,"added.txt"));err!=nil||
    string(content)!="upstream file"{t.Fatal("reviewed remote Pull did not update worktree")}
  })
 }
}


func TestRemoteGitPullFilesFFDeniesConfiguredForeignWorktree(t *testing.T){
 root,old,_:=remotePullChangesFixture(t)
 digest:=pullFilesReviewDigest(t,root)
 outside:=t.TempDir()
 sentinel:=filepath.Join(outside,"MUST_NOT_CHANGE.txt")
 if err:=os.WriteFile(sentinel,[]byte("untouched"),0600);err!=nil{t.Fatal(err)}
 remotePullTestGit(t,root,"config","core.worktree",outside)
 if remoteGitPullFilesWorktreeConfigSafe(context.Background(),root){
  t.Fatal("core.worktree redirection not detected")
 }
 if manifest,e:=remoteGitPullChangesReview(context.Background(),root);e==nil||len(manifest)!=0{
  t.Fatal("unsafe core.worktree admitted a read-only Pull manifest")
 }
 if updated,e:=remoteGitPullApplyFiles(context.Background(),root,digest);
  e==nil||updated!=""{t.Fatal("configured foreign worktree used for Pull")}
 if data,e:=os.ReadFile(sentinel);e!=nil||string(data)!="untouched"{
  t.Fatal("Pull modified an external workspace")
 }
 if head:=remotePullTestGit(t,root,"rev-parse","HEAD");head!=old{
  t.Fatal("denied foreign-worktree Pull advanced HEAD")
 }
}
