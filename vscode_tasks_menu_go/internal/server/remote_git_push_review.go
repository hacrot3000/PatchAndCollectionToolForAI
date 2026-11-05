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
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Credential storage stays in a locally installed OS credential manager.
// No remote browser or PHP input can select a credential executable.
func remoteGitHTTPSPushHelper(root string)(string,bool){
 path:=os.Getenv("TASKDECK_REMOTE_GIT_PUSH_HELPER")
 allowed:=map[string]bool{
  "/usr/bin/git-credential-libsecret":true,
  "/usr/lib/git-core/git-credential-libsecret":true,
  "/usr/bin/git-credential-manager":true,
  "/usr/local/bin/git-credential-manager":true,
  "/opt/homebrew/bin/git-credential-manager":true,
  "/usr/lib/git-core/git-credential-manager":true,
  "/usr/bin/git-credential-osxkeychain":true,
  "/usr/libexec/git-core/git-credential-osxkeychain":true,
 }
 return path,allowed[path]&&tasks.RemoteExecutableAllowed(root,path)
}

func remoteGitPushRead(ctx context.Context,git,root string,args ...string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil{return "",denied}
 fixed:=[]string{"-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false","-c","core.untrackedCache=false","-C",root}
 cmd:=exec.CommandContext(ctx,git,append(fixed,args...)...)
 cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
 cmd.Stderr=io.Discard
 var buf cappedRemoteGitDiffOutput
 cmd.Stdout=&buf
 if cmd.Run()!=nil||ctx.Err()!=nil{return "",denied}
 return strings.TrimSpace(string(buf.Bytes())),nil
}

func remoteGitHTTPSPushReview(ctx context.Context,root string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!remoteGitPullFilesWorktreeConfigSafe(ctx,root)||
  !remoteGitHTTPSFetchObjectStoreSafe(root){return nil,denied}
 pinnedURL:=os.Getenv("TASKDECK_REMOTE_GIT_FETCH_URL")
 host,valid:=remoteGitTrustedFetchURL(pinnedURL)
 if !valid{return nil,denied}
 helper,valid:=remoteGitHTTPSPushHelper(root)
 if !valid{return nil,denied}
 branch,head,git,err:=remoteGitBranchRead(ctx,root)
 if err!=nil||!strings.HasPrefix(branch,"refs/heads/")||
  !remoteValidGitOIDForCommitReview(head){return nil,denied}
 name:=strings.TrimPrefix(branch,"refs/heads/")
 upstream:="refs/remotes/origin/"+name
 if !remoteGitPullBranchSafe(name)||
  !remoteGitHTTPSFetchMetadataSafe(root,branch,upstream)||
  remoteGitReadOnlyConfigSafe(ctx,root,git)!=nil||
  !remoteGitHTTPSFetchConfigKeysSafe(ctx,root,git,name)||
  !remoteGitHTTPSFetchConfigOriginSafe(ctx,root,git,pinnedURL)||
  !remoteGitHTTPSFetchWorktreeClean(ctx,root,git){return nil,denied}
 remoteOID,err:=remoteGitPushRead(ctx,git,root,
  "rev-parse","--verify",upstream+"^{commit}")
 if err!=nil||!remoteValidGitOIDForCommitReview(remoteOID)||
  remoteOID==head||len(remoteOID)!=len(head){return nil,denied}
 // Explicit ancestor proof is mandatory. Do not trust ahead counters.
 _,err=remoteGitPushRead(ctx,git,root,
  "merge-base","--is-ancestor",remoteOID,head)
 if err!=nil{return nil,denied}
 index,err:=remoteGitPullFilesIndex(ctx,filepath.Join(root,".git","index"))
 if err!=nil{return nil,denied}
 config,err:=remoteGitHTTPSFetchConfigBytes(root)
 if err!=nil{return nil,denied}
 urlSHA:=sha256.Sum256([]byte(pinnedURL))
 configSHA:=sha256.Sum256(config)
 indexSHA:=sha256.Sum256(index)
 helperSHA:=sha256.Sum256([]byte(helper))
 report:="GIT_PUSH_HTTPS_REVIEW_V1\nBRANCH "+branch+
  "\nHEAD "+head+"\nUPSTREAM "+upstream+
  "\nUPSTREAM_HEAD "+remoteOID+"\nHTTPS_HOST "+host+
  "\nURL_SHA256 "+hex.EncodeToString(urlSHA[:])+
  "\nCONFIG_SHA256 "+hex.EncodeToString(configSHA[:])+
  "\nINDEX_SHA256 "+hex.EncodeToString(indexSHA[:])+
  "\nHELPER_SHA256 "+hex.EncodeToString(helperSHA[:])+
  "\nSTATUS fast-forward-only\nWORKTREE clean"+
  "\nREMOTE_REF existing\nNETWORK HTTPS-approval-required\n"
 if !remotetransport.ValidRemoteGitPushReviewReport(report){return nil,denied}
 againBranch,againHead,_,err:=remoteGitBranchRead(ctx,root)
 if err!=nil||againBranch!=branch||againHead!=head||
  !remoteGitHTTPSFetchWorktreeClean(ctx,root,git){return nil,denied}
 after,err:=remoteGitHTTPSFetchConfigBytes(root)
 if err!=nil||!bytes.Equal(config,after){return nil,denied}
 later,err:=remoteGitPullFilesIndex(ctx,filepath.Join(root,".git","index"))
 if err!=nil||!bytes.Equal(index,later){return nil,denied}
 return []byte(report),nil
}
