package server

import(
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "sync/atomic"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// The only parallel shapes are two or three independent, fully validated processes
// followed by a successful-only root. The existing E2EE workflow start
// carries the host-issued root Task.ID and never a graph or command.
type remoteParallelPrepared struct{
 execution tasks.Execution
 executable string
}

// A re-prepared task must be EXACTLY the same process. Merely comparing
// Task ID, cwd and executable path is insufficient: the process inherits
// os.Environ, whose values can change while the attempted audit is being
// written, even when tasks.json and all Task IDs remain unchanged.
// Native workflows accept neither browser-selected env nor argument
// overrides. A mismatch denies the ENTIRE fork before any worker runs.
func remoteParallelSameExecution(a,b remoteParallelPrepared)bool{
 if a.executable!=b.executable||
  a.execution.TaskID!=b.execution.TaskID||
  a.execution.Command!=b.execution.Command||
  a.execution.Cwd!=b.execution.Cwd||
  len(a.execution.Args)!=len(b.execution.Args)||
  len(a.execution.Env)!=len(b.execution.Env){
  return false
 }
 for i,arg:=range a.execution.Args{
  if arg!=b.execution.Args[i]{return false}
 }
 for i,variable:=range a.execution.Env{
  if variable!=b.execution.Env[i]{return false}
 }
 return true
}

// A host can revoke process permission while a fork is already running.
// Unlike a one-shot preflight, this cheap host-owned check is safe to
// repeat while the bounded native worker pool is active.
func remoteParallelHostConsent(session *remoteTaskDeckSession)bool{
 return session!=nil&&!session.closed.Load()&&
  remoteWorkflowHostExecutionEnabled()&&
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL")=="1"
}
func remoteParallelWorkflowConsent(session *remoteTaskDeckSession,prefix int)bool{
 return remoteParallelHostConsent(session)&&
  (prefix==0||(prefix==1&&
   os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX")=="1"))
}

// A serial prerequisite uses the same per-process audit/revalidation and
// process-group cleanup as a parallel worker, but must finish BEFORE any
// sibling receives an attempted audit. No browser-provided task is accepted.
func(s *Server)runRemoteParallelPrelude(ctx context.Context,
 session *remoteTaskDeckSession,job *remoteLongTaskJob,
 step remoteWorkflowStep,prepared remoteParallelPrepared,hash [32]byte,
 total int)(string,int){
 if ctx.Err()!=nil||!remoteParallelWorkflowConsent(session,1){
  return "cancelled",-1
 }
 if err:=s.auditRemoteWorkflowStep(session,job.taskID,step.taskID,
  1,total,1,job.workflowRunID,"attempted",-1);err!=nil{
  return "failed",-1
 }
 verified,err:=s.prepareRemoteParallelStep(ctx,session,step,hash)
 if err!=nil||!remoteParallelSameExecution(verified,prepared)||
  !remoteParallelWorkflowConsent(session,1){
  status:="failed"
  if !remoteParallelWorkflowConsent(session,1){status="cancelled"}
  if s.auditRemoteWorkflowStep(session,job.taskID,step.taskID,
   1,total,1,job.workflowRunID,status,-1)!=nil{status="failed"}
  job.updateWorkflowStep(1,total,1,1,status,-1,0)
  return status,-1
 }
 childCtx,cancel:=context.WithCancel(ctx)
 defer cancel()
 var revoked atomic.Bool
 stop:=make(chan struct{})
 done:=make(chan struct{})
 go func(){
  defer close(done)
  flagTicker:=time.NewTicker(100*time.Millisecond)
  defer flagTicker.Stop()
  permissionTicker:=time.NewTicker(500*time.Millisecond)
  defer permissionTicker.Stop()
  for{
   select{
   case <-stop:return
   case <-childCtx.Done():return
   case <-flagTicker.C:
    if !remoteParallelWorkflowConsent(session,1){
     revoked.Store(true);cancel();return
    }
   case <-permissionTicker.C:
    checkCtx,finish:=context.WithTimeout(childCtx,2*time.Second)
    allowed:=s.remoteWorkflowLocalRunAllowed(checkCtx,session)
    finish()
    if !allowed{revoked.Store(true);cancel();return}
   }
  }
 }()
 status,code:=s.executeRemoteParallelPrepared(
  childCtx,session,job,step,verified,1,total,true)
 close(stop);<-done
 if revoked.Load()||!remoteParallelWorkflowConsent(session,1){
  return "cancelled",-1
 }
 return status,code
}


// Prepare every sibling before ANY parallel child starts. If one task's
// source, executable, role or host consent changed, fail the whole join.
// The per-process mandatory attempted audits are also written before any
// child starts; failed storage cannot lead to an unaudited process.
func(s *Server)prepareRemoteParallelStep(ctx context.Context,
 session *remoteTaskDeckSession,step remoteWorkflowStep,hash [32]byte,
)(remoteParallelPrepared,error){
 denied:=errors.New("parallel workflow step denied")
 var empty remoteParallelPrepared
 if ctx==nil||ctx.Err()!=nil||session.closed.Load()||
  !remoteWorkflowHostExecutionEnabled()||
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL")!="1"{
  return empty,denied
 }
 if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,
  session.channel,identity.PermissionTasksRun);err!=nil{return empty,denied}
 execution,_,err:=prepareRemoteTaskExecutionInputMode(ctx,s.Workspace,hash,
  step.taskID,0,false,"",true)
 if err!=nil||execution.TaskID!=step.taskID{return empty,denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil||validateRemoteWorkflowStepCwd(root,execution.Cwd)!=nil||
  !tasks.RemoteExecutableAllowed(root,execution.Command){return empty,denied}
 executable,ok:=remoteWorkflowResolvedExecutable(execution.Command)
 if !ok||!tasks.RemoteExecutableAllowed(root,executable){return empty,denied}
 return remoteParallelPrepared{execution:execution,executable:executable},nil
}

// Only called after the attempted audit was persisted. Step history and
// capture are synchronized by their existing mutexes; the root is never
// started until BOTH siblings exited and their final audits succeeded.
func(s *Server)executeRemoteParallelPrepared(ctx context.Context,
 session *remoteTaskDeckSession,job *remoteLongTaskJob,
 step remoteWorkflowStep,prepared remoteParallelPrepared,
 index,total int,prefixRequired bool)(string,int){
 result,exitCode:="failed",-1
 if ctx.Err()!=nil||session.closed.Load()||
  !remoteParallelWorkflowConsent(session,func()int{
   if prefixRequired{return 1};return 0
  }()){
  result="cancelled"
  _=s.auditRemoteWorkflowStep(session,job.taskID,step.taskID,index,total,
   1,job.workflowRunID,result,-1)
  job.updateWorkflowStep(index,total,1,1,result,-1,0)
  return result,-1
 }
 job.updateWorkflowStep(index,total,1,1,"running",-1,0)
 _,_=captureWorkflowText(job.capture,fmt.Sprintf(
  "[TaskDeck workflow] Step %d/%d: Task #%d (parallel attempt 1/1)\n",
  index,total,step.taskID))
 cmd:=exec.CommandContext(ctx,prepared.executable,prepared.execution.Args...)
 cmd.Dir=prepared.execution.Cwd
 cmd.Env=prepared.execution.Env
 cmd.Stdout=job.capture
 cmd.Stderr=job.capture
 prepareGitJobProcess(cmd)
 cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
 cmd.WaitDelay=time.Second
 started:=time.Now()
 err:=cmd.Run()
 elapsed:=time.Since(started)
 killGitJobProcess(cmd)
 exitCode=0
 if err!=nil{
  var exit *exec.ExitError
  if errors.As(err,&exit)&&exit.ExitCode()>=0&&exit.ExitCode()<=255{
   exitCode=exit.ExitCode()
  }else{exitCode=-1}
 }
 result="completed"
 if errors.Is(ctx.Err(),context.DeadlineExceeded){result="timed_out"
 }else if ctx.Err()!=nil||session.closed.Load(){result="cancelled"
 }else if !s.remoteWorkflowPostExitAuthorized(ctx,session,step,1,true)||
  (prefixRequired&&!remoteParallelWorkflowConsent(session,1)){
  // A sibling/root may have exited in the gap before the 500ms watcher.
  // Deny any subsequent root and never record a false completed result.
  result="cancelled";exitCode=-1
  _,_=captureWorkflowText(job.capture,
   "[TaskDeck workflow] Post-exit host permission denied; root blocked\n")
 }else if err!=nil{result="failed"}
 if auditErr:=s.auditRemoteWorkflowStep(session,job.taskID,step.taskID,
  index,total,1,job.workflowRunID,result,exitCode);auditErr!=nil{
  result="failed";exitCode=-1
  _,_=captureWorkflowText(job.capture,fmt.Sprintf(
   "[TaskDeck workflow] Step %d/%d: result audit unavailable; stopped\n",index,total))
 }
 job.updateWorkflowStep(index,total,1,1,result,exitCode,elapsed)
 _,_=captureWorkflowText(job.capture,fmt.Sprintf(
  "[TaskDeck workflow] Step %d/%d: %s (exit=%d)\n",
  index,total,result,exitCode))
 return result,exitCode
}

func(s *Server)runRemoteParallelJoinWorkflow(ctx context.Context,
 session *remoteTaskDeckSession,job *remoteLongTaskJob,
 plan []remoteWorkflowStep,hash [32]byte){
 status,exitCode:="completed",0
 defer func(){
  acceptedCancel:=job.beginFinalizing()
  if errors.Is(ctx.Err(),context.DeadlineExceeded){status="timed_out"
  }else if acceptedCancel||ctx.Err()!=nil||session.closed.Load(){
   status="cancelled"
  }
  s.auditRemoteTaskRunFinish(session,job.taskID,status,exitCode,
   time.Since(job.started))
  job.publishCompletion(status,exitCode)
  job.cancel()
 }()
 if len(plan)<3||len(plan)>remoteWorkflowMaxSteps{
  status="failed";exitCode=-1;return
 }
 prefixCount:=plan[len(plan)-1].parallelPrefix
 if !plan[len(plan)-1].parallelJoin||
  (prefixCount!=0&&prefixCount!=1)||
  len(plan)-1-prefixCount<2||
  !remoteParallelWorkflowConsent(session,prefixCount){
  status="failed";exitCode=-1;return
 }
 leafCount:=len(plan)-1-prefixCount
 prepared:=make([]remoteParallelPrepared,len(plan)-1)
 for i:=0;i<len(prepared);i++{
  item,err:=s.prepareRemoteParallelStep(ctx,session,plan[i],hash)
  if err!=nil{
   status="failed";exitCode=-1;return
  }
  prepared[i]=item
 }
 if prefixCount==1{
  status,exitCode=s.runRemoteParallelPrelude(ctx,session,job,plan[0],
   prepared[0],hash,len(plan))
  if status!="completed"{return}
  if !remoteParallelWorkflowConsent(session,prefixCount){
   status="cancelled";exitCode=-1;return
  }
 }
 // Strict audit barrier: no sibling process starts until ALL attempted
 // rows have been durably written for this unique workflow run.
 // An attempted audit is NOT proof that a process started. If the
 // subsequent audit or a host guard fails, close every already-attempted
 // outcome and stop without launching either child.
 attempted:=0
 finishUnlaunched:=func(result string){
  for i:=0;i<attempted;i++{
   // A failed final audit still fails closed: no process was launched.
   index:=prefixCount+i
   if err:=s.auditRemoteWorkflowStep(session,job.taskID,plan[index].taskID,
    index+1,len(plan),1,job.workflowRunID,result,-1);err!=nil{
    status="failed"
   }
   job.updateWorkflowStep(index+1,len(plan),1,1,result,-1,0)
  }
 }
 for i:=prefixCount;i<prefixCount+leafCount;i++{
  if ctx.Err()!=nil||!remoteParallelWorkflowConsent(session,prefixCount){
   status="cancelled";exitCode=-1
   finishUnlaunched(status)
   return
  }
  if err:=s.auditRemoteWorkflowStep(session,job.taskID,plan[i].taskID,
   i+1,len(plan),1,job.workflowRunID,"attempted",-1);err!=nil{
   status="failed";exitCode=-1
   finishUnlaunched(status)
   return
  }
  attempted++
 }
 // All attempted rows have been persisted. A local host may revoke
 // permission, replace tasks.json, or change a cwd symlink during either
 // database write. Recheck ALL siblings *after* the audit barrier and
 // *before* starting ANY process, so one stale dependency cannot begin
 // while its peer fails final preflight.
 for i:=prefixCount;i<prefixCount+leafCount;i++{
  verified,err:=s.prepareRemoteParallelStep(ctx,session,plan[i],hash)
  if err!=nil||!remoteParallelSameExecution(verified,prepared[i]){
   status="failed";exitCode=-1
   if ctx.Err()!=nil||session.closed.Load()||
    !remoteParallelWorkflowConsent(session,prefixCount){
    status="cancelled"
   }
   finishUnlaunched(status)
   _,_=captureWorkflowText(job.capture,
    "[TaskDeck workflow] Parallel prelaunch revalidation denied; no child started\n")
   return
  }
  prepared[i]=verified
 }
 type outcome struct{status string;code int}
 siblingCtx,cancelSibling:=context.WithCancel(ctx)
 defer cancelSibling()
 // The parent context already handles user cancellation, session expiry,
 // and grant/channel teardown. This extra watcher handles host flag
 // revocation AFTER processes have started. Cancel the entire process
 // group via exec.Cmd.Cancel, then join every worker before releasing the
 // existing workspace lease. Never start the root on a revoked grant.
 var hostRevoked atomic.Bool
 stopWatch:=make(chan struct{})
 watchDone:=make(chan struct{})
 go func(){
  defer close(watchDone)
  ticker:=time.NewTicker(100*time.Millisecond)
  defer ticker.Stop()
  permissionTicker:=time.NewTicker(500*time.Millisecond)
  defer permissionTicker.Stop()
  for{
   select{
   case <-stopWatch:return
   case <-siblingCtx.Done():return
   case <-permissionTicker.C:
    // The login may remain valid after a TaskDeck role downgrade.
    // Explicitly revoke the active fork/root when TasksRun disappears.
    checkCtx,done:=context.WithTimeout(siblingCtx,2*time.Second)
    allowed:=s.remoteWorkflowLocalRunAllowed(checkCtx,session)
    done()
    if !allowed{
     hostRevoked.Store(true)
     cancelSibling()
     return
    }
   case <-ticker.C:
    if !remoteParallelWorkflowConsent(session,prefixCount){
     hostRevoked.Store(true)
     cancelSibling()
     return
    }
   }
  }
 }()
 defer func(){close(stopWatch);<-watchDone}()
 outcomes:=make(chan outcome,leafCount)
 for i:=prefixCount;i<prefixCount+leafCount;i++{
  go func(index int){
   state,code:=s.executeRemoteParallelPrepared(siblingCtx,session,job,
    plan[index],prepared[index],index+1,len(plan),prefixCount==1)
   outcomes<-outcome{status:state,code:code}
  }(i)
 }
 for i:=0;i<leafCount;i++{
  result:=<-outcomes
  if result.status!="completed"{
   if status=="completed"||result.status=="failed"{
    status=result.status;exitCode=result.code
   }
   // Cancel the other worker's process group, then JOIN it before the
   // root decision and before releasing the exclusive workspace lease.
   cancelSibling()
  }
 }
 if hostRevoked.Load()||!remoteParallelWorkflowConsent(session,prefixCount){
  status="cancelled";exitCode=-1
  _,_=captureWorkflowText(job.capture,
   "[TaskDeck workflow] Parallel host consent revoked; workers stopped\n")
  return
 }
 if status!="completed"||ctx.Err()!=nil{return}
 rootIndex:=len(plan)-1
 root,err:=s.prepareRemoteParallelStep(ctx,session,plan[rootIndex],hash)
 if err!=nil{status="failed";exitCode=-1;return}
 if err:=s.auditRemoteWorkflowStep(session,job.taskID,plan[rootIndex].taskID,
  len(plan),len(plan),1,job.workflowRunID,"attempted",-1);err!=nil{
  status="failed";exitCode=-1;return
 }
 // The root is side-effecting too: a host may revoke approval or mutate
 // tasks.json DURING its attempted-audit write. The pre-audit approval
 // cannot authorize execution after that gap. Revalidate the exact issued
 // snapshot and process before starting, just as for every sibling.
 verified,err:=s.prepareRemoteParallelStep(ctx,session,plan[rootIndex],hash)
 if err!=nil||!remoteParallelSameExecution(verified,root){
  status="failed";exitCode=-1
  if ctx.Err()!=nil||!remoteParallelWorkflowConsent(session,prefixCount){
   status="cancelled"
  }
  result:=status
  if auditErr:=s.auditRemoteWorkflowStep(session,job.taskID,
   plan[rootIndex].taskID,len(plan),len(plan),1,job.workflowRunID,
   result,-1);auditErr!=nil{
   // No process was launched. Failure to record that fact is itself
   // a hard failure; do not pretend the aborted root completed.
   status="failed"
  }
  job.updateWorkflowStep(len(plan),len(plan),1,1,result,-1,0)
  _,_=captureWorkflowText(job.capture,
   "[TaskDeck workflow] Root prelaunch revalidation denied; root not started\n")
  return
 }
 // Use the SAME watchdog-cancellable context for the final root.
 // Cancelling only siblingCtx while the root uses ctx would allow an
 // already-running root to ignore host-side privilege revocation.
 status,exitCode=s.executeRemoteParallelPrepared(siblingCtx,session,job,
  plan[rootIndex],verified,len(plan),len(plan),prefixCount==1)
 if hostRevoked.Load()||!remoteParallelWorkflowConsent(session,prefixCount){
  status="cancelled";exitCode=-1
  _,_=captureWorkflowText(job.capture,
   "[TaskDeck workflow] Parallel host consent revoked; root stopped\n")
 }
}
