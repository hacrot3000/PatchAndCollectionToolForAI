package server

import (
 "context"
 "crypto/sha256"
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
 // Hash index entries incrementally, never retaining staged paths or
 // contents in audit, UI output or an unbounded in-memory buffer. The
 // executable/hooks configuration remains explicitly host-owned.
 indexFingerprint:=func(parent context.Context)([32]byte,error){
  var empty [32]byte
  if parent==nil||parent.Err()!=nil{return empty,denied}
  checkCtx,cancel:=context.WithTimeout(parent,3*time.Second)
  defer cancel()
  args:=append(append([]string(nil),base...),
   "ls-files","--stage","-z")
  command:=exec.CommandContext(checkCtx,"git",args...)
  command.Dir=root
  command.Env=env
  hasher:=sha256.New()
  command.Stdout=hasher
  command.Stderr=io.Discard
  prepareGitJobProcess(command)
  command.Cancel=func()error{killGitJobProcess(command);return nil}
  command.WaitDelay=time.Second
  err:=command.Run()
  killGitJobProcess(command)
  if err!=nil||checkCtx.Err()!=nil||session.closed.Load(){
   return empty,denied
  }
  copy(empty[:],hasher.Sum(nil))
  return empty,nil
 }
 top,err:=run(ctx,256,"rev-parse","--show-toplevel")
 if err!=nil{return empty,denied}
 canonicalTop,err:=filepath.EvalSymlinks(top)
 if err!=nil{return empty,denied}
 canonicalTop,err=filepath.Abs(canonicalTop)
 if err!=nil||filepath.Clean(canonicalTop)!=filepath.Clean(root){return empty,denied}
 branchRef,err:=run(ctx,256,"symbolic-ref","-q","HEAD")
 // HEAD can be manually pointed at arbitrary refs/ namespaces. Remote
 // Commit is authorized only to advance an existing local branch,
 // never tags, notes or a custom symbolic ref under refs/.
 if err!=nil||!strings.HasPrefix(branchRef,"refs/heads/")||
  len(branchRef)<=len("refs/heads/")||len(branchRef)>256{
  return empty,denied
 }
 before,err:=run(ctx,256,"rev-parse","--verify","HEAD")
 if err!=nil||!remoteGitObjectID.MatchString(before){return empty,denied}
 gitDir,err:=run(ctx,1024,"rev-parse","--absolute-git-dir")
 if err!=nil||gitDir==""{return empty,denied}
 initialGitDir,err:=filepath.EvalSymlinks(gitDir)
 if err!=nil{return empty,denied}
 initialGitDir,err=filepath.Abs(initialGitDir)
 if err!=nil{return empty,denied}
 gitDirInfo,err:=os.Stat(initialGitDir)
 if err!=nil||!gitDirInfo.IsDir(){return empty,denied}
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
 stagedIndex,err:=indexFingerprint(ctx)
 if err!=nil{return empty,denied}
 // Persistent local audit before the commit is allowed to mutate HEAD.
 if err:=s.auditRemoteGitCommitAttempt(ctx,session);err!=nil{return empty,denied}
 if !s.remoteGitMutationPostAuditAuthorized(ctx,session,"TASKDECK_REMOTE_GIT_COMMIT"){
  return empty,denied
 }
 // Git's own short-lived process lock cannot exclude unrelated local
 // processes. Re-verify repository inode, branch, HEAD and staged index
 // against the exact state approved BEFORE the attempted audit. A
 // changing local config must not install an executable clean filter.
 revalidate:=func(checkCtx context.Context)bool{
  if checkCtx==nil||checkCtx.Err()!=nil||session.closed.Load()||
   !remoteGitNoExecutableFilters(checkCtx,root,env){return false}
  freshTop,err:=run(checkCtx,256,"rev-parse","--show-toplevel")
  if err!=nil{return false}
  resolvedTop,err:=filepath.EvalSymlinks(freshTop)
  if err!=nil{return false}
  resolvedTop,err=filepath.Abs(resolvedTop)
  if err!=nil||resolvedTop!=root{return false}
  freshDir,err:=run(checkCtx,1024,"rev-parse","--absolute-git-dir")
  if err!=nil{return false}
  resolvedDir,err:=filepath.EvalSymlinks(freshDir)
  if err!=nil{return false}
  resolvedDir,err=filepath.Abs(resolvedDir)
  if err!=nil||resolvedDir!=initialGitDir{return false}
  info,err:=os.Stat(resolvedDir)
  if err!=nil||!os.SameFile(gitDirInfo,info){return false}
  ref,err:=run(checkCtx,256,"symbolic-ref","-q","HEAD")
  if err!=nil||ref!=branchRef{return false}
  current,err:=run(checkCtx,256,"rev-parse","--verify","HEAD")
  if err!=nil||current!=before{return false}
  stagedNow,err:=indexFingerprint(checkCtx)
  return err==nil&&stagedNow==stagedIndex
 }
 if !revalidate(ctx){return empty,denied}
 commitCtx,cancel:=context.WithTimeout(ctx,remoteGitCommitTimeout)
 defer cancel()
 if commitCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 err=s.runRemoteGitMutationWithLiveRBAC(commitCtx,session,
  "TASKDECK_REMOTE_GIT_COMMIT",func(runCtx context.Context)error{
   if !revalidate(runCtx){return denied}
   _,runErr:=run(runCtx,0,"commit","--no-verify","--no-gpg-sign","-m",message)
   return runErr
  })
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
