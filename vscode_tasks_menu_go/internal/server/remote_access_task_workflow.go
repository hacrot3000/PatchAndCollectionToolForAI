package server

import(
 "context"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "fmt"
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

// Bounded workflow executor: two to four fixed, non-interactive process
// tasks. In addition to linear chains it accepts explicitly sequential DAG
// joins; implicit parallel joins, shell tasks, policies, secrets and
// interpolated inputs remain denied. No browser-supplied graph or command.
const remoteWorkflowMaxSteps=4

type remoteWorkflowStep struct{
 taskID int
 retries int
 timeout time.Duration
 condition string // "" or "success" normally; "always" only at the final root.
}

// Process workflows intentionally exclude executable scripts, including
// shebang files with innocuous names. Check native object headers rather than
// trusting only executable mode and a command basename.
func remoteWorkflowNativeExecutable(path string)bool{
 file,err:=os.Open(path)
 if err!=nil{return false}
 defer file.Close()
 var magic [4]byte
 if _,err:=io.ReadFull(file,magic[:]);err!=nil{return false}
 switch runtime.GOOS{
 case "linux":
  return magic==[4]byte{0x7f,'E','L','F'}
 case "darwin":
  switch magic{
  case [4]byte{0xfe,0xed,0xfa,0xce}, // Mach-O 32-bit
       [4]byte{0xce,0xfa,0xed,0xfe},
       [4]byte{0xfe,0xed,0xfa,0xcf}, // Mach-O 64-bit
       [4]byte{0xcf,0xfa,0xed,0xfe},
       [4]byte{0xca,0xfe,0xba,0xbe}, // FAT/universal
       [4]byte{0xbe,0xba,0xfe,0xca},
       [4]byte{0xca,0xfe,0xba,0xbf}, // FAT64
       [4]byte{0xbf,0xba,0xfe,0xca}:
   return true
  }
 }
 return false
}

// Resolve executable aliases before checking for interpreters. Looking only
// at the user-supplied basename allowed a symlink called "safe-tool" to
// bypass the workflow's explicit shell/interpreter denylist.
func remoteWorkflowResolvedExecutable(command string)(string,bool){
 if command==""||!filepath.IsAbs(command)||
  strings.ContainsAny(command,"\x00\r\n"){return "",false}
 resolved,err:=filepath.EvalSymlinks(command)
 if err!=nil||!filepath.IsAbs(resolved){return "",false}
 info,err:=os.Stat(resolved)
 if err!=nil||!info.Mode().IsRegular()||info.Mode().Perm()&0111==0||
  !remoteWorkflowNativeExecutable(resolved){return "",false}
 switch strings.ToLower(filepath.Base(resolved)){
 case "bash","sh","dash","zsh","ksh","fish","csh","tcsh","pwsh","powershell",
      "python","python3","perl","ruby","node","php","env","sudo","su":
  return "",false
 }
 return resolved,true
}


// Only a tiny, independently opted-in subset of TaskDeck workflow
// policies is executable remotely. The host owns this metadata; browser
// callers cannot set retry counts, timeouts, command or arguments.
// Per-step timeouts must not outlive the existing 90-second job deadline.
func remoteWorkflowPolicyForTask(task tasks.Task)(int,time.Duration,string,bool){
 raw,exists:=task.Raw["taskdeck"]
 if !exists||raw==nil{return 0,0,"",true}
 meta,ok:=raw.(map[string]any)
 if !ok||len(meta)!=1{return 0,0,"",false}
 config,ok:=meta["workflow"].(map[string]any)
 if !ok||len(config)<1||len(config)>3{return 0,0,"",false}
 retries:=0
 var timeout time.Duration
 condition:=""
 for key,value:=range config{
  if key=="condition"{
   c,ok:=value.(string)
   if !ok||os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION")!="1"{
    return 0,0,"",false
   }
   // Only these two non-branching modes are currently supported.
   // "failure" and continueOnError require a separate graph contract.
   if c!="success"&&c!="always"&&c!="failure"{
    return 0,0,"",false
   }
   condition=c
   continue
  }
  integer:=0
  switch v:=value.(type){
  case float64:
   if v!=float64(int(v)){return 0,0,"",false}
   integer=int(v)
  case int:
   integer=v
  default:
   return 0,0,"",false
  }
  switch key{
  case "retry":
   if integer!=1||os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY")!="1"{
    return 0,0,"",false
   }
   retries=1
  case "timeoutSeconds":
   if integer<1||integer>30||
    os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT")!="1"{
    return 0,0,"",false
   }
   timeout=time.Duration(integer)*time.Second
  default:
   return 0,0,"",false
  }
 }
 return retries,timeout,condition,true
}

