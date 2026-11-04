package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A deliberately restricted worktree-changing, ALREADY FETCHED fast-forward.
// The only values the remote operator supplies are the SHA256 digest of the
// complete host-generated E2EE review and an explicit one-use confirmation
// in the trusted native GUI. Never accept Git argv, branch, URL or file data.
// This does NOT fetch, merge divergent histories, push or run task commands.
func remoteGitPullApplyFiles(ctx context.Context,root,reviewDigest string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!validRemoteGitSwitchDigest(reviewDigest){
  return "",denied
 }
 if !remoteGitPullFilesWorktreeConfigSafe(ctx,root){return "",denied}
 preview,err:=remoteGitPullChangesReview(ctx,root)
 if err!=nil||!remotetransport.ValidRemoteGitPullChangesReport(string(preview)){
  return "",denied
 }
 hash:=sha256.Sum256(preview)
 if hex.EncodeToString(hash[:])!=reviewDigest{return "",denied}
 lines:=strings.Split(string(preview),"\n")
 if len(lines)<10{return "",denied}
 branch:=strings.TrimPrefix(lines[1],"BRANCH ")
 old:=strings.TrimPrefix(lines[2],"HEAD ")
 upstream:=strings.TrimPrefix(lines[3],"UPSTREAM ")
 target:=strings.TrimPrefix(lines[4],"UPSTREAM_HEAD ")
 if !remoteValidGitOIDForCommitReview(old)||
  !remoteValidGitOIDForCommitReview(target)||len(old)!=len(target)||
  !remoteGitPullFilesMetadataSafe(root,branch,upstream){return "",denied}

 // Git itself owns the index, HEAD and ref locks for the ff-only operation.
 // Do not pre-create index.lock or HEAD.lock: Git's checkout and ref update
 // need them. Refuse any outstanding foreign locks before starting.
 for _,relative:=range []string{"index.lock","HEAD.lock"}{
  if _,e:=os.Lstat(filepath.Join(root,".git",relative));!os.IsNotExist(e){
   return "",denied
  }
 }
 idxPath:=filepath.Join(root,".git","index")
 originalIndex,err:=remoteGitPullFilesIndex(ctx,idxPath)
 if err!=nil{return "",denied}
 git,err:=remoteTrustedGitExecutable(root)
 if err!=nil||remoteGitReadOnlyConfigSafe(ctx,root,git)!=nil||
  !remoteGitPullFilesWorktreeConfigSafe(ctx,root){return "",denied}

 // File manifest and tree entries must be small, complete, conventional
 // regular files, never symlinks, submodules, or Git pathspec metacharacters.
 // Existing filesystem ancestors must be real directories, never symlinks.
 for _,line:=range lines[8:len(lines)-1]{
  if len(line)<3||line[1]!=' '{return "",denied}
  kind,path:=line[:1],line[2:]
  if !remotetransport.ValidRemotePullChangedPath(path)||
   !remoteGitPullFilesSafeWorktreePath(root,path,kind){return "",denied}
  from,e:=remoteGitPullFilesTreeEntry(ctx,git,root,old,path)
  if e!=nil{return "",denied}
  to,e:=remoteGitPullFilesTreeEntry(ctx,git,root,target,path)
  if e!=nil{return "",denied}
  switch kind{
  case "A":
   if from!=""||to==""{return "",denied}
  case "M":
   if from==""||to==""{return "",denied}
  case "D":
   if from==""||to!=""{return "",denied}
  default:return "",denied
  }
 }
 // Recheck all host-owned inputs immediately before touching workspace.
 current,err:=remoteGitPullChangesReview(ctx,root)
 if err!=nil||!bytes.Equal(current,preview)||ctx.Err()!=nil||
  !remoteGitPullFilesWorktreeConfigSafe(ctx,root){return "",denied}
 again,err:=remoteGitPullFilesIndex(ctx,idxPath)
 if err!=nil||!bytes.Equal(again,originalIndex)||
  !remoteGitPullFilesMetadataSafe(root,branch,upstream){return "",denied}
 // Fixed Git built-in with ff-only, hooks, external filters and network
 // disabled. It updates refs/index/worktree under Git's OWN locking; it
 // cannot apply a merge commit or choose a client supplied refspec. Local
 // non-cooperating writers remain a separate real-host concurrency risk.
 cmd:=exec.CommandContext(ctx,git,
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
  "-c","merge.ff=only","-c","merge.autostash=false",
  "-c","submodule.recurse=false","-c","commit.gpgsign=false",
  "-c","merge.gpgsign=false",
  "-C",root,"merge","--ff-only","--no-stat","--no-edit",target)
 cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1",
  "GIT_DIR="+filepath.Join(root,".git"),"GIT_WORK_TREE="+root)
 cmd.Stderr=io.Discard
 cmd.Stdout=io.Discard
 if cmd.Run()!=nil||ctx.Err()!=nil{
  // An uncertain/failed Git checkout is NEVER retried by TaskDeck.
  return "",denied
 }
 // The target and branch MUST match the exact reviewed values. Failure
 // after the merge is reported as uncertain, not rolled back silently.
 nowBranch,nowHead,_,e:=remoteGitBranchRead(ctx,root)
 if e!=nil||nowBranch!=branch||nowHead!=target{return "",denied}
 status,e:=remoteGitPullReview(ctx,root)
 if e!=nil||!remotetransport.ValidRemoteGitPullReviewReport(string(status))||
  !strings.Contains(string(status),"HEAD "+target+"\n")||
  !strings.Contains(string(status),"STATUS up-to-date\n")||
  !remoteGitPullFilesMetadataSafe(root,branch,upstream){return "",denied}
 return target,nil
}

