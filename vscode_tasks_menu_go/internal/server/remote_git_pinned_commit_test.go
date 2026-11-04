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

func pinnedCommitGit(t *testing.T,root string,args ...string)[]byte{
 t.Helper()
 out,err:=exec.Command("git",append([]string{"-C",root},args...)...).CombinedOutput()
 if err!=nil{t.Fatalf("git %v: %v %s",args,err,out)}
 return bytes.TrimSpace(out)
}
func pinnedCommitFixture(t *testing.T,root string)(string,string,[]byte){
 t.Helper()
 if err:=exec.Command("git","init","-q",root).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
 file:=filepath.Join(root,"code.txt")
 if err:=os.WriteFile(file,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 pinnedCommitGit(t,root,"add","--","code.txt")
 pinnedCommitGit(t,root,"-c","user.name=TaskDeck CI",
  "-c","user.email=ci@example.invalid","commit","-qm","initial")
 old:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"))
 if err:=os.WriteFile(file,[]byte("staged\n"),0600);err!=nil{t.Fatal(err)}
 pinnedCommitGit(t,root,"add","--","code.txt")
 index,err:=os.ReadFile(filepath.Join(root,".git","index"))
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(file,[]byte("working tree\n"),0600);err!=nil{t.Fatal(err)}
 return file,old,index
}
func pinnedCommitReviewHash(t *testing.T,root string)string{
 t.Helper()
 review,err:=remoteGitCommitReview(context.Background(),root)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(review)
 return hex.EncodeToString(sum[:])
}

func TestGitCommitPinnedHostCommitsReviewedIndexWithoutHooksOrTreeEdits(t *testing.T){
 root:=t.TempDir()
 file,parent,index:=pinnedCommitFixture(t,root)
 // A malicious repository's commit hook and reference-transaction hook
 // must never be executed by our fixed Git plumbing.
 marker:=filepath.Join(root,"HOOK_EXECUTED")
 for _,name:=range []string{"pre-commit","reference-transaction","post-commit"}{
  path:=filepath.Join(root,".git","hooks",name)
  if err:=os.WriteFile(path,[]byte("#!/bin/sh\ntouch '"+marker+"'\n"),0700);err!=nil{t.Fatal(err)}
 }
 before:=pinnedCommitReviewHash(t,root)
 committed,err:=remoteGitCommitPinned(context.Background(),root,"Safe reviewed update",before)
 if err!=nil||!remoteValidGitOIDForCommitReview(committed){
  t.Fatalf("commit failed: %q %v",committed,err)
 }
 if got:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"));got!=committed{
  t.Fatalf("HEAD not updated through CAS: %s != %s",got,committed)
 }
 if got:=string(pinnedCommitGit(t,root,"rev-parse","HEAD^"));got!=parent{
  t.Fatal("commit parent different from reviewed HEAD")
 }
 stagedBlob:=pinnedCommitGit(t,root,"show","HEAD:code.txt")
 if !bytes.Equal(stagedBlob,[]byte("staged")){ // trim by test helper
  t.Fatalf("committed worktree bytes instead of reviewed index: %q",stagedBlob)
 }
 currentIndex,err:=os.ReadFile(filepath.Join(root,".git","index"))
 if err!=nil||!bytes.Equal(index,currentIndex){t.Fatal("commit mutated Git index")}
 now,err:=os.ReadFile(file)
 if err!=nil||!bytes.Equal(now,[]byte("working tree\n")){t.Fatal("commit mutated working tree")}
 if _,err:=os.Stat(marker);!os.IsNotExist(err){
  t.Fatalf("hook executed despite protected Git command: %v",err)
 }
 if _,err:=os.Stat(filepath.Join(root,".git","index.lock"));!os.IsNotExist(err){
  t.Fatalf("commit left Git index.lock behind: %v",err)
 }
 subject:=string(pinnedCommitGit(t,root,"log","-1","--format=%s"))
 if subject!="Safe reviewed update"{t.Fatalf("unexpected commit subject %q",subject)}
 if _,err:=remoteGitCommitPinned(context.Background(),root,"Second commit",before);err==nil{
  t.Fatal("stale/replayed review committed a second time")
 }
}

