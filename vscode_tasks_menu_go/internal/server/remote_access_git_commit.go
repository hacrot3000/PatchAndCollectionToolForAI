package server

import (
 "context"
 "errors"
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

const remoteGitCommitTimeout=8*time.Second

// executeRemoteGitCommit is called exclusively behind the authenticated
// git.write action gate. It commits ONLY entries already staged on the exact
// TaskDeck workspace Git index; it NEVER stages paths or runs a shell.
func (s *Server) executeRemoteGitCommit(
 ctx context.Context,session *remoteTaskDeckSession,message string,
)(remotetransport.RemoteGitCommitResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 empty:=remotetransport.RemoteGitCommitResult{}
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  !remotetransport.ValidRemoteGitCommitMessage(message)||
  runtime.GOOS=="windows"||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_COMMIT")!="1"||
  session.closed.Load()||session.channel==nil||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return empty,denied
 }
 // A trusted GUI disables these while a task runs, but only the host
 // can enforce exclusion against another authenticated E2EE action.
 release,err:=session.reserveRemoteGitMutation(ctx)
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
  "LC_ALL=C","GIT_TERMINAL_PROMPT=0",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_PROTOCOL_FROM_USER=0",
 }
 // Both git diff/status preflights and git commit may invoke repository
 // clean/process/smudge filters even with hooks disabled. Deny all
 // executable filter drivers before any status-like Git call.
 if !remoteGitNoExecutableFilters(ctx,root,env){return empty,denied}
 // Every Git invocation uses a fixed argv, no shell and no caller-supplied
 // path or environment. Disable local fsmonitor commands and Git hooks.
 base:=[]string{
  "-c","core.fsmonitor=false",
  "-c","core.hooksPath="+os.DevNull,
  "-c","commit.gpgsign=false",
  "-c","protocol.ext.allow=never",
  "-C",root,
 }
 run:=func(runCtx context.Context,limit int,args ...string)(string,error){
  if runCtx.Err()!=nil||session.closed.Load(){return "",denied}
  command:=exec.CommandContext(runCtx,"git",append(append([]string(nil),base...),args...)...)
  command.Dir=root
  command.Env=env
  var output *boundedCommandOutput
  if limit>0{
   output=&boundedCommandOutput{remaining:limit}
   command.Stdout=output
  }else{command.Stdout=io.Discard}
  command.Stderr=io.Discard
  prepareGitJobProcess(command)
  command.Cancel=func()error{killGitJobProcess(command);return nil}
  command.WaitDelay=time.Second
  err:=command.Run()
  killGitJobProcess(command)
  if err!=nil||runCtx.Err()!=nil||session.closed.Load(){return "",denied}
  if output==nil{return "",nil}
  if output.truncated{return "",denied}
  return strings.TrimSpace(output.String()),nil
 }
 top,err:=run(ctx,256,"rev-parse","--show-toplevel")
 if err!=nil{return empty,denied}
 canonicalTop,err:=filepath.EvalSymlinks(top)
 if err!=nil{return empty,denied}
 canonicalTop,err=filepath.Abs(canonicalTop)
 if err!=nil||filepath.Clean(canonicalTop)!=filepath.Clean(root){return empty,denied}
 branchRef,err:=run(ctx,256,"symbolic-ref","-q","HEAD")
 if err!=nil||branchRef==""{return empty,denied}
 before,err:=run(ctx,256,"rev-parse","--verify","HEAD")
 if err!=nil||!remoteGitObjectID.MatchString(before){return empty,denied}
 // git diff --cached --quiet returns 1 only when the index differs from
 // HEAD. Any other code, signal, cancellation, or no changes is a denial.
 staged:=exec.CommandContext(ctx,"git",append(append([]string(nil),base...),
  "diff","--cached","--quiet","--no-ext-diff","--exit-code","--")...)
 staged.Dir=root
 staged.Env=env
 staged.Stdout=io.Discard
 staged.Stderr=io.Discard
 prepareGitJobProcess(staged)
 staged.Cancel=func()error{killGitJobProcess(staged);return nil}
 staged.WaitDelay=time.Second
 stagedErr:=staged.Run()
 killGitJobProcess(staged)
 var changed *exec.ExitError
 if !errors.As(stagedErr,&changed)||changed.ExitCode()!=1||
  ctx.Err()!=nil||session.closed.Load(){return empty,denied}
 // Persistent local audit before the commit is allowed to mutate HEAD.
 if err:=s.auditRemoteGitCommitAttempt(ctx,session);err!=nil{return empty,denied}
 commitCtx,cancel:=context.WithTimeout(ctx,remoteGitCommitTimeout)
 defer cancel()
 if commitCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 _,err=run(commitCtx,0,"commit","--no-verify","--no-gpg-sign","-m",message)
 if err!=nil||commitCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 after,err:=run(ctx,256,"rev-parse","--verify","HEAD")
 if err!=nil||!remoteGitObjectID.MatchString(after)||after==before{
  return empty,denied
 }
 return remotetransport.RemoteGitCommitResult{Before:before,After:after},nil
}

// The audit contains no commit message, file path or credentials. Never
// start an authenticated Git mutation if this mandatory audit fails.
func (s *Server) auditRemoteGitCommitAttempt(
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
  Action:"remote.action.git_commit",ResourceType:"git",Result:"attempted",
  Details:`{"transport":"e2ee","git_action":"commit","staged_only":true}`,
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}