// A local core.worktree directive could redirect a normal git -C root
// checkout into a different directory. No remote privilege can authorize
// writes outside the host workspace. Standard TaskDeck repositories use
// a regular .git directory and do not require core.worktree overrides.
func remoteGitPullFilesWorktreeConfigSafe(ctx context.Context,root string)bool{
 if ctx==nil||ctx.Err()!=nil||root==""||
  !remoteGitCachedMetadataDirectoriesSafe(root){return false}
 gitDir:=filepath.Join(root,".git")
 // Linked worktrees and alternate/common object stores are not supported:
 // Git can redirect refs, locks or object access outside this workspace.
 for _,relative:=range []string{"commondir","config.worktree",
  filepath.Join("objects","info","alternates")}{
  if _,e:=os.Lstat(filepath.Join(gitDir,relative));!os.IsNotExist(e){
   return false
  }
 }
 for _,relative:=range []string{"config","objects"}{
  info,e:=os.Lstat(filepath.Join(gitDir,relative))
  if e!=nil||info.Mode()&os.ModeSymlink!=0{return false}
  if relative=="config"&&!info.Mode().IsRegular(){return false}
  if relative=="objects"&&!info.IsDir(){return false}
 }
 for _,relative:=range []string{
  filepath.Join("objects","info"),filepath.Join("objects","pack"),
 }{
  if info,e:=os.Lstat(filepath.Join(gitDir,relative));e==nil{
   if !info.IsDir()||info.Mode()&os.ModeSymlink!=0{return false}
  }else if !os.IsNotExist(e){return false}
 }
 git,e:=remoteTrustedGitExecutable(root)
 if e!=nil{return false}
 cmd:=exec.CommandContext(ctx,git,
  "-c","core.hooksPath="+os.DevNull,"-C",root,
  "config","--local","--includes","--name-only","--get-regexp",
  "^core\\.worktree$")
 cmd.Env=isolatedRemoteGitEnvironment()
 cmd.Stdout=io.Discard
 cmd.Stderr=io.Discard
 err:=cmd.Run()
 if ctx.Err()!=nil||err==nil{return false}
 var exit *exec.ExitError
 return errors.As(err,&exit)&&exit.ExitCode()==1
}

func remoteGitPullFilesIndex(ctx context.Context,path string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil{return nil,denied}
 info,err:=os.Lstat(path)
 if err!=nil||!info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0||
  info.Size()<1||info.Size()>1<<20{return nil,denied}
 f,e:=os.Open(path)
 if e!=nil{return nil,denied}
 defer f.Close()
 data,e:=io.ReadAll(io.LimitReader(f,1<<20+1))
 if e!=nil||len(data)<1||len(data)>1<<20{return nil,denied}
 return data,nil
}

