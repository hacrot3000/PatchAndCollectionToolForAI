//go:build linux || darwin

package server

import (
 "context"
 "strings"
 "testing"
 "time"
)

// Independent worker deadlines are host-specified. A timeout must
// cancel running siblings and block the final root even when their
// asynchronous terminal results arrive in the opposite order.
func TestRemoteParallelWorkflowWorkerTimeoutCancelsSiblings(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL_TIMEOUT","1")
 source:=`{"tasks":[
  {"label":"Timeout Worker","type":"process","command":"/bin/sleep",
   "args":["5"],"taskdeck":{"workflow":{"timeoutSeconds":1}}},
  {"label":"Sibling","type":"process","command":"/bin/sleep","args":["8"]},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["FORBIDDEN-JOIN"],"dependsOn":["Timeout Worker","Sibling"],
   "dependsOrder":"parallel"}]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!deps.RunAvailable{
  t.Fatalf("host-approved timed parallel workflow not offered: %+v %v",deps,err)
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("timed parallel workflow did not start: %v",err)
 }
 if state:=awaitRemoteWorkflow(t,session);state!="timed_out"{
  t.Fatalf("worker deadline became %q instead of timed_out",state)
 }
 session.longTask.mu.Lock()
 history:=session.longTask.workflowStepHistory
 session.longTask.mu.Unlock()
 if len(history)!=3||history[0]!='T'||history[2]!='-'{
  t.Fatalf("timed worker history did not block join: %q",history)
 }
 output:=remoteWorkflowCwdFullCapture(session.longTask)
 if strings.Contains(output,"FORBIDDEN-JOIN"){
  t.Fatal("parallel root started after worker deadline")
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

func TestRemoteParallelWorkflowRootDeadline(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL_TIMEOUT","1")
 source:=`{"tasks":[
  {"label":"Leaf A","type":"process","command":"/bin/true"},
  {"label":"Leaf B","type":"process","command":"/bin/true"},
  {"label":"Join","type":"process","command":"/bin/sleep","args":["5"],
   "taskdeck":{"workflow":{"timeoutSeconds":1}},
   "dependsOn":["Leaf A","Leaf B"],"dependsOrder":"parallel"}]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="timed_out"{
  t.Fatalf("root deadline not enforced: %s",got)
 }
 session.longTask.mu.Lock()
 history:=session.longTask.workflowStepHistory
 session.longTask.mu.Unlock()
 if history!="CCT"{t.Fatalf("root was not timed out after completed leaves: %q",history)}
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

func TestRemoteParallelWorkflowPreludeDeadlineStopsFork(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL_TIMEOUT","1")
 source:=`{"tasks":[
  {"label":"Prepare","type":"process","command":"/bin/sleep",
   "args":["5"],"taskdeck":{"workflow":{"timeoutSeconds":1}}},
  {"label":"Left","type":"process","command":"/bin/echo",
   "args":["NEVER-LEFT"],"dependsOn":"Prepare"},
  {"label":"Right","type":"process","command":"/bin/echo",
   "args":["NEVER-RIGHT"],"dependsOn":"Prepare"},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["NEVER-JOIN"],"dependsOn":["Left","Right"],
   "dependsOrder":"parallel"}]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="timed_out"{
  t.Fatalf("timed-out prelude incorrectly started the fork: %q",got)
 }
 session.longTask.mu.Lock()
 history:=session.longTask.workflowStepHistory
 session.longTask.mu.Unlock()
 if history!="T---"{t.Fatalf("prelude deadline did not stop dependent steps: %q",history)}
 transcript:=remoteWorkflowCwdFullCapture(session.longTask)
 if strings.Contains(transcript,"NEVER-"){
  t.Fatalf("fork started after prelude deadline: %q",transcript)
 }
}

func TestRemoteParallelWorkflowTimeoutRequiresIndependentHostOptIn(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL_TIMEOUT","0")
 source:=`{"tasks":[
  {"label":"Timed","type":"process","command":"/bin/sleep","args":["3"],
   "taskdeck":{"workflow":{"timeoutSeconds":1}}},
  {"label":"Other","type":"process","command":"/bin/true"},
  {"label":"Join","type":"process","command":"/bin/echo",
   "dependsOn":["Timed","Other"],"dependsOrder":"parallel"}]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!deps.Available||deps.RunAvailable{
  t.Fatalf("unapproved timeout exposed executable workflow: %+v %v",deps,err)
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
  t.Fatal("parallel timeout executed with host flag disabled")
 }
 session.longTaskMu.Lock()
 job:=session.longTask
 session.longTaskMu.Unlock()
 if job!=nil{t.Fatal("denied timeout allocated background job")}
}

func TestRemoteParallelWorkflowActiveTimeoutOptOutCancelsWorkers(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL_TIMEOUT","1")
 source:=`{"tasks":[
  {"label":"A","type":"process","command":"/bin/sleep","args":["8"],
   "taskdeck":{"workflow":{"timeoutSeconds":6}}},
  {"label":"B","type":"process","command":"/bin/sleep","args":["8"]},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["FORBIDDEN-ROOT"],"dependsOn":["A","B"],
   "dependsOrder":"parallel"}]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 job:=session.longTask
 deadline:=time.Now().Add(3*time.Second)
 started:=false
 for time.Now().Before(deadline){
  job.mu.Lock()
  running:=job.workflowStepHistory=="RR-"
  job.mu.Unlock()
  if running{started=true;break}
  time.Sleep(20*time.Millisecond)
 }
 if !started{
  t.Fatal("parallel workers never started before timeout opt-out")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL_TIMEOUT","0")
 if got:=awaitRemoteWorkflow(t,session);got!="cancelled"{
  t.Fatalf("host timeout opt-out did not cancel both workers: %q",got)
 }
 if strings.Contains(remoteWorkflowCwdFullCapture(job),"FORBIDDEN-ROOT"){
  t.Fatal("root launched after timeout opt-out")
 }
}
