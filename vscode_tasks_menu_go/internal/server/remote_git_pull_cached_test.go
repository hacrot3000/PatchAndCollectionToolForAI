package server

import (
 "context"
 "os/exec"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func remotePullCachedFixture(t *testing.T)(string,string,string){
 t.Helper()
 root,_:=remotePullTestRepo(t)
 // A real nonempty Git index is required by the immutable-index CAS gate.
 if err:=os.WriteFile(filepath.Join(root,"tracked.txt"),[]byte("stable"),0600);err!=nil{t.Fatal(err)}
 remotePullTestGit(t,root,"add","tracked.txt")
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid","commit","-q","-m","baseline tracked file")
 old:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",old)
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid",
  "commit","-q","--allow-empty","-m","upstream metadata-only")
 target:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",target)
 remotePullTestGit(t,root,"reset","--hard",old)
 return root,old,target
}

func TestRemoteGitCachedFastForwardUpdatesOnlyBranchRefWithCAS(t *testing.T){
 root,old,target:=remotePullCachedFixture(t)
 indexBefore,err:=os.ReadFile(filepath.Join(root,".git","index"))
 if err!=nil{t.Fatal(err)}
 original,err:=remoteGitPullCachedReview(context.Background(),root)
 if err!=nil||!remotetransport.ValidRemoteGitPullCachedReviewReport(string(original)){
  t.Fatalf("trusted cached review refused: %v %q",err,original)
 }
 digest:=sha256.Sum256(original)
 newOID,err:=remoteGitPullApplyCached(context.Background(),root,hex.EncodeToString(digest[:]))
 if err!=nil||newOID!=target{t.Fatalf("cached FF denied: %q %v",newOID,err)}
 if got:=remotePullTestGit(t,root,"rev-parse","HEAD");got!=target{t.Fatal("branch ref not updated")}
 if got:=remotePullTestGit(t,root,"rev-parse","HEAD^{tree}");
  got!=remotePullTestGit(t,root,"rev-parse",old+"^{tree}"){
  t.Fatal("cached FF incorrectly changed the tree")
 }
 indexAfter,err:=os.ReadFile(filepath.Join(root,".git","index"))
 if err!=nil||string(indexBefore)!=string(indexAfter){t.Fatal("cached FF modified live Git index")}
 if status:=remotePullTestGit(t,root,"status","--porcelain=v1","--untracked-files=all");status!=""{
  t.Fatalf("cached FF dirtied worktree: %q",status)
 }
 if _,err:=remoteGitPullApplyCached(context.Background(),root,hex.EncodeToString(digest[:]));err==nil{
  t.Fatal("already-consumed cached review still accepted")
 }
}
func TestRemoteGitCachedFastForwardRejectsStaleReviewAndDirtyWorkspace(t *testing.T){
 root,old,target:=remotePullCachedFixture(t)
 original,err:=remoteGitPullCachedReview(context.Background(),root)
 if err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(original)
 stale:=hex.EncodeToString(digest[:])
 // Upstream changed after the operator saw the review. Never update local
 // branch to an unreviewed revision or one re-pointed after confirmation.
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",old,target)
 if _,err:=remoteGitPullApplyCached(context.Background(),root,stale);err==nil{
  t.Fatal("upstream CAS bypassed stale review")
 }
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",target)
 if err:=os.WriteFile(filepath.Join(root,"untracked-from-user"),[]byte("keep"),0600);err!=nil{t.Fatal(err)}
 if _,err:=remoteGitPullApplyCached(context.Background(),root,stale);err==nil{
  t.Fatal("dirty worktree allowed a ref update")
 }
}
func TestRemoteGitCachedFastForwardRefusesTreeChanges(t *testing.T){
 root,old,_:=remotePullCachedFixture(t)
 if err:=os.WriteFile(filepath.Join(root,"tracked.txt"),[]byte("different tree"),0600);err!=nil{t.Fatal(err)}
 remotePullTestGit(t,root,"add","tracked.txt")
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid","commit","-q","-m","changes files")
 target:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",target)
 remotePullTestGit(t,root,"reset","--hard",old)
 if got,err:=remoteGitPullCachedReview(context.Background(),root);err==nil||len(got)!=0{
  t.Fatal("tree-changing upstream improperly eligible for ref-only FF")
 }
}

func TestRemoteGitCachedApplyE2EERequiresHostGateAndLocalApproval(t *testing.T){
 for _,tc:=range []struct{name,gitFlag,pullFlag string;allow bool}{
  {"both-off","","",false},
  {"only-git-write","1","",false},
  {"only-cached-pull","","1",false},
  {"both-host-flags","1","1",true},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_GIT_WRITE",tc.gitFlag)
   t.Setenv("TASKDECK_REMOTE_GIT_PULL_CACHED",tc.pullFlag)
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
   remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
    "-c","user.email=test@localhost.invalid",
    "commit","-q","--allow-empty","-m","remote metadata only")
   target:=remotePullTestGit(t,root,"rev-parse","HEAD")
   remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",target)
   remotePullTestGit(t,root,"reset","--hard",old)
   constReviewID:="0123456789abcdef0123456789abcdef"
   req,e:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:constReviewID,Operation:remotetransport.RemoteActionGitPullCachedReview,
   })
   if e!=nil{t.Fatal(e)}
   reply,e:=s.executeRemoteE2EEPing(context.Background(),owner,agent,req)
   if e!=nil{t.Fatalf("host review denied: %v",e)}
   review,e:=remotetransport.OpenRemoteGitPullCachedReviewResult(client,reply,constReviewID)
   if e!=nil{t.Fatalf("E2EE preview denied: %v",e)}
   hash:=sha256.Sum256([]byte(review))
   constApplyID:="1123456789abcdef0123456789abcdef"
   request,e:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:constApplyID,Operation:remotetransport.RemoteActionGitPullApplyCached,
    ExpectedSHA256:hex.EncodeToString(hash[:]),
   })
   if e!=nil{t.Fatal(e)}
   response,e:=s.executeRemoteE2EEPing(context.Background(),owner,agent,request)
   if !tc.allow{
    if e==nil||len(response)!=0{t.Fatal("disabled host flags permitted remote ref update")}
    if current:=remotePullTestGit(t,root,"rev-parse","HEAD");current!=old{
     t.Fatal("denied operation still advanced Git")
    }
    return
   }
   if e!=nil{t.Fatalf("host gate enabled but cached FF failed: %v",e)}
   got,e:=remotetransport.OpenRemoteGitPullApplyCachedResult(client,response,constApplyID)
   if e!=nil||got!=target{t.Fatalf("wrong authenticated target OID: %q %v",got,e)}
   if current:=remotePullTestGit(t,root,"rev-parse","HEAD");current!=target{
    t.Fatal("cached FF did not advance branch")
   }
  })
 }
}
