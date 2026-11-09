package server

import (
 "context"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const remoteGitFetchTimeout=9*time.Second

// Fetch only the current branch's existing tracking reference from the
// preconfigured, single SSH/HTTPS host remote. No client-selected URL,
// refspec, command, directory, credentials or environment ever arrive here.
func(s *Server)executeRemoteGitFetch(
 ctx context.Context,session *remoteTaskDeckSession,
)(remotetransport.RemoteGitFetchResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteGitFetchResult
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  runtime.GOOS=="windows"||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_FETCH")!="1"||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return empty,denied
 }
 release,err:=s.reserveRemoteGitMutation(ctx,session)
 if err!=nil{return empty,denied}
 defer release()
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return empty,denied}
 env:=[]string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),
  "SSH_AUTH_SOCK="+os.Getenv("SSH_AUTH_SOCK"),
  "LC_ALL=C","GIT_TERMINAL_PROMPT=0","GCM_INTERACTIVE=never",
  "GIT_ASKPASS=/usr/bin/false","SSH_ASKPASS=/usr/bin/false",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_PROTOCOL_FROM_USER=0","GIT_SSH_COMMAND=ssh -oBatchMode=yes",
 }
 base:=[]string{
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false",
  // Empty override resets helpers configured by the repository; the
  // fixed GIT_ASKPASS binary denies any remaining interactive prompt.
  "-c","credential.helper=",
  "-c","protocol.allow=never",
  "-c","protocol.ssh.allow=always",
  "-c","protocol.https.allow=always",
  "-c","protocol.ext.allow=never",
  "-c","protocol.file.allow=never",
  "-c","fetch.recurseSubmodules=false",
  "-c","fetch.prune=false",
  "-c","fetch.writeCommitGraph=false",
  "-C",root,
 }
 run:=func(callCtx context.Context,limit int,args ...string)(string,error){
  if callCtx.Err()!=nil||session.closed.Load(){return "",denied}
  cmd:=exec.CommandContext(callCtx,"git",append(append([]string(nil),base...),args...)...)
  cmd.Dir=root
  cmd.Env=env
  var out *boundedCommandOutput
  if limit>0{
   out=&boundedCommandOutput{remaining:limit}
   cmd.Stdout=out
  }else{cmd.Stdout=io.Discard}
  cmd.Stderr=io.Discard
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  err:=cmd.Run()
  killGitJobProcess(cmd)
  if err!=nil||callCtx.Err()!=nil||session.closed.Load(){return "",denied}
  if out==nil{return "",nil}
  if out.truncated{return "",denied}
  return strings.TrimSpace(out.String()),nil
 }
 top,err:=run(ctx,4096,"rev-parse","--show-toplevel")
 if err!=nil{return empty,denied}
 canonical,err:=filepath.EvalSymlinks(top)
 if err!=nil{return empty,denied}
 canonical,err=filepath.Abs(canonical)
 if err!=nil||filepath.Clean(canonical)!=filepath.Clean(root){return empty,denied}
 localRef,err:=run(ctx,256,"symbolic-ref","-q","HEAD")
 if err!=nil||!strings.HasPrefix(localRef,"refs/heads/"){return empty,denied}
 branch:=strings.TrimPrefix(localRef,"refs/heads/")
 if branch==""||len(branch)>180||strings.ContainsAny(branch,"\r\n"){return empty,denied}
 remote,err:=run(ctx,256,"config","--get","branch."+branch+".remote")
 if err!=nil||!remoteGitPushName.MatchString(remote){return empty,denied}
 merge,err:=run(ctx,256,"config","--get","branch."+branch+".merge")
 if err!=nil||merge!=localRef{return empty,denied}
 // Resolve insteadOf rewrites and reject multiple fetch URLs.
 fetchURL,err:=run(ctx,1024,"remote","get-url","--all",remote)
 if err!=nil||!validRemoteGitPushURL(fetchURL){return empty,denied}
 head,err:=run(ctx,256,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(head){return empty,denied}
 tracking:="refs/remotes/"+remote+"/"+branch
 // An existing tracking reference is mandatory. This prevents remote
 // creation or branch selection through a single trusted GUI click.
 upstream,err:=run(ctx,256,"rev-parse","--symbolic-full-name","@{upstream}")
 if err!=nil||upstream!=tracking{return empty,denied}
 before,err:=run(ctx,256,"rev-parse","--verify",tracking+"^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(before){return empty,denied}
 if err:=s.auditRemoteGitFetchAttempt(ctx,session);err!=nil{return empty,denied}
 fetchCtx,cancel:=context.WithTimeout(ctx,remoteGitFetchTimeout)
 defer cancel()
 if fetchCtx.Err()!=nil||session.closed.Load()||
  !session.portalGrant.ActiveForSession(session.channel){return empty,denied}
 // No "+" refspec: rewritten/non-fast-forward remote branches fail closed.
 // The host's worktree, index, local HEAD, tags and other refs are not
 // selected for updates.
 _,err=run(fetchCtx,0,"fetch","--no-tags","--no-recurse-submodules",
  "--no-write-fetch-head",remote,
  "refs/heads/"+branch+":"+tracking)
 if err!=nil||fetchCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 after,err:=run(ctx,256,"rev-parse","--verify",tracking+"^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(after){return empty,denied}
 newHead,err:=run(ctx,256,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||newHead!=head{return empty,denied}
 return remotetransport.RemoteGitFetchResult{
  Head:head,Before:before,After:after,Updated:after!=before,
 },nil
}

func(s *Server)auditRemoteGitFetchAttempt(ctx context.Context,
 session *remoteTaskDeckSession)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Identity==nil||session==nil||session.userID==""||
  ctx==nil||ctx.Err()!=nil{return denied}
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled{return denied}
 eventID,err:=identity.NewID()
 if err!=nil{return denied}
 userID,projectID:=session.userID,project.ID
 if err:=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),
  UserID:&userID,ProjectID:&projectID,
  Action:"remote.action.git_fetch",ResourceType:"git",Result:"attempted",
  Details:`{"transport":"e2ee","git_action":"fetch","tracking_only":true,"force":false}`,
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}
