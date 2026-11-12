//go:build linux || darwin

package server

import (
 "context"
 "fmt"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func remoteDoublePrefixFixture(t *testing.T,secondCommand string,secondArgs string)(
 *Server,*remoteTaskDeckSession,*remotetransport.Channel,*remotetransport.Channel,int){
 t.Helper()
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX2","1")
 source:=fmt.Sprintf(`{"tasks":[
 {"label":"Prepare One","type":"process","command":"/bin/echo","args":["FIRST-PRELUDE"]},
 {"label":"Prepare Two","type":"process","command":%q,"args":[%s],
  "dependsOn":"Prepare One"},
 {"label":"Leaf A","type":"process","command":"/bin/sleep",
  "args":["2"],"dependsOn":"Prepare Two"},
 {"label":"Leaf B","type":"process","command":"/bin/sleep",
  "args":["2"],"dependsOn":"Prepare Two"},
 {"label":"Join","type":"process","command":"/bin/echo",
  "args":["DOUBLE-PREFIX-JOIN"],"dependsOn":["Leaf A","Leaf B"],
  "dependsOrder":"parallel"}
 ]}`,secondCommand,secondArgs)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 return s,session,client,agent,root
}

func TestRemoteWorkflowDoublePrefixThenParallelJoin(t *testing.T){
 s,session,client,agent,root:=remoteDoublePrefixFixture(t,
  "/bin/echo",`"SECOND-PRELUDE"`)
 graph,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!graph.RunAvailable||len(graph.Order)!=5||len(graph.Edges)!=5{
  t.Fatalf("host rejected two-prelude fork: %+v %v",graph,err)
 }
 plan,_,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,root)
 if err!=nil||!plan[4].parallelJoin||plan[4].parallelPrefix!=2{
  t.Fatalf("double-prefix type lost: %+v %v",plan,err)
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("host-approved two-prelude workflow failed to start: %v",err)
 }
 job:=session.longTask
 deadline:=time.Now().Add(1600*time.Millisecond)
 overlap:=false
 for time.Now().Before(deadline){
  job.mu.Lock()
  progress:=job.workflowStepHistory
  job.mu.Unlock()
  if progress=="CCRR-"{overlap=true;break}
  time.Sleep(10*time.Millisecond)
 }
 if !overlap{t.Fatal("two leaves never overlapped after BOTH preludes")}
 if got:=awaitRemoteWorkflow(t,session);got!="completed"{
  t.Fatalf("double-prefix workflow finished as %s",got)
 }
 job.mu.Lock()
 history,exitCode:=job.workflowStepHistory,job.exitCode
 job.mu.Unlock()
 if history!="CCCCC"||exitCode!=0{
  t.Fatalf("signed five-step outcomes missing: %q exit=%d",history,exitCode)
 }
 job.capture.mu.Lock()
 text:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 sequence:=[]string{"FIRST-PRELUDE","SECOND-PRELUDE","DOUBLE-PREFIX-JOIN"}
 last:=-1
 for _,part:=range sequence{
  if strings.Count(text,part)!=1{
   t.Fatalf("process was skipped or replayed (%q): %q",part,text)
  }
  i:=strings.Index(text,part)
  if i<=last{t.Fatalf("serial prelude/root order changed: %q",text)}
  last=i
 }
 progress,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,root,0,"",
  "45454545454545454545454545454545")
 if err!=nil||!progress.Parallel||progress.StepTotal!=5||
  progress.StepHistory!="CCCCC"||len(progress.Output)>16{
  t.Fatalf("five-step double-prefix E2EE invalid: %+v %v",progress,err)
 }
}

func TestRemoteWorkflowDoublePrefixRequiresIndependentOptIn(t *testing.T){
 s,session,_,_,root:=remoteDoublePrefixFixture(t,
  "/bin/echo",`"SECOND-PRELUDE"`)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX2","0")
 graph,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!graph.Available||graph.RunAvailable{
  t.Fatalf("double prelude enabled by single opt-in: %+v %v",graph,err)
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
  t.Fatal("two preludes started without second host opt-in")
 }
}

func TestRemoteWorkflowDoublePrefixSecondFailureBlocksChildren(t *testing.T){
 s,session,_,_,root:=remoteDoublePrefixFixture(t,"/bin/false",`"unimportant"`)
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("failed second prelude should start for audited outcome: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="failed"{
  t.Fatalf("failed second prelude started children/root: %q",got)
 }
 job:=session.longTask
 job.mu.Lock()
 history:=job.workflowStepHistory
 job.mu.Unlock()
 if history!="CF---"{t.Fatalf("unexpected branches after prelude failure: %q",history)}
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Contains(output,"DOUBLE-PREFIX-JOIN"){
  t.Fatalf("join ran after failed second prelude: %q",output)
 }
}

func TestRemoteWorkflowDoublePrefixRevocationStopsSecondPrelude(t *testing.T){
 s,session,_,_,root:=remoteDoublePrefixFixture(t,"/bin/sleep",`"8"`)
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 job:=session.longTask
 deadline:=time.Now().Add(3*time.Second)
 secondRunning:=false
 for time.Now().Before(deadline){
  job.mu.Lock()
  current:=job.workflowStepHistory
  job.mu.Unlock()
  if current=="CR---"{secondRunning=true;break}
  time.Sleep(10*time.Millisecond)
 }
 if !secondRunning{t.Fatal("second prelude did not start")}
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX2","0")
 if got:=awaitRemoteWorkflow(t,session);got!="cancelled"{
  t.Fatalf("second prelude ignored live opt-out: %q",got)
 }
 job.mu.Lock()
 history:=job.workflowStepHistory
 job.mu.Unlock()
 if history[0]!='C'||strings.ContainsAny(history[2:],"RC"){
  t.Fatalf("children or join ran after second opt-out: %q",history)
 }
}
