//go:build linux || darwin

package server

import(
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

const remoteWorkflowLinearSource=`{"tasks":[
 {"label":"Step A","type":"process","command":"/bin/echo","args":["ONE"]},
 {"label":"Step B","type":"process","command":"/bin/echo","args":["TWO"],"dependsOn":"Step A"},
 {"label":"Step C","type":"process","command":"/bin/echo","args":["THREE"],"dependsOn":"Step B"}
]}`

func remoteWorkflowHostFlags(t *testing.T){
 t.Helper()
 for _,env:=range []string{
  "TASKDECK_REMOTE_TASKS_PREVIEW","TASKDECK_REMOTE_TASKS_DEPS",
  "TASKDECK_REMOTE_TASKS_RUN","TASKDECK_REMOTE_TASKS_LONG",
  "TASKDECK_REMOTE_TASKS_WORKFLOW_RUN",
 }{t.Setenv(env,"1")}
}

func remoteWorkflowIssuedRoot(t *testing.T,s *Server,
 session *remoteTaskDeckSession,client,agent *remotetransport.Channel,
 source,rootLabel string)int{
 t.Helper()
 writeRemoteTasksPreviewFixture(t,s.Workspace,source)
 list,err:=tasks.Load(s.Workspace)
 if err!=nil||len(list)==0{t.Fatal(err)}
 root:=0
 for _,task:=range list{if task.Label==rootLabel{root=task.ID}}
 if root==0{t.Fatalf("missing workflow root %q",rootLabel)}
 loaded:=0
 for pageNo:=0;pageNo<16&&loaded<len(list);pageNo++{
  op:=remotetransport.RemoteActionTasksPreviewNext
  if pageNo==0{op=remotetransport.RemoteActionTasksPreview}
  page,err:=remoteTaskMenuAction(t,s,session,client,agent,
   "11111111111111111111111111111111",op)
  if err!=nil{t.Fatal(err)}
  for _,id:=range page.IDs{if id==root{return root}}
  loaded+=len(page.Labels)
  if loaded>=page.Total{break}
 }
 t.Fatalf("host did not E2EE-issue workflow root %d",root)
 return 0
}

func awaitRemoteWorkflow(t *testing.T,session *remoteTaskDeckSession)string{
 t.Helper()
 session.longTaskMu.Lock()
 job:=session.longTask
 session.longTaskMu.Unlock()
 if job==nil{t.Fatal("workflow never reserved host long-task slot")}
 select{
 case <-job.done:
 case <-time.After(6*time.Second):t.Fatal("workflow job did not finish or cancel")
 }
 job.mu.Lock()
 result:=job.status
 job.mu.Unlock()
 return result
}

func TestRemoteWorkflowStartE2EELinearAuditAndClear(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,
  remoteWorkflowLinearSource,"Step C")
 started,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksWorkflowStart,id,0,"",
  "22222222222222222222222222222222")
 if err!=nil||started.Status!="started"{t.Fatalf("E2EE workflow start: %+v %v",started,err)}
 if state:=awaitRemoteWorkflow(t,session);state!="completed"{
  t.Fatalf("linear workflow did not complete: %s",state)
 }
 session.longTaskMu.Lock()
 job:=session.longTask
 session.longTaskMu.Unlock()
 job.capture.mu.Lock()
 got:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Index(got,"Step 1/3")<0||
  !strings.Contains(got,"ONE")||!strings.Contains(got,"TWO")||
  !strings.Contains(got,"THREE")||
  strings.Index(got,"ONE")>strings.Index(got,"TWO")||
  strings.Index(got,"TWO")>strings.Index(got,"THREE"){
  t.Fatalf("non-sequential or missing workflow capture: %q",got)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,UserID:session.userID,
  Action:"remote.action.tasks_workflow_step",Limit:16,
 })
 if err!=nil||len(audit)!=6{t.Fatalf("expected 3 attempted and 3 finished local audits: %+v %v",audit,err)}
 for _,row:=range audit{
  if strings.Contains(row.Details,"ONE")||strings.Contains(row.Details,"TWO")||
   strings.Contains(row.Details,"THREE"){t.Fatal("workflow output persisted to audit")}
 }
 cleared,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksClear,id,0,"",
  "33333333333333333333333333333333")
 if err!=nil||cleared.Status!="cleared"{t.Fatalf("workflow slot not released: %+v %v",cleared,err)}
}

