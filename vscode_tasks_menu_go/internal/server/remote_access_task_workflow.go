package server

import(
 "context"
 "errors"
 "fmt"
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

// Intentionally small first executor: exactly one linear dependency chain
// with 2..4 fixed, non-interactive process tasks. DAGs with branches,
// shell tasks, workflow policies, secrets or interpolated input are denied.
const remoteWorkflowMaxSteps=4

type remoteWorkflowStep struct{ taskID int }

func remoteWorkflowSafeProcess(task tasks.Task)bool{
 if task.Type!="process"||len(task.Inputs)!=0||
  len(task.SecretEnv)!=0||len(task.Options)!=0||
  task.Raw["taskdeck"]!=nil{return false}
 command,ok:=task.Command.(string)
 if !ok||command==""||strings.Contains(command,"${")||
  !filepath.IsAbs(command){return false}
 switch strings.ToLower(filepath.Base(command)){
 case "bash","sh","dash","zsh","ksh","fish","csh","tcsh","pwsh","powershell",
      "python","python3","perl","ruby","node","php","env","sudo","su":
  return false
 }
 if task.Args!=nil {
  args,ok:=task.Args.([]any)
  if !ok||len(args)>16{return false}
  for _,v:=range args{
   arg,ok:=v.(string)
   if !ok||len(arg)>256||strings.Contains(arg,"${")||
    strings.ContainsAny(arg,"\x00\r\n"){return false}
  }
 }
 return true
}

func(s *Server)prepareRemoteLinearWorkflow(ctx context.Context,
 session *remoteTaskDeckSession,rootTaskID int,
)([]remoteWorkflowStep,[32]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var hash [32]byte
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  runtime.GOOS=="windows"||
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_DEPS")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_RUN")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_LONG")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RUN")!="1"||
  !session.taskPageReady||!session.taskIssuedIDs[rootTaskID]{
  return nil,hash,denied
 }
 deps,err:=s.readRemoteTaskDependencies(ctx,session,rootTaskID)
 if err!=nil||!deps.Available||len(deps.Order)<2||
  len(deps.Order)>remoteWorkflowMaxSteps||
  len(deps.Edges)!=len(deps.Order)-1{return nil,hash,denied}
 for i,edge:=range deps.Edges{
  if edge[0]!=deps.Order[i]||edge[1]!=deps.Order[i+1]{
   return nil,hash,denied
  }
 }
 hash=session.taskPageHash
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return nil,hash,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return nil,hash,denied}
 list,err:=tasks.Load(root)
 if err!=nil||len(list)>remotetransport.MaxRemoteTasksCount{return nil,hash,denied}
 byID:=make(map[int]tasks.Task,len(list))
 for _,t:=range list {
  if _,dup:=byID[t.ID];dup{return nil,hash,denied}
  byID[t.ID]=t
 }
 plan:=make([]remoteWorkflowStep,0,len(deps.Order))
 for i,id:=range deps.Order{
  task,ok:=byID[id]
  if !ok||!remoteWorkflowSafeProcess(task){return nil,hash,denied}
  node:=tasks.WorkflowNodeForTask(task)
  if i==0{
   if len(node.DependsOn)!=0{return nil,hash,denied}
  }else if len(node.DependsOn)!=1||
   node.DependsOn[0]!=byID[deps.Order[i-1]].Label{
   return nil,hash,denied
  }
  // Reuse the existing, separately tested single-task source/inode,
  // workspace boundary and stable-ID preflight, with the ONLY exception
  // that the host-owned dependency label is now allowed.
  spec,_,err:=prepareRemoteTaskExecutionInputMode(
   ctx,s.Workspace,hash,id,0,false,"",true)
  if err!=nil||spec.TaskID!=id{return nil,hash,denied}
  if err:=validateRemoteWorkflowStepCwd(root,spec.Cwd);err!=nil{
   return nil,hash,denied
  }
  plan=append(plan,remoteWorkflowStep{taskID:id})
 }
 if ctx.Err()!=nil||session.closed.Load(){return nil,hash,denied}
 return plan,hash,nil
}

func validateRemoteWorkflowStepCwd(root,cwd string)error{
 denied:=remotetransport.ErrRemoteActionDenied
 resolved,err:=filepath.EvalSymlinks(cwd)
 if err!=nil{return denied}
 rel,err:=filepath.Rel(root,resolved)
 if err!=nil||filepath.IsAbs(rel)||rel==".."||
  strings.HasPrefix(rel,".."+string(filepath.Separator)){return denied}
 return nil
}

// Append a durable attempted row before EACH child process. Never persist
// command, argv, stdout, secret values or cwd in the audit database.
func(s *Server)auditRemoteWorkflowStep(session *remoteTaskDeckSession,
 rootID,stepID,index,total int,status string,exit int)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Identity==nil||session==nil||rootID<1||stepID<1||
  index<1||index>total||total>remoteWorkflowMaxSteps{return denied}
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Second)
 defer cancel()
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled{return denied}
 id,err:=identity.NewID()
 if err!=nil{return denied}
 user,pid:=session.userID,project.ID
 err=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:id,Timestamp:time.Now().UTC(),UserID:&user,ProjectID:&pid,
  Action:"remote.action.tasks_workflow_step",ResourceType:"tasks",Result:status,
  Details:fmt.Sprintf("{\"transport\":\"e2ee\",\"root_task_id\":%d,\"task_id\":%d,\"step\":%d,\"total\":%d,\"exit_code\":%d}",
   rootID,stepID,index,total,exit),
 })
 if err!=nil{return denied}
 return nil
}

