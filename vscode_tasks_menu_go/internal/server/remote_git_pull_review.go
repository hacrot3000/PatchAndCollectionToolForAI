package server

import (
 "context"
 "io"
 "os"
 "os/exec"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// remoteGitPullReview checks ONLY the already-fetched local upstream.
// It does not fetch, contact a Git remote, update refs, merge or run hooks.
// The caller needs a real TaskDeck principal plus GitStatus, GitLog,
// FilesRead and local audit; this function never accepts a client URL,
// branch, command, environment or refspec.
//
// This is a prerequisite to implementing a separately gated network Pull,
// not permission to update the repository from an untrusted PHP portal.
func remoteGitPullReview(ctx context.Context,root string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||root==""{return nil,denied}
 branch,head,git,err:=remoteGitBranchRead(ctx,root)
 if err!=nil||!remoteGitPullBranchSafe(strings.TrimPrefix(branch,"refs/heads/")){
  return nil,denied
 }
 // A shallow clone cannot prove full ancestry from local objects.
 run:=func(args ...string)(string,error){
  fixed:=[]string{
   "-c","core.hooksPath="+os.DevNull,
   "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
   "-c","diff.external=","-c","log.showSignature=false",
   "-C",root,
  }
  cmd:=exec.CommandContext(ctx,git,append(fixed,args...)...)
  cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
  cmd.Stderr=io.Discard
  var output cappedRemoteGitDiffOutput
  cmd.Stdout=&output
  if err:=cmd.Run();err!=nil||ctx.Err()!=nil{return "",denied}
  return strings.TrimSuffix(string(output.Bytes()),"\n"),nil
 }
 if shallow,e:=run("rev-parse","--is-shallow-repository");e!=nil||shallow!="false"{
  return nil,denied
 }
 // @{upstream} is a FIXED symbolic name, not remotely supplied Git argv.
 // Never query a URL or read a remote token from repo config.
 upstream,e:=run("rev-parse","--symbolic-full-name","@{upstream}")
 if e!=nil||!strings.HasPrefix(upstream,"refs/remotes/")||
  strings.HasSuffix(upstream,"/HEAD")||
  !validRemoteTrackingName(strings.TrimPrefix(upstream,"refs/remotes/")){
  return nil,denied
 }
 target,e:=run("rev-parse","--verify",upstream+"^{commit}")
 if e!=nil||!remoteValidGitOIDForCommitReview(target)||len(target)!=len(head){
  return nil,denied
 }
 status:="up-to-date"
 if target!=head{
  // Do not infer fast-forward from ahead/behind counters or remote labels.
  // The full commit object ancestry must already be present locally.
  cmd:=exec.CommandContext(ctx,git,
   "-c","core.hooksPath="+os.DevNull,
   "-c","core.fsmonitor=false",
   "-C",root,"merge-base","--is-ancestor",head,target)
  cmd.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
  cmd.Stderr=io.Discard
  cmd.Stdout=io.Discard
  if cmd.Run()!=nil||ctx.Err()!=nil{return nil,denied}
  status="fast-forward"
 }
 // Refuse even a read-only "ready to pull" preview on a dirty workspace.
 // No partial porcelain output or untracked filename escapes the host.
 porcelain,e:=run("status","--porcelain=v1","--untracked-files=all",
  "--ignore-submodules=none")
 if e!=nil||porcelain!=""{return nil,denied}
 // Guard a review from silently describing a different HEAD/upstream after
 // a concurrent local Git operation. A later write MUST do fresh CAS checks.
 branchNow,headNow,_,e:=remoteGitBranchRead(ctx,root)
 if e!=nil||branchNow!=branch||headNow!=head{return nil,denied}
 upstreamNow,e:=run("rev-parse","--symbolic-full-name","@{upstream}")
 if e!=nil||upstreamNow!=upstream{return nil,denied}
 targetNow,e:=run("rev-parse","--verify",upstream+"^{commit}")
 if e!=nil||targetNow!=target{return nil,denied}
 report:="GIT_PULL_REVIEW_V1\nBRANCH "+branch+"\nHEAD "+head+
  "\nUPSTREAM "+upstream+"\nUPSTREAM_HEAD "+target+
  "\nSTATUS "+status+"\nWORKTREE clean\nNETWORK none\n"
 if !remotetransport.ValidRemoteGitPullReviewReport(report){return nil,denied}
 return []byte(report),nil
}

func remoteGitPullBranchSafe(name string)bool{
 if len(name)<1||len(name)>160{return false}
 parts:=strings.Split(name,"/")
 if len(parts)>6{return false}
 for _,part:=range parts{
  if !validRemoteGitName(part)||part=="HEAD"||
   strings.HasSuffix(part,".lock"){return false}
 }
 return true
}
