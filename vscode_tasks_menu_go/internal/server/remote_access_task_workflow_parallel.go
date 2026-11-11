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

// A host can revoke process permission while a fork is already running.
// Unlike a one-shot preflight, this cheap host-owned check is safe to
// repeat while the bounded native worker pool is active.
func remoteParallelHostConsent(session *remoteTaskDeckSession)bool{
 return session!=nil&&!session.closed.Load()&&
  remoteWorkflowHostExecutionEnabled()&&
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL")=="1"
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
 index,total int)(string,int){
 result,exitCode:="failed",-1
 if ctx.Err()!=nil||session.closed.Load()||
  !remoteWorkflowHostExecutionEnabled()||
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL")!="1"{
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
 if len(plan)<3||len(plan)>remoteWorkflowMaxSteps||
  !plan[len(plan)-1].parallelJoin||
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL")!="1"{
  status="failed";exitCode=-1;return
 }
 leafCount:=len(plan)-1
 prepared:=make([]remoteParallelPrepared,leafCount)
 for i:=0;i<leafCount;i++{
  item,err:=s.prepareRemoteParallelStep(ctx,session,plan[i],hash)
  if err!=nil{
   status="failed";exitCode=-1;return
  }
  prepared[i]=item
 }
 // Strict audit barrier: no sibling process starts until BOTH attempted
 // rows have been durably written for this unique workflow run.
 // An attempted audit is NOT proof that a process started. If the
 // second audit or a host guard fails, close every already-attempted
 // outcome and stop without launching either child.
 attempted:=0
 finishUnlaunched:=func(result string){
  for i:=0;i<attempted;i++{
   // A failed final audit still fails closed: no process was launched.
   if err:=s.auditRemoteWorkflowStep(session,job.taskID,plan[i].taskID,
    i+1,len(plan),1,job.workflowRunID,result,-1);err!=nil{
    status="failed"
   }
   job.updateWorkflowStep(i+1,len(plan),1,1,result,-1,0)
  }
 }
 for i:=0;i<leafCount;i++{
  if ctx.Err()!=nil||session.closed.Load()||
   !remoteWorkflowHostExecutionEnabled()||
   os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL")!="1"{
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
 // Both attempted rows have been persisted. A local host may revoke
 // permission, replace tasks.json, or change a cwd symlink during either
 // database write. Recheck BOTH siblings *after* the audit barrier and
 // *before* starting ANY process, so one stale dependency cannot begin
 // while its peer fails final preflight.
 for i:=0;i<leafCount;i++{
  verified,err:=s.prepareRemoteParallelStep(ctx,session,plan[i],hash)
  if err!=nil||verified.executable!=prepared[i].executable||
   verified.execution.TaskID!=prepared[i].execution.TaskID||
   verified.execution.Cwd!=prepared[i].execution.Cwd{
   status="failed";exitCode=-1
   if ctx.Err()!=nil||session.closed.Load()||
    !remoteWorkflowHostExecutionEnabled()||
    os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL")!="1"{
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
  for{
   select{
   case <-stopWatch:return
   case <-siblingCtx.Done():return
   case <-ticker.C:
    if !remoteParallelHostConsent(session){
     hostRevoked.Store(true)
     cancelSibling()
     return
    }
   }
  }
 }()
 defer func(){close(stopWatch);<-watchDone}()
 outcomes:=make(chan outcome,leafCount)
 for i:=0;i<leafCount;i++{
  go func(index int){
   state,code:=s.executeRemoteParallelPrepared(siblingCtx,session,job,
    plan[index],prepared[index],index+1,len(plan))
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
 if hostRevoked.Load()||!remoteParallelHostConsent(session){
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
 status,exitCode=s.executeRemoteParallelPrepared(ctx,session,job,
  plan[rootIndex],root,len(plan),len(plan))
}