// Starts a cancellable host-owned sequence in the existing long-job slot.
// The native client only sends the E2EE-issued ROOT ID. Poll/cancel/clear
// are exactly the existing long-job actions and use the same Task.ID.
func(s *Server)startRemoteLinearWorkflow(ctx context.Context,
 session *remoteTaskDeckSession,taskID int)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  session.closed.Load(){return denied}
 // Enforce PTY -> long-job lock order and disallow concurrent process
 // mutation from this E2EE session. Lock isn't held while running tasks.
 session.terminalMu.Lock()
 defer session.terminalMu.Unlock()
 if session.terminal!=nil{return denied}
 session.longTaskMu.Lock()
 defer session.longTaskMu.Unlock()
 if session.longTask!=nil{return denied}
 plan,hash,err:=s.prepareRemoteLinearWorkflow(ctx,session,taskID)
 if err!=nil{return denied}
 // Durable aggregate attempted audit is BEFORE any irreversible start.
 if err:=s.auditRemoteTaskRunAttempt(ctx,session,taskID);err!=nil{return denied}
 jobCtx,cancel:=context.WithTimeout(context.Background(),remoteLongTaskMaxDuration)
 capture:=newRemoteLongTaskCapture()
 job:=&remoteLongTaskJob{
  taskID:taskID,started:time.Now(),cancel:cancel,
  done:make(chan struct{}),capture:capture,status:"running",exitCode:-1,
 }
 session.longTask=job
 go func(){
  select{
  case <-session.lifetimeDone:cancel()
  case <-session.portalGrant.Done():cancel()
  case <-session.channel.Done():cancel()
  case <-job.done:
  }
 }()
 go s.runRemoteLinearWorkflow(jobCtx,session,job,plan,hash)
 return nil
}

func(s *Server)runRemoteLinearWorkflow(ctx context.Context,
 session *remoteTaskDeckSession,job *remoteLongTaskJob,
 plan []remoteWorkflowStep,hash [32]byte){
 exitCode:=0
 status:="completed"
 for i,step:=range plan{
  if ctx.Err()!=nil||session.closed.Load(){
   status="cancelled";exitCode=-1;break
  }
  // Live Portal authorization and local password/key/role are rechecked
  // before EVERY process, not only before the first workflow action.
  if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,
   session.channel,identity.PermissionTasksRun);err!=nil{
   status="cancelled";exitCode=-1;break
  }
  spec,_,err:=prepareRemoteTaskExecutionInputMode(ctx,s.Workspace,hash,
   step.taskID,0,false,"",true)
  if err!=nil{status="failed";exitCode=-1;break}
  root,err:=filepath.EvalSymlinks(s.Workspace)
  if err!=nil{status="failed";exitCode=-1;break}
  root,err=filepath.Abs(root)
  if err!=nil||validateRemoteWorkflowStepCwd(root,spec.Cwd)!=nil{
   status="failed";exitCode=-1;break
  }
  if err:=s.auditRemoteWorkflowStep(session,job.taskID,
   step.taskID,i+1,len(plan),"attempted",-1);err!=nil{
   status="failed";exitCode=-1;break
  }
  _,_=captureWorkflowText(job.capture,fmt.Sprintf(
   "[TaskDeck workflow] Step %d/%d: Task #%d\n",i+1,len(plan),step.taskID))
  cmd:=exec.CommandContext(ctx,spec.Command,spec.Args...)
  cmd.Dir=spec.Cwd
  cmd.Env=spec.Env
  cmd.Stdout=job.capture
  cmd.Stderr=job.capture
  prepareGitJobProcess(cmd)
  cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
  cmd.WaitDelay=time.Second
  if ctx.Err()!=nil||session.closed.Load(){
   status="cancelled";exitCode=-1;break
  }
  err=cmd.Run()
  killGitJobProcess(cmd)
  code:=0
  if err!=nil{
   var exit *exec.ExitError
   if errors.As(err,&exit)&&exit.ExitCode()>=0&&exit.ExitCode()<=255{
    code=exit.ExitCode()
   }else{code=-1}
  }
  stepStatus:="completed"
  if errors.Is(ctx.Err(),context.DeadlineExceeded){stepStatus="timed_out"
  }else if ctx.Err()!=nil||session.closed.Load(){stepStatus="cancelled"
  }else if err!=nil{stepStatus="failed"}
  _=s.auditRemoteWorkflowStep(session,job.taskID,step.taskID,
   i+1,len(plan),stepStatus,code)
  if stepStatus!="completed"{
   status=stepStatus;exitCode=code;break
  }
 }
 if errors.Is(ctx.Err(),context.DeadlineExceeded){status="timed_out"
 }else if ctx.Err()!=nil||session.closed.Load(){status="cancelled"}
 s.auditRemoteTaskRunFinish(session,job.taskID,status,exitCode,
  time.Since(job.started))
 job.mu.Lock()
 job.status=status
 job.exitCode=exitCode
 job.mu.Unlock()
 close(job.done)
 select{case job.capture.notify<-struct{}{}:default:}
 job.cancel()
}

func captureWorkflowText(c *remoteLongTaskCapture,message string)(int,error){
 if c==nil{return 0,remotetransport.ErrRemoteActionDenied}
 return c.Write([]byte(message))
}
