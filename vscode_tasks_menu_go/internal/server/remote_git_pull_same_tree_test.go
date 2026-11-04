package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestRemotePullSameTreeAdvancesOnlyRefsWithExactReviewDigest(t *testing.T){
 root,old:=remotePullTestRepo(t)
 // Empty commit changes the commit OID, but not the tree or worktree.
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid",
  "commit","-q","--allow-empty","-m","remote empty commit")
 tip:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",tip)
 remotePullTestGit(t,root,"reset","--hard",old)
 review,err:=remoteGitPullReview(context.Background(),root)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(review)
 digest:=hex.EncodeToString(sum[:])
 if _,err:=remoteGitPullSameTree(context.Background(),root,strings.Repeat("f",64));err==nil{
  t.Fatal("stale or fabricated digest advanced branch")
 }
 result,err:=remoteGitPullSameTree(context.Background(),root,digest)
 if err!=nil||result!=tip{t.Fatalf("same-tree FF denied: %s %v",result,err)}
 if got:=remotePullTestGit(t,root,"rev-parse","HEAD");got!=tip{
  t.Fatal("HEAD did not advance to reviewed tip")
 }
 if got:=remotePullTestGit(t,root,"status","--porcelain");got!=""{
  t.Fatalf("same-tree FF modified working tree: %q",got)
 }
 if _,err:=remoteGitPullSameTree(context.Background(),root,digest);err==nil{
  t.Fatal("same review replay advanced refs twice")
 }
}
func TestRemotePullSameTreeRejectsChangedTreeAndDirtyWorktree(t *testing.T){
 root,old:=remotePullTestRepo(t)
 filename:=filepath.Join(root,"new-file.txt")
 if err:=os.WriteFile(filename,[]byte("new content"),0600);err!=nil{t.Fatal(err)}
 remotePullTestGit(t,root,"add","new-file.txt")
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid",
  "commit","-q","-m","files changed")
 tip:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",tip)
 remotePullTestGit(t,root,"reset","--hard",old)
 review,err:=remoteGitPullReview(context.Background(),root)
 if err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(review)
 if _,err:=remoteGitPullSameTree(context.Background(),root,hex.EncodeToString(digest[:]));err==nil{
  t.Fatal("changed-tree FF silently skipped checkout")
 }
 if got:=remotePullTestGit(t,root,"rev-parse","HEAD");got!=old{
  t.Fatal("failure path changed HEAD")
 }
 // Review may become stale even if two commit trees are the same.
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",old)
 if _,err:=remoteGitPullSameTree(context.Background(),root,hex.EncodeToString(digest[:]));err==nil{
  t.Fatal("upstream ref race bypassed digest comparison")
 }
}


func TestRemotePullSameTreeRejectsReflogSymlinkBeforeAnyRefWrite(t *testing.T){
 root,old:=remotePullTestRepo(t)
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid",
  "commit","-q","--allow-empty","-m","same tree")
 tip:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",tip)
 remotePullTestGit(t,root,"reset","--hard",old)
 review,err:=remoteGitPullReview(context.Background(),root)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(review)
 logPath:=filepath.Join(root,".git","logs","refs","heads","main")
 if err:=os.Remove(logPath);err!=nil{t.Fatal(err)}
 secret:=filepath.Join(t.TempDir(),"marker")
 if err:=os.WriteFile(secret,[]byte("unchanged"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(secret,logPath);err!=nil{t.Skipf("symlink unsupported: %v",err)}
 if _,err:=remoteGitPullSameTree(context.Background(),root,hex.EncodeToString(sum[:]));err==nil{
  t.Fatal("symlinked Git reflog was writable by E2EE FF")
 }
 if data,err:=os.ReadFile(secret);err!=nil||string(data)!="unchanged"{
  t.Fatalf("external reflog target modified: %q %v",data,err)
 }
 if got:=remotePullTestGit(t,root,"rev-parse","HEAD");got!=old{
  t.Fatal("rejected reflog path still advanced HEAD")
 }
}
