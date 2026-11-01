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

// remoteStagePinnedGitFile never runs git add: repository .gitattributes can
// specify clean filters that execute arbitrary programs. Instead write a
// filter-free blob from already-verified bytes, then update one index entry
// using trusted Git plumbing. Call ONLY under the local authenticated
// TaskDeck session executionMu after the host audit and uncached RBAC checks.
func remoteStagePinnedGitFile(ctx context.Context,root,relative string,content []byte,executableMode bool)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||len(content)>remotetransport.RemoteFilePreviewMaxBytes||
  !projectTextBytesValid(content)||relative==""||
  strings.HasPrefix(relative,"-")||strings.ContainsAny(relative,"\r\n\\")||
  strings.Contains(relative,"..") {
  return "",denied
 }
 // Only local, ordinary Git directories, not linked worktrees, alternate
 // repositories or a symlinked .git/index / object store, in this phase.
 gitDir:=filepath.Join(root,".git")
 gi,err:=os.Lstat(gitDir)
 if err!=nil||!gi.IsDir()||gi.Mode()&os.ModeSymlink!=0{return "",denied}
 objects,err:=os.Lstat(filepath.Join(gitDir,"objects"))
 if err!=nil||!objects.IsDir()||objects.Mode()&os.ModeSymlink!=0{return "",denied}
 index,err:=os.Lstat(filepath.Join(gitDir,"index"))
 if err==nil && (!index.Mode().IsRegular()||index.Mode()&os.ModeSymlink!=0){return "",denied}
 if err!=nil&&!os.IsNotExist(err){return "",denied}
 executable,err:=remoteTrustedGitExecutable(root)
 if err!=nil{return "",denied}
 common:=[]string{"-c","core.fsmonitor=false","-c","core.untrackedCache=false",
  "-c","core.hooksPath="+os.DevNull,"-C",root}
 run:=func(input []byte,args ...string)([]byte,error){
  cmd:=exec.CommandContext(ctx,executable,append(append([]string(nil),common...),args...)...)
  cmd.Env=isolatedRemoteGitEnvironment()
  cmd.Stdin=bytes.NewReader(input)
  cmd.Stderr=io.Discard
  var output cappedRemoteGitDiffOutput
  cmd.Stdout=&output
  if err:=cmd.Run();err!=nil||ctx.Err()!=nil{return nil,denied}
  return bytes.TrimSpace(output.Bytes()),nil
 }
 top,err:=run(nil,"rev-parse","--show-toplevel")
 if err!=nil||filepath.Clean(string(top))!=filepath.Clean(root){return "",denied}
 // No filters, no repository-supplied shell, no user Git args.
 oidBytes,err:=run(content,"hash-object","--no-filters","-w","--stdin")
 if err!=nil{return "",denied}
 oid:=string(oidBytes)
 if len(oid)!=40&&len(oid)!=64{return "",denied}
 for _,c:=range oid{
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return "",denied}
 }
 mode:="100644"
 if executableMode{mode="100755"}
 if _,err=run(nil,"update-index","--add","--cacheinfo",mode+","+oid+","+relative);err!=nil{
  return "",denied
 }
 // Verify exactly the intended staged blob without invoking filters.
 staged,err:=run(nil,"rev-parse","--verify",":"+relative)
 if err!=nil||string(staged)!=oid{return "",denied}
 sum:=sha256.Sum256(content)
 _=hex.EncodeToString(sum[:]) // Full source bytes were SHA pinned by the caller.
 return oid,nil
}
