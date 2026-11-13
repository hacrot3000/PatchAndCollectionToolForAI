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
 "sync/atomic"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Bounded workflow executor: two to five fixed, non-interactive process
// tasks for explicit sequential dependencies (linear or multi-parent).
// Parallel joins remain limited to two through four independent leaves
// plus one root. Implicit parallel diamonds, shell scripts, secrets and
// interpolated inputs remain denied. No browser-supplied graph or command.
const remoteWorkflowMaxSteps=5

type remoteWorkflowStep struct{
 taskID int
 retries int
 timeout time.Duration
 condition string // "" or "success" normally; "always" only at the final root.
 parallelJoin bool // Host-opted-in fork/join; never caller supplied.
 parallelPrefix int // 0–2 serial prerequisites before the parallel leaves.
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


// Validate three host-authored shapes, never an arbitrary DAG:
// independent 2–4-leaf fork, 1 serial prelude + 2–3 leaves, or TWO
// serial preludes + exactly two leaves, all ending in a single root.
// The two-prelude shape requires two independent extra opt-ins.
// Return -1 denied; 0/1/2 serial preludes before the fork.
func remoteWorkflowParallelPrefixLength(items []tasks.Task,
 order []int,edges [][2]int)int{
 n:=len(order)
 if os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL")!="1"||
  n<3||n>remoteWorkflowMaxSteps{return -1}
 byID:=make(map[int]tasks.Task,len(items))
 byLabel:=make(map[string]tasks.Task,len(items))
 for _,task:=range items{
  if _,duplicate:=byID[task.ID];duplicate{return -1}
  if _,duplicate:=byLabel[task.Label];duplicate{return -1}
  byID[task.ID]=task
  byLabel[task.Label]=task
 }
 seen:=make(map[int]bool,n)
 for _,id:=range order{
  task,ok:=byID[id]
  if !ok||seen[id]||!remoteWorkflowSafeProcess(task){return -1}
  seen[id]=true
  retry,timeout,condition,ok:=remoteWorkflowPolicyForTask(task)
  if !ok||retry!=0||timeout!=0||condition!=""{return -1}
 }
 root:=byID[order[n-1]]
 join:=tasks.WorkflowNodeForTask(root)
 if join.DependsOrder!="parallel"{return -1}
 prefix:=0
 switch len(join.DependsOn){
 case n-1: // Flat fork.
 case n-2:
  if n<4||os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX")!="1"{
   return -1
  }
  prefix=1
 case n-3:
  // Five total steps: Prepare A -> Prepare B -> two leaves -> root.
  // A second prelude must not be implicitly enabled by PREFIX alone.
  if n!=5||os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX")!="1"||
   os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX2")!="1"{
   return -1
  }
  prefix=2
 default:return -1
 }
 leaves:=order[prefix:n-1]
 if len(leaves)<2||len(leaves)>4{return -1}
 expected:=make(map[[2]int]bool,2*len(leaves)+prefix)
 if prefix>=1{
  first:=tasks.WorkflowNodeForTask(byID[order[0]])
  if len(first.DependsOn)!=0{return -1}
 }
 if prefix==2{
  second:=tasks.WorkflowNodeForTask(byID[order[1]])
  if len(second.DependsOn)!=1||
   second.DependsOn[0]!=byID[order[0]].Label{return -1}
  expected[[2]int{order[0],order[1]}]=true
 }
 for _,id:=range leaves{
  node:=tasks.WorkflowNodeForTask(byID[id])
  if prefix==0{
   if len(node.DependsOn)!=0{return -1}
  }else{
   // TaskDeck normalizes an omitted dependsOrder to "parallel".
   // With exactly ONE prerequisite the order has no effect; deny
   // multiple dependencies instead of requiring an artificial
   // dependsOrder: "sequence" on every leaf.
   if len(node.DependsOn)!=1||
    node.DependsOn[0]!=byID[order[prefix-1]].Label{return -1}
   expected[[2]int{order[prefix-1],id}]=true
  }
 }
 for _,label:=range join.DependsOn{
  leaf,ok:=byLabel[label]
  if !ok||!seen[leaf.ID]||leaf.ID==order[n-1]||
   (prefix>0&&
    (leaf.ID==order[0]||(prefix==2&&leaf.ID==order[1]))){return -1}
  expected[[2]int{leaf.ID,order[n-1]}]=true
 }
 if len(expected)!=len(edges){return -1}
 for _,edge:=range edges{
  if !expected[edge]{return -1}
  delete(expected,edge)
 }
 if len(expected)!=0{return -1}
 return prefix
}