func TestGitCommitPinnedHostRejectsStaleReviewCASAndEmptyIndex(t *testing.T){
 t.Run("index changed",func(t *testing.T){
  root:=t.TempDir()
  file,old,_:=pinnedCommitFixture(t,root)
  before:=pinnedCommitReviewHash(t,root)
  if err:=os.WriteFile(file,[]byte("changed again\n"),0600);err!=nil{t.Fatal(err)}
  pinnedCommitGit(t,root,"add","--","code.txt")
  if _,err:=remoteGitCommitPinned(context.Background(),root,"Safe change",before);err==nil{
   t.Fatal("stale Git index CAS accepted")
  }
  if got:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"));got!=old{
   t.Fatal("stale commit advanced HEAD")
  }
 })
 t.Run("no staged diff",func(t *testing.T){
  root:=t.TempDir()
  _,_,_ = pinnedCommitFixture(t,root)
  pinnedCommitGit(t,root,"reset","--hard","HEAD")
  if _,err:=remoteGitCommitPinned(context.Background(),root,"Safe commit",strings.Repeat("a",64));err==nil{
   t.Fatal("empty index accepted")
  }
 })
 t.Run("unsafe commit message",func(t *testing.T){
  root:=t.TempDir()
  _,_,_ = pinnedCommitFixture(t,root)
  review:=pinnedCommitReviewHash(t,root)
  for _,s:=range []string{"sh; cat /etc/passwd","one\ntwo","$(pwd)","-S --amend","X\x1b[2J"}{
   if _,err:=remoteGitCommitPinned(context.Background(),root,s,review);err==nil{
    t.Fatalf("dangerous Git message accepted %q",s)
   }
  }
 })
}

func TestGitCommitPinnedHostFlagsRBACAndNoPHPLoginBypass(t *testing.T){
 for _,variant:=range []string{"optin_off","php_only","viewer","owner"}{
  t.Run(variant,func(t *testing.T){
   role:=remotetransport.RoleAdmin
   if variant=="viewer"{role=remotetransport.RoleViewer}
   s,session,client,agent:=remoteDispatchFixtureWithRole(t,role)
   _,old,_:=pinnedCommitFixture(t,s.Workspace)
   t.Setenv("TASKDECK_REMOTE_GIT_WRITE","1")
   t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","1")
   if variant=="optin_off"{t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","")}
   review:=pinnedCommitReviewHash(t,s.Workspace)
   req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitCommit,
    Content:"Authorized safe commit",ExpectedSHA256:review,
   })
   if err!=nil{t.Fatal(err)}
   if variant=="php_only"{session=nil}
   sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
   if variant!="owner"{
    if err==nil||len(sealed)!=0{t.Fatalf("%s bypassed TaskDeck authority",variant)}
    if got:=string(pinnedCommitGit(t,s.Workspace,"rev-parse","HEAD"));got!=old{
     t.Fatal("unauthorized commit changed HEAD")
    }
   }else{
    if err!=nil{t.Fatalf("approved TaskDeck commit failed: %v",err)}
    oid,err:=remotetransport.OpenRemoteGitCommitResult(client,sealed,remoteDispatchTestID)
    if err!=nil||!remoteValidGitOIDForCommitReview(oid)||oid==old{
     t.Fatalf("commit result authentication failed: %q %v",oid,err)
    }
    if _,err=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil{
     t.Fatal("replayed Git commit request succeeded")
    }
   }
  })
 }
}

func TestGitCommitPinnedRejectsAlternatesAndUnsafeFilters(t *testing.T){
 for _,variant:=range []string{"alternates","cleanfilter"}{
  t.Run(variant,func(t *testing.T){
   root:=t.TempDir()
   _,old,_:=pinnedCommitFixture(t,root)
   before:=pinnedCommitReviewHash(t,root)
   if variant=="alternates"{
    p:=filepath.Join(root,".git","objects","info","alternates")
    if err:=os.MkdirAll(filepath.Dir(p),0700);err!=nil{t.Fatal(err)}
    if err:=os.WriteFile(p,[]byte(t.TempDir()+"\n"),0600);err!=nil{t.Fatal(err)}
   }else{
    pinnedCommitGit(t,root,"config","filter.evil.clean","sh -c 'exit 99'")
   }
   if _,err:=remoteGitCommitPinned(context.Background(),root,"Safe message",before);err==nil{
    t.Fatalf("unsafe repository %s allowed commit",variant)
   }
   if got:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"));got!=old{
    t.Fatal("rejected commit unexpectedly advanced HEAD")
   }
  })
 }
}

func TestGitCommitPinnedRefusesAlreadyLockedIndex(t *testing.T){
 root:=t.TempDir()
 _,old,_:=pinnedCommitFixture(t,root)
 review:=pinnedCommitReviewHash(t,root)
 lock:=filepath.Join(root,".git","index.lock")
 keep:=[]byte("external Git writer owns index lock")
 if err:=os.WriteFile(lock,keep,0600);err!=nil{t.Fatal(err)}
 if _,err:=remoteGitCommitPinned(context.Background(),root,"Safe message",review);err==nil{
  t.Fatal("Git commit ignored an external writer index lock")
 }
 actual,err:=os.ReadFile(lock)
 if err!=nil||!bytes.Equal(actual,keep){t.Fatal("commit destroyed external Git index lock")}
 if got:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"));got!=old{
  t.Fatal("commit advanced HEAD despite an existing index lock")
 }
}