func TestRemoteWorkflowFailureStopsNextStep(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
 {"label":"First","type":"process","command":"/bin/echo","args":["GOOD"]},
 {"label":"Second","type":"process","command":"/bin/false","dependsOn":"First"},
 {"label":"Third","type":"process","command":"/bin/echo","args":["SHOULD-NOT-RUN"],"dependsOn":"Second"}
 ]}`
 id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Third")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err!=nil{t.Fatal(err)}
 if got:=awaitRemoteWorkflow(t,session);got!="failed"{t.Fatalf("failed dependency did not stop workflow: %s",got)}
 job:=session.longTask
 capture,_,_,_:=job.capture.next(0)
 if strings.Contains(capture,"SHOULD-NOT-RUN")||strings.Contains(capture,"Step 3/3"){
  t.Fatalf("workflow ran after failed step: %q",capture)
 }
}

func TestRemoteWorkflowCancelStopsLaterSteps(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
 {"label":"Wait","type":"process","command":"/bin/sleep","args":["20"]},
 {"label":"Next","type":"process","command":"/bin/echo","args":["SHOULD-NOT-RUN"],"dependsOn":"Wait"}
 ]}`
 id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Next")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err!=nil{t.Fatal(err)}
 if err:=session.cancelRemoteLongTask(id);err!=nil{t.Fatal(err)}
 if got:=awaitRemoteWorkflow(t,session);got!="cancelled"{
  t.Fatalf("workflow cancellation did not terminate: %s",got)
 }
 capture,_,_,_:=session.longTask.capture.next(0)
 if strings.Contains(capture,"SHOULD-NOT-RUN"){t.Fatal("cancelled workflow started another step")}
 if session.closed.Load(){t.Fatal("cancelling workflow revoked entire E2EE login")}
}

func TestRemoteWorkflowPreflightFailsClosedOnBranchesPolicyAndDrift(t *testing.T){
 remoteWorkflowHostFlags(t)
 for name,source:=range map[string]string{
  "branch":`{"tasks":[
   {"label":"A","type":"process","command":"/bin/echo"},
   {"label":"B","type":"process","command":"/bin/echo"},
   {"label":"C","type":"process","command":"/bin/echo","dependsOn":["A","B"]}]}`,
  "shell":`{"tasks":[{"label":"A","type":"shell","command":"echo safe"},
   {"label":"B","type":"process","command":"/bin/echo","dependsOn":"A"}]}`,
  "interpreter":`{"tasks":[{"label":"A","type":"process","command":"/bin/sh"},
   {"label":"B","type":"process","command":"/bin/echo","dependsOn":"A"}]}`,
  "policy":`{"tasks":[{"label":"A","type":"process","command":"/bin/echo","taskdeck":{"workflow":{"retry":2}}},
   {"label":"B","type":"process","command":"/bin/echo","dependsOn":"A"}]}`,
  "input":`{"inputs":[{"id":"x","type":"promptString"}],"tasks":[
   {"label":"A","type":"process","command":"/bin/echo","args":["${input:x}"]},
   {"label":"B","type":"process","command":"/bin/echo","dependsOn":"A"}]}`,
 }{
  t.Run(name,func(t *testing.T){
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:= "B"
   if name=="branch"{root="C"}
   id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,root)
   if _,_,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,id);err==nil{
    t.Fatal("unsafe workflow accepted")
   }
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err==nil{
    t.Fatal("unsafe workflow began")
   }
   if session.longTask!=nil{t.Fatal("rejected workflow left a live job")}
  })
 }
 t.Run("source drift",func(t *testing.T){
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,remoteWorkflowLinearSource,"Step C")
  if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),
   []byte(remoteWorkflowLinearSource+" "),0600);err!=nil{t.Fatal(err)}
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err==nil{
   t.Fatal("replaced task source was allowed to start")
  }
 })
 t.Run("opt out",func(t *testing.T){
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,remoteWorkflowLinearSource,"Step C")
  t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RUN","0")
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err==nil{
   t.Fatal("workflow execution bypassed explicit host opt-in")
  }
 })
}

