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

// This limited operation supports only a normal on-disk .git directory.
// Deny symlinked metadata ancestors before even running its Git preflight:
// a symlinked refs/heads parent could redirect update-ref outside the
// host-approved workspace despite the branch leaf being a regular file.
func remoteGitCachedMetadataDirectoriesSafe(root string)bool{
 paths:=[]string{
  root,filepath.Join(root,".git"),
  filepath.Join(root,".git","refs"),
  filepath.Join(root,".git","refs","heads"),
  filepath.Join(root,".git","logs"),
  filepath.Join(root,".git","logs","refs"),
  filepath.Join(root,".git","logs","refs","heads"),
 }
 for _,path:=range paths{
  info,err:=os.Lstat(path)
  if err!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{
   return false
  }
 }
 return true
}

// This is intentionally narrower than git pull. An already-fetched
// fast-forward can be applied remotely ONLY if HEAD and the upstream commit
// have exactly the SAME Git tree. Consequently neither the index nor any
// worktree path needs a checkout. It does not fetch, merge or run hooks.
func remoteGitPullCachedReview(ctx context.Context,root string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!remoteGitCachedMetadataDirectoriesSafe(root){
  return nil,denied
 }
 first,err:=remoteGitPullReview(ctx,root)
 if err!=nil{return nil,denied}
 lines:=strings.Split(string(first),"\n")
 if len(lines)!=9||lines[5]!="STATUS fast-forward"{return nil,denied}
 branch:=strings.TrimPrefix(lines[1],"BRANCH ")
 head:=strings.TrimPrefix(lines[2],"HEAD ")
 upstream:=strings.TrimPrefix(lines[3],"UPSTREAM ")
 target:=strings.TrimPrefix(lines[4],"UPSTREAM_HEAD ")
 if !remoteGitPullBranchSafe(strings.TrimPrefix(branch,"refs/heads/"))||
  !remoteValidGitOIDForCommitReview(head)||
  !remoteValidGitOIDForCommitReview(target)||
  !remotetransport.ValidRemoteGitPullReviewReport(string(first)){return nil,denied}
 executable,err:=remoteTrustedGitExecutable(root)
 if err!=nil||remoteGitReadOnlyConfigSafe(ctx,root,executable)!=nil{
  return nil,denied
 }
 tree:=func(oid string)(string,error){
  cmd:=exec.CommandContext(ctx,executable,
   "-c","core.hooksPath="+os.DevNull,"-c","core.fsmonitor=false",
   "-C",root,"rev-parse","--verify",oid+"^{tree}")
  cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
  cmd.Stderr=io.Discard
  var out cappedRemoteGitDiffOutput
  cmd.Stdout=&out
  if err:=cmd.Run();err!=nil||ctx.Err()!=nil{return "",denied}
  value:=strings.TrimSpace(string(out.Bytes()))
  if !remoteValidGitOIDForCommitReview(value){return "",denied}
  return value,nil
 }
 headTree,e:=tree(head)
 if e!=nil{return nil,denied}
 targetTree,e:=tree(target)
 if e!=nil||targetTree!=headTree{return nil,denied}
 indexPath:=filepath.Join(root,".git","index")
 info,e:=os.Lstat(indexPath)
 if e!=nil||!info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0||
  info.Size()<1||info.Size()>1<<20{return nil,denied}
 indexBytes,e:=os.ReadFile(indexPath)
 if e!=nil||len(indexBytes)<1||len(indexBytes)>1<<20{return nil,denied}
 idxSum:=sha256.Sum256(indexBytes)
 after,e:=remoteGitPullReview(ctx,root)
 if e!=nil||!bytes.Equal(first,after){return nil,denied}
 again,e:=os.ReadFile(indexPath)
 if e!=nil||!bytes.Equal(indexBytes,again){return nil,denied}
 out:="GIT_PULL_CACHED_V1\nBRANCH "+branch+
  "\nHEAD "+head+"\nUPSTREAM "+upstream+"\nUPSTREAM_HEAD "+target+
  "\nINDEX_SHA256 "+hex.EncodeToString(idxSum[:])+
  "\nTREE unchanged\nWORKTREE clean\nNETWORK none\n"
 if !remotetransport.ValidRemoteGitPullCachedReviewReport(out){
  return nil,denied
 }
 return []byte(out),nil
}

