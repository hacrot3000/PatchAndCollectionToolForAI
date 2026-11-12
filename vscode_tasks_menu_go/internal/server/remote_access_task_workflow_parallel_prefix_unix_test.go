//go:build linux || darwin

package server

import(
 "context"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// One explicitly host-approved sequential prerequisite may precede two
// or three simultaneous process leaves, then exactly one final join.
// The browser still sends only the previously issued root Task.ID.
func TestRemoteWorkflowSerialPreludeThenParallelJoin(t *testing.T){
 for _,leaves:=range []int{2,3}{
  t.Run(fmt.Sprintf("%d_leaves",leaves),func(t *testing.T){
   remoteWorkflowHostFlags(t)
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX","1")
   inputs:=[]string{
    `{"label":"Prepare","type":"process",
      "command":"/bin/echo","args":["PREFIX-COMPLETED"]}`,
   }
   labels:=[]string{}
   for i:=0;i<leaves;i++{
    label:=fmt.Sprintf("Worker %d",i+1)
    labels=append(labels,label)
    inputs=append(inputs,fmt.Sprintf(
     `{"label":%q,"type":"process","command":"/bin/sleep",
      "args":["2"],"dependsOn":"Prepare"}`,label))
   }
   deps:=[]string{}
   for _,label:=range labels{deps=append(deps,fmt.Sprintf("%q",label))}
   inputs=append(inputs,fmt.Sprintf(
    `{"label":"Join","type":"process","command":"/bin/echo",
     "args":["JOIN-AFTER-PREFIX-AND-LEAVES"],"dependsOn":[%s],
     "dependsOrder":"parallel"}`,strings.Join(deps,",")))
   source:=`{"tasks":[`+strings.Join(inputs,",")+`]}`
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
   graph,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
   if err!=nil||!graph.RunAvailable||len(graph.Order)!=leaves+2||
    len(graph.Edges)!=2*leaves{
    t.Fatalf("host rejected prefixed parallel DAG: %+v %v",graph,err)
   }
   plan,_,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,root)
   if err!=nil||!plan[len(plan)-1].parallelJoin||
    plan[len(plan)-1].parallelPrefix!=1{
    t.Fatalf("prefix shape not preserved at execution: %+v %v",plan,err)
   }
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
    t.Fatalf("opted-in prefix DAG did not start: %v",err)
   }
   job:=session.longTask
   overlapping:=false
   deadline:=time.Now().Add(1600*time.Millisecond)
   want:="C"+strings.Repeat("R",leaves)+"-"
   for time.Now().Before(deadline){
    job.mu.Lock()
    current:=job.workflowStepHistory
    job.mu.Unlock()
    if current==want{overlapping=true;break}
    time.Sleep(10*time.Millisecond)
   }
   if !overlapping{t.Fatalf("workers never overlapped after prefix: want %q",want)}
   if status:=awaitRemoteWorkflow(t,session);status!="completed"{
    t.Fatalf("prefixed parallel DAG status=%q",status)
   }
   job.mu.Lock()
   history:=job.workflowStepHistory
   code:=job.exitCode
   job.mu.Unlock()
   if history!=strings.Repeat("C",leaves+2)||code!=0{
    t.Fatalf("signed outcomes incomplete: %q, exit=%d",history,code)
   }
   job.capture.mu.Lock()
   output:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if strings.Count(output,"PREFIX-COMPLETED")!=1||
    strings.Count(output,"JOIN-AFTER-PREFIX-AND-LEAVES")!=1||
    strings.Index(output,"PREFIX-COMPLETED")>=
     strings.Index(output,"JOIN-AFTER-PREFIX-AND-LEAVES"){
    t.Fatalf("prefix/root ordering or exactly-once violated: %q",output)
   }
   progress,err:=longTaskE2EEAction(t,s,session,client,agent,
    remotetransport.RemoteActionTasksPoll,root,0,"",
    "38383838383838383838383838383838")
   cap:=48
   if leaves==3{cap=16}
   if err!=nil||!progress.Parallel||progress.StepTotal!=leaves+2||
    progress.StepHistory!=strings.Repeat("C",leaves+2)||
    len(progress.Output)>cap{
    t.Fatalf("signed prefixed parallel progress rejected: %+v %v",progress,err)
   }
  })
 }
}