func TestRemoteWorkflowExecutableAliasesAndPermissions(t *testing.T){
 root:=t.TempDir()
 trusted:=filepath.Join(root,"trusted-helper")
 shellAlias:=filepath.Join(root,"trusted-looking-shell")
 missing:=filepath.Join(root,"missing")
 noExec:=filepath.Join(root,"not-executable")
 script:=filepath.Join(root,"trusted-native-name")
 scriptAlias:=filepath.Join(root,"script-symlink")
 textExec:=filepath.Join(root,"executable-text")
 if err:=os.WriteFile(script,[]byte("#!/bin/sh\necho should-not-run\n"),0700);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(script,scriptAlias);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(textExec,[]byte("just text, not native executable"),0700);err!=nil{t.Fatal(err)}
 if err:=os.Symlink("/bin/echo",trusted);err!=nil{t.Fatal(err)}
 if err:=os.Symlink("/bin/sh",shellAlias);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(noExec,[]byte("nothing"),0600);err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{path string;safe bool}{
  {"/bin/echo",true},{trusted,true},{shellAlias,false},
  {missing,false},{noExec,false},{script,false},{scriptAlias,false},
  {textExec,false},{"/bin/sh",false},
  {"/bin/echo\x00ignored",false},
 }{
  task:=tasks.Task{Type:"process",Command:tc.path}
  if got:=remoteWorkflowSafeProcess(task);got!=tc.safe{
   t.Fatalf("executable %q: safe=%t, want %t",tc.path,got,tc.safe)
  }
 }
}

func TestRemoteWorkflowGUIRunAvailableMatchesExecutablePreflight(t *testing.T){
 remoteWorkflowHostFlags(t)
 for name,setup:=range map[string]func(*testing.T)string{
  "missing":func(t *testing.T)string{return filepath.Join(t.TempDir(),"absent")},
  "shell-symlink":func(t *testing.T)string{
   link:=filepath.Join(t.TempDir(),"innocent-name")
   if err:=os.Symlink("/bin/sh",link);err!=nil{t.Fatal(err)}
   return link
  },
  "nonexecutable":func(t *testing.T)string{
   path:=filepath.Join(t.TempDir(),"tool")
   if err:=os.WriteFile(path,[]byte("data"),0600);err!=nil{t.Fatal(err)}
   return path
  },
  "script-executable":func(t *testing.T)string{
   path:=filepath.Join(t.TempDir(),"looks-like-binary")
   if err:=os.WriteFile(path,[]byte("#!/bin/sh\necho never\n"),0700);err!=nil{t.Fatal(err)}
   return path
  },
 }{
  t.Run(name,func(t *testing.T){
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   unsafePath:=setup(t)
   source:=fmt.Sprintf(`{"tasks":[
    {"label":"First","type":"process","command":"/bin/echo"},
    {"label":"Last","type":"process","command":%q,"dependsOn":"First"}
   ]}`,unsafePath)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Last")
   deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
   if err!=nil||!deps.Available||deps.RunAvailable{
    t.Fatalf("GUI offered impossible or unsafe workflow: %+v %v",deps,err)
   }
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
    t.Fatal("unsafe workflow actually started")
   }
   if session.longTask!=nil{t.Fatal("rejected workflow reserved execution slot")}
  })
 }
 t.Run("allowed",func(t *testing.T){
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,
   remoteWorkflowLinearSource,"Step C")
  deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
  if err!=nil||!deps.RunAvailable{
   t.Fatalf("valid executable workflow is not offered: %+v %v",deps,err)
  }
 })
}

