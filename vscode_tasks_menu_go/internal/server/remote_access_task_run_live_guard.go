package server

import (
 "context"
 "os"
 "sync/atomic"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A durable attempted audit does not grant continued task execution. Local
// TasksRun RBAC, the approved host opt-ins and the authenticated E2EE grant
// can be revoked while that audit row is being persisted.
func(s *Server)remoteShortTaskHostConsent(ctx context.Context,
 session *remoteTaskDeckSession)bool{
 if s==nil||ctx==nil||ctx.Err()!=nil||
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_RUN")!="1"{return false}
 checkCtx,cancel:=context.WithTimeout(ctx,remoteLocalRevokeCheckDeadline)
 defer cancel()
 return s.remoteWorkflowLocalRunAllowed(checkCtx,session)&&
  checkCtx.Err()==nil&&ctx.Err()==nil&&
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")=="1"&&
  os.Getenv("TASKDECK_REMOTE_TASKS_RUN")=="1"
}

// exec.CommandContext must use the CHILD context passed to execute; its
// process-group Cancel then retires shell children on host revocation.
// Keep the original process error separately: a nonzero exit is still a
// valid audited task result if authorization never changed. An already
// completed external side effect cannot be rolled back.
func(s *Server)runRemoteShortTaskWithLiveRBAC(ctx context.Context,
 session *remoteTaskDeckSession,
 execute func(context.Context)error)(error,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if execute==nil||!s.remoteShortTaskHostConsent(ctx,session){
  return nil,denied
 }
 runCtx,cancel:=context.WithCancel(ctx)
 defer cancel()
 stopWatch:=make(chan struct{})
 watchDone:=make(chan struct{})
 var revoked atomic.Bool
 go func(){
  defer close(watchDone)
  ticker:=time.NewTicker(remoteLocalRevokePoll)
  defer ticker.Stop()
  for{
   select{
   case <-stopWatch:return
   case <-runCtx.Done():return
   case <-ticker.C:
    if !s.remoteShortTaskHostConsent(runCtx,session){
     revoked.Store(true)
     cancel()
     return
    }
   }
  }
 }()
 err:=execute(runCtx)
 close(stopWatch)
 <-watchDone
 // Recheck after a successful, failed or cancelled command. A short
 // process may finish before the periodic watchdog sees a role change.
 if ctx.Err()!=nil||revoked.Load()||
  !s.remoteShortTaskHostConsent(ctx,session){
  return err,denied
 }
 return err,nil
}
