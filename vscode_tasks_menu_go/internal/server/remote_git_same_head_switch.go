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

// This intentionally does NOT implement git checkout or switching trees.
// The target must be a loose, ordinary taskdeck/ ref pointing to exactly
// the current HEAD. A clean index and working tree are required. This
// prevents a remote client from overwriting any worktree content.
func remoteGitSameHeadSnapshot(ctx context.Context,root,slug string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||
  !remotetransport.ValidRemoteTaskDeckBranchSlug(slug){return nil,denied}
 branch,oid,executable,err:=remoteGitBranchRead(ctx,root)
 if err!=nil{return nil,denied}
 gitDir:=filepath.Join(root,".git")
 headFile:=filepath.Join(gitDir,"HEAD")
 headInfo,err:=os.Lstat(headFile)
 if err!=nil||!headInfo.Mode().IsRegular()||headInfo.Size()>256||
  headInfo.Mode()&os.ModeSymlink!=0{return nil,denied}
 headBytes,err:=os.ReadFile(headFile)
 if err!=nil||string(headBytes)!="ref: "+branch+"\n"{return nil,denied}
 ref:="refs/heads/taskdeck/"+slug
 targetPath:=filepath.Join(gitDir,"refs","heads","taskdeck",slug)
 targetInfo,err:=os.Lstat(targetPath)
 if err!=nil||!targetInfo.Mode().IsRegular()||
  targetInfo.Mode()&os.ModeSymlink!=0||targetInfo.Size()>128{
  return nil,denied
 }
 targetBytes,err:=os.ReadFile(targetPath)
 if err!=nil||string(targetBytes)!=oid+"\n"{return nil,denied}
 indexPath:=filepath.Join(gitDir,"index")
 idx,err:=os.Lstat(indexPath)
 if err!=nil||!idx.Mode().IsRegular()||idx.Size()<1||
  idx.Size()>1<<20||idx.Mode()&os.ModeSymlink!=0{return nil,denied}
 indexBytes,err:=os.ReadFile(indexPath)
 if err!=nil||len(indexBytes)<1||len(indexBytes)>1<<20{return nil,denied}
 indexSum:=sha256.Sum256(indexBytes)
 // Running Git is read-only. No arbitrary Git arguments, executable
 // filters, external diff or user hooks are exposed to the relay.
 cmd:=exec.CommandContext(ctx,executable,
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
  "-c","diff.external=","-C",root,
  "status","--porcelain=v1","--untracked-files=all",
  "--ignore-submodules=none","--no-renames")
 cmd.Env=append(isolatedRemoteGitEnvironment(),
  "GIT_NO_REPLACE_OBJECTS=1","GIT_OPTIONAL_LOCKS=0")
 cmd.Stderr=io.Discard
 var output cappedRemoteGitDiffOutput
 cmd.Stdout=&output
 if err:=cmd.Run();err!=nil||ctx.Err()!=nil||
  len(output.Bytes())!=0{return nil,denied}
 // Recheck HEAD and target after expensive Git status. Ref mutation by
 // an external actor between these checks is handled by held lockfiles
 // in the subsequent authenticated write operation.
 afterBranch,afterOID,_,err:=remoteGitBranchRead(ctx,root)
 if err!=nil||afterBranch!=branch||afterOID!=oid{return nil,denied}
 again,err:=os.ReadFile(targetPath)
 if err!=nil||!bytes.Equal(again,targetBytes){return nil,denied}
 currentIndex,err:=os.ReadFile(indexPath)
 if err!=nil||!bytes.Equal(currentIndex,indexBytes){return nil,denied}
 currentHead,err:=os.ReadFile(headFile)
 if err!=nil||!bytes.Equal(currentHead,headBytes){return nil,denied}
 return []byte("HEAD "+oid+"\nBRANCH "+branch+"\nTARGET "+ref+
  "\nTARGET_HEAD "+oid+"\nINDEX_SHA256 "+hex.EncodeToString(indexSum[:])+
  "\nCLEAN yes\n"),nil
}

