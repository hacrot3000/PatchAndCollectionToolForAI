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

const remoteGitIndexTimeout=8*time.Second

// Stage/unstage only host-owned, fixed index scopes. No path, ref, shell,
// file content or command can be supplied by the E2EE client. In particular,
// stage -u NEVER adds previously untracked files.
func(s *Server)executeRemoteGitIndex(
 ctx context.Context,session *remoteTaskDeckSession,op string,
)(remotetransport.RemoteGitIndexResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteGitIndexResult
 optIn:=""
 switch op{
 case remotetransport.RemoteActionGitStageTracked:
  optIn="TASKDECK_REMOTE_GIT_STAGE_TRACKED"
 case remotetransport.RemoteActionGitUnstageAll:
  optIn="TASKDECK_REMOTE_GIT_UNSTAGE_ALL"
 default:
  return empty,denied
 }
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  runtime.GOOS=="windows"||s.Workspace==""||os.Getenv(optIn)!="1"||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return empty,denied
 }
 // Hold the admission through the Git mutation; a quick busy-state check
 // alone races with tasks.start and terminal-open after the check returns.
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
  "GIT_CONFIG_SYSTEM="+os.DevNull,"GIT_OPTIONAL_LOCKS=0",
  "GIT_PROTOCOL_FROM_USER=0",
 }
 base:=[]string{
  "-c","core.fsmonitor=false",
  "-c","core.hooksPath="+os.DevNull,
  "-c","protocol.ext.allow=never",
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
 head,err:=run(ctx,256,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(head){return empty,denied}
 // Exit status 1 is the ONLY accepted evidence of a nonempty index
 // mutation. Exit 0 (nothing to change), signal or any other failure denies.
 diffArgs:=[]string{"diff","--quiet","--no-ext-diff","--no-textconv","--exit-code","--"}
 if op==remotetransport.RemoteActionGitUnstageAll{
  diffArgs=[]string{"diff","--cached","--quiet","--no-ext-diff","--no-textconv","--exit-code","--"}
 }
 check:=func(callCtx context.Context)(int,error){
  if callCtx.Err()!=nil||session.closed.Load(){return -1,denied}
  cmd:=exec.CommandContext(callCtx,"git",append(append([]string(nil),base...),diffArgs...)...)
  cmd.Dir=root
  cmd.Env=env
  cmd.Stdout=io.Discard
  cmd.Stderr=io.Discard
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  err:=cmd.Run()
  killGitJobProcess(cmd)
  if callCtx.Err()!=nil||session.closed.Load(){return -1,denied}
  if err==nil{return 0,nil}
  var ex *exec.ExitError
  if errors.As(err,&ex){return ex.ExitCode(),nil}
  return -1,denied
 }
 // The initial git diff --quiet can invoke clean/process filters before
 // the old stage-only guard below is reached. Apply the shared strict
 // preflight to both stage and unstage index mutations.
 if !remoteGitNoExecutableFilters(ctx,root,env){return empty,denied}
 code,err:=check(ctx)
 if err!=nil||code!=1{return empty,denied}
 // Git add can execute user-configured clean/process filters from
 // .gitattributes and repository-local Git configuration. Never grant an
 // E2EE "stage" click the ability to start such external executables.
 if op==remotetransport.RemoteActionGitStageTracked{
  for _,key:=range []string{
   "^filter\\..*\\.clean$","^filter\\..*\\.process$",
  }{
   if ctx.Err()!=nil||session.closed.Load(){return empty,denied}
   command:=exec.CommandContext(ctx,"git",append(append([]string(nil),base...),
    "config","--includes","--get-regexp",key)...)
   command.Dir=root
   command.Env=env
   command.Stdout=io.Discard
   command.Stderr=io.Discard
   prepareGitJobProcess(command)
   command.Cancel=func()error{killGitJobProcess(command);return nil}
   command.WaitDelay=time.Second
   checkErr:=command.Run()
   killGitJobProcess(command)
   var exit *exec.ExitError
   // Exit 1 means no matching filter driver. A matching executable (0),
   // signals or a config parse failure deny the mutation.
   if !errors.As(checkErr,&exit)||exit.ExitCode()!=1||
    ctx.Err()!=nil||session.closed.Load(){return empty,denied}
  }
 }
 if err:=s.auditRemoteGitIndexAttempt(ctx,session,op);err!=nil{return empty,denied}
 mutateCtx,cancel:=context.WithTimeout(ctx,remoteGitIndexTimeout)
 defer cancel()
 if mutateCtx.Err()!=nil||session.closed.Load()||
  !session.portalGrant.ActiveForSession(session.channel){return empty,denied}
 if op==remotetransport.RemoteActionGitStageTracked{
  _,err=run(mutateCtx,0,"add","-u","--",":/")
 }else{
  // Restore the index to HEAD only; working files stay untouched.
  _,err=run(mutateCtx,0,"restore","--staged","--",":/")
 }
 if err!=nil||mutateCtx.Err()!=nil||session.closed.Load(){return empty,denied}
 newHead,err:=run(ctx,256,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||newHead!=head{return empty,denied}
 code,err=check(ctx)
 if err!=nil||code!=0{return empty,denied}
 return remotetransport.RemoteGitIndexResult{Operation:op,Head:head},nil
}

func(s *Server)auditRemoteGitIndexAttempt(ctx context.Context,
 session *remoteTaskDeckSession,op string)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Identity==nil||session==nil||session.userID==""||
  ctx==nil||ctx.Err()!=nil{return denied}
 var action string
 switch op{
 case remotetransport.RemoteActionGitStageTracked:action="remote.action.git_stage_tracked"
 case remotetransport.RemoteActionGitUnstageAll:action="remote.action.git_unstage_all"
 case remotetransport.RemoteActionGitStageFile:action="remote.action.git_stage_file"
 case remotetransport.RemoteActionGitUnstageFile:action="remote.action.git_unstage_file"
 default:return denied
 }
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled{return denied}
 eventID,err:=identity.NewID()
 if err!=nil{return denied}
 userID,projectID:=session.userID,project.ID
 if err:=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),
  UserID:&userID,ProjectID:&projectID,
  Action:action,ResourceType:"git",Result:"attempted",
  Details:`{"transport":"e2ee","git_action":"index","fixed_scope":true}`,
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}
