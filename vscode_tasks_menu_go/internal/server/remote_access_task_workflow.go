package server

import(
 "context"
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

type remoteWorkflowStep struct{ taskID int; retries int }

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


// A retry is an explicit potentially repeated side effect, not a safe
// default. Admit ONLY the host-authored retry:1 workflow policy and ONLY
// with its own default-OFF host opt-in. Do not accept conditions, outputs,
// timeouts, dynamic inputs or other taskdeck metadata. This is deliberately
// narrower than the ordinary local TaskDeck workflow policy.
func remoteWorkflowRetryForTask(task tasks.Task)(int,bool){
 raw,exists:=task.Raw["taskdeck"]
 if !exists||raw==nil{return 0,true}
 if os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY")!="1"{
  return 0,false
 }
 meta,ok:=raw.(map[string]any)
 if !ok||len(meta)!=1{return 0,false}
 config,ok:=meta["workflow"].(map[string]any)
 if !ok||len(config)!=1{return 0,false}
 switch n:=config["retry"].(type){
 case float64:
  if n==1{return 1,true}
 case int:
  if n==1{return 1,true}
 }
 return 0,false
}

func remoteWorkflowSafeProcess(task tasks.Task)bool{
 if task.Type!="process"||len(task.Inputs)!=0||
  len(task.SecretEnv)!=0||len(task.Options)!=0{return false}
 if _,ok:=remoteWorkflowRetryForTask(task);!ok{return false}
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
  retries,ok:=remoteWorkflowRetryForTask(byID[id])
  if !ok{return nil,hash,denied}
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
  plan=append(plan,remoteWorkflowStep{taskID:id,retries:retries})
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
 rootID,stepID,index,total,attempt int,status string,exit int)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Identity==nil||session==nil||rootID<1||stepID<1||
  index<1||index>total||total>remoteWorkflowMaxSteps||
  attempt<1||attempt>2{return denied}
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
  Details:fmt.Sprintf("{\"transport\":\"e2ee\",\"root_task_id\":%d,\"task_id\":%d,\"step\":%d,\"total\":%d,\"attempt\":%d,\"exit_code\":%d}",
   rootID,stepID,index,total,attempt,exit),
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
Workflow:
 for i,step:=range plan{
  // A failed native process may run ONCE more only with an explicit
  // host-authorized retry:1 policy. Each try re-checks permission, source
  // hash, cwd, executable identity and durable start/finish audit.
  for attempt:=1;attempt<=step.retries+1;attempt++{
   if ctx.Err()!=nil||session.closed.Load(){
    status="cancelled";exitCode=-1;break Workflow
   }
   if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,
    session.channel,identity.PermissionTasksRun);err!=nil{
    status="cancelled";exitCode=-1;break Workflow
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
    step.taskID,i+1,len(plan),attempt,"attempted",-1);err!=nil{
    status="failed";exitCode=-1;break Workflow
   }
   executable,ok:=remoteWorkflowResolvedExecutable(spec.Command)
   if !ok||!tasks.RemoteExecutableAllowed(root,executable){
    status="failed";exitCode=-1;break Workflow
   }
   _,_=captureWorkflowText(job.capture,fmt.Sprintf(
    "[TaskDeck workflow] Step %d/%d: Task #%d (attempt %d/%d)\n",
    i+1,len(plan),step.taskID,attempt,step.retries+1))
   cmd:=exec.CommandContext(ctx,executable,spec.Args...)
   cmd.Dir=spec.Cwd
   cmd.Env=spec.Env
   cmd.Stdout=job.capture
   cmd.Stderr=job.capture
   prepareGitJobProcess(cmd)
   cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
   cmd.WaitDelay=time.Second
   if ctx.Err()!=nil||session.closed.Load(){
    status="cancelled";exitCode=-1;break Workflow
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
   // The result audit is a hard boundary between attempts, including
   // retries of the SAME step. A failed audit can never trigger a replay.
   if err:=s.auditRemoteWorkflowStep(session,job.taskID,step.taskID,
    i+1,len(plan),attempt,stepStatus,code);err!=nil{
    status="failed";exitCode=-1
    _,_=captureWorkflowText(job.capture,fmt.Sprintf(
     "[TaskDeck workflow] Step %d/%d: audit unavailable; stopped\n",
     i+1,len(plan)))
    break Workflow
   }
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
   status=stepStatus;exitCode=code;break Workflow
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