func TestRemoteWorkflowExplicitSequentialDiamondRunAndParallelDenial(t *testing.T){
 remoteWorkflowHostFlags(t)
 source:=`{"tasks":[
  {"label":"A","type":"process","command":"/bin/echo","args":["FIRST"]},
  {"label":"B","type":"process","command":"/bin/echo","args":["SECOND"],"dependsOn":"A"},
  {"label":"C","type":"process","command":"/bin/echo","args":["THIRD"],"dependsOn":"A"},
  {"label":"D","type":"process","command":"/bin/echo","args":["FOURTH"],
   "dependsOn":["B","C"],"dependsOrder":"sequence"}
 ]}`
 t.Run("explicit sequence",func(t *testing.T){
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"D")
  info,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
  if err!=nil||!info.RunAvailable||len(info.Order)!=4||
   len(info.Edges)!=4{
   t.Fatalf("explicit sequential diamond not offered: %+v %v",info,err)
  }
  started,err:=longTaskE2EEAction(t,s,session,client,agent,
   remotetransport.RemoteActionTasksWorkflowStart,root,0,"",
   "22222222222222222222222222222222")
  if err!=nil||started.Status!="started"{t.Fatalf("start: %+v %v",started,err)}
  if got:=awaitRemoteWorkflow(t,session);got!="completed"{
   t.Fatalf("sequential diamond failed: %s",got)
  }
  job:=session.longTask
  output,_,_,_:=job.capture.next(0)
  // Chunks are capped; read full host capture only inside this test.
  job.capture.mu.Lock()
  complete:=string(append([]byte(nil),job.capture.data...))
  job.capture.mu.Unlock()
  if !strings.Contains(output,"Step 1/4")||
   !strings.Contains(complete,"FIRST")||
   !strings.Contains(complete,"SECOND")||
   !strings.Contains(complete,"THIRD")||
   !strings.Contains(complete,"FOURTH")||
   strings.Index(complete,"FIRST")>strings.Index(complete,"SECOND")||
   strings.Index(complete,"SECOND")>strings.Index(complete,"THIRD")||
   strings.Index(complete,"THIRD")>strings.Index(complete,"FOURTH"){
   t.Fatalf("expected each dependency exactly once, in serial order: %q",complete)
  }
 })
 t.Run("implicit parallel join",func(t *testing.T){
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  unsafe:=strings.Replace(source,`,"dependsOrder":"sequence"`,"",1)
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,unsafe,"D")
  info,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
  if err!=nil||!info.Available||info.RunAvailable{
   t.Fatalf("parallel join marked executable: %+v %v",info,err)
  }
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
   t.Fatal("implicit parallel join was silently serialized")
  }
  if session.longTask!=nil{t.Fatal("rejected graph reserved a job slot")}
 })
}

 
// Preserve every other identity behavior; inject only a durable step-audit
// failure at the exact transition between workflow child processes.
type remoteWorkflowRejectStepAudit struct{
 identity.Store
 rejectResult string
}
func(store remoteWorkflowRejectStepAudit)AppendAudit(
 ctx context.Context,event identity.AuditEvent,
)error{
 if event.Action=="remote.action.tasks_workflow_step"&&
  event.Result==store.rejectResult{
  return errors.New("injected unavailable step audit")
 }
 return store.Store.AppendAudit(ctx,event)
}

