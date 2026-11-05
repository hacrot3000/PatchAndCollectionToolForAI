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

// A fixed non-forcing Push of the exact REVIEWED commit is used. Never
// add --force-with-lease: this flag itself can force an update despite
// --no-force, as independently reproduced with a real divergent Git repo.
func remoteGitHTTPSPushArgs(root,url,helper,branch,head string,write bool)[]string{
 args:=[]string{
  "-c","core.hooksPath="+os.DevNull,
  "-c","protocol.allow=never","-c","protocol.https.allow=always",
  "-c","http.followRedirects=false","-c","http.sslVerify=true",
  "-c","http.proxy=","-c","credential.helper=",
  "-c","credential.helper="+helper,
  "-c","push.default=nothing","-c","push.followTags=false",
  "-c","push.gpgSign=false","-c","gc.auto=0",
  "-c","maintenance.auto=false","-C",root,
 }
 if write{
  return append(args,"push","--porcelain","--no-force",
   "--no-verify","--no-follow-tags","--no-signed",
   url,head+":refs/heads/"+branch)
 }
 return append(args,"ls-remote","--exit-code","--heads",
  url,"refs/heads/"+branch)
}

func remoteGitHTTPSPushRemoteOID(ctx context.Context,git,root,url,helper,branch string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 cmd:=exec.CommandContext(ctx,git,remoteGitHTTPSPushArgs(
  root,url,helper,branch,"",false)...)
 cmd.Env=append(isolatedRemoteGitEnvironment(),
  "GIT_NO_REPLACE_OBJECTS=1",
  "GIT_ASKPASS=/bin/false",
  "GIT_DIR="+filepath.Join(root,".git"),"GIT_WORK_TREE="+root)
 cmd.Stderr=io.Discard
 var out cappedRemoteGitDiffOutput
 cmd.Stdout=&out
 if cmd.Run()!=nil||ctx.Err()!=nil{return "",denied}
 parts:=strings.Split(string(out.Bytes()),"\t")
 if len(parts)!=2||parts[1]!="refs/heads/"+branch+"\n"||
  !remoteValidGitOIDForCommitReview(parts[0]){return "",denied}
 return parts[0],nil
}

func remoteGitHTTPSPushApply(ctx context.Context,root,digest string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!validRemoteGitSwitchDigest(digest){return "",denied}
 review,err:=remoteGitHTTPSPushReview(ctx,root)
 if err!=nil{return "",denied}
 sum:=sha256.Sum256(review)
 if hex.EncodeToString(sum[:])!=digest{return "",denied}
 fields:=strings.Split(string(review),"\n")
 if len(fields)!=15{return "",denied}
 branch:=strings.TrimPrefix(fields[1],"BRANCH ")
 name:=strings.TrimPrefix(branch,"refs/heads/")
 head:=strings.TrimPrefix(fields[2],"HEAD ")
 tracking:=strings.TrimPrefix(fields[3],"UPSTREAM ")
 expectedRemote:=strings.TrimPrefix(fields[4],"UPSTREAM_HEAD ")
 url:=os.Getenv("TASKDECK_REMOTE_GIT_FETCH_URL")
 helper,ok:=remoteGitHTTPSPushHelper(root)
 if !ok{return "",denied}
 git,err:=remoteTrustedGitExecutable(root)
 if err!=nil{return "",denied}
 oldIndex,err:=remoteGitPullFilesIndex(ctx,filepath.Join(root,".git","index"))
 if err!=nil{return "",denied}
 // Compare the actual server ref with the reviewed local tracking ref.
 // No force, no branch creation, and no separate URL supplied by client.
 observed,err:=remoteGitHTTPSPushRemoteOID(ctx,git,root,url,helper,name)
 if err!=nil||observed!=expectedRemote{return "",denied}
 fresh,err:=remoteGitHTTPSPushReview(ctx,root)
 if err!=nil||!bytes.Equal(fresh,review)||ctx.Err()!=nil||
  !remoteGitHTTPSFetchMetadataSafe(root,branch,tracking){return "",denied}
 // The remote can still race after ls-remote. Git's *normal* push must
 // reject any non-FF update. A compatible concurrent update is possible;
 // the final ls-remote ensures we never claim a mismatched target as ours.
 cmd:=exec.CommandContext(ctx,git,remoteGitHTTPSPushArgs(
  root,url,helper,name,head,true)...)
 cmd.Env=append(isolatedRemoteGitEnvironment(),
  "GIT_NO_REPLACE_OBJECTS=1","GIT_ASKPASS=/bin/false",
  "GIT_DIR="+filepath.Join(root,".git"),"GIT_WORK_TREE="+root)
 cmd.Stdin=nil
 cmd.Stderr=io.Discard
 var out cappedRemoteGitDiffOutput
 cmd.Stdout=&out
 if cmd.Run()!=nil||ctx.Err()!=nil{return "",denied}
 remoteAfter,err:=remoteGitHTTPSPushRemoteOID(ctx,git,root,url,helper,name)
 if err!=nil||remoteAfter!=head{return "",denied}
 currentBranch,currentHead,_,err:=remoteGitBranchRead(ctx,root)
 if err!=nil||currentBranch!=branch||currentHead!=head||
  !remoteGitHTTPSFetchWorktreeClean(ctx,root,git)||
  !remoteGitHTTPSFetchMetadataSafe(root,branch,tracking){return "",denied}
 nowIndex,err:=remoteGitPullFilesIndex(ctx,filepath.Join(root,".git","index"))
 if err!=nil||!bytes.Equal(oldIndex,nowIndex){return "",denied}
 // A separately authorized Fetch refreshes refs/remotes/origin; no silent
 // local tracking ref updates or automatic retries after uncertain Push.
 return head,nil
}
