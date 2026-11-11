package server

import (
 "context"
 "os"
 "path/filepath"
 "sync"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A completed job can remain attached for authenticated output Poll and
// explicit Clear, without continuing to own the workspace. Do not admit
// Git from a merely finalizing job: its done channel closes only after
// Wait, process-group cleanup and workspace release. Fail closed for
// nil done channels, inconsistent states or unreaped processes.
func remoteLongTaskBlocksGit(job *remoteLongTaskJob)bool{
 if job==nil{return false}
 if job.done==nil{return true}
 select{
 case <-job.done:
 default:return true
 }
 job.mu.Lock()
 finished:=job.finalizing&&
  (job.status=="completed"||job.status=="failed"||
   job.status=="cancelled"||job.status=="timed_out")
 job.mu.Unlock()
 return !finished
}

// reserveRemoteGitMutation prevents per-session task/PTY execution from
// overlapping Git operations that mutate the workspace index, HEAD,
// branch refs or remote-tracking refs on that same TaskDeck workspace.
// Keep the pre-existing lock order: terminalMu -> longTaskMu. TryLock
// rejects concurrency promptly rather than waiting behind a long task.
// Hold both mutexes until the complete bounded Git operation exits.
//
// This guard is NOT a permission grant. Authentication, host opt-in, per-
// action RBAC, live Portal grant and E2EE admission remain required.
func (session *remoteTaskDeckSession) reserveRemoteGitMutation(
 ctx context.Context,
)(func(),error){
 denied:=remotetransport.ErrRemoteActionDenied
 if session==nil||ctx==nil||ctx.Err()!=nil||session.closed.Load(){
  return nil,denied
 }
 if !session.terminalMu.TryLock(){return nil,denied}
 if !session.longTaskMu.TryLock(){
  session.terminalMu.Unlock()
  return nil,denied
 }
 if ctx.Err()!=nil||session.closed.Load()||
  session.terminal!=nil||remoteLongTaskBlocksGit(session.longTask){
  session.longTaskMu.Unlock()
  session.terminalMu.Unlock()
  return nil,denied
 }
 return func(){
  session.longTaskMu.Unlock()
  session.terminalMu.Unlock()
 },nil
}


// A successful durable attempted-audit is not an execution grant. Host
// consent can change while SQLite persists the row, even when the same
// E2EE request and caller context are still alive. Recheck BEFORE running
// the fixed, side-effecting Git command. This is not a substitute for the
// dispatcher's live GitWrite RBAC check, cross-session workspace lease or
// the OS-enforced remote server's own push permissions.
func remoteGitMutationPostAuditAllowed(ctx context.Context,
 session *remoteTaskDeckSession,flag string)bool{
 switch flag{
 case "TASKDECK_REMOTE_GIT_PULL","TASKDECK_REMOTE_GIT_COMMIT",
  "TASKDECK_REMOTE_GIT_PUSH":
 default:return false
 }
 return ctx!=nil&&ctx.Err()==nil&&session!=nil&&!session.closed.Load()&&
  session.channel!=nil&&session.channel.Established()&&
  session.portalGrant!=nil&&
  session.portalGrant.ActiveForSession(session.channel)&&
  os.Getenv(flag)=="1"
}

// A durable attempted audit does not retain the caller's GitWrite grant.
// The host may change its RBAC membership during the SQLite audit write;
// check the current local identity again before any side-effecting Git.
func(s *Server)remoteGitMutationPostAuditAuthorized(ctx context.Context,
 session *remoteTaskDeckSession,flag string)bool{
 if s==nil||!remoteGitMutationPostAuditAllowed(ctx,session,flag){
  return false
 }
 checkCtx,cancel:=context.WithTimeout(ctx,remoteLocalRevokeCheckDeadline)
 defer cancel()
 principal,err:=s.verifyRemoteSessionLocalPrincipal(checkCtx,session)
 return err==nil&&checkCtx.Err()==nil&&
  remotetransport.AuthorizeAction(true,session.role,&principal,
   session.projectKey,identity.PermissionGitWrite)==nil&&
  remoteGitMutationPostAuditAllowed(ctx,session,flag)
}

// Remote Git, Task and PTY executions share this workspace guard across
// authenticated E2EE sessions. Paths are canonical host-owned directories;
// the OS advisory directory lock also excludes distinct Agent processes
// on the same local filesystem. No client-supplied token or project lock
// file exists. The registry entry disappears when its owner releases it.
var remoteWorkspaceExecutions=struct{
 sync.Mutex
 active map[string]bool
}{active:make(map[string]bool)}

func reserveRemoteWorkspaceExecution(ctx context.Context,workspace string)(
 func(),error,
){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||workspace==""{return nil,denied}
 root,err:=filepath.EvalSymlinks(workspace)
 if err!=nil{return nil,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return nil,denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir()||ctx.Err()!=nil{return nil,denied}
 remoteWorkspaceExecutions.Lock()
 if remoteWorkspaceExecutions.active[root]{
  remoteWorkspaceExecutions.Unlock()
  return nil,denied
 }
 remoteWorkspaceExecutions.active[root]=true
 remoteWorkspaceExecutions.Unlock()
 // The local map rejects concurrent goroutines in this process. An
 // advisory lock ON the directory itself also excludes separate TaskDeck
 // daemon processes without creating a raceable .lock file in a project.
 // Failing to obtain it must release the in-process reservation too.
 releaseOS,err:=acquireRemoteWorkspaceOSLock(root,info)
 if err!=nil{
  remoteWorkspaceExecutions.Lock()
  delete(remoteWorkspaceExecutions.active,root)
  remoteWorkspaceExecutions.Unlock()
  return nil,denied
 }
 var once sync.Once
 release:=func(){
  once.Do(func(){
   // Keep the local entry live until the OS lock has been released.
   // Another local goroutine must not race a separate Agent process.
   releaseOS()
   remoteWorkspaceExecutions.Lock()
   delete(remoteWorkspaceExecutions.active,root)
   remoteWorkspaceExecutions.Unlock()
  })
 }
 if ctx.Err()!=nil{
  release()
  return nil,denied
 }
 return release,nil
}

// Hold the pre-existing local session PTY -> long-task mutex ordering,
// then an exclusive canonical workspace reservation until Git exits.
// No wait queues or unbounded blocking in the native E2EE action path.
// External Git/task processes that do not cooperate with TaskDeck's
// advisory directory lock remain out of scope.
func (s *Server) reserveRemoteGitMutation(
 ctx context.Context,session *remoteTaskDeckSession,
)(func(),error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Workspace==""||ctx==nil||ctx.Err()!=nil{
  return nil,denied
 }
 releaseSession,err:=session.reserveRemoteGitMutation(ctx)
 if err!=nil{return nil,denied}
 releaseWorkspace,err:=reserveRemoteWorkspaceExecution(ctx,s.Workspace)
 if err!=nil{
  releaseSession()
  return nil,denied
 }
 if ctx.Err()!=nil||session.closed.Load(){
  releaseWorkspace()
  releaseSession()
  return nil,denied
 }
 var once sync.Once
 return func(){
  once.Do(func(){
   releaseWorkspace()
   releaseSession()
  })
 },nil
}