func TestRemoteWorkflowStopsBeforeNextStepIfStepAuditFails(t *testing.T){
 remoteWorkflowHostFlags(t)
 source:=`{"tasks":[
  {"label":"A","type":"process","command":"/bin/echo","args":["FIRST-ONLY"]},
  {"label":"B","type":"process","command":"/bin/echo","args":["MUST-NOT-RUN"],"dependsOn":"A"},
  {"label":"C","type":"process","command":"/bin/echo","args":["NEITHER-THIS"],"dependsOn":"B"}
 ]}`
 for _,reject:=range []string{"attempted","completed"}{
  t.Run(reject,func(t *testing.T){
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"C")
   store:=s.Identity
   s.Identity=remoteWorkflowRejectStepAudit{
    Store:store,rejectResult:reject,
   }
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err!=nil{
    t.Fatalf("workflow start was denied before injected audit failure: %v",err)
   }
   if status:=awaitRemoteWorkflow(t,session);status!="failed"{
    t.Fatalf("missing security audit allowed workflow to finish as %s",status)
   }
   job:=session.longTask
   job.mu.Lock()
   code:=job.exitCode
   job.mu.Unlock()
   if code!= -1{t.Fatalf("audit failure returned a normal exit code: %d",code)}
   job.capture.mu.Lock()
   output:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if strings.Contains(output,"MUST-NOT-RUN")||
    strings.Contains(output,"NEITHER-THIS")||
    strings.Contains(output,"Step 2/3"){
    t.Fatalf("subsequent process ran after audit rejection: %q",output)
   }
   if reject=="attempted"&&strings.Contains(output,"FIRST-ONLY"){
    t.Fatalf("first process ran before attempted audit: %q",output)
   }
   if reject=="completed"&&!strings.Contains(output,"FIRST-ONLY"){
    t.Fatalf("setup did not execute the first successful process: %q",output)
   }
   project,err:=store.ProjectByKey(context.Background(),session.projectKey)
   if err!=nil{t.Fatal(err)}
   events,err:=store.ListAudit(context.Background(),identity.AuditQuery{
    ProjectID:project.ID,UserID:session.userID,
    Action:"remote.action.tasks_run_finished",Limit:5,
   })
   if err!=nil||len(events)!=1||events[0].Result!="failed"{
    t.Fatalf("root outcome not durably marked failed: %+v %v",events,err)
   }
   attempts,err:=store.ListAudit(context.Background(),identity.AuditQuery{
    ProjectID:project.ID,UserID:session.userID,
    Action:"remote.action.tasks_workflow_step",Limit:12,
   })
   if err!=nil{t.Fatal(err)}
   for _,row:=range attempts{
    if strings.Contains(row.Details,`"step":2`)||
     strings.Contains(row.Details,`"step":3`){
     t.Fatalf("later step was audited after rejected transition: %+v",row)
    }
   }
  })
 }
}

 
func TestRemoteWorkflowSingleRetryHostOptInAndAudit(t *testing.T){
 remoteWorkflowHostFlags(t)
 source:=`{"tasks":[
  {"label":"Fail once more","type":"process","command":"/bin/false",
   "taskdeck":{"workflow":{"retry":1}}},
  {"label":"Must stop","type":"process","command":"/bin/echo",
   "args":["MUST-NOT-RUN"],"dependsOn":"Fail once more"}
 ]}`
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Must stop")
 deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!deps.Available||!deps.RunAvailable{
  t.Fatalf("host-approved one-retry workflow not offered: %+v %v",deps,err)
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("host-approved one-retry workflow not started: %v",err)
 }
 if result:=awaitRemoteWorkflow(t,session);result!="failed"{
  t.Fatalf("exhausted retry did not fail: %s",result)
 }
 job:=session.longTask
 job.mu.Lock()
 code:=job.exitCode
 job.mu.Unlock()
 if code!=1{t.Fatalf("nonzero child exit not preserved after retry: %d",code)}
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if !strings.Contains(output,"attempt 1/2")||
  !strings.Contains(output,"attempt 2/2")||
  strings.Contains(output,"MUST-NOT-RUN")||
  strings.Contains(output,"Step 2/2"){
  t.Fatalf("retry count, or fail-fast contract was broken: %q",output)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,UserID:session.userID,
  Action:"remote.action.tasks_workflow_step",Limit:12,
 })
 if err!=nil||len(events)!=4{
  t.Fatalf("each retry must have attempted and failed audit: %+v %v",events,err)
 }
 attempts:=map[string]int{"1":0,"2":0}
 for _,row:=range events{
  if row.Result!="attempted"&&row.Result!="failed"{
   t.Fatalf("unexpected per-attempt audit status: %+v",row)
  }
  if !strings.Contains(row.Details,`"step":1`)||
   !strings.Contains(row.Details,`"task_id":`){
   t.Fatalf("retry audit mixed another step: %+v",row)
  }
  if strings.Contains(row.Details,`"attempt":1`){attempts["1"]++
  }else if strings.Contains(row.Details,`"attempt":2`){attempts["2"]++
  }else{t.Fatalf("audit omitted attempt number: %+v",row)}
 }
 if attempts["1"]!=2||attempts["2"]!=2{
  t.Fatalf("wrong per-attempt local audits: %+v",attempts)
 }
}

