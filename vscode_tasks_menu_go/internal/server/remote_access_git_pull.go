package server

import (
 "context"
 "fmt"
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
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_PROTOCOL_FROM_USER=0",
 }
 run:=func(ctx context.Context,limit int,args ...string)(string,error){
  if ctx.Err()!=nil{return "",denied}
  cmd:=exec.CommandContext(ctx,"git",args...)
  cmd.Dir=root
  cmd.Env=gitEnv
  stdout:=&boundedCommandOutput{remaining:limit}
  cmd.Stdout=stdout
  cmd.Stderr=io.Discard
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  err:=cmd.Run()
  killGitJobProcess(cmd)
  if err!=nil||ctx.Err()!=nil||stdout.truncated{return "",denied}
  return strings.TrimSpace(stdout.String()),nil
 }
 read:=func(ctx context.Context,args ...string)(string,error){
  return run(ctx,256,append([]string{
   "-c","core.fsmonitor=false",
   "-c","protocol.ext.allow=never",
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
 branch,err:=read(ctx,"symbolic-ref","-q","--short","HEAD")
 if err!=nil||branch==""{return empty,denied}
 upstream,err:=read(ctx,"rev-parse","--abbrev-ref","--symbolic-full-name","@{upstream}")
 if err!=nil||upstream==""{return empty,denied}
 // A clean tree is a strict prerequisite, including untracked files.
 // Reject >256 bytes rather than trusting a truncated clean indication.
 dirty,err:=read(ctx,"--no-optional-locks","status","--porcelain=v1","--untracked-files=normal")
 if err!=nil||dirty!=""{return empty,denied}
 before,err:=read(ctx,"rev-parse","--verify","HEAD")
 if err!=nil||!remoteGitObjectID.MatchString(before){return empty,denied}
 if ctx.Err()!=nil||session.closed.Load(){return empty,denied}
 // The attempted audit is durable BEFORE the network fetch / branch update.
 if err:=s.auditRemoteGitPullAttempt(ctx,session);err!=nil{return empty,denied}
 pullCtx,cancel:=context.WithTimeout(ctx,remoteGitPullTimeout)
 defer cancel()
 if pullCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 _,err=run(pullCtx,32,
  "-c","core.hooksPath="+os.DevNull,
  "-c","core.fsmonitor=false",
  "-c","protocol.ext.allow=never",
  "-c","protocol.file.allow=never",
  "-c","submodule.recurse=false",
  "-c","fetch.recurseSubmodules=false",
  "-C",root,"pull","--ff-only","--no-recurse-submodules","--no-edit")
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
  Details:fmt.Sprintf(`{"transport":"e2ee","git_action":"pull","ff_only":true}`),
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}