func remoteGitPullFilesMetadataSafe(root,branch,upstream string)bool{
 if !remoteGitCachedMetadataDirectoriesSafe(root)||
  !remoteGitCachedUpstreamRefSafe(root,upstream)||
  !strings.HasPrefix(branch,"refs/heads/"){return false}
 name:=strings.TrimPrefix(branch,"refs/heads/")
 if !remoteGitPullBranchSafe(name){return false}
 parts:=strings.Split(name,"/")
 for _,base:=range []string{
  filepath.Join(root,".git","refs","heads"),
  filepath.Join(root,".git","logs","refs","heads"),
 }{
  parent:=base
  for _,segment:=range parts[:len(parts)-1]{
   parent=filepath.Join(parent,segment)
   info,err:=os.Lstat(parent)
   if err!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{return false}
  }
  leaf:=filepath.Join(parent,parts[len(parts)-1])
  info,err:=os.Lstat(leaf)
  if err!=nil||!info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0{
   return false
  }
 }
 headPath:=filepath.Join(root,".git","HEAD")
 info,err:=os.Lstat(headPath)
 if err!=nil||!info.Mode().IsRegular()||info.Size()>256{return false}
 bytes,e:=os.ReadFile(headPath)
 if e!=nil||string(bytes)!="ref: "+branch+"\n"{return false}
 // A ff-only merge writes ORIG_HEAD and may touch MERGE_* metadata. No
 // symlink may redirect those ancillary writes into foreign files.
 for _,leaf:=range []string{
  "ORIG_HEAD","MERGE_HEAD","MERGE_MSG","MERGE_MODE","MERGE_AUTOSTASH",
  "AUTO_MERGE",
 }{
  info,e:=os.Lstat(filepath.Join(root,".git",leaf))
  if os.IsNotExist(e){continue}
  if e!=nil||!info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0{
   return false
  }
  // An in-progress merge/rebase is not an admissible clean ff preflight.
  if leaf!="ORIG_HEAD"{return false}
 }
 return true
}

func remoteGitPullFilesSafeWorktreePath(root,path,kind string)bool{
 if !remotetransport.ValidRemotePullChangedPath(path){return false}
 parts:=strings.Split(path,"/")
 parent:=root
 for _,component:=range parts[:len(parts)-1]{
  parent=filepath.Join(parent,component)
  info,err:=os.Lstat(parent)
  if os.IsNotExist(err){continue}
  if err!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{return false}
 }
 leaf:=filepath.Join(root,filepath.FromSlash(path))
 info,e:=os.Lstat(leaf)
 if kind=="A"{return os.IsNotExist(e)}
 return e==nil&&info.Mode().IsRegular()&&info.Mode()&os.ModeSymlink==0
}

// ls-tree may otherwise produce an entire directory, a gitlink or a
// symlink for the same apparent file name. Only exact single-file regular
// blobs are valid; path is a host-derived, strictly checked literal.
func remoteGitPullFilesTreeEntry(ctx context.Context,git,root,oid,path string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!remotetransport.ValidRemotePullChangedPath(path){
  return "",denied
 }
 cmd:=exec.CommandContext(ctx,git,"--literal-pathspecs",
  "-c","core.hooksPath="+os.DevNull,
  "-C",root,"ls-tree","-z","-r","--full-tree",oid,"--",path)
 cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1",
  "GIT_DIR="+filepath.Join(root,".git"),"GIT_WORK_TREE="+root)
 cmd.Stderr=io.Discard
 var out cappedRemoteGitDiffOutput
 cmd.Stdout=&out
 if cmd.Run()!=nil||ctx.Err()!=nil{return "",denied}
 data:=out.Bytes()
 if len(data)==0{return "",nil}
 if len(data)<2||data[len(data)-1]!=0||
  bytes.Count(data,[]byte{0})!=1{return "",denied}
 parts:=bytes.SplitN(data[:len(data)-1],[]byte{'\t'},2)
 if len(parts)!=2||string(parts[1])!=path{return "",denied}
 fields:=strings.Fields(string(parts[0]))
 if len(fields)!=3||fields[1]!="blob"||
  (fields[0]!="100644"&&fields[0]!="100755")||
  !remoteValidGitOIDForCommitReview(fields[2]){return "",denied}
 return fields[2],nil
}
