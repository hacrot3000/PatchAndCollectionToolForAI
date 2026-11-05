package server

import (
 "context"

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