func TestRemoteWorkflowRetryDeniedWithoutHostOptInOrWithOtherPolicies(t *testing.T){
 remoteWorkflowHostFlags(t)
 for _,scenario:=range []struct{name,policy,flag string;safe bool}{
  {"disabled",`{"retry":1}`,"0",false},
  {"explicit-repeat",`{"retry":1}`,"1",true},
  {"too-many-repeats",`{"retry":2}`,"1",false},
  {"conditional",`{"retry":1,"condition":"always"}`,"1",false},
  {"continue-on-error",`{"retry":1,"continueOnError":true}`,"1",false},
  {"output-exfiltration",`{"retry":1,"outputs":["output"]}`,"1",false},
  {"extra-taskdeck-metadata",`{"retry":1}`,"1",false},
 }{
  t.Run(scenario.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY",scenario.flag)
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   extra:=""
   if scenario.name=="extra-taskdeck-metadata"{extra=`,"other":"extra"`}
   source:=fmt.Sprintf(`{"tasks":[
    {"label":"First","type":"process","command":"/bin/echo",
     "taskdeck":{"workflow":%s%s}},
    {"label":"Last","type":"process","command":"/bin/echo","dependsOn":"First"}
   ]}`,scenario.policy,extra)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Last")
   deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
   if err!=nil||!deps.Available||deps.RunAvailable!=scenario.safe{
    t.Fatalf("wrong retry policy admission: %+v %v",deps,err)
   }
   _,_,err=s.prepareRemoteLinearWorkflow(context.Background(),session,root)
   if (err==nil)!=scenario.safe{
    t.Fatalf("mismatched executor retry admission: %v",err)
   }
  })
 }
}

type remoteWorkflowReadyAfterFailedAttempt struct{
 identity.Store
 marker string
}
func(store remoteWorkflowReadyAfterFailedAttempt)AppendAudit(
 ctx context.Context,event identity.AuditEvent,
)error{
 if event.Action=="remote.action.tasks_workflow_step"&&
  event.Result=="failed"&&strings.Contains(event.Details,`"attempt":1`){
  if err:=os.WriteFile(store.marker,[]byte("ready"),0600);err!=nil{return err}
 }
 return store.Store.AppendAudit(ctx,event)
}

func TestRemoteWorkflowSingleRetrySucceedsThenContinues(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 marker:=filepath.Join(s.Workspace,"ready-after-first-failure")
 source:=fmt.Sprintf(`{"tasks":[
  {"label":"Probe","type":"process","command":"/bin/test","args":["-f",%q],
   "taskdeck":{"workflow":{"retry":1}}},
  {"label":"Downstream","type":"process","command":"/bin/echo",
   "args":["AFTER-RETRY"],"dependsOn":"Probe"}
 ]}`,marker)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Downstream")
 original:=s.Identity
 s.Identity=remoteWorkflowReadyAfterFailedAttempt{Store:original,marker:marker}
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("retry task did not start: %v",err)
 }
 if status:=awaitRemoteWorkflow(t,session);status!="completed"{
  t.Fatalf("successful second attempt did not unblock dependent task: %s",status)
 }
 job:=session.longTask
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if !strings.Contains(output,"Step 1/2: failed (exit=1, attempt=1)")||
  !strings.Contains(output,"Step 1/2: completed (exit=0, attempt=2)")||
  !strings.Contains(output,"AFTER-RETRY")||
  strings.Count(output,"Step 1/2: Task #")!=2||
  strings.Count(output,"Step 2/2: Task #")!=1{
  t.Fatalf("successful retry/order not observed: %q",output)
 }
 project,err:=original.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 rows,err:=original.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,UserID:session.userID,
  Action:"remote.action.tasks_workflow_step",Limit:12,
 })
 if err!=nil||len(rows)!=6{
  t.Fatalf("successful retry must retain both attempts and next step: %+v %v",rows,err)
 }
}

