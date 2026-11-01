package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Remote deletion is deliberately limited to a *redundant* TaskDeck ref:
// the exact loose target must point to current HEAD, may not be checked
// out, and the repository must be clean. No arbitrary Git ref is accepted.
// In particular, it cannot delete a branch with unmerged commits.
func remoteGitSafeBranchDeleteReview(ctx context.Context,root,slug string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!remotetransport.ValidRemoteTaskDeckBranchSlug(slug){
  return nil,denied
 }
 gitDir:=filepath.Join(root,".git")
 // A branch may be checked out by another linked worktree. Rather than
 // trusting local worktree bookkeeping from a remote action, deny all
 // repositories with linked-worktree metadata.
 if _,err:=os.Lstat(filepath.Join(gitDir,"worktrees"));err==nil||!os.IsNotExist(err){
  return nil,denied
 }
 leaf:=filepath.Join(gitDir,"logs","refs","heads","taskdeck",slug)
 if info,err:=os.Lstat(leaf);err==nil{
  if !info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0{return nil,denied}
 }else if !os.IsNotExist(err){return nil,denied}
 review,err:=remoteGitSameHeadSwitchReview(ctx,root,slug)
 if err!=nil{return nil,denied}
 // This review is pinned to HEAD, index, target ref and clean worktree,
 // and is hashed in its entirety before an authenticated delete.
 ref:="refs/heads/taskdeck/"+slug
 if bytes.Contains(review,[]byte("\nBRANCH "+ref+"\n")){return nil,denied}
 return review,nil
}

func remoteGitSafeBranchDelete(ctx context.Context,root,slug,reviewSHA string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!remotetransport.ValidRemoteTaskDeckBranchSlug(slug)||
  !validRemoteGitSwitchDigest(reviewSHA){return "",denied}
 review,err:=remoteGitSafeBranchDeleteReview(ctx,root,slug)
 if err!=nil{return "",denied}
 sum:=sha256.Sum256(review)
 if hex.EncodeToString(sum[:])!=reviewSHA{return "",denied}

 // Cooperating Git operations and TaskDeck's same-HEAD switch must hold
 // HEAD.lock. This also protects the review against a concurrent switch.
 gitDir:=filepath.Join(root,".git")
 headLock:=filepath.Join(gitDir,"HEAD.lock")
 fd,err:=os.OpenFile(headLock,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return "",denied}
 identity,err:=fd.Stat()
 if err!=nil{_ = fd.Close();return "",denied}
 defer func(){
  if info,e:=os.Lstat(headLock);e==nil&&os.SameFile(info,identity){
   _ = os.Remove(headLock)
  }
  _ = fd.Close()
 }()
 again,err:=remoteGitSafeBranchDeleteReview(ctx,root,slug)
 if err!=nil||!bytes.Equal(review,again)||ctx.Err()!=nil{return "",denied}
 lines:=strings.Split(string(review),"\n")
 if len(lines)!=7||!strings.HasPrefix(lines[0],"HEAD "){return "",denied}
 oid:=strings.TrimPrefix(lines[0],"HEAD ")
 if !remoteValidGitOIDForCommitReview(oid){return "",denied}
 ref:="refs/heads/taskdeck/"+slug
 if lines[2]!="TARGET "+ref||lines[3]!="TARGET_HEAD "+oid{return "",denied}

 _,_,git,err:=remoteGitBranchRead(ctx,root)
 if err!=nil{return "",denied}
 cmd:=exec.CommandContext(ctx,git,
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.logAllRefUpdates=false",
  "-c","core.fsmonitor=false",
  "-c","core.untrackedCache=false",
  "-C",root,
  "update-ref","--no-deref","-d",ref,oid)
 cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
 cmd.Stderr=io.Discard
 var output cappedRemoteGitDiffOutput
 cmd.Stdout=&output
 if err:=cmd.Run();err!=nil||ctx.Err()!=nil||len(output.Bytes())!=0{
  return "",denied
 }
 // Git update-ref performs the locked compare-and-delete; the old OID
 // cannot be silently replaced by a concurrent writer.
 if _,e:=os.Lstat(filepath.Join(gitDir,"refs","heads","taskdeck",slug));!os.IsNotExist(e){
  return "",denied
 }
 return "deleted",nil
}
