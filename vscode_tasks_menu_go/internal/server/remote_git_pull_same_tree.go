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

// An intentionally narrow, OFFLINE fast-forward. A checked-out branch may
// advance ONLY when its source and target commit trees are IDENTICAL.
// Unlike git pull/merge/checkout, this NEVER changes worktree/index bytes.
// It cannot fetch objects, execute hooks/filters, or accept a remote URL.
func remoteGitPullSameTree(ctx context.Context,root,expectedDigest string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!validRemoteGitSwitchDigest(expectedDigest){
  return "",denied
 }
 initial,err:=remoteGitPullReview(ctx,root)
 if err!=nil{return "",denied}
 digest:=sha256.Sum256(initial)
 if hex.EncodeToString(digest[:])!=expectedDigest{return "",denied}
 parts:=strings.Split(string(initial),"\n")
 if len(parts)!=9||parts[5]!="STATUS fast-forward"{return "",denied}
 branch:=strings.TrimPrefix(parts[1],"BRANCH ")
 old:=strings.TrimPrefix(parts[2],"HEAD ")
 upstream:=strings.TrimPrefix(parts[3],"UPSTREAM ")
 target:=strings.TrimPrefix(parts[4],"UPSTREAM_HEAD ")
 // Only flat checked-out branches in standard .git directories are
 // initially writable; nested refs need additional directory/symlink
 // and linked-worktree validation before they can be admitted.
 name:=strings.TrimPrefix(branch,"refs/heads/")
 if name==branch||!validRemoteGitName(name)||name=="HEAD"||
  !remoteValidGitOIDForCommitReview(old)||
  !remoteValidGitOIDForCommitReview(target)||old==target{
  return "",denied
 }
 gitDir:=filepath.Join(root,".git")
 if _,err:=os.Lstat(filepath.Join(gitDir,"worktrees"));err==nil||!os.IsNotExist(err){
  return "",denied
 }
 headPath:=filepath.Join(gitDir,"HEAD")
 headInfo,err:=os.Lstat(headPath)
 if err!=nil||!headInfo.Mode().IsRegular()||headInfo.Mode()&os.ModeSymlink!=0||
  headInfo.Size()>256{return "",denied}
 rawHead,err:=os.ReadFile(headPath)
 if err!=nil||string(rawHead)!="ref: "+branch+"\n"{return "",denied}
 // Ref leaf symlinks, packed-only heads and non-cooperating external
 // worktrees are not an allowed mutation surface for this first phase.
 leaf:=filepath.Join(gitDir,"refs","heads",name)
 info,err:=os.Lstat(leaf)
 if err!=nil||!info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0||
  info.Size()>128{return "",denied}
 value,err:=os.ReadFile(leaf)
 if err!=nil||string(value)!=old+"\n"{return "",denied}
 // Git update-ref may append an existing reflog. Never follow a symlink
 // or accept a special file at that host-controlled metadata leaf.
 logLeaf:=filepath.Join(gitDir,"logs","refs","heads",name)
 if logInfo,logErr:=os.Lstat(logLeaf);logErr==nil{
  if !logInfo.Mode().IsRegular()||logInfo.Mode()&os.ModeSymlink!=0||
   logInfo.Size()>1<<20{return "",denied}
 }else if !os.IsNotExist(logErr){return "",denied}
 _,_,git,err:=remoteGitBranchRead(ctx,root)
 if err!=nil{return "",denied}
 run:=func(args ...string)(string,error){
  a:=append([]string{"-c","core.hooksPath="+os.DevNull,
   "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
   "-c","diff.external=","-C",root},args...)
  cmd:=exec.CommandContext(ctx,git,a...)
  cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
  cmd.Stderr=io.Discard
  var output cappedRemoteGitDiffOutput
  cmd.Stdout=&output
  if cmd.Run()!=nil||ctx.Err()!=nil{return "",denied}
  return strings.TrimSpace(string(output.Bytes())),nil
 }
 oldTree,e:=run("rev-parse","--verify",old+"^{tree}")
 if e!=nil||!remoteValidGitOIDForCommitReview(oldTree){return "",denied}
 newTree,e:=run("rev-parse","--verify",target+"^{tree}")
 if e!=nil||newTree!=oldTree{return "",denied}

 // Hold standard Git HEAD and index lockfiles through a second complete
 // review and the compare-and-swap ref write. No checkout/write-tree.
 locks:=[]string{filepath.Join(gitDir,"HEAD.lock"),filepath.Join(gitDir,"index.lock")}
 fds:=make([]*os.File,0,len(locks))
 identities:=make([]os.FileInfo,0,len(locks))
 defer func(){
  for i,fd:=range fds{
   if current,e:=os.Lstat(locks[i]);e==nil&&os.SameFile(current,identities[i]){
    _=os.Remove(locks[i])
   }
   _=fd.Close()
  }
 }()
 for _,lock:=range locks{
  fd,e:=os.OpenFile(lock,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
  if e!=nil{return "",denied}
  fi,e:=fd.Stat()
  if e!=nil{_ =fd.Close();return "",denied}
  fds=append(fds,fd)
  identities=append(identities,fi)
 }
 rechecked,e:=remoteGitPullReview(ctx,root)
 if e!=nil||!bytes.Equal(initial,rechecked)||ctx.Err()!=nil{return "",denied}
 leafNow,e:=os.ReadFile(leaf)
 if e!=nil||!bytes.Equal(leafNow,value){return "",denied}
 headNow,e:=os.ReadFile(headPath)
 if e!=nil||!bytes.Equal(headNow,rawHead){return "",denied}
 // The target tracking ref must not have been replaced since review.
 upstreamNow,e:=run("rev-parse","--verify",upstream+"^{commit}")
 if e!=nil||upstreamNow!=target{return "",denied}
 // Git's update-ref checks old OID atomically using a per-ref lock.
 // No shell, merge/checkout, remote helper, user-provided argv or hook.
 moved,e:=run("update-ref","--no-deref","-m",
  "TaskDeck approved offline same-tree fast-forward",branch,target,old)
 if e!=nil||moved!=""{return "",denied}
 current,e:=run("rev-parse","--verify","HEAD")
 if e!=nil||current!=target{return "",denied}
 return target,nil
}
