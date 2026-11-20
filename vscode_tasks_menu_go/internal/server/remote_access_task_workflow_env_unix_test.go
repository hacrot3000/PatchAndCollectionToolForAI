//go:build linux || darwin

package server

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

// Only host-owned fixed values are admitted. The E2EE browser transmits
// the previously issued root Task.ID, never environment keys or values.
func TestRemoteWorkflowFixedEnvSequential(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_ENV","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 if err:=os.Mkdir(filepath.Join(s.Workspace,"build-dir"),0700);err!=nil{
  t.Fatal(err)
 }
 source:=`{"tasks":[
  {"label":"Read environment","type":"process","command":"/usr/bin/printenv",
   "args":["TD_WORKFLOW_MODE"],
   "options":{"cwd":"build-dir","env":{"TD_WORKFLOW_MODE":"ENV-SEQUENTIAL-VALUE"}}},
  {"label":"Finish","type":"process","command":"/bin/echo",
   "args":["ENV-WORKFLOW-OK"],"dependsOn":"Read environment"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Finish")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("fixed host env was denied: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="completed"{
  t.Fatalf("fixed env workflow failed: %s",got)
 }
 output:=remoteWorkflowCwdFullCapture(session.longTask)
 if !strings.Contains(output,"ENV-SEQUENTIAL-VALUE")||
  !strings.Contains(output,"ENV-WORKFLOW-OK"){
  t.Fatalf("host-authored fixed env not passed to native task: %q",output)
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

func TestRemoteParallelWorkflowFixedEnvWorkers(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_ENV","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Left","type":"process","command":"/usr/bin/printenv",
   "args":["TD_WORKFLOW_MODE"],
   "options":{"env":{"TD_WORKFLOW_MODE":"ENV-PARALLEL-LEFT"}}},
  {"label":"Right","type":"process","command":"/usr/bin/printenv",
   "args":["TD_WORKFLOW_MODE"],
   "options":{"env":{"TD_WORKFLOW_MODE":"ENV-PARALLEL-RIGHT"}}},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["ENV-PARALLEL-JOIN"],"dependsOn":["Left","Right"],
   "dependsOrder":"parallel"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("parallel fixed environment denied: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="completed"{
  t.Fatalf("parallel fixed env failed: %s",got)
 }
 output:=remoteWorkflowCwdFullCapture(session.longTask)
 for _,wanted:=range []string{"ENV-PARALLEL-LEFT","ENV-PARALLEL-RIGHT","ENV-PARALLEL-JOIN"}{
  if !strings.Contains(output,wanted){
   t.Fatalf("missing %q from host workflow env: %q",wanted,output)
  }
 }
 if err:=session.clearRemoteLongTask(root);err!=nil{t.Fatal(err)}
}

func TestRemoteWorkflowFixedEnvPolicyRejectsUnsafeOptions(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_ENV","1")
 for _,name:=range []string{
  "PATH","HOME","GIT_CONFIG_COUNT","LD_PRELOAD","DYLD_INSERT_LIBRARIES",
  "PYTHONPATH","SSH_ASKPASS","NODE_OPTIONS","JAVA_TOOL_OPTIONS",
  "TASKDECK_REMOTE_TASKS_RUN","IFS","BAD-NAME","BAD NAME","1NOTVALID",
  strings.Repeat("A",33),
 }{
  if remoteWorkflowSafeEnvName(name){
   t.Fatalf("unsafe workflow env variable name admitted: %q",name)
  }
 }
 for _,name:=range []string{"BUILD_MODE","TD_WORKFLOW_MODE","CI","_CUSTOM"}{
  if !remoteWorkflowSafeEnvName(name){
   t.Fatalf("safe fixed workflow env name denied: %q",name)
  }
 }
 valid:=map[string]any{"TD_WORKFLOW_MODE":"fixed build","BUILD_MODE":"\u0024{workspaceFolderBasename}"}
 if !remoteWorkflowSafeEnvOption(valid){
  t.Fatal("safe fixed env values denied")
 }
 bad:=[]map[string]any{
  {"PATH":"/tmp"},
  {"LD_PRELOAD":"/tmp/lib.so"},
  {"TD_WORKFLOW_MODE":"\u0024{env:HOME}"},
  {"TD_WORKFLOW_MODE":"\u0024{input:user}"},
  {"TD_WORKFLOW_MODE":"bad\nnewline"},
  {"TD_WORKFLOW_MODE":123},
  {"TD_WORKFLOW_MODE":strings.Repeat("X",257)},
  {},
 }
 for _,env:=range bad{
  if remoteWorkflowSafeEnvOption(env){
   t.Fatalf("unsafe fixed workflow env admitted: %#v",env)
  }
 }
 taskOptions:=map[string]any{"cwd":".","env":valid}
 if !remoteWorkflowSafeProcessOptions(taskOptions){
  t.Fatal("fixed env and cwd should compose")
 }
 if remoteWorkflowSafeProcessOptions(map[string]any{
  "env":valid,"shell":map[string]any{"executable":"/bin/sh"},
 }){
  t.Fatal("shell accepted alongside fixed workflow environment")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_ENV","0")
 if remoteWorkflowSafeEnvOption(valid)||
  remoteWorkflowSafeProcessOptions(taskOptions){
  t.Fatal("host env opt-out allowed fixed values")
 }
}

func TestRemoteWorkflowFixedEnvOptOutDeniedBeforeJob(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_ENV","0")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Env step","type":"process","command":"/bin/echo",
   "args":["NEVER-STARTED"],
   "options":{"env":{"TD_WORKFLOW_MODE":"test"}}},
  {"label":"Join","type":"process","command":"/bin/echo",
   "dependsOn":"Env step"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
  t.Fatal("workflow with fixed env executed without host approval")
 }
 session.longTaskMu.Lock()
 job:=session.longTask
 session.longTaskMu.Unlock()
 if job!=nil{t.Fatal("unapproved env allocated workflow job")}
}

func TestRemoteWorkflowFixedEnvRevocationStopsActiveStep(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_ENV","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Wait","type":"process","command":"/bin/sleep","args":["8"],
   "options":{"env":{"TD_WORKFLOW_MODE":"active"}}},
  {"label":"Never","type":"process","command":"/bin/echo",
   "args":["FORBIDDEN-SECOND-STEP"],"dependsOn":"Wait"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Never")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{t.Fatal(err)}
 job:=session.longTask
 deadline:=time.Now().Add(3*time.Second)
 started:=false
 for time.Now().Before(deadline){
  job.mu.Lock()
  started=strings.HasPrefix(job.workflowStepHistory,"R")
  job.mu.Unlock()
  if started{break}
  time.Sleep(20*time.Millisecond)
 }
 if !started{t.Fatal("fixed-env process did not reach running state")}
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_ENV","0")
 if got:=awaitRemoteWorkflow(t,session);got!="cancelled"{
  t.Fatalf("revoked fixed-env workflow stayed active: %q",got)
 }
 if strings.Contains(remoteWorkflowCwdFullCapture(job),"FORBIDDEN-SECOND-STEP"){
  t.Fatal("revocation did not block following task")
 }
}

func TestRemoteParallelWorkflowFixedEnvRevocationStopsFork(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_ENV","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Left","type":"process","command":"/bin/sleep","args":["8"],
   "options":{"env":{"TD_WORKFLOW_MODE":"one"}}},
  {"label":"Right","type":"process","command":"/bin/sleep","args":["8"]},
  {"label":"Join","type":"process","command":"/bin/echo",
   "args":["FORBIDDEN-ROOT"],"dependsOn":["Left","Right"],
   "dependsOrder":"parallel"}]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{t.Fatal(err)}
 job:=session.longTask
 deadline:=time.Now().Add(3*time.Second)
 started:=false
 for time.Now().Before(deadline){
  job.mu.Lock()
  started=job.workflowStepHistory=="RR-"
  job.mu.Unlock()
  if started{break}
  time.Sleep(20*time.Millisecond)
 }
 if !started{t.Fatal("parallel workers did not start before host opt-out")}
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_ENV","0")
 if got:=awaitRemoteWorkflow(t,session);got!="cancelled"{
  t.Fatalf("parallel fixed-env opt-out did not cancel fork: %q",got)
 }
 if strings.Contains(remoteWorkflowCwdFullCapture(job),"FORBIDDEN-ROOT"){
  t.Fatal("revocation allowed parallel root")
 }
}
