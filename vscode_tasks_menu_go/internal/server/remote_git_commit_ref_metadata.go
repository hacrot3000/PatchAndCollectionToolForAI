package server

import (
 "os"
 "path/filepath"
 "strings"
)

// A reviewed Git commit writes a branch ref and can append to both HEAD
// and branch reflogs through Git's update-ref plumbing. The Git executable
// is trusted, but an untrusted repository may contain symlinked metadata
// redirecting those writes outside the TaskDeck workspace. Reject it
// BEFORE object creation and again while index.lock is held before CAS.
func remoteGitCommitRefMetadataSafe(root,ref string)bool{
 const prefix="refs/heads/"
 if !strings.HasPrefix(ref,prefix)||!remoteGitCachedMetadataDirectoriesSafe(root){
  return false
 }
 name:=strings.TrimPrefix(ref,prefix)
 if !remoteGitPullBranchSafe(name){return false}
 gitDir:=filepath.Join(root,".git")
 head,err:=os.ReadFile(filepath.Join(gitDir,"HEAD"))
 if err!=nil||string(head)!="ref: "+ref+"\n"{return false}
 parts:=strings.Split(name,"/")
 // Both the branch metadata and its reflog must be reached through
 // real directories, not links or bindable arbitrary paths.
 for _,base:=range []string{
  filepath.Join(gitDir,"refs","heads"),
  filepath.Join(gitDir,"logs","refs","heads"),
 }{
  parent:=base
  for _,part:=range parts[:len(parts)-1]{
   parent=filepath.Join(parent,part)
   info,err:=os.Lstat(parent)
   if err!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{
    return false
   }
  }
 }
 for _,entry:=range []struct{path string;size int64}{
  {filepath.Join(gitDir,"refs","heads",filepath.FromSlash(name)),128},
  {filepath.Join(gitDir,"logs","refs","heads",filepath.FromSlash(name)),1<<20},
 }{
  info,err:=os.Lstat(entry.path)
  if os.IsNotExist(err){continue} // packed branch or reflog not created yet
  if err!=nil||!info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0||
   info.Size()>entry.size{return false}
 }
 return true
}