func TestRemoteWorkflowRetryNeverReplaysFailedOutcomeAudit(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Fail","type":"process","command":"/bin/false",
   "taskdeck":{"workflow":{"retry":1}}},
  {"label":"Never","type":"process","command":"/bin/echo",
   "args":["NO-RUN"],"dependsOn":"Fail"}
 ]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Never")
 original:=s.Identity
 s.Identity=remoteWorkflowRejectStepAudit{Store:original,rejectResult:"failed"}
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if status:=awaitRemoteWorkflow(t,session);status!="failed"{
  t.Fatalf("failed audit resumed a retry task: %s",status)
 }
 job:=session.longTask
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Count(output,"Step 1/2: Task #")!=1||
  strings.Contains(output,"attempt 2/2")||
  strings.Contains(output,"NO-RUN"){
  t.Fatalf("retry executed after denied outcome audit: %q",output)
 }
}


func TestRemoteWorkflowDeniesMutableOrWorkspaceControlledBinaries(t *testing.T){
 remoteWorkflowHostFlags(t)
 scenarios:=[]struct{
  name string
  setup func(*testing.T,*Server)string
 }{
  {"workspace-binary",func(t *testing.T,s *Server)string{
   command:=filepath.Join(s.Workspace,"local-echo")
   data,err:=os.ReadFile("/bin/echo")
   if err!=nil{t.Fatal(err)}
   if err:=os.WriteFile(command,data,0700);err!=nil{t.Fatal(err)}
   return command
  }},
  {"workspace-symlink-alias",func(t *testing.T,s *Server)string{
   command:=filepath.Join(s.Workspace,"echo-alias")
   if err:=os.Symlink("/bin/echo",command);err!=nil{t.Fatal(err)}
   return command
  }},
  {"world-writable-binary",func(t *testing.T,s *Server)string{
   command:=filepath.Join(t.TempDir(),"untrusted-echo")
   data,err:=os.ReadFile("/bin/echo")
   if err!=nil{t.Fatal(err)}
   if err:=os.WriteFile(command,data,0700);err!=nil{t.Fatal(err)}
   if err:=os.Chmod(command,0777);err!=nil{t.Fatal(err)}
   return command
  }},
  {"external-symlink-into-workspace",func(t *testing.T,s *Server)string{
   target:=filepath.Join(s.Workspace,"local-echo")
   data,err:=os.ReadFile("/bin/echo")
   if err!=nil{t.Fatal(err)}
   if err:=os.WriteFile(target,data,0700);err!=nil{t.Fatal(err)}
   command:=filepath.Join(t.TempDir(),"external-alias")
   if err:=os.Symlink(target,command);err!=nil{t.Fatal(err)}
   return command
  }},
 }
 for _,tc:=range scenarios{
  t.Run(tc.name,func(t *testing.T){
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   command:=tc.setup(t,s)
   source:=fmt.Sprintf(`{"tasks":[
    {"label":"Trusted","type":"process","command":"/bin/echo"},
    {"label":"Unsafe","type":"process","command":%q,"dependsOn":"Trusted"}
   ]}`,command)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Unsafe")
   result,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
   if err!=nil||!result.Available||result.RunAvailable{
    t.Fatalf("GUI permitted mutable executable: %+v %v",result,err)
   }
   if _,_,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,root);err==nil{
    t.Fatal("mutable binary passed start-time preflight")
   }
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
    t.Fatal("mutable binary started host workflow")
   }
   if session.longTask!=nil{t.Fatal("rejected binary reserved long-task slot")}
  })
 }
}

// The standalone native-file guard is a defense-in-depth check. A script
// may have an innocent filename and valid executable permissions but is
// always denied before attempting any workflow launch.
func TestRemoteWorkflowAcceptedSystemExecutableRemainsAllowed(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,
  remoteWorkflowLinearSource,"Step C")
 result,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!result.RunAvailable{
  t.Fatalf("safe system process unexpectedly denied: %+v %v",result,err)
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if state:=awaitRemoteWorkflow(t,session);state!="completed"{
  t.Fatalf("system process failed after guard: %s",state)
 }
}
