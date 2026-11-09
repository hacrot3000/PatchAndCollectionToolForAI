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

const remoteGitBranchCreateTimeout=8*time.Second

// git.branch_create creates ONE locally configured branch pointing to the
// verified host HEAD. It does not checkout the branch, modify the worktree,
// fetch, push or accept ANY caller-selected Git argument.
func(s *Server)executeRemoteGitBranchCreate(ctx context.Context,
 session *remoteTaskDeckSession)(remotetransport.RemoteGitBranchCreateResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteGitBranchCreateResult
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  runtime.GOOS=="windows"||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE")!="1"||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return empty,denied
 }
 name:=os.Getenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE_NAME")
 if !remotetransport.ValidRemoteGitCreatedBranchName(name){
  return empty,denied
 }
 // Keep the same exclusive in-session Git slot from preflight through
 // local ref creation. Busy tasks/PTY must fail without a check/use race.
 release,err:=s.reserveRemoteGitMutation(ctx,session)
 if err!=nil{return empty,denied}
 defer release()
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 st,err:=os.Stat(root)
 if err!=nil||!st.IsDir(){return empty,denied}
 env:=[]string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),
  "LC_ALL=C","GIT_OPTIONAL_LOCKS=0","GIT_TERMINAL_PROMPT=0",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_CONFIG_SYSTEM="+os.DevNull,"GIT_PROTOCOL_FROM_USER=0",
 }
 base:=[]string{"-c","core.fsmonitor=false",
  "-c","core.hooksPath="+os.DevNull,
  "-c","protocol.ext.allow=never","-C",root}
 run:=func(callCtx context.Context,limit int,args ...string)(string,error){
  if callCtx.Err()!=nil||session.closed.Load(){return "",denied}
  command:=exec.CommandContext(callCtx,"git",append(append([]string(nil),base...),args...)...)
  command.Dir=root
  command.Env=env
  output:=&boundedCommandOutput{remaining:limit}
  if limit>0{command.Stdout=output}else{command.Stdout=io.Discard}
  command.Stderr=io.Discard
  prepareGitJobProcess(command)
  command.Cancel=func()error{killGitJobProcess(command);return nil}
  command.WaitDelay=time.Second
  err:=command.Run()
  killGitJobProcess(command)
  if err!=nil||callCtx.Err()!=nil||session.closed.Load()||output.truncated{return "",denied}
  if limit<=0{return "",nil}
  return strings.TrimSpace(output.String()),nil
 }
 top,err:=run(ctx,4096,"rev-parse","--show-toplevel")
 if err!=nil{return empty,denied}
 canon,err:=filepath.EvalSymlinks(top)
 if err!=nil{return empty,denied}
 canon,err=filepath.Abs(canon)
 if err!=nil||filepath.Clean(canon)!=filepath.Clean(root){
  return empty,denied
 }
 // Detached/unborn HEAD is not permitted. The source commit is immutable.
 current,err:=run(ctx,128,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||!validRemoteGitBranchName(current){return empty,denied}
 head,err:=run(ctx,128,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(head){return empty,denied}
 // A local Git filter.clean/process/smudge command can run even during
 // read-only status preflight. Deny these commands before touching status
 // or creating any branch ref; only the host config decides the opt-in.
 if !remoteGitNoExecutableFilters(ctx,root,env){return empty,denied}
 // No dirty worktree/index or untracked files: avoid suggesting that the
 // newly created branch contains working changes (it points only to HEAD).
 work,err:=run(ctx,256,"status","--porcelain=v1","--untracked-files=normal")
 if err!=nil||work!=""{return empty,denied}
 // The only expected exit status is 1 (ref does not exist). A Git
 // invocation error, signal or an existing branch must fail closed.
 check:=exec.CommandContext(ctx,"git",append(append([]string(nil),base...),
  "show-ref","--quiet","--verify","refs/heads/"+name)...)
 check.Dir=root
 check.Env=env
 check.Stdout=io.Discard
 check.Stderr=io.Discard
 prepareGitJobProcess(check)
 check.Cancel=func()error{killGitJobProcess(check);return nil}
 check.WaitDelay=time.Second
 checkErr:=check.Run()
 killGitJobProcess(check)
 var exit *exec.ExitError
 if !errors.As(checkErr,&exit)||exit.ExitCode()!=1||
  ctx.Err()!=nil||session.closed.Load(){return empty,denied}
 // Never audit or create an unsafe ref, including one Git would interpret
 // as a symbolic ref revision or option.
 validated,err:=run(ctx,128,"check-ref-format","--branch",name)
 if err!=nil||validated!=name{return empty,denied}
 if err:=s.auditRemoteGitBranchCreateAttempt(ctx,session);err!=nil{
  return empty,denied
 }
 mutateCtx,cancel:=context.WithTimeout(ctx,remoteGitBranchCreateTimeout)
 defer cancel()
 if mutateCtx.Err()!=nil||session.closed.Load()||
  !session.portalGrant.ActiveForSession(session.channel){return empty,denied}
 _,err=run(mutateCtx,0,"branch","--no-track",name,head)
 if err!=nil||mutateCtx.Err()!=nil||session.closed.Load(){
  return empty,denied
 }
 newTip,err:=run(ctx,128,"rev-parse","--verify","refs/heads/"+name+"^{commit}")
 if err!=nil||newTip!=head{return empty,denied}
 currentAfter,err:=run(ctx,128,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||currentAfter!=current{return empty,denied}
 newHead,err:=run(ctx,128,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||newHead!=head{return empty,denied}
 return remotetransport.RemoteGitBranchCreateResult{Branch:name,Head:head},nil
}

func(s *Server)auditRemoteGitBranchCreateAttempt(ctx context.Context,
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
  Action:"remote.action.git_branch_create",ResourceType:"git",
  Result:"attempted",
  Details:`{"transport":"e2ee","git_action":"branch_create","fixed_host_name":true,"checkout":false}`,
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}