func remoteWorkflowSafeProcess(task tasks.Task)bool{
 if task.Type!="process"||len(task.Inputs)!=0||
  len(task.SecretEnv)!=0||len(task.Options)!=0{return false}
 if _,_,_,ok:=remoteWorkflowPolicyForTask(task);!ok{return false}
 command,ok:=task.Command.(string)
 if !ok||strings.Contains(command,"${"){return false}
 if _,safe:=remoteWorkflowResolvedExecutable(command);!safe{return false}
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


// A shared HOST-ONLY gate for both GUI RunAvailable and actual execution.
// A multi-parent join is admitted only when the author explicitly set
// dependsOrder:"sequence"; the usual TaskDeck parallel join is denied.
// All nodes execute once in topological order, and any nonzero exit stops
// before later nodes. This is intentionally NOT parallel execution.
func remoteWorkflowSafeSequentialGraph(items []tasks.Task,
 order []int,edges [][2]int)bool{
 if len(order)<2||len(order)>remoteWorkflowMaxSteps||
  len(edges)<len(order)-1||
  len(edges)>remotetransport.MaxRemoteTaskDependencyEdges{return false}
 byID:=make(map[int]tasks.Task,len(items))
 byLabel:=make(map[string]tasks.Task,len(items))
 for _,task:=range items{
  if _,duplicate:=byID[task.ID];duplicate{return false}
  if _,duplicate:=byLabel[task.Label];duplicate{return false}
  byID[task.ID]=task
  byLabel[task.Label]=task
 }
 positions:=make(map[int]int,len(order))
 for i,id:=range order{
  task,exists:=byID[id]
  if !exists||!remoteWorkflowSafeProcess(task){return false}
  _,_,condition,ok:=remoteWorkflowPolicyForTask(task)
  // A conditional recovery/cleanup task can only be the root/final
  // node. Earlier branches would bypass dependencies without a DAG
  // scheduler or a separate result-authority contract.
  if !ok||((condition=="always"||condition=="failure")&&
   i!=len(order)-1){return false}
  if _,duplicate:=positions[id];duplicate{return false}
  positions[id]=i
 }
 expected:=make(map[[2]int]bool,len(edges))
 for _,id:=range order{
  task:=byID[id]
  node:=tasks.WorkflowNodeForTask(task)
  if len(node.DependsOn)>1&&node.DependsOrder!="sequence"{return false}
  for _,parentLabel:=range node.DependsOn{
   parent,exists:=byLabel[parentLabel]
   parentIndex,inPlan:=positions[parent.ID]
   if !exists||!inPlan||parentIndex>=positions[id]{return false}
   expected[[2]int{parent.ID,id}]=true
  }
 }
 if len(expected)!=len(edges){return false}
 seen:=make(map[[2]int]bool,len(edges))
 for _,edge:=range edges{
  if !expected[edge]||seen[edge]{return false}
  seen[edge]=true
 }
 // No unrelated process can be executed by adding it to the response:
 // every published node must be a transitive dependency of the root.
 reachable:=map[int]bool{order[len(order)-1]:true}
 for i:=len(order)-1;i>=0;i--{
  id:=order[i]
  if !reachable[id]{continue}
  for edge:=range expected{
   if edge[1]==id{reachable[edge[0]]=true}
  }
 }
 return len(reachable)==len(order)
}

func remoteWorkflowHostExecutionEnabled()bool{
 return os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")=="1"&&
  os.Getenv("TASKDECK_REMOTE_TASKS_DEPS")=="1"&&
  os.Getenv("TASKDECK_REMOTE_TASKS_RUN")=="1"&&
  os.Getenv("TASKDECK_REMOTE_TASKS_LONG")=="1"&&
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RUN")=="1"
}

func(s *Server)prepareRemoteLinearWorkflow(ctx context.Context,
 session *remoteTaskDeckSession,rootTaskID int,
)([]remoteWorkflowStep,[32]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var hash [32]byte
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  runtime.GOOS=="windows"||
  !remoteWorkflowHostExecutionEnabled()||
  !session.taskPageReady||!session.taskIssuedIDs[rootTaskID]{
  return nil,hash,denied
 }
 deps,err:=s.readRemoteTaskDependencies(ctx,session,rootTaskID)
 if err!=nil||!deps.Available||len(deps.Order)<2||
  len(deps.Order)>remoteWorkflowMaxSteps{return nil,hash,denied}
 hash=session.taskPageHash
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return nil,hash,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return nil,hash,denied}
 list,err:=tasks.Load(root)
 if err!=nil||len(list)>remotetransport.MaxRemoteTasksCount{return nil,hash,denied}
 if !remoteWorkflowSafeSequentialGraph(list,deps.Order,deps.Edges){
  return nil,hash,denied
 }
 byID:=make(map[int]tasks.Task,len(list))
 for _,item:=range list{byID[item.ID]=item}
 plan:=make([]remoteWorkflowStep,0,len(deps.Order))
 for _,id:=range deps.Order{
  retries,timeout,condition,ok:=remoteWorkflowPolicyForTask(byID[id])
  if !ok||((condition=="always"||condition=="failure")&&id!=rootTaskID){
   return nil,hash,denied
  }
  // Reuse the existing, separately tested single-task source/inode,
  // workspace boundary and stable-ID preflight, with the ONLY exception
  // that the host-owned dependency label is now allowed.
  spec,_,err:=prepareRemoteTaskExecutionInputMode(
   ctx,s.Workspace,hash,id,0,false,"",true)
  if err!=nil||spec.TaskID!=id{return nil,hash,denied}
  if err:=validateRemoteWorkflowStepCwd(root,spec.Cwd);err!=nil||
   !tasks.RemoteExecutableAllowed(root,spec.Command){
   return nil,hash,denied
  }
  plan=append(plan,remoteWorkflowStep{
   taskID:id,retries:retries,timeout:timeout,condition:condition,
  })
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
 rootID,stepID,index,total,attempt int,runID,status string,exit int)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Identity==nil||session==nil||rootID<1||stepID<1||
  index<1||index>total||total>remoteWorkflowMaxSteps||
  attempt<1||attempt>2||
  !validRemoteWorkflowRunID(runID){return denied}
 // A skipped step is a verified decision, not a launched process:
 // never allow a caller to forge skipped intermediate work or invent
 // a successful exit. Its sole legal location is the final root step.
 switch status{
 case "attempted":
  if exit!= -1{return denied}
 case "skipped":
  if index!=total||stepID!=rootID||attempt!=1||exit!= -1{return denied}
 case "completed":
  if exit!=0{return denied}
 case "failed","cancelled","timed_out":
  if exit< -1||exit>255{return denied}
 default:
  return denied
 }
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
  Details:fmt.Sprintf("{\"transport\":\"e2ee\",\"root_task_id\":%d,\"task_id\":%d,\"step\":%d,\"total\":%d,\"attempt\":%d,\"exit_code\":%d,\"run_id\":%q}",
   rootID,stepID,index,total,attempt,exit,runID),
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
 releaseWorkspace,err:=reserveRemoteWorkspaceExecution(ctx,s.Workspace)
 if err!=nil{return denied}
 launched:=false
 defer func(){if !launched{releaseWorkspace()}}()
 plan,hash,err:=s.prepareRemoteLinearWorkflow(ctx,session,taskID)
 if err!=nil{return denied}
 // Create a new random identifier for this execution, not the task
 // definition. It is never used as authority or a transferable token.
 var nonce [8]byte
 if _,err:=rand.Read(nonce[:]);err!=nil{return denied}
 runID:=hex.EncodeToString(nonce[:])
 // Durable aggregate attempted audit is BEFORE any irreversible start.
 if err:=s.auditRemoteTaskRunAttempt(ctx,session,taskID);err!=nil{return denied}
 jobCtx,cancel:=context.WithTimeout(context.Background(),remoteLongTaskMaxDuration)
 capture:=newRemoteLongTaskCapture()
 job:=&remoteLongTaskJob{
  taskID:taskID,started:time.Now(),cancel:cancel,
  done:make(chan struct{}),capture:capture,status:"running",exitCode:-1,
  workspaceRelease:releaseWorkspace,
  workflowRunID:runID,
  workflowStepHistory:strings.Repeat("-",len(plan)),
  workflowStepDetails:func()[]string{
   parts:=make([]string,len(plan))
   for i:=range parts{parts[i]="-"}
   return parts
  }(),
 }
 session.longTask=job
 launched=true
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
Workflow:
 for i,step:=range plan{
  // A failed native process may run ONCE more only with an explicit
  // host-authorized retry:1 policy. Each try re-checks permission, source
  // hash, cwd, executable identity and durable start/finish audit.
  for attempt:=1;attempt<=step.retries+1;attempt++{
   // Host revocation takes effect before the NEXT process/attempt,
   // even when the E2EE session and TaskDeck role remain valid.
   if !remoteWorkflowHostExecutionEnabled(){
    status="cancelled";exitCode=-1;break Workflow
   }
   // Disabling a host opt-in while a workflow is already running also
   // revokes future retries/timeout-governed steps.
   if (attempt>1&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY")!="1")||
    (step.timeout>0&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT")!="1")||
    (step.condition!=""&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION")!="1"){
    status="failed";exitCode=-1;break Workflow
   }
   if ctx.Err()!=nil||session.closed.Load(){
    status="cancelled";exitCode=-1;break Workflow
   }
   if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,
    session.channel,identity.PermissionTasksRun);err!=nil{
    status="cancelled";exitCode=-1;break Workflow
   }
   if i==len(plan)-1&&step.condition=="failure"&&
    status=="completed"&&exitCode==0{
    // The host-owned failure-only final step must not execute after
    // successful dependencies. It is a documented skip, not a fake
    // successful process (no attempted audit and no side effects).
    // Persist a step outcome with NO attempted/process audit. Without
    // this row, re-authenticating later falsely shows a partial run.
    // Failure to append the host audit fails closed; no process is started.
    if err:=s.auditRemoteWorkflowStep(session,job.taskID,step.taskID,
     i+1,len(plan),1,job.workflowRunID,"skipped",-1);err!=nil{
     status="failed";exitCode=-1
     job.updateWorkflowStep(i+1,len(plan),1,step.retries+1,"failed",-1,0)
     _,_=captureWorkflowText(job.capture,
      "[TaskDeck workflow] Recovery skip audit unavailable; stopped\n")
     break Workflow
    }
    job.updateWorkflowStep(i+1,len(plan),1,step.retries+1,"skipped",-1,0)
    _,_=captureWorkflowText(job.capture,fmt.Sprintf(
     "[TaskDeck workflow] Step %d/%d: skipped (condition failure not met)\n",
     i+1,len(plan)))
    break Workflow
   }
   spec,_,err:=prepareRemoteTaskExecutionInputMode(ctx,s.Workspace,hash,
    step.taskID,0,false,"",true)
   if err!=nil{status="failed";exitCode=-1;break Workflow}
   root,err:=filepath.EvalSymlinks(s.Workspace)
   if err!=nil{status="failed";exitCode=-1;break Workflow}
   root,err=filepath.Abs(root)
   if err!=nil||validateRemoteWorkflowStepCwd(root,spec.Cwd)!=nil||
    !tasks.RemoteExecutableAllowed(root,spec.Command){
    status="failed";exitCode=-1;break Workflow
   }
   // An unavailable audit denies this attempt BEFORE any process starts.
   if err:=s.auditRemoteWorkflowStep(session,job.taskID,
    step.taskID,i+1,len(plan),attempt,job.workflowRunID,"attempted",-1);err!=nil{
    status="failed";exitCode=-1;break Workflow
   }
   executable,ok:=remoteWorkflowResolvedExecutable(spec.Command)
   if !ok||!tasks.RemoteExecutableAllowed(root,executable){
    // Even an executable failure after the durable attempted audit must
    // produce a final, content-free step outcome whenever storage works.
    _=s.auditRemoteWorkflowStep(session,job.taskID,
     step.taskID,i+1,len(plan),attempt,job.workflowRunID,"failed",-1)
    job.updateWorkflowStep(i+1,len(plan),attempt,step.retries+1,"failed",-1,0)
    status="failed";exitCode=-1;break Workflow
   }
   job.updateWorkflowStep(i+1,len(plan),attempt,step.retries+1,"running",-1,0)
   _,_=captureWorkflowText(job.capture,fmt.Sprintf(
    "[TaskDeck workflow] Step %d/%d: Task #%d (attempt %d/%d)\n",
    i+1,len(plan),step.taskID,attempt,step.retries+1))
   runCtx:=ctx
   cancelStep:=func(){}
   if step.timeout>0{
    runCtx,cancelStep=context.WithTimeout(ctx,step.timeout)
   }
   cmd:=exec.CommandContext(runCtx,executable,spec.Args...)
   cmd.Dir=spec.Cwd
   cmd.Env=spec.Env
   cmd.Stdout=job.capture
   cmd.Stderr=job.capture
   prepareGitJobProcess(cmd)
   cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
   cmd.WaitDelay=time.Second
   if ctx.Err()!=nil||session.closed.Load()||
    !remoteWorkflowHostExecutionEnabled()||
    (step.timeout>0&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT")!="1")||
    (attempt>1&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY")!="1")||
    (step.condition!=""&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION")!="1"){
    cancelStep()
    _=s.auditRemoteWorkflowStep(session,job.taskID,
     step.taskID,i+1,len(plan),attempt,job.workflowRunID,"cancelled",-1)
    job.updateWorkflowStep(i+1,len(plan),attempt,step.retries+1,"cancelled",-1,0)
    status="cancelled";exitCode=-1;break Workflow
   }
   stepStarted:=time.Now()
   err=cmd.Run()
   elapsed:=time.Since(stepStarted)
   killGitJobProcess(cmd)
   timedOut:=errors.Is(runCtx.Err(),context.DeadlineExceeded)
   cancelStep()
   code:=0
   if err!=nil{
    var exit *exec.ExitError
    if errors.As(err,&exit)&&exit.ExitCode()>=0&&exit.ExitCode()<=255{
     code=exit.ExitCode()
    }else{code=-1}
   }
   stepStatus:="completed"
   if timedOut||errors.Is(ctx.Err(),context.DeadlineExceeded){stepStatus="timed_out"
   }else if ctx.Err()!=nil||session.closed.Load(){stepStatus="cancelled"
   }else if err!=nil{stepStatus="failed"}
   // The result audit is a hard boundary between attempts, including
   // retries of the SAME step. A failed audit can never trigger a replay.
   if err:=s.auditRemoteWorkflowStep(session,job.taskID,step.taskID,
    i+1,len(plan),attempt,job.workflowRunID,stepStatus,code);err!=nil{
    job.updateWorkflowStep(i+1,len(plan),attempt,step.retries+1,"failed",-1,0)
    status="failed";exitCode=-1
    _,_=captureWorkflowText(job.capture,fmt.Sprintf(
     "[TaskDeck workflow] Step %d/%d: audit unavailable; stopped\n",
     i+1,len(plan)))
    break Workflow
   }
   job.updateWorkflowStep(i+1,len(plan),attempt,step.retries+1,stepStatus,code,elapsed)
   _,_=captureWorkflowText(job.capture,fmt.Sprintf(
    "[TaskDeck workflow] Step %d/%d: %s (exit=%d, attempt=%d)\n",
    i+1,len(plan),stepStatus,code,attempt))
   if stepStatus=="completed"{break}
   if stepStatus=="failed"&&code>0&&attempt<=step.retries{
    _,_=captureWorkflowText(job.capture,fmt.Sprintf(
     "[TaskDeck workflow] Step %d/%d: retry authorized (one repeat)\n",
     i+1,len(plan)))
    continue
   }
   if stepStatus=="failed"&&i==len(plan)-2&&
    (plan[i+1].condition=="always"||
     plan[i+1].condition=="failure")&&
    os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION")=="1"{
    // Permit ONLY the final, host-declared cleanup after the immediately
    // preceding failure. The aggregate result stays failed even if the
    // cleanup succeeds. Cancellation and timeouts never trigger cleanup.
    status="failed";exitCode=code
    continue Workflow
   }
   // Preserve the original dependency's failure code if the last
   // cleanup/recovery process also failed. Its own outcome still
   // appears separately in per-step history and audit.
   if !(i==len(plan)-1&&status=="failed"&&stepStatus=="failed"&&
    (step.condition=="always"||step.condition=="failure")){
    status=stepStatus;exitCode=code
   }
   break Workflow
  }
 }
 // No workflow subprocess is still running after this loop. Close Cancel
 // admission BEFORE the final aggregate status read and host-local audit.
 acceptedCancel:=job.beginFinalizing()
 if errors.Is(ctx.Err(),context.DeadlineExceeded){status="timed_out"
 }else if acceptedCancel||ctx.Err()!=nil||session.closed.Load(){status="cancelled"}
 s.auditRemoteTaskRunFinish(session,job.taskID,status,exitCode,
  time.Since(job.started))
 // Share the long-task finalization barrier: reject Cancel during
 // workspace release, and publish a final Poll status only AFTER release.
 job.publishCompletion(status,exitCode)
 job.cancel()
}

func captureWorkflowText(c *remoteLongTaskCapture,message string)(int,error){
 if c==nil{return 0,remotetransport.ErrRemoteActionDenied}
 return c.Write([]byte(message))
}