func TestGitCommitPinnedDeniesChangedHeadOrDetachedBranch(t *testing.T){
 t.Run("HEAD changed after preview",func(t *testing.T){
  root:=t.TempDir()
  file,_,_:=pinnedCommitFixture(t,root)
  approved:=pinnedCommitReviewHash(t,root)
  // A separate local Git user advances the branch after review.
  if err:=os.WriteFile(file,[]byte("independent local commit\n"),0600);err!=nil{t.Fatal(err)}
  pinnedCommitGit(t,root,"add","--","code.txt")
  pinnedCommitGit(t,root,"-c","user.name=TaskDeck CI",
   "-c","user.email=ci@example.invalid","commit","-qm","local writer")
  localHEAD:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"))
  if _,err:=remoteGitCommitPinned(context.Background(),root,"Should be rejected",approved);err==nil{
   t.Fatal("outdated approved HEAD created remote Git commit")
  }
  if now:=string(pinnedCommitGit(t,root,"rev-parse","HEAD"));now!=localHEAD{
   t.Fatal("stale request overwrote new HEAD")
  }
 })
 t.Run("detached HEAD",func(t *testing.T){
  root:=t.TempDir()
  _,_,_ = pinnedCommitFixture(t,root)
  pinnedCommitGit(t,root,"checkout","--detach","-q")
  if _,err:=remoteGitCommitReview(context.Background(),root);err==nil{
   t.Fatal("remote commit review accepted detached HEAD")
  }
  if _,err:=remoteGitCommitPinned(context.Background(),root,
   "Must be rejected",strings.Repeat("a",64));err==nil{
   t.Fatal("remote commit accepted detached HEAD")
  }
 })
}


func TestGitCommitPinnedDeniesSymlinkedRefAndReflogMetadata(t *testing.T){
 cases:=[]string{
  "HEAD",
  filepath.Join("logs","HEAD"),
  "packed-refs",
  filepath.Join("refs","heads"),
  filepath.Join("logs","refs","heads"),
  "branch-leaf",
  "branch-log-leaf",
 }
 for _,name:=range cases{
  t.Run(name,func(t *testing.T){
   root:=t.TempDir()
   _,original,_:=pinnedCommitFixture(t,root)
   review:=pinnedCommitReviewHash(t,root)
   branch:=strings.TrimPrefix(
    string(pinnedCommitGit(t,root,"symbolic-ref","HEAD")),"refs/heads/")
   relative:=name
   if name=="branch-leaf"{relative=filepath.Join("refs","heads",branch)}
   if name=="branch-log-leaf"{relative=filepath.Join("logs","refs","heads",branch)}
   path:=filepath.Join(root,".git",relative)
   relocated:=filepath.Join(t.TempDir(),"outside-metadata")
   if _,err:=os.Lstat(path);err==nil{
    if err:=os.Rename(path,relocated);err!=nil{t.Fatal(err)}
   }else if os.IsNotExist(err){
    if err:=os.WriteFile(relocated,[]byte("# pack-refs with: peeled fully-peeled sorted\n"),0600);
     err!=nil{t.Fatal(err)}
   }else{t.Fatal(err)}
   if err:=os.Symlink(relocated,path);err!=nil{t.Fatal(err)}
   if remoteGitCommitRefMetadataSafe(root,"refs/heads/"+branch){
    t.Fatal("symlinked writable Git metadata accepted")
   }
   got,err:=remoteGitCommitPinned(context.Background(),root,"Safe reviewed update",review)
   if err==nil||got!=""{
    t.Fatalf("ref-symlink metadata admitted commit: %q %v",got,err)
   }
   // External symlink targets must never be rewritten as a consequence of
   // a rejected commit; a future fix must not merely hide the error.
   if st,e:=os.Lstat(path);e!=nil||st.Mode()&os.ModeSymlink==0{
    t.Fatal("Git metadata symlink was replaced despite denial")
   }
   if out,e:=exec.Command("git","-C",root,"rev-parse","--verify","HEAD").Output();e==nil&&
    string(bytes.TrimSpace(out))!=original{
    t.Fatal("Git commit advanced HEAD despite unsafe ref metadata")
   }
  })
 }
}

func TestGitCommitPinnedRefMetadataHelperKeepsOrdinaryCommitsWorking(t *testing.T){
 root:=t.TempDir()
 pinnedCommitFixture(t,root)
 branch:=string(pinnedCommitGit(t,root,"symbolic-ref","HEAD"))
 if !remoteGitCommitRefMetadataSafe(root,branch){
  t.Fatal("ordinary locally initialized commit metadata rejected")
 }
 digest:=pinnedCommitReviewHash(t,root)
 got,err:=remoteGitCommitPinned(context.Background(),root,"Safe reviewed update",digest)
 if err!=nil||!remoteValidGitOIDForCommitReview(got){
  t.Fatalf("existing reviewed Git Commit regressed: %q %v",got,err)
 }
}