func remoteGitSameHeadSwitchReview(ctx context.Context,root,slug string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 first,err:=remoteGitSameHeadSnapshot(ctx,root,slug)
 if err!=nil{return nil,denied}
 second,err:=remoteGitSameHeadSnapshot(ctx,root,slug)
 if err!=nil||!bytes.Equal(first,second){return nil,denied}
 return first,nil
}

// Only the .git/HEAD symbolic reference is atomically replaced; the
// worktree, index, loose target ref and underlying commit are untouched.
// Git-cooperating concurrent HEAD/ref writers respect these lockfiles.
func remoteGitSwitchSameHead(ctx context.Context,root,slug,reviewSHA string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||
  !remotetransport.ValidRemoteTaskDeckBranchSlug(slug)||
  !validRemoteGitSwitchDigest(reviewSHA){return "",denied}
 initial,err:=remoteGitSameHeadSwitchReview(ctx,root,slug)
 if err!=nil{return "",denied}
 sum:=sha256.Sum256(initial)
 if hex.EncodeToString(sum[:])!=reviewSHA{return "",denied}
 gitDir:=filepath.Join(root,".git")
 // Standard Git writers coordinate with HEAD.lock, and update-ref
 // coordinates with the target ref lock. The extra locked verification
 // rejects a stale user review before mutating even HEAD.
 headLock:=filepath.Join(gitDir,"HEAD.lock")
 headFd,err:=os.OpenFile(headLock,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return "",denied}
 headIdentity,err:=headFd.Stat()
 if err!=nil{_ = headFd.Close();return "",denied}
 defer func(){
  info,e:=os.Lstat(headLock)
  if e==nil&&os.SameFile(info,headIdentity){_ = os.Remove(headLock)}
  _ = headFd.Close()
 }()
 refLock:=filepath.Join(gitDir,"refs","heads","taskdeck",slug+".lock")
 refFd,err:=os.OpenFile(refLock,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return "",denied}
 refIdentity,err:=refFd.Stat()
 if err!=nil{_ = refFd.Close();return "",denied}
 defer func(){
  info,e:=os.Lstat(refLock)
  if e==nil&&os.SameFile(info,refIdentity){_ = os.Remove(refLock)}
  _ = refFd.Close()
 }()
 rechecked,err:=remoteGitSameHeadSwitchReview(ctx,root,slug)
 if err!=nil||!bytes.Equal(initial,rechecked)||ctx.Err()!=nil{return "",denied}
 expectedHead:=strings.TrimPrefix(strings.SplitN(string(initial),"\n",2)[0],"HEAD ")
 if !remoteValidGitOIDForCommitReview(expectedHead){return "",denied}
 ref:="refs/heads/taskdeck/"+slug
 original:=strings.TrimPrefix(strings.SplitN(string(initial),"\n",3)[1],"BRANCH ")
 if original==ref{return "",denied}
 // Recheck the HEAD leaf under lock; do not follow symlinks or overwrite
 // a different symbolic-ref introduced by a noncooperative writer.
 currentInfo,err:=os.Lstat(filepath.Join(gitDir,"HEAD"))
 if err!=nil||!currentInfo.Mode().IsRegular()||
  currentInfo.Mode()&os.ModeSymlink!=0{return "",denied}
 headData,err:=os.ReadFile(filepath.Join(gitDir,"HEAD"))
 if err!=nil||string(headData)!="ref: "+original+"\n"{return "",denied}
 if _,err=headFd.Write([]byte("ref: "+ref+"\n"));err!=nil{return "",denied}
 if err=headFd.Sync();err!=nil||ctx.Err()!=nil{return "",denied}
 if err=os.Rename(headLock,filepath.Join(gitDir,"HEAD"));err!=nil{return "",denied}
 // Verified without a Git checkout: we changed only the symbolic HEAD.
 branchAfter,oidAfter,_,checkErr:=remoteGitBranchRead(ctx,root)
 if checkErr!=nil||branchAfter!=ref||oidAfter!=expectedHead{return "",denied}
 return expectedHead,nil
}

func validRemoteGitSwitchDigest(d string)bool{
 if len(d)!=64{return false}
 for _,c:=range d{
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return false}
 }
 return true
}