func remoteWorkflowSafeParallelJoinGraph(items []tasks.Task,
 order []int,edges [][2]int)bool{
 return remoteWorkflowParallelPrefixLength(items,order,edges)>=0
}

func remoteWorkflowHostExecutionEnabled()bool{
 return os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")=="1"&&
  os.Getenv("TASKDECK_REMOTE_TASKS_DEPS")=="1"&&
  os.Getenv("TASKDECK_REMOTE_TASKS_RUN")=="1"&&
  os.Getenv("TASKDECK_REMOTE_TASKS_LONG")=="1"&&
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RUN")=="1"
}


 // Only active workflows need the TasksRun capability. The independent
 // session watcher validates account/key/membership, but a valid user may
 // lose just TasksRun while retaining read-only Git access. Cancel the
 // current process without revoking that unrelated read-only session.
 // This check is host-local: PHP never supplies a role or process policy.
func(s *Server)remoteWorkflowLocalRunAllowed(ctx context.Context,
 session *remoteTaskDeckSession)bool{
 if s==nil||session==nil||ctx==nil||ctx.Err()!=nil||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){return false}
 principal,err:=s.verifyRemoteSessionLocalPrincipal(ctx,session)
 return err==nil&&ctx.Err()==nil&&
  remotetransport.AuthorizeAction(true,session.role,&principal,
   session.projectKey,identity.PermissionTasksRun)==nil
}


