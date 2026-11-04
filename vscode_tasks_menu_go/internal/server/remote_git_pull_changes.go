package server

import (
 "bytes"
 "context"
 "fmt"
 "io"
 "os"
 "os/exec"
 "strconv"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Read-only inventory of path changes between locally available, already
// verified fast-forward commits. No network, checkout, ref update, hooks,
// externally supplied Git command, refspec, remote URL or credentials.
// This deliberately fails closed on unsafe names or >32 changed paths;
// it is not a promise that tree-changing Pull can be executed remotely.
func remoteGitPullChangesReview(ctx context.Context,root string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||root==""{return nil,denied}
 initial,err:=remoteGitPullReview(ctx,root)
 if err!=nil||!remotetransport.ValidRemoteGitPullReviewReport(string(initial)){
  return nil,denied
 }
 lines:=strings.Split(string(initial),"\n")
 if len(lines)!=9||lines[5]!="STATUS fast-forward"{return nil,denied}
 branch:=strings.TrimPrefix(lines[1],"BRANCH ")
 old:=strings.TrimPrefix(lines[2],"HEAD ")
 upstream:=strings.TrimPrefix(lines[3],"UPSTREAM ")
 target:=strings.TrimPrefix(lines[4],"UPSTREAM_HEAD ")
 if !remoteValidGitOIDForCommitReview(old)||
  !remoteValidGitOIDForCommitReview(target){return nil,denied}
 git,err:=remoteTrustedGitExecutable(root)
 if err!=nil||remoteGitReadOnlyConfigSafe(ctx,root,git)!=nil{
  return nil,denied
 }
 // -z yields unambiguous status/path NUL pairs. --no-renames means a
 // rename appears as a deleted and added file; no lossy summarization.
 cmd:=exec.CommandContext(ctx,git,
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false",
  "-c","core.untrackedCache=false",
  "-C",root,"diff-tree","--no-commit-id","-r",
  "--no-renames","--no-ext-diff","--no-textconv",
  "--name-status","-z",old,target,"--")
 cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
 cmd.Stderr=io.Discard
 var output cappedRemoteGitDiffOutput
 cmd.Stdout=&output
 if cmd.Run()!=nil||ctx.Err()!=nil{return nil,denied}
 raw:=output.Bytes()
 if len(raw)==0||len(raw)>remotetransport.RemoteGitDiffMaxBytes||
  raw[len(raw)-1]!=0{return nil,denied}
 fields:=bytes.Split(raw[:len(raw)-1],[]byte{0})
 if len(fields)<2||len(fields)%2!=0||
  len(fields)/2>remotetransport.RemoteGitPullChangesMaxFiles{return nil,denied}
 var records []string
 previous:=""
 for i:=0;i<len(fields);i+=2{
  status:=string(fields[i])
  path:=string(fields[i+1])
  if (status!="A"&&status!="M"&&status!="D")||
   !remotetransport.ValidRemotePullChangedPath(path)||
   path<=previous{return nil,denied}
  previous=path
  records=append(records,status+" "+path)
 }
 report:=fmt.Sprintf("GIT_PULL_CHANGES_V1\nBRANCH %s\nHEAD %s\nUPSTREAM %s\nUPSTREAM_HEAD %s\nFILES %s\nNETWORK none\n-- FILE CHANGES --\n%s\n",
  branch,old,upstream,target,strconv.Itoa(len(records)),strings.Join(records,"\n"))
 if !remotetransport.ValidRemoteGitPullChangesReport(report){
  return nil,denied
 }
 // The target and clean worktree must still match the pinned preflight
 // after walking every changed entry; an intervening local checkout or
 // fetch invalidates this entire preview, not just its display label.
 latest,err:=remoteGitPullReview(ctx,root)
 if err!=nil||!bytes.Equal(latest,initial)||ctx.Err()!=nil{return nil,denied}
 return []byte(report),nil
}
