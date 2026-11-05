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

// The only writable namespace is refs/heads/taskdeck/<canonical-slug>.
// This action neither changes HEAD nor modifies the working tree/index.
// It never accepts arbitrary Git argv, parent OIDs, refnames or a shell.
func remoteGitBranchRead(ctx context.Context,root string)(string,string,string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||root==""{return "","","",denied}
 gitDir:=filepath.Join(root,".git")
 for _,dir:=range []string{
  gitDir,filepath.Join(gitDir,"objects"),filepath.Join(gitDir,"refs"),
  filepath.Join(gitDir,"refs","heads"),
 }{
  info,err:=os.Lstat(dir)
  if err!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{
   return "","","",denied
  }
 }
 // Symlinks in ref/log parents and Git alternates can redirect updates,
 // cause executable paths or expose object stores outside this project.
 for _,candidate:=range []string{
  filepath.Join(gitDir,"objects","info","alternates"),
 }{
  if _,err:=os.Lstat(candidate);err==nil||!os.IsNotExist(err){
   return "","","",denied
  }
 }
 for _,candidate:=range []string{
  filepath.Join(gitDir,"refs","heads","taskdeck"),
  filepath.Join(gitDir,"logs"),
  filepath.Join(gitDir,"logs","refs"),
  filepath.Join(gitDir,"logs","refs","heads"),
  filepath.Join(gitDir,"logs","refs","heads","taskdeck"),
 }{
  info,err:=os.Lstat(candidate)
  if err==nil {
   if !info.IsDir()||info.Mode()&os.ModeSymlink!=0{return "","","",denied}
  }else if !os.IsNotExist(err){return "","","",denied}
 }
 for _,candidate:=range []string{
  filepath.Join(gitDir,"packed-refs"),
 }{
  info,err:=os.Lstat(candidate)
  if err==nil{
   if !info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0{
    return "","","",denied
   }
  }else if !os.IsNotExist(err){return "","","",denied}
 }
 executable,err:=remoteTrustedGitExecutable(root)
 if err!=nil{return "","","",denied}
 if err:=remoteGitReadOnlyConfigSafe(ctx,root,executable);err!=nil{
  return "","","",denied
 }
 run:=func(args ...string)(string,error){
  fixed:=[]string{
   "-c","core.hooksPath="+os.DevNull,
   "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
   "-c","diff.external=","-C",root,
  }
  cmd:=exec.CommandContext(ctx,executable,append(fixed,args...)...)
  cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
  cmd.Stderr=io.Discard
  var out cappedRemoteGitDiffOutput
  cmd.Stdout=&out
  if err:=cmd.Run();err!=nil||ctx.Err()!=nil{return "",denied}
  return strings.TrimSpace(string(out.Bytes())),nil
 }
 top,err:=run("rev-parse","--show-toplevel")
 if err!=nil||filepath.Clean(top)!=filepath.Clean(root){
  return "","","",denied
 }
 branch,err:=run("symbolic-ref","-q","HEAD")
 if err!=nil||!strings.HasPrefix(branch,"refs/heads/")||
  len(branch)>160||strings.ContainsAny(branch,"\r\n\x00"){
  return "","","",denied
 }
 oid,err:=run("rev-parse","--verify","HEAD")
 if err!=nil||!remoteValidGitOIDForCommitReview(oid){
  return "","","",denied
 }
 return branch,oid,executable,nil
}

func remoteGitBranchReview(ctx context.Context,root string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 branch,oid,_,err:=remoteGitBranchRead(ctx,root)
 if err!=nil{return nil,denied}
 // Read twice: local Git operations are allowed to run independently.
 // There is no promise to snapshot an external writer's ref in this step.
 afterBranch,afterOID,_,err:=remoteGitBranchRead(ctx,root)
 if err!=nil||branch!=afterBranch||oid!=afterOID{return nil,denied}
 return []byte("HEAD "+oid+"\nBRANCH "+branch+"\n"),nil
}

// CAS creation protects against replacing ANY existing loose or packed
// ref. A local operator must enable TWO host-only flags and grant GitWrite;
// the caller also requires pre-mutation audit and live TaskDeck session.
func remoteGitBranchCreate(ctx context.Context,root,slug,reviewSHA string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||
  !remotetransport.ValidRemoteTaskDeckBranchSlug(slug)||
  len(reviewSHA)!=64{return "",denied}
 for _,c:=range reviewSHA {
  if !((c>='a'&&c<='f')||(c>='0'&&c<='9')){return "",denied}
 }
 review,err:=remoteGitBranchReview(ctx,root)
 if err!=nil{return "",denied}
 digest:=sha256.Sum256(review)
 if hex.EncodeToString(digest[:])!=reviewSHA{return "",denied}
 lines:=bytes.Split(review,[]byte{10})
 if len(lines)!=3||len(lines[2])!=0||
  !bytes.HasPrefix(lines[0],[]byte("HEAD "))||
  !bytes.HasPrefix(lines[1],[]byte("BRANCH refs/heads/")){
  return "",denied
 }
 oid:=strings.TrimPrefix(string(lines[0]),"HEAD ")
 if !remoteValidGitOIDForCommitReview(oid){return "",denied}
 // The lock-and-ref CAS takes place inside the authenticated session
 // executionMu, and only a fixed canonical TaskDeck ref can be created.
 _,current,executable,err:=remoteGitBranchRead(ctx,root)
 if err!=nil||current!=oid{return "",denied}
 ref:="refs/heads/taskdeck/"+slug
 // If a local actor created a loose leaf/symlink/symbolic-ref, do
 // not follow it. Packed refs are separately protected by zero-old CAS.
 leaf:=filepath.Join(root,".git","refs","heads","taskdeck",slug)
 if _,e:=os.Lstat(leaf);e==nil||!os.IsNotExist(e){return "",denied}
 emptyOID:=strings.Repeat("0",len(oid))
 cmd:=exec.CommandContext(ctx,executable,
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false",
  "-c","core.untrackedCache=false",
  "-C",root,
  "update-ref","--no-deref","-m","TaskDeck remote approved branch create",
  ref,oid,emptyOID)
 cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
 cmd.Stderr=io.Discard
 var out cappedRemoteGitDiffOutput
 cmd.Stdout=&out
 if err=cmd.Run();err!=nil||ctx.Err()!=nil||len(out.Bytes())!=0{
  return "",denied
 }
 verify:=exec.CommandContext(ctx,executable,"-C",root,
  "rev-parse","--verify",ref)
 verify.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
 verify.Stderr=io.Discard
 var result cappedRemoteGitDiffOutput
 verify.Stdout=&result
 if err=verify.Run();err!=nil||ctx.Err()!=nil||
  strings.TrimSpace(string(result.Bytes()))!=oid{return "",denied}
 return oid,nil
}
