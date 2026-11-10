package server

import (
 "context"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "runtime"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const remoteGitPullTimeout=9*time.Second
var remoteGitObjectID=regexp.MustCompile(`^[0-9a-f]{40}$`)

// Git pull is an independently opted-in, fixed fast-forward operation on the
// Agent's exact workspace repository. No remote-selected Git arguments,
// branch, path, repository, remote, username or credentials are accepted.
// The device's PHP rendezvous service is never part of Git authentication.
func (s *Server) executeRemoteGitPull(
 ctx context.Context,session *remoteTaskDeckSession,
)(remotetransport.RemoteGitPullResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteGitPullResult
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  runtime.GOOS=="windows"||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_PULL")!="1"||
  session.closed.Load()||session.channel==nil||
  session.portalGrant==nil||!session.portalGrant.ActiveForSession(session.channel){
  return empty,denied
 }
 // A trusted GUI disables these while a task runs, but only the host
 // can enforce exclusion against another authenticated E2EE action.
 release,err:=s.reserveRemoteGitMutation(ctx,session)
 if err!=nil{return empty,denied}
 defer release()
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return empty,denied}
 // Explicit environment: prevent inherited GIT_* variables and credential
 // hooks from turning a fixed operation into an arbitrary Git override.
 gitEnv:=[]string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),
  "SSH_AUTH_SOCK="+os.Getenv("SSH_AUTH_SOCK"),"LC_ALL=C",
  "GIT_TERMINAL_PROMPT=0","GCM_INTERACTIVE=never",
  "GIT_ASKPASS=/usr/bin/false","SSH_ASKPASS=/usr/bin/false",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_PROTOCOL_FROM_USER=0",
  // Ignore repo-supplied core.sshCommand for this fixed, noninteractive
  // operation; the host's approved SSH agent may still authenticate.
  "GIT_SSH_COMMAND=ssh -oBatchMode=yes",
 }
 run:=func(ctx context.Context,limit int,args ...string)(string,error){
  if ctx.Err()!=nil{return "",denied}
  cmd:=exec.CommandContext(ctx,"git",args...)
  cmd.Dir=root
  cmd.Env=gitEnv
  var stdout *boundedCommandOutput
  if limit>0{
   stdout=&boundedCommandOutput{remaining:limit}
   cmd.Stdout=stdout
  }else{cmd.Stdout=io.Discard}
  cmd.Stderr=io.Discard
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  err:=cmd.Run()
  killGitJobProcess(cmd)
  if err!=nil||ctx.Err()!=nil{return "",denied}
  if stdout==nil{return "",nil}
  if stdout.truncated{return "",denied}
  return strings.TrimSpace(stdout.String()),nil
 }
 read:=func(ctx context.Context,args ...string)(string,error){
  return run(ctx,256,append([]string{
   "-c","core.fsmonitor=false",
   "-c","protocol.ext.allow=never",
   "-c","credential.helper=",
   "-C",root,
  },args...)...)
 }
 // No nested repository, parent repository or detached HEAD.
 top,err:=read(ctx,"rev-parse","--show-toplevel")
 if err!=nil{return empty,denied}
 canonicalTop,err:=filepath.EvalSymlinks(top)
 if err!=nil{return empty,denied}
 canonicalTop,err=filepath.Abs(canonicalTop)
 if err!=nil||filepath.Clean(canonicalTop)!=filepath.Clean(root){return empty,denied}
 // Enforce the same single, host-configured SSH/HTTPS upstream as the
 // existing Git fetch/push controls. An arbitrary local/file/ext remote,
 // multi-URL config or differently named tracking branch is not pullable.
 localRef,err:=read(ctx,"symbolic-ref","-q","HEAD")
 if err!=nil||!strings.HasPrefix(localRef,"refs/heads/"){return empty,denied}
 branch:=strings.TrimPrefix(localRef,"refs/heads/")
 if branch==""||len(branch)>180||strings.ContainsAny(branch,"\r\n"){
  return empty,denied
 }
 remote,err:=read(ctx,"config","--get","branch."+branch+".remote")
 if err!=nil||!remoteGitPushName.MatchString(remote){return empty,denied}
 merge,err:=read(ctx,"config","--get","branch."+branch+".merge")
 if err!=nil||merge!=localRef{return empty,denied}
 upstream,err:=read(ctx,"rev-parse","--symbolic-full-name","@{upstream}")
 if err!=nil||upstream!="refs/remotes/"+remote+"/"+branch{
  return empty,denied
 }
 fetchURL,err:=run(ctx,1024,
  "-c","core.fsmonitor=false","-C",root,
  "remote","get-url","--all",remote)
 if err!=nil||!validRemoteGitPushURL(fetchURL){return empty,denied}
 if !remoteGitNoRepositoryTLSOverride(ctx,root,gitEnv){return empty,denied}
 // A fixed, no-command Git config scan must precede even a read-only
 // status: local clean filters can execute while Git checks worktree files.
 if !remoteGitNoExecutableFilters(ctx,root,gitEnv){return empty,denied}
 // A clean tree is a strict prerequisite, including every untracked
 // path and dirty submodule. Repository-controlled submodule.<name>.ignore
 // and status.showUntrackedFiles settings must not hide local changes.
 // Reject >256 bytes rather than trusting a truncated clean indication.
 dirty,err:=read(ctx,"--no-optional-locks","status","--porcelain=v1",
  "--untracked-files=all","--ignore-submodules=none")
 if err!=nil||dirty!=""{return empty,denied}
 before,err:=read(ctx,"rev-parse","--verify","HEAD")
 if err!=nil||!remoteGitObjectID.MatchString(before){return empty,denied}
 if ctx.Err()!=nil||session.closed.Load(){return empty,denied}
 // The attempted audit is durable BEFORE the network fetch / branch update.
 if err:=s.auditRemoteGitPullAttempt(ctx,session);err!=nil{return empty,denied}
 pullCtx,cancel:=context.WithTimeout(ctx,remoteGitPullTimeout)
 defer cancel()
 if pullCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 tlsFlags:=[]string{"-c","http.sslVerify=true"}
 if strings.HasPrefix(fetchURL,"https://"){
  tlsFlags=append(tlsFlags,"-c","http."+fetchURL+".sslVerify=true")
 }
 pullArgs:=append(tlsFlags,
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false",
  // An HTTPS remote must never launch a repo-owned credential.helper
  // command or askpass program, even when the server replies 401.
  "-c","credential.helper=",
  "-c","protocol.allow=never",
  "-c","protocol.ssh.allow=always",
  "-c","protocol.https.allow=always",
  "-c","protocol.ext.allow=never",
  "-c","protocol.file.allow=never",
  "-c","submodule.recurse=false",
  "-c","fetch.recurseSubmodules=false",
  "-c","fetch.prune=false",
  "-c","fetch.pruneTags=false",
  "-c","pull.rebase=false",
  // Pin the fetch to this already-validated upstream branch. Git pull
  // without an explicit remote+ref can fetch every remote.fetch refspec
  // and auto-follow tags, mutating unrelated tracking refs/tags.
  "-C",root,"pull","--ff-only","--no-recurse-submodules","--no-edit",
  "--no-tags","--upload-pack=git-upload-pack",remote,branch)
 _,err=run(pullCtx,0,pullArgs...)
 if err!=nil||pullCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 after,err:=read(ctx,"rev-parse","--verify","HEAD")
 if err!=nil||!remoteGitObjectID.MatchString(after){return empty,denied}
 return remotetransport.RemoteGitPullResult{Before:before,After:after,Updated:before!=after},nil
}

func (s *Server) auditRemoteGitPullAttempt(
 ctx context.Context,session *remoteTaskDeckSession,
)error{
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
  Action:"remote.action.git_pull",ResourceType:"git",Result:"attempted",
  Details:`{"transport":"e2ee","git_action":"pull","ff_only":true}`,
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}
