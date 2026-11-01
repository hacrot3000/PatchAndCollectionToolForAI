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
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A commit-review is strictly read-only. Git commit and index writes are NOT
// authorized by this action. The entire staged diff must fit the bounded
// encrypted result; never silently show a partial "approved" commit.
func remoteGitCommitReview(ctx context.Context,root string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||root==""{return nil,denied}
 gitDir:=filepath.Join(root,".git")
 dir,err:=os.Lstat(gitDir)
 if err!=nil||!dir.IsDir()||dir.Mode()&os.ModeSymlink!=0{return nil,denied}
 objects:=filepath.Join(gitDir,"objects")
 objectInfo,err:=os.Lstat(objects)
 if err!=nil||!objectInfo.IsDir()||objectInfo.Mode()&os.ModeSymlink!=0{return nil,denied}
 // Git alternate object stores can disclose commits outside the approved
 // workspace. They are not supported by this whole-index review.
 alternate:=filepath.Join(objects,"info","alternates")
 if _,e:=os.Lstat(alternate);e==nil||!os.IsNotExist(e){return nil,denied}
 executable,err:=remoteTrustedGitExecutable(root)
 if err!=nil{return nil,denied}
 if err:=remoteGitReadOnlyConfigSafe(ctx,root,executable);err!=nil{return nil,denied}
 git:=func(args ...string)([]byte,error){
  fixed:=[]string{"-c","core.fsmonitor=false",
   "-c","core.untrackedCache=false","-c","diff.external=","-C",root}
  cmd:=exec.CommandContext(ctx,executable,append(fixed,args...)...)
  cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
  cmd.Stderr=io.Discard
  var out cappedRemoteGitDiffOutput
  cmd.Stdout=&out
  if err:=cmd.Run();err!=nil||ctx.Err()!=nil{return nil,denied}
  return append([]byte(nil),out.Bytes()...),nil
 }
 indexDigest:=func()(string,error){
  path:=filepath.Join(gitDir,"index")
  info,err:=os.Lstat(path)
  if err!=nil||!info.Mode().IsRegular()||info.Size()>1<<20||
   info.Mode()&os.ModeSymlink!=0{return "",denied}
  f,err:=os.Open(path)
  if err!=nil{return "",denied}
  defer f.Close()
  body,err:=io.ReadAll(io.LimitReader(f,1<<20+1))
  if err!=nil||len(body)==0||len(body)>1<<20{return "",denied}
  sum:=sha256.Sum256(body)
  return hex.EncodeToString(sum[:]),nil
 }
 first,err:=indexDigest()
 if err!=nil{return nil,denied}
 branch,err:=git("symbolic-ref","-q","HEAD")
 if err!=nil{return nil,denied} // detached HEAD is not committable here
 name:=strings.TrimSuffix(string(branch),"\n")
 if !strings.HasPrefix(name,"refs/heads/")||len(name)>160||
  strings.ContainsAny(name,"\r\n\x00")||!utf8.ValidString(name){
  return nil,denied
 }
 head,err:=git("rev-parse","--verify","HEAD")
 if err!=nil{return nil,denied}
 oldOID:=strings.TrimSpace(string(head))
 if !remoteValidGitOIDForCommitReview(oldOID){return nil,denied}
 staged,err:=git("--no-pager","diff","--cached",
  "--no-ext-diff","--no-textconv","--no-color","--no-renames","--")
 if err!=nil||len(staged)==0||!utf8.Valid(staged)||
  bytes.Contains(staged,[]byte("Binary files "))||
  bytes.Contains(staged,[]byte("GIT binary patch")){return nil,denied}
 second,err:=indexDigest()
 if err!=nil||first!=second{return nil,denied}
 // Revalidate HEAD/branch after Git diff; an external Git process may have
 // switched branches while the preview was assembled.
 branchAfter,err:=git("symbolic-ref","-q","HEAD")
 if err!=nil||!bytes.Equal(branchAfter,branch){return nil,denied}
 headAfter,err:=git("rev-parse","--verify","HEAD")
 if err!=nil||!bytes.Equal(headAfter,head){return nil,denied}
 report:=[]byte("HEAD "+oldOID+"\nBRANCH "+name+
  "\nINDEX_SHA256 "+first+"\n-- COMPLETE STAGED DIFF --\n")
 if len(report)+len(staged)>remotetransport.RemoteGitDiffMaxBytes{
  return nil,denied
 }
 return append(report,staged...),nil
}

func remoteValidGitOIDForCommitReview(value string)bool{
 if len(value)!=40&&len(value)!=64{return false}
 for _,c:=range value{
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return false}
 }
 return true
}
