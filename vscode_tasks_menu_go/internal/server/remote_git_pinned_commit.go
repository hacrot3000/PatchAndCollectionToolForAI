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

// Must be called ONLY inside a locally authenticated TaskDeck session's
// executionMu, after host audit and uncached RBAC+Portal grant checks.
// No Git command line, parent, ref, identity or hook is accepted from PHP,
// the Relay, or the remote operator. Never use git commit, shell or git add.
func remoteGitCommitPinned(ctx context.Context,root,message,expectedReviewSHA string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!remotetransport.ValidRemoteCommitMessage(message)||
  len(expectedReviewSHA)!=64{return "",denied}
 for _,c:=range expectedReviewSHA{
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return "",denied}
 }
 // The existing review already refuses a detached HEAD, executable
 // filters, .git symlinks, alternates, binary patch and truncated diff.
 review,err:=remoteGitCommitReview(ctx,root)
 if err!=nil{return "",denied}
 digest:=sha256.Sum256(review)
 if hex.EncodeToString(digest[:])!=expectedReviewSHA{return "",denied}
 lines:=strings.SplitN(string(review),"\n",5)
 if len(lines)!=5||
  !strings.HasPrefix(lines[0],"HEAD ")||
  !strings.HasPrefix(lines[1],"BRANCH refs/heads/")||
  !strings.HasPrefix(lines[2],"INDEX_SHA256 ")||
  lines[3]!="-- COMPLETE STAGED DIFF --"||
  len(lines[4])==0{return "",denied}
 oldOID:=strings.TrimPrefix(lines[0],"HEAD ")
 ref:=strings.TrimPrefix(lines[1],"BRANCH ")
 indexSHA:=strings.TrimPrefix(lines[2],"INDEX_SHA256 ")
 if !remoteValidGitOIDForCommitReview(oldOID)||
  !strings.HasPrefix(ref,"refs/heads/")||
  strings.ContainsAny(ref,"\r\n\x00 ~^:?*[\\")||
  strings.Contains(ref,"..")||strings.HasSuffix(ref,".")||
  len(ref)>160||len(indexSHA)!=64{return "",denied}
 gitDir:=filepath.Join(root,".git")
 indexPath:=filepath.Join(gitDir,"index")
 stat,err:=os.Lstat(indexPath)
 if err!=nil||!stat.Mode().IsRegular()||stat.Mode()&os.ModeSymlink!=0||
  stat.Size()<1||stat.Size()>1<<20{return "",denied}
 readIndex:=func()([]byte,error){
  now,err:=os.Lstat(indexPath)
  if err!=nil||!now.Mode().IsRegular()||now.Mode()&os.ModeSymlink!=0||
   now.Size()<1||now.Size()>1<<20{return nil,denied}
  f,err:=os.Open(indexPath)
  if err!=nil{return nil,denied}
  defer f.Close()
  data,err:=io.ReadAll(io.LimitReader(f,1<<20+1))
  if err!=nil||len(data)<1||len(data)>1<<20{return nil,denied}
  h:=sha256.Sum256(data)
  if hex.EncodeToString(h[:])!=indexSHA{return nil,denied}
  return data,nil
 }
 snapshot,err:=readIndex()
 if err!=nil{return "",denied}
 executable,err:=remoteTrustedGitExecutable(root)
 if err!=nil{return "",denied}
 // A private index snapshot decouples write-tree from the live Git index.
 // The remote client cannot select an alternate index path or content.
 f,err:=os.CreateTemp("","taskdeck-git-index-")
 if err!=nil{return "",denied}
 snapshotPath:=f.Name()
 defer os.Remove(snapshotPath)
 if _,err=f.Write(snapshot);err!=nil{_ = f.Close();return "",denied}
 if err=f.Sync();err!=nil{_ = f.Close();return "",denied}
 if err=f.Close();err!=nil{return "",denied}
 run:=func(input []byte,envExtra []string,args ...string)(string,error){
  fixed:=[]string{"-c","core.hooksPath="+os.DevNull,
   "-c","commit.gpgsign=false","-c","core.fsmonitor=false",
   "-c","core.untrackedCache=false","-c","diff.external=","-C",root}
  cmd:=exec.CommandContext(ctx,executable,append(fixed,args...)...)
  cmd.Env=append(isolatedRemoteGitEnvironment(),
   "GIT_NO_REPLACE_OBJECTS=1",
   "GIT_AUTHOR_NAME=TaskDeck Remote",
   "GIT_AUTHOR_EMAIL=taskdeck-remote@localhost.invalid",
   "GIT_COMMITTER_NAME=TaskDeck Remote",
   "GIT_COMMITTER_EMAIL=taskdeck-remote@localhost.invalid")
  cmd.Env=append(cmd.Env,envExtra...)
  cmd.Stdin=bytes.NewReader(input)
  cmd.Stderr=io.Discard
  var output cappedRemoteGitDiffOutput
  cmd.Stdout=&output
  if err:=cmd.Run();err!=nil||ctx.Err()!=nil{return "",denied}
  return strings.TrimSpace(string(output.Bytes())),nil
 }
 // A private index may contain a conflicted entry or unsupported filemode.
 // write-tree refuses unmerged stages and never consults working-tree bytes.
 tree,err:=run(nil,[]string{"GIT_INDEX_FILE="+snapshotPath},"write-tree")
 if err!=nil||!remoteValidGitOIDForCommitReview(tree){return "",denied}
 currentTree,err:=run(nil,nil,"rev-parse","--verify",oldOID+"^{tree}")
 if err!=nil||!remoteValidGitOIDForCommitReview(currentTree)||tree==currentTree{
  return "",denied // no empty commits or parent substitutions
 }
 // git commit-tree reads only the fixed tree+parent and message from stdin.
 // No Git commit hooks, user Git argv, config-defined editor or signing.
 newOID,err:=run([]byte(message+"\n"),nil,
  "commit-tree",tree,"-p",oldOID,"-F","-")
 if err!=nil||!remoteValidGitOIDForCommitReview(newOID){return "",denied}
 // A second CAS check before writing branch is mandatory. Git update-ref
 // itself checks expected-old-oid atomically, so competing HEAD changes
 // cannot be overwritten; external index writers still need a shared lock.
 again,err:=remoteGitCommitReview(ctx,root)
 if err!=nil||!bytes.Equal(again,review){return "",denied}
 if _,err=readIndex();err!=nil||ctx.Err()!=nil{return "",denied}
 currentBranch,err:=run(nil,nil,"symbolic-ref","-q","HEAD")
 if err!=nil||currentBranch!=ref{return "",denied}
 currentHead,err:=run(nil,nil,"rev-parse","--verify","HEAD")
 if err!=nil||currentHead!=oldOID{return "",denied}
 if _,err=run(nil,nil,"update-ref","-m","TaskDeck remote authorized commit",
  ref,newOID,oldOID);err!=nil{return "",denied}
 // No success without reading back the branch that was atomically updated.
 confirmed,err:=run(nil,nil,"rev-parse","--verify",ref)
 if err!=nil||confirmed!=newOID{return "",denied}
 return newOID,nil
}