func remoteGitPullApplyCached(ctx context.Context,root,reviewDigest string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!validRemoteGitSwitchDigest(reviewDigest){
  return "",denied
 }
 first,err:=remoteGitPullCachedReview(ctx,root)
 if err!=nil{return "",denied}
 digest:=sha256.Sum256(first)
 if hex.EncodeToString(digest[:])!=reviewDigest{return "",denied}
 lines:=strings.Split(string(first),"\n")
 if len(lines)!=10{return "",denied}
 branch:=strings.TrimPrefix(lines[1],"BRANCH ")
 head:=strings.TrimPrefix(lines[2],"HEAD ")
 upstream:=strings.TrimPrefix(lines[3],"UPSTREAM ")
 target:=strings.TrimPrefix(lines[4],"UPSTREAM_HEAD ")
 indexHash:=strings.TrimPrefix(lines[5],"INDEX_SHA256 ")
 if !remotetransport.ValidRemoteGitPullCachedReviewReport(string(first)){
  return "",denied
 }
 gitDir:=filepath.Join(root,".git")
 // Index lock serializes this ref-only advance against cooperating local
 // checkout/index writers. Do NOT precreate HEAD.lock: Git update-ref needs
 // that lock itself for the symbolic HEAD/reflog transaction, so holding it
 // here makes every valid fast-forward fail. The ref transaction below
 // supplies its own atomic branch/upstream CAS and HEAD locking.
 locks:=[]string{filepath.Join(gitDir,"index.lock")}
 type lock struct{path string;file *os.File;info os.FileInfo}
 var held []lock
 defer func(){
  for i:=len(held)-1;i>=0;i--{
   l:=held[i]
   if now,e:=os.Lstat(l.path);e==nil&&os.SameFile(now,l.info){
    _=os.Remove(l.path)
   }
   _=l.file.Close()
  }
 }()
 for _,p:=range locks{
  fd,e:=os.OpenFile(p,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
  if e!=nil{return "",denied}
  info,e:=fd.Stat()
  if e!=nil{_ =fd.Close();return "",denied}
  held=append(held,lock{path:p,file:fd,info:info})
 }
 // Revalidate metadata ancestors with both cooperative Git locks held.
 if !remoteGitCachedMetadataDirectoriesSafe(root){return "",denied}
 // Never follow branch path symlinks or support a linked worktree.
 rel:=strings.TrimPrefix(branch,"refs/heads/")
 if !remoteGitPullBranchSafe(rel){return "",denied}
 parent:=filepath.Join(gitDir,"refs","heads")
 for _,segment:=range strings.Split(rel,"/")[:len(strings.Split(rel,"/"))-1]{
  parent=filepath.Join(parent,segment)
  info,e:=os.Lstat(parent)
  if e!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{
   return "",denied
  }
 }
 leaf:=filepath.Join(gitDir,"refs","heads",filepath.FromSlash(rel))
 if info,e:=os.Lstat(leaf);e==nil{
  if !info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0{
   return "",denied
  }
 }else if !os.IsNotExist(e){return "",denied}
 // git update-ref can append to .git/logs/refs/heads/BRANCH even though
 // the caller never passes a reflog pathname. Reject local workspace
 // symlinks in every reflog ancestor and the leaf before any update.
 logBase:=filepath.Join(gitDir,"logs")
 for _,name:=range []string{"", "refs",filepath.Join("refs","heads")}{
  candidate:=filepath.Join(logBase,name)
  info,e:=os.Lstat(candidate)
  if e!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{
   return "",denied
  }
 }
 logParent:=filepath.Join(logBase,"refs","heads")
 branchParts:=strings.Split(rel,"/")
 for _,segment:=range branchParts[:len(branchParts)-1]{
  logParent=filepath.Join(logParent,segment)
  info,e:=os.Lstat(logParent)
  if e!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{
   return "",denied
  }
 }
 logLeaf:=filepath.Join(logParent,branchParts[len(branchParts)-1])
 if info,e:=os.Lstat(logLeaf);e==nil{
  if !info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0{
   return "",denied
  }
 }else if !os.IsNotExist(e){return "",denied}
 headFile:=filepath.Join(gitDir,"HEAD")
 headInfo,e:=os.Lstat(headFile)
 if e!=nil||!headInfo.Mode().IsRegular()||headInfo.Mode()&os.ModeSymlink!=0{
  return "",denied
 }
 headData,e:=os.ReadFile(headFile)
 if e!=nil||string(headData)!="ref: "+branch+"\n"{return "",denied}
 indexData,e:=os.ReadFile(filepath.Join(gitDir,"index"))
 idx:=sha256.Sum256(indexData)
 if e!=nil||hex.EncodeToString(idx[:])!=indexHash{return "",denied}
 confirmed,e:=remoteGitPullCachedReview(ctx,root)
 if e!=nil||!bytes.Equal(confirmed,first)||ctx.Err()!=nil{
  return "",denied
 }
 git,e:=remoteTrustedGitExecutable(root)
 if e!=nil{return "",denied}
 // One Git ref transaction locks AND verifies the upstream tracking
 // ref alongside the checked-out branch, rather than relying on a
 // preflight read that could race an external git fetch. The local
 // operator cannot supply any refspec: both refs come from the host's
 // previously validated upstream configuration and review hash.
 if !strings.HasPrefix(upstream,"refs/remotes/")||
  !remotetransport.ValidRemoteGitPullCachedReviewReport(string(first)){
  return "",denied
 }
 commands:="start\nverify "+upstream+" "+target+
  "\nupdate "+branch+" "+target+" "+head+"\nprepare\ncommit\n"
 cmd:=exec.CommandContext(ctx,git,
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false",
  "-C",root,"update-ref","--stdin","--no-deref",
  "-m","TaskDeck approved cached tree-identical fast-forward")
 cmd.Stdin=strings.NewReader(commands)
 cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
 cmd.Stderr=io.Discard
 var output cappedRemoteGitDiffOutput
 cmd.Stdout=&output
 if e=cmd.Run();e!=nil||ctx.Err()!=nil||len(output.Bytes())>256{
  return "",denied
 }
 // This stage did not write the index or working tree. Read back the HEAD
 // and require its new commit to equal the approved target.
 newBranch,newHead,_,e:=remoteGitBranchRead(ctx,root)
 if e!=nil||newBranch!=branch||newHead!=target{return "",denied}
 return target,nil
}
