package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func remotePullCachedFixture(t *testing.T)(string,string,string){
 t.Helper()
 root,old:=remotePullTestRepo(t)
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
 root,old:=remotePullTestRepo(t)
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
