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
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// This operation changes only the CURRENT local branch to the HOST-selected
// existing branch IF BOTH refs already point to exactly the same HEAD commit.
// No arbitrary checkout, network, branch creation, reset, file path or remote
// command is part of this authenticated E2EE action.
func(s *Server)executeRemoteGitSwitchSameHead(
 ctx context.Context,session *remoteTaskDeckSession,
)(remotetransport.RemoteGitSwitchResult,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteGitSwitchResult
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  runtime.GOOS=="windows"||s.Workspace==""||
  os.Getenv("TASKDECK_REMOTE_GIT_SWITCH")!="1"||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){return empty,denied}
 target:=os.Getenv("TASKDECK_REMOTE_GIT_SWITCH_NAME")
 if !remotetransport.ValidRemoteGitCreatedBranchName(target){return empty,denied}
 release,err:=session.reserveRemoteGitMutation(ctx)
 if err!=nil{return empty,denied}
 defer release()
 deadline,cancel:=context.WithTimeout(ctx,12*time.Second)
 defer cancel()
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return empty,denied}
 executable:=""
 for _,candidate:=range []string{"/usr/bin/git","/bin/git","/opt/homebrew/bin/git","/usr/local/bin/git"}{
  if tasks.RemoteExecutableAllowed(root,candidate){executable=candidate;break}
 }
 if executable==""{return empty,denied}
 env:=[]string{
  "PATH=/usr/bin:/bin","HOME=/nonexistent","LC_ALL=C","LANG=C",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_CONFIG_SYSTEM="+os.DevNull,"GIT_OPTIONAL_LOCKS=0",
  "GIT_TERMINAL_PROMPT=0","GIT_NO_LAZY_FETCH=1",
  "GIT_NO_REPLACE_OBJECTS=1","GIT_PROTOCOL_FROM_USER=0",
 }
 base:=[]string{
  "-c","core.fsmonitor=false","-c","core.untrackedCache=false",
  "-c","core.hooksPath="+os.DevNull,"-c","diff.external=",
  "-c","protocol.allow=never","-C",root,
 }
 run:=func(limit int,args ...string)(string,error){
  if deadline.Err()!=nil||session.closed.Load(){return "",denied}
  cmd:=exec.CommandContext(deadline,executable,append(append([]string(nil),base...),args...)...)
  cmd.Dir=root;cmd.Env=env
  var output *boundedCommandOutput
  if limit>0{
   output=&boundedCommandOutput{remaining:limit}
   cmd.Stdout=output
  }else{cmd.Stdout=io.Discard}
  cmd.Stderr=io.Discard
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  err:=cmd.Run()
  killGitJobProcess(cmd)
  if err!=nil||deadline.Err()!=nil||session.closed.Load(){return "",denied}
  if output==nil{return "",nil}
  if output.truncated{return "",denied}
  return strings.TrimSpace(output.String()),nil
 }
 top,err:=run(4096,"rev-parse","--show-toplevel")
 if err!=nil{return empty,denied}
 canonicalTop,err:=filepath.EvalSymlinks(top)
 if err!=nil{return empty,denied}
 canonicalTop,err=filepath.Abs(canonicalTop)
 if err!=nil||filepath.Clean(canonicalTop)!=filepath.Clean(root){return empty,denied}
 current,err:=run(128,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||!remotetransport.ValidRemoteGitSwitchFrom(current)||
  current==target{return empty,denied}
 head,err:=run(128,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||!remoteGitObjectID.MatchString(head){return empty,denied}
 expected,err:=run(128,"rev-parse","--verify","refs/heads/"+target+"^{commit}")
 if err!=nil||expected!=head{return empty,denied}
 // Any staged, unstaged, ignored-submodule or untracked change is a hard
 // refusal: this action never offers a checkout to a dirty workspace.
 status,err:=run(256,"status","--porcelain=v1","--untracked-files=all",
  "--ignore-submodules=none")
 if err!=nil||status!=""{return empty,denied}
 // Recheck the immutable HEAD/target relation immediately before mutation.
 verifyCurrent,err:=run(128,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||verifyCurrent!=current{return empty,denied}
 verifyHead,err:=run(128,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||verifyHead!=head{return empty,denied}
 verifyTarget,err:=run(128,"rev-parse","--verify","refs/heads/"+target+"^{commit}")
 if err!=nil||verifyTarget!=head{return empty,denied}
 if err:=s.auditRemoteGitSwitchAttempt(deadline,session);err!=nil{return empty,denied}
 if deadline.Err()!=nil||session.closed.Load()||
  !session.portalGrant.ActiveForSession(session.channel){return empty,denied}
 // Git switch checks other worktrees and index conflicts. Hooks are
 // disabled; the equal-tip requirement means no tree files should change.
 _,err=run(0,"switch","--no-guess",target)
 if err!=nil{return empty,denied}
 afterBranch,err:=run(128,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||afterBranch!=target{return empty,denied}
 afterHead,err:=run(128,"rev-parse","--verify","HEAD^{commit}")
 if err!=nil||afterHead!=head{return empty,denied}
 afterStatus,err:=run(256,"status","--porcelain=v1",
  "--untracked-files=all","--ignore-submodules=none")
 if err!=nil||afterStatus!=""{return empty,denied}
 return remotetransport.RemoteGitSwitchResult{
  From:current,Branch:target,Head:head,
 },nil
}

func(s *Server)auditRemoteGitSwitchAttempt(ctx context.Context,
 session *remoteTaskDeckSession)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Identity==nil||session==nil||session.userID==""||
  ctx==nil||ctx.Err()!=nil{return denied}
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled{return denied}
 id,err:=identity.NewID()
 if err!=nil{return denied}
 userID,projectID:=session.userID,project.ID
 if err:=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:id,Timestamp:time.Now().UTC(),
  UserID:&userID,ProjectID:&projectID,
  Action:"remote.action.git_switch_same_head",ResourceType:"git",
  Result:"attempted",
  Details:`{"transport":"e2ee","git_action":"switch_same_head","fixed_host_target":true,"same_tip":true}`,
 });err!=nil||ctx.Err()!=nil{return denied}
 return nil
}
