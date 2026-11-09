package server

import (
 "context"
 "os"
 "path/filepath"
 "sync"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

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
  session.terminal!=nil||session.longTask!=nil{
  session.longTaskMu.Unlock()
  session.terminalMu.Unlock()
  return nil,denied
 }
 return func(){
  session.longTaskMu.Unlock()
  session.terminalMu.Unlock()
 },nil
}

// A local session mutex alone is insufficient: two independently authorized
// E2EE sessions can target the same TaskDeck workspace. Serialize ALL remote
// Git mutations in this Agent process by the canonical workspace path, not
// by the caller, raw path spelling, task ID or client-supplied project name.
// Entries exist ONLY while acquired: inactive workspaces do not accumulate.
var remoteGitWorkspaceMutations=struct{
 sync.Mutex
 active map[string]bool
}{active:make(map[string]bool)}

// reserveRemoteGitMutation extends the per-session task/PTY guard with an
// in-process, cross-session workspace reservation. Both locks are acquired
// with immediate failure rather than waiting on an in-progress network Git
// operation. This is NOT a cross-process OS lock and does not constrain
// external/local Git commands or remote tasks in unrelated E2EE sessions.
func (s *Server) reserveRemoteGitMutation(
 ctx context.Context,session *remoteTaskDeckSession,
)(func(),error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Workspace==""||ctx==nil||ctx.Err()!=nil{
  return nil,denied
 }
 releaseSession,err:=session.reserveRemoteGitMutation(ctx)
 if err!=nil{return nil,denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{
  releaseSession()
  return nil,denied
 }
 root,err=filepath.Abs(root)
 if err!=nil{
  releaseSession()
  return nil,denied
 }
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir()||ctx.Err()!=nil||session.closed.Load(){
  releaseSession()
  return nil,denied
 }
 remoteGitWorkspaceMutations.Lock()
 if remoteGitWorkspaceMutations.active[root]{
  remoteGitWorkspaceMutations.Unlock()
  releaseSession()
  return nil,denied
 }
 remoteGitWorkspaceMutations.active[root]=true
 remoteGitWorkspaceMutations.Unlock()
 releaseWorkspace:=func(){
  remoteGitWorkspaceMutations.Lock()
  delete(remoteGitWorkspaceMutations.active,root)
  remoteGitWorkspaceMutations.Unlock()
 }
 if ctx.Err()!=nil||session.closed.Load(){
  releaseWorkspace()
  releaseSession()
  return nil,denied
 }
 // Idempotent cleanup avoids a leaked global reservation even if a
 // caller's defer and an error/Close path both attempt the same release.
 var once sync.Once
 return func(){
  once.Do(func(){
   releaseWorkspace()
   releaseSession()
  })
 },nil
}