func TestRemoteWorkflowParallelPrefixRequiresExtraHostOptIn(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX","0")
 const source=`{"tasks":[
  {"label":"Prepare","type":"process","command":"/bin/echo"},
  {"label":"Leaf A","type":"process","command":"/bin/echo","dependsOn":"Prepare"},
  {"label":"Leaf B","type":"process","command":"/bin/echo","dependsOn":"Prepare"},
  {"label":"Join","type":"process","command":"/bin/echo",
   "dependsOn":["Leaf A","Leaf B"],"dependsOrder":"parallel"}
 ]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 graph,err:=s.readRemoteTaskDependencies(context.Background(),session,id)
 if err!=nil||!graph.Available||graph.RunAvailable{
  t.Fatalf("prefix shape advertised without extra opt-in: %+v %v",graph,err)
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err==nil{
  t.Fatal("prefixed fork was launched without specific host opt-in")
 }
}

func TestRemoteWorkflowParallelPrefixFailureStopsAllBranches(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX","1")
 const source=`{"tasks":[
  {"label":"Prepare","type":"process","command":"/bin/false"},
  {"label":"Leaf A","type":"process","command":"/bin/echo",
   "args":["UNSAFE-LEAF-A"],"dependsOn":"Prepare"},
  {"label":"Leaf B","type":"process","command":"/bin/echo",
   "args":["UNSAFE-LEAF-B"],"dependsOn":"Prepare"},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["UNSAFE-ROOT"],"dependsOn":["Leaf A","Leaf B"],
   "dependsOrder":"parallel"}
 ]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("host prefixed workflow failed to launch: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="failed"{
  t.Fatalf("failed prefix incorrectly started fork: %q",got)
 }
 job:=session.longTask
 job.mu.Lock()
 history:=job.workflowStepHistory
 job.mu.Unlock()
 if history!="F---"{t.Fatalf("unrun leaves/root got outcomes: %q",history)}
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Contains(output,"UNSAFE-"){
  t.Fatalf("dependent process ran after prefix failure: %q",output)
 }
}

func TestRemoteWorkflowParallelPrefixLiveOptOutStopsPrerequisite(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX","1")
 const source=`{"tasks":[
  {"label":"Prepare","type":"process","command":"/bin/sleep","args":["8"]},
  {"label":"Leaf A","type":"process","command":"/bin/echo","args":["UNSAFE-A"],
   "dependsOn":"Prepare"},
  {"label":"Leaf B","type":"process","command":"/bin/echo","args":["UNSAFE-B"],
   "dependsOn":"Prepare"},
  {"label":"Join","type":"process","command":"/bin/echo","args":["UNSAFE-JOIN"],
   "dependsOn":["Leaf A","Leaf B"],"dependsOrder":"parallel"}
 ]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 job:=session.longTask
 until:=time.Now().Add(2*time.Second)
 for time.Now().Before(until){
  job.mu.Lock()
  history:=job.workflowStepHistory
  job.mu.Unlock()
  if history=="R---"{break}
  time.Sleep(10*time.Millisecond)
 }
 job.mu.Lock()
 started:=job.workflowStepHistory=="R---"
 job.mu.Unlock()
 if !started{t.Fatal("prerequisite never entered running state")}
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX","0")
 if state:=awaitRemoteWorkflow(t,session);state!="cancelled"{
  t.Fatalf("live prefix opt-out left process alive: %q",state)
 }
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Contains(output,"UNSAFE-"){
  t.Fatalf("prefix opt-out started leaves/root: %q",output)
 }
}

// This audit hook edits the host-owned source after the prelude's durable
// attempted row. The original E2EE-issued snapshot must become unusable.
type remotePrefixChangeDuringAttempt struct{
 identity.Store
 filename string
}
func(store remotePrefixChangeDuringAttempt)AppendAudit(ctx context.Context,
 event identity.AuditEvent)error{
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action=="remote.action.tasks_workflow_step"&&
  event.Result=="attempted"&&strings.Contains(event.Details,`"step":1`){
  data,err:=os.ReadFile(store.filename)
  if err!=nil{return err}
  return os.WriteFile(store.filename,append(data,' '),0600)
 }
 return nil
}
func TestRemoteWorkflowParallelPrefixAuditDriftStopsBeforeExecution(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX","1")
 const source=`{"tasks":[
  {"label":"Prepare","type":"process","command":"/bin/echo","args":["UNSAFE-PREFIX"]},
  {"label":"Leaf A","type":"process","command":"/bin/echo","dependsOn":"Prepare"},
  {"label":"Leaf B","type":"process","command":"/bin/echo","dependsOn":"Prepare"},
  {"label":"Join","type":"process","command":"/bin/echo",
   "dependsOn":["Leaf A","Leaf B"],"dependsOrder":"parallel"}
 ]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 s.Identity=remotePrefixChangeDuringAttempt{
  Store:s.Identity,filename:filepath.Join(s.Workspace,".vscode","tasks.json"),
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if state:=awaitRemoteWorkflow(t,session);state!="failed"{
  t.Fatalf("stale prefix was executed: %q",state)
 }
 job:=session.longTask
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Contains(output,"UNSAFE-PREFIX"){
  t.Fatalf("stale prefix ran despite attempted-audit source drift: %q",output)
 }
}