// After a process exits, re-check host-local RBAC before persisting a
// successful/failed outcome or admitting a retry/cleanup/root. The 500ms
// active-process watcher cannot catch a permission downgrade that races a
// short-lived process. This cannot undo already completed side effects:
// it only denies a false success and any following workflow process.
func(s *Server)remoteWorkflowPostExitAuthorized(ctx context.Context,
 session *remoteTaskDeckSession,step remoteWorkflowStep,attempt int,
 parallel bool)bool{
 if ctx==nil||ctx.Err()!=nil||session==nil||session.closed.Load()||
  !remoteWorkflowHostExecutionEnabled()||
  (parallel&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL")!="1")||
  (attempt>1&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY")!="1")||
  (step.timeout>0&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT")!="1")||
  (step.condition!=""&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION")!="1"){
  return false
 }
 checkCtx,done:=context.WithTimeout(ctx,remoteLocalRevokeCheckDeadline)
 defer done()
 return s.remoteWorkflowLocalRunAllowed(checkCtx,session)
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
 parallelPrefix:=remoteWorkflowParallelPrefixLength(list,deps.Order,deps.Edges)
 parallel:=parallelPrefix>=0
 if !remoteWorkflowSafeSequentialGraph(list,deps.Order,deps.Edges)&&!parallel{
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
   parallelJoin:parallel&&id==rootTaskID,
   parallelPrefix:func()int{if id==rootTaskID{return parallelPrefix};return 0}(),
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
 // Unlike a child process, the long job context intentionally outlives
 // the native GUI's short Start HTTP request. If host opt-in, RBAC or the
 // request are revoked during the aggregate audit, do not create a
 // background job slot that will never execute an authorized step.
 // A denied attempted audit still receives its terminal audit outcome.
 last:=plan[len(plan)-1]
 parallel:=len(plan)>=3&&last.parallelJoin
 if ctx.Err()!=nil||session.closed.Load()||
  !remoteWorkflowHostExecutionEnabled()||
  !s.remoteShortTaskHostConsent(ctx,session)||
  (parallel&&!remoteParallelWorkflowConsent(session,last.parallelPrefix)){
  state:="cancelled"
  if errors.Is(ctx.Err(),context.DeadlineExceeded){state="timed_out"}
  s.auditRemoteTaskRunFinish(session,taskID,state,-1,0)
  return denied
 }
 jobCtx,cancel:=context.WithTimeout(context.Background(),remoteLongTaskMaxDuration)
 capture:=newRemoteLongTaskCapture()
 job:=&remoteLongTaskJob{
  taskID:taskID,started:time.Now(),cancel:cancel,
  done:make(chan struct{}),capture:capture,status:"running",exitCode:-1,
  workspaceRelease:releaseWorkspace,
  workflowRunID:runID,
  workflowParallel:len(plan)>=3&&plan[len(plan)-1].parallelJoin,
  workflowStepHistory:strings.Repeat("-",len(plan)),
  workflowStepDetails:func()[]string{
   parts:=make([]string,len(plan))
   for i:=range parts{parts[i]="-"}
   return parts
  }(),
 }
 // Guard once more after creating the in-memory descriptor but before
 // assigning the session slot. No child goroutine has been started yet.
 // The outer defer still owns the workspace lease on this denial.
 if ctx.Err()!=nil||session.closed.Load()||
  !remoteWorkflowHostExecutionEnabled()||
  !s.remoteShortTaskHostConsent(ctx,session)||
  (parallel&&!remoteParallelWorkflowConsent(session,last.parallelPrefix)){
  cancel()
  state:="cancelled"
  if errors.Is(ctx.Err(),context.DeadlineExceeded){state="timed_out"}
  s.auditRemoteTaskRunFinish(session,taskID,state,-1,0)
  return denied
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
 if len(plan)>=3&&plan[len(plan)-1].parallelJoin{
  go s.runRemoteParallelJoinWorkflow(jobCtx,session,job,plan,hash)
 }else{
  go s.runRemoteLinearWorkflow(jobCtx,session,job,plan,hash)
 }
 return nil
}


// A sequential workflow is an independently audited chain of executable
// processes. A host-local writer may change tasks.json, executable symlinks,
// inherited environment or cwd while a step audit/RBAC check runs.
// Compare the exact pre-audit approval again at the final launch boundary.
// Unlike a generic task-runner, the E2EE client supplies no command/argv.
func(s *Server)revalidateRemoteLinearStepAtLaunch(
 ctx context.Context,session *remoteTaskDeckSession,step remoteWorkflowStep,
 hash [32]byte,attempt int,approved remoteParallelPrepared,approvedRoot string,
)(remoteParallelPrepared,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remoteParallelPrepared
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  session.closed.Load()||!remoteWorkflowHostExecutionEnabled()||
  (attempt>1&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY")!="1")||
  (step.timeout>0&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT")!="1")||
  (step.condition!=""&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION")!="1"){
  return empty,denied
 }
 if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,
  session.channel,identity.PermissionTasksRun);err!=nil{return empty,denied}
 execution,_,err:=prepareRemoteTaskExecutionInputMode(
  ctx,s.Workspace,hash,step.taskID,0,false,"",true)
 if err!=nil||execution.TaskID!=step.taskID{return empty,denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil||root!=approvedRoot||
  validateRemoteWorkflowStepCwd(root,execution.Cwd)!=nil||
  !tasks.RemoteExecutableAllowed(root,execution.Command){
  return empty,denied
 }
 cwd,err:=filepath.EvalSymlinks(execution.Cwd)
 if err!=nil{return empty,denied}
 cwd,err=filepath.Abs(cwd)
 if err!=nil||validateRemoteWorkflowStepCwd(root,cwd)!=nil{
  return empty,denied
 }
 executable,ok:=remoteWorkflowResolvedExecutable(execution.Command)
 if !ok||!tasks.RemoteExecutableAllowed(root,executable){return empty,denied}
 current:=remoteParallelPrepared{
  execution:execution,executable:executable,resolvedCwd:cwd,
 }
 if !remoteParallelSameExecution(current,approved)||ctx.Err()!=nil||
  session.closed.Load(){return empty,denied}
 return current,nil
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
   // The executable MUST be resolved BEFORE the attempted audit. In the
   // old sequence a host writer could retarget a symlink during that
   // audit, and both later checks would approve the substituted binary.
   executable,ok:=remoteWorkflowResolvedExecutable(spec.Command)
   if !ok||!tasks.RemoteExecutableAllowed(root,executable){
    status="failed";exitCode=-1;break Workflow
   }
   resolvedCwd,err:=filepath.EvalSymlinks(spec.Cwd)
   if err!=nil{status="failed";exitCode=-1;break Workflow}
   resolvedCwd,err=filepath.Abs(resolvedCwd)
   if err!=nil||validateRemoteWorkflowStepCwd(root,resolvedCwd)!=nil{
    status="failed";exitCode=-1;break Workflow
   }
   approved:=remoteParallelPrepared{
    execution:spec,executable:executable,resolvedCwd:resolvedCwd,
   }
   // An unavailable audit denies this attempt BEFORE any process starts.
   if err:=s.auditRemoteWorkflowStep(session,job.taskID,
    step.taskID,i+1,len(plan),attempt,job.workflowRunID,"attempted",-1);err!=nil{
    status="failed";exitCode=-1;break Workflow
   }
   // A durable "attempted" record does NOT authorize executing a
   // stale command. The host can change tasks.json, inherited environment,
   // permissions or approved client keys during the audit write. Recheck
   // the full process identity BEFORE ANY subprocess (including retries
   // and conditional cleanup) and persist the aborted outcome. Never
   // replay an attempt just because the prelaunch guard failed.
   deniedState:=""
   if ctx.Err()!=nil||session.closed.Load()||
    !remoteWorkflowHostExecutionEnabled()||
    (step.timeout>0&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT")!="1")||
    (attempt>1&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY")!="1")||
    (step.condition!=""&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION")!="1"){
    deniedState="cancelled"
   }else if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,
    session.channel,identity.PermissionTasksRun);err!=nil{
    deniedState="cancelled"
   }else if _,err:=s.revalidateRemoteLinearStepAtLaunch(
    ctx,session,step,hash,attempt,approved,root);err!=nil{
    deniedState="failed"
   }
   if deniedState!=""{
    if err:=s.auditRemoteWorkflowStep(session,job.taskID,
     step.taskID,i+1,len(plan),attempt,job.workflowRunID,
     deniedState,-1);err!=nil{
     deniedState="failed"
    }
    job.updateWorkflowStep(i+1,len(plan),attempt,step.retries+1,
     deniedState,-1,0)
    _,_=captureWorkflowText(job.capture,fmt.Sprintf(
     "[TaskDeck workflow] Step %d/%d: post-audit prelaunch denied; not started\n",
     i+1,len(plan)))
    status=deniedState;exitCode=-1;break Workflow
   }
   job.updateWorkflowStep(i+1,len(plan),attempt,step.retries+1,"running",-1,0)
   _,_=captureWorkflowText(job.capture,fmt.Sprintf(
    "[TaskDeck workflow] Step %d/%d: Task #%d (attempt %d/%d)\n",
    i+1,len(plan),step.taskID,attempt,step.retries+1))
   // Every step needs a cancellable child context, even without a
   // timeout: a host can revoke Run/Workflow/Retry/Timeout/Condition
   // DURING a running process. A live native GUI Poll is not required.
   var runCtx context.Context
   var cancelStep context.CancelFunc
   if step.timeout>0{
    runCtx,cancelStep=context.WithTimeout(ctx,step.timeout)
   }else{
    runCtx,cancelStep=context.WithCancel(ctx)
   }
   cmd:=exec.CommandContext(runCtx,approved.executable,approved.execution.Args...)
   // Keep host-authored argv[0], but never follow its lexical executable
   // symlink again after the pre-audit identity was pinned.
   cmd.Args[0]=approved.execution.Command
   cmd.Dir=approved.resolvedCwd
   cmd.Env=approved.execution.Env
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
   // The running process (including retry and conditional cleanup)
   // must stop on a host policy revocation. A bounded watcher shares
   // the process context and terminates with the worker; it does not
   // poll the Portal or expose host flags to the remote client.
   var revokedDuringStep atomic.Bool
   watcherDone:=make(chan struct{})
   stopWatch:=make(chan struct{})
   go func(){
    defer close(watcherDone)
    ticker:=time.NewTicker(100*time.Millisecond)
    defer ticker.Stop()
    permissionTicker:=time.NewTicker(500*time.Millisecond)
    defer permissionTicker.Stop()
    for{
     select{
     case <-stopWatch:return
     case <-runCtx.Done():return
     case <-permissionTicker.C:
      // A host may downgrade this user's TasksRun permission without
      // disabling the account or revoking the E2EE login. Never let a
      // previously admitted child keep mutating after that downgrade.
      checkCtx,done:=context.WithTimeout(runCtx,2*time.Second)
      allowed:=s.remoteWorkflowLocalRunAllowed(checkCtx,session)
      done()
      if !allowed{
       revokedDuringStep.Store(true)
       cancelStep()
       return
      }
     case <-ticker.C:
      if !remoteWorkflowHostExecutionEnabled()||
       (step.timeout>0&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT")!="1")||
       (attempt>1&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY")!="1")||
       (step.condition!=""&&os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION")!="1"){
       revokedDuringStep.Store(true)
       cancelStep()
       return
      }
     }
    }
   }()
   // The local RBAC query and watchdog startup also yield to host writers.
   // Re-check the EXACT process a final time, inside the cancellable
   // launch context, before creating any external side effects.
   if _,checkErr:=s.revalidateRemoteLinearStepAtLaunch(
    runCtx,session,step,hash,attempt,approved,root);checkErr!=nil{
    // Snapshot the original denial reason BEFORE cancelling our own
    // child context. Otherwise cancelStep makes every stale-source or
    // changed-executable failure look like an operator cancellation.
    deniedState:="failed"
    if errors.Is(runCtx.Err(),context.DeadlineExceeded){
     deniedState="timed_out"
    }else if runCtx.Err()!=nil||session.closed.Load()||
     !remoteWorkflowHostExecutionEnabled(){deniedState="cancelled"}
    close(stopWatch)
    <-watcherDone
    cancelStep()
    if auditErr:=s.auditRemoteWorkflowStep(session,job.taskID,
     step.taskID,i+1,len(plan),attempt,job.workflowRunID,
     deniedState,-1);auditErr!=nil{deniedState="failed"}
    job.updateWorkflowStep(i+1,len(plan),attempt,step.retries+1,
     deniedState,-1,0)
    _,_=captureWorkflowText(job.capture,fmt.Sprintf(
     "[TaskDeck workflow] Step %d/%d: final launch preflight denied; not started\n",
     i+1,len(plan)))
    status=deniedState;exitCode=-1;break Workflow
   }
   stepStarted:=time.Now()
   err=cmd.Run()
   close(stopWatch)
   <-watcherDone
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
   }else if revokedDuringStep.Load()||ctx.Err()!=nil||session.closed.Load(){
    stepStatus="cancelled"
    if revokedDuringStep.Load(){
     code= -1
     _,_=captureWorkflowText(job.capture,
      "[TaskDeck workflow] Host permission revoked; active step stopped\n")
    }
   }else if !s.remoteWorkflowPostExitAuthorized(ctx,session,step,attempt,false){
    // Covers the interval between the last watcher tick and cmd.Run exit.
    // Do not report a successful dependency or start its retry/cleanup.
    stepStatus="cancelled";code=-1
    _,_=captureWorkflowText(job.capture,
     "[TaskDeck workflow] Post-exit host permission denied; next step blocked\n")
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
