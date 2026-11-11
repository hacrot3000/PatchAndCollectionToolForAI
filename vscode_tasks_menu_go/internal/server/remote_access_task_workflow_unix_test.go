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

// Workflow completion must share the finalization barrier of ordinary
// long tasks. A late Cancel cannot acknowledge an already-exited workflow.
func TestRemoteWorkflowFinalizationClosesCancelWindow(t *testing.T){
 remoteWorkflowHostFlags(t)
 cases:=[]struct{name,source,root,want string}{
  {"completed",remoteWorkflowLinearSource,"Step C","completed"},
  {"failed",`{"tasks":[
   {"label":"First","type":"process","command":"/bin/echo","args":["READY"]},
   {"label":"Second","type":"process","command":"/bin/false","dependsOn":"First"}
  ]}`,"Second","failed"},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,tc.source,tc.root)
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err!=nil{
    t.Fatalf("start workflow: %v",err)
   }
   if got:=awaitRemoteWorkflow(t,session);got!=tc.want{
    t.Fatalf("workflow status = %q, want %q",got,tc.want)
   }
   session.longTaskMu.Lock()
   job:=session.longTask
   session.longTaskMu.Unlock()
   job.mu.Lock()
   finalizing,status,code:=job.finalizing,job.status,job.exitCode
   job.mu.Unlock()
   if !finalizing||status!=tc.want{
    t.Fatalf("workflow bypassed finalization barrier: finalizing=%v status=%q",finalizing,status)
   }
   if err:=session.cancelRemoteLongTask(id);!errors.Is(err,remotetransport.ErrRemoteActionDenied){
    t.Fatalf("Cancel accepted after workflow %s: %v",tc.want,err)
   }
   job.mu.Lock()
   finalStatus,finalCode,requested:=job.status,job.exitCode,job.cancelRequested
   job.mu.Unlock()
   if finalStatus!=tc.want||finalCode!=code||requested{
    t.Fatalf("late Cancel changed result: status=%q exit=%d requested=%v",finalStatus,finalCode,requested)
   }
   if err:=session.clearRemoteLongTask(id);err!=nil{
    t.Fatalf("clear finalized workflow: %v",err)
   }
  })
 }
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

 
func TestRemoteWorkflowPerStepTimeoutHostOptInAndAudit(t *testing.T){
 remoteWorkflowHostFlags(t)
 source:=`{"tasks":[
  {"label":"Slow","type":"process","command":"/bin/sleep","args":["3"],
   "taskdeck":{"workflow":{"timeoutSeconds":1}}},
  {"label":"Never","type":"process","command":"/bin/echo",
   "args":["MUST-NOT-RUN"],"dependsOn":"Slow"}
 ]}`
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT","0")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Never")
 info,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!info.Available||info.RunAvailable{
  t.Fatalf("timeout workflow admitted without host opt-in: %+v %v",info,err)
 }
 if _,_,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,root);err==nil{
  t.Fatal("timeout bypassed disabled opt-in at start")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT","1")
 info,err=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!info.RunAvailable{
  t.Fatalf("valid host timeout not offered in GUI: %+v %v",info,err)
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("timeout workflow start: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="timed_out"{
  t.Fatalf("step did not time out before global 90-second deadline: %s",got)
 }
 job:=session.longTask
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if !strings.Contains(output,"Step 1/2: timed_out")||
  strings.Contains(output,"MUST-NOT-RUN")||
  strings.Contains(output,"Step 2/2"){
  t.Fatalf("timeout ran a dependent task or lost status: %q",output)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 rows,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,UserID:session.userID,
  Action:"remote.action.tasks_workflow_step",Limit:12,
 })
 if err!=nil||len(rows)!=2{
  t.Fatalf("timeout audit must have attempted and timed_out: %+v %v",rows,err)
 }
 found:=false
 for _,row:=range rows{
  if row.Result=="timed_out"{found=true}
 }
 if !found{t.Fatalf("missing durable step timeout audit: %+v",rows)}
}

func TestRemoteWorkflowTimeoutRejectsUnknownPolicyAndUnapprovedRanges(t *testing.T){
 remoteWorkflowHostFlags(t)
 for _,tc:=range []struct{name,policy,enabled string;allowed bool}{
  {"disabled",`{"timeoutSeconds":1}`,"0",false},
  {"one-second",`{"timeoutSeconds":1}`,"1",true},
  {"thirty-seconds",`{"timeoutSeconds":30}`,"1",true},
  {"zero-seconds",`{"timeoutSeconds":0}`,"1",false},
  {"thirty-one-seconds",`{"timeoutSeconds":31}`,"1",false},
  {"fractional-seconds",`{"timeoutSeconds":1.5}`,"1",false},
  {"string-seconds",`{"timeoutSeconds":"1"}`,"1",false},
  {"zero-retries",`{"retry":0,"timeoutSeconds":1}`,"1",false},
  {"conditional",`{"timeoutSeconds":1,"condition":"always"}`,"1",false},
  {"outputs",`{"timeoutSeconds":1,"outputs":["x"]}`,"1",false},
  {"unknown",`{"timeoutSeconds":1,"other":1}`,"1",false},
  {"combined",`{"retry":1,"timeoutSeconds":1}`,"1",true},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT",tc.enabled)
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY","1")
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   source:=fmt.Sprintf(`{"tasks":[
    {"label":"First","type":"process","command":"/bin/echo",
     "taskdeck":{"workflow":%s}},
    {"label":"Last","type":"process","command":"/bin/echo",
     "dependsOn":"First"}
   ]}`,tc.policy)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Last")
   result,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
   if tc.allowed{
    if err!=nil||!result.Available||!result.RunAvailable{
     t.Fatalf("approved timeout policy denied: %+v %v",result,err)
    }
   }else if err==nil&&result.RunAvailable{
    t.Fatalf("unsafe timeout policy shown as executable: %+v",result)
   }
   plan,_,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,root)
   if (err==nil)!=tc.allowed{
    t.Fatalf("executor/preflight mismatch: %+v %v",plan,err)
   }
   if tc.allowed&&len(plan)>0&&plan[0].timeout<=0{
    t.Fatalf("approved timeout lost in execution plan: %+v",plan)
   }
  })
 }
}

func TestRemoteWorkflowStepTimeoutNeverRetriesOrStartsNextTask(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Timed","type":"process","command":"/bin/sleep","args":["3"],
   "taskdeck":{"workflow":{"retry":1,"timeoutSeconds":1}}},
  {"label":"After","type":"process","command":"/bin/echo",
   "args":["MUST-NOT-RUN"],"dependsOn":"Timed"}
 ]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"After")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if status:=awaitRemoteWorkflow(t,session);status!="timed_out"{
  t.Fatalf("timeout erroneously retried or completed: %s",status)
 }
 job:=session.longTask
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Count(output,"Step 1/2: Task #")!=1||
  strings.Contains(output,"attempt 2/2")||
  strings.Contains(output,"Step 2/2")||
  strings.Contains(output,"MUST-NOT-RUN"){
  t.Fatalf("timeout retried or launched a dependent task: %q",output)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,UserID:session.userID,
  Action:"remote.action.tasks_workflow_step",Limit:12,
 })
 if err!=nil||len(audits)!=2{
  t.Fatalf("timeout must create exactly two first-attempt audits: %+v %v",audits,err)
 }
 for _,row:=range audits{
  if !strings.Contains(row.Details,`"attempt":1`)||
   !strings.Contains(row.Details,`"step":1`){
   t.Fatalf("timeout ran or audited a second attempt: %+v",row)
  }
 }
}

type remoteWorkflowRevokeHostAfterStep struct{
 identity.Store
 name string
}
func(store remoteWorkflowRevokeHostAfterStep)AppendAudit(
 ctx context.Context,event identity.AuditEvent,
)error{
 err:=store.Store.AppendAudit(ctx,event)
 if err==nil&&event.Action=="remote.action.tasks_workflow_step"&&
  event.Result=="completed"&&strings.Contains(event.Details,`"step":1`){
  // Deliberately revoke one HOST-owned, default-OFF opt-in after the
  // first process and before the next process's admission check.
  if setErr:=os.Setenv(store.name,"0");setErr!=nil{return setErr}
 }
 return err
}
func TestRemoteWorkflowHostRevocationStopsSubsequentSteps(t *testing.T){
 for _,name:=range []string{
  "TASKDECK_REMOTE_TASKS_PREVIEW",
  "TASKDECK_REMOTE_TASKS_DEPS",
  "TASKDECK_REMOTE_TASKS_RUN",
  "TASKDECK_REMOTE_TASKS_LONG",
  "TASKDECK_REMOTE_TASKS_WORKFLOW_RUN",
 }{
  t.Run(name,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,
    remoteWorkflowLinearSource,"Step C")
   original:=s.Identity
   s.Identity=remoteWorkflowRevokeHostAfterStep{Store:original,name:name}
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
    t.Fatalf("workflow not admitted before revocation: %v",err)
   }
   if status:=awaitRemoteWorkflow(t,session);status!="cancelled"{
    t.Fatalf("host revocation did not stop workflow: %s",status)
   }
   job:=session.longTask
   job.capture.mu.Lock()
   output:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if !strings.Contains(output,"ONE")||
    strings.Contains(output,"TWO")||
    strings.Contains(output,"THREE")||
    strings.Contains(output,"Step 2/3"){
    t.Fatalf("process executed after host opt-in revocation: %q",output)
   }
  })
 }
}

type remoteWorkflowRevokeDuringAttemptAudit struct{
 identity.Store
 flag string
}
func(store remoteWorkflowRevokeDuringAttemptAudit)AppendAudit(
 ctx context.Context,event identity.AuditEvent,
)error{
 err:=store.Store.AppendAudit(ctx,event)
 if err==nil&&event.Action=="remote.action.tasks_workflow_step"&&
  event.Result=="attempted"&&
  strings.Contains(event.Details,`"step":1`){
  if e:=os.Setenv(store.flag,"0");e!=nil{return e}
 }
 return err
}

func TestRemoteWorkflowRevocationDuringAttemptAuditPreventsFirstProcess(t *testing.T){
 for _,flag:=range []string{
  "TASKDECK_REMOTE_TASKS_WORKFLOW_RUN",
  "TASKDECK_REMOTE_TASKS_RUN",
  "TASKDECK_REMOTE_TASKS_DEPS",
 }{
  t.Run(flag,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   id:=remoteWorkflowIssuedRoot(t,s,session,client,agent,
    remoteWorkflowLinearSource,"Step C")
   original:=s.Identity
   s.Identity=remoteWorkflowRevokeDuringAttemptAudit{Store:original,flag:flag}
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,id);err!=nil{
    t.Fatal(err)
   }
   if status:=awaitRemoteWorkflow(t,session);status!="cancelled"{
    t.Fatalf("pre-run host revocation did not cancel: %s",status)
   }
   job:=session.longTask
   job.capture.mu.Lock()
   output:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if strings.Contains(output,"ONE")||strings.Contains(output,"TWO")||
    strings.Contains(output,"THREE"){
    t.Fatalf("host launched process after attempted audit revoked opt-in: %q",output)
   }
   project,err:=original.ProjectByKey(context.Background(),session.projectKey)
   if err!=nil{t.Fatal(err)}
   events,err:=original.ListAudit(context.Background(),identity.AuditQuery{
    ProjectID:project.ID,UserID:session.userID,
    Action:"remote.action.tasks_workflow_step",Limit:12,
   })
   if err!=nil||len(events)!=2{
    t.Fatalf("pre-exec revocation lost step final audit: %+v %v",events,err)
   }
   results:=map[string]int{}
   for _,event:=range events{results[event.Result]++}
   if results["attempted"]!=1||results["cancelled"]!=1{
    t.Fatalf("pre-exec revocation audit mismatch: %+v",results)
   }
  })
 }
}

 
func TestRemoteWorkflowPollReturnsAuthenticatedStepResult(t *testing.T){
 remoteWorkflowHostFlags(t)
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,
  remoteWorkflowLinearSource,"Step C")
 started,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksWorkflowStart,root,0,"",
  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
 if err!=nil||started.Status!="started"{
  t.Fatalf("workflow start: %+v %v",started,err)
 }
 if status:=awaitRemoteWorkflow(t,session);status!="completed"{
  t.Fatalf("workflow outcome: %s",status)
 }
 progress,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,root,0,"",
  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
 if err!=nil||progress.Status!="completed"||
  progress.Step!=3||progress.StepTotal!=3||
  progress.Attempt!=1||progress.AttemptTotal!=1||
  progress.StepStatus!="completed"||
  progress.StepHistory!="CCC"||len(progress.Output)>64||
  progress.StepDetails==""{
  t.Fatalf("typed E2EE workflow progress unavailable: %+v %v",progress,err)
 }
 // The step record comes from the host, independent of a child process's
 // arbitrary stdout. A single-step legacy task must not invent metadata.
 for pollIndex:=0;progress.More;pollIndex++{
  if pollIndex>=32{t.Fatal("host output cursor did not terminate")}
  progress,err=longTaskE2EEAction(t,s,session,client,agent,
   remotetransport.RemoteActionTasksPoll,root,0,"",
   fmt.Sprintf("%032d",1000+pollIndex))
  if err!=nil||progress.Step!=3||progress.StepTotal!=3||
   len(progress.Output)>64||progress.StepHistory!="CCC"||
   progress.StepDetails==""{
   t.Fatalf("subsequent E2EE page lost metadata: %+v %v",progress,err)
  }
 }
}

 
func TestRemoteWorkflowHostStepHistoryLiveAndFailed(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Run("live-then-complete",func(t *testing.T){
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  source:=`{"tasks":[
   {"label":"Slow","type":"process","command":"/bin/sleep","args":["2"]},
   {"label":"Done","type":"process","command":"/bin/echo","args":["DONE"],"dependsOn":"Slow"}
  ]}`
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Done")
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
   t.Fatal(err)
  }
  during,err:=longTaskE2EEAction(t,s,session,client,agent,
   remotetransport.RemoteActionTasksPoll,root,0,"",
   "10101010101010101010101010101010")
  if err!=nil||during.Status!="running"||during.Step!=1||
   during.StepTotal!=2||during.StepStatus!="running"||
   during.StepHistory!="R-"||during.StepDetails!="-|-"{
   t.Fatalf("first step live history missing: %+v %v",during,err)
  }
  if state:=awaitRemoteWorkflow(t,session);state!="completed"{
   t.Fatalf("workflow outcome: %s",state)
  }
  final,err:=longTaskE2EEAction(t,s,session,client,agent,
   remotetransport.RemoteActionTasksPoll,root,0,"",
   "20202020202020202020202020202020")
  if err!=nil||final.Status!="completed"||
   final.Step!=2||final.StepStatus!="completed"||
   final.StepHistory!="CC"||len(strings.Split(final.StepDetails,"|"))!=2{
   t.Fatalf("completed host history incorrect: %+v %v",final,err)
  }
 })
 t.Run("failed-dependency-not-started",func(t *testing.T){
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  source:=`{"tasks":[
   {"label":"Okay","type":"process","command":"/bin/echo","args":["ONE"]},
   {"label":"Fail","type":"process","command":"/bin/false","dependsOn":"Okay"},
   {"label":"Never","type":"process","command":"/bin/echo",
    "args":["MUST-NOT-RUN"],"dependsOn":"Fail"}
  ]}`
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Never")
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
   t.Fatal(err)
  }
  if state:=awaitRemoteWorkflow(t,session);state!="failed"{
   t.Fatalf("failed dependency did not stop job: %s",state)
  }
  final,err:=longTaskE2EEAction(t,s,session,client,agent,
   remotetransport.RemoteActionTasksPoll,root,0,"",
   "30303030303030303030303030303030")
  if err!=nil||final.Status!="failed"||final.Step!=2||
   final.StepTotal!=3||final.StepStatus!="failed"||
   final.StepHistory!="CF-"||
   !strings.HasSuffix(final.StepDetails,"|-"){
   t.Fatalf("failed workflow history leaked/omitted a step: %+v %v",final,err)
  }
 })
}

 
func TestRemoteWorkflowFinalCleanupAlwaysOptInAndStickyFailure(t *testing.T){
 remoteWorkflowHostFlags(t)
 source:=`{"tasks":[
  {"label":"Fail","type":"process","command":"/bin/false"},
  {"label":"Cleanup","type":"process","command":"/bin/echo",
   "args":["CLEANUP_OK"],"dependsOn":"Fail",
   "taskdeck":{"workflow":{"condition":"always"}}}
 ]}`
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","0")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Cleanup")
 deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!deps.Available||deps.RunAvailable{
  t.Fatalf("cleanup condition was offered without explicit opt-in: %+v %v",deps,err)
 }
 if _,_,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,root);err==nil{
  t.Fatal("executor bypassed disabled conditional workflow gate")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","1")
 deps,err=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!deps.RunAvailable{t.Fatalf("cleanup not offered to GUI: %+v %v",deps,err)}
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("start cleanup workflow: %v",err)
 }
 if state:=awaitRemoteWorkflow(t,session);state!="failed"{
  t.Fatalf("successful cleanup masked failed dependency: %s",state)
 }
 session.longTaskMu.Lock()
 job:=session.longTask
 session.longTaskMu.Unlock()
 job.mu.Lock()
 history,exit:=job.workflowStepHistory,job.exitCode
 job.mu.Unlock()
 if history!="FC"||exit!=1{
  t.Fatalf("wrong cleanup result/status: history=%q exit=%d",history,exit)
 }
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if !strings.Contains(output,"CLEANUP_OK")||
  strings.Index(output,"Step 1/2: failed")>strings.Index(output,"CLEANUP_OK"){
  t.Fatalf("cleanup not executed after failed dependency: %q",output)
 }
 progress,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,root,0,"",
  "87878787878787878787878787878787")
 if err!=nil||progress.Status!="failed"||progress.ExitCode!=1||
  progress.StepStatus!="completed"||progress.StepHistory!="FC"{
  t.Fatalf("failed/cleanup status not authenticated: %+v %v",progress,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 rows,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,UserID:session.userID,
  Action:"remote.action.tasks_workflow_step",Limit:8,
 })
 if err!=nil||len(rows)!=4{
  t.Fatalf("expected failed step and completed cleanup audits: %+v %v",rows,err)
 }
 count:=map[string]int{}
 for _,row:=range rows{count[row.Result]++}
 if count["attempted"]!=2||count["failed"]!=1||count["completed"]!=1{
  t.Fatalf("unexpected cleanup audit sequence: %+v",count)
 }
}

func TestRemoteWorkflowFinalCleanupAfterSuccessfulDependency(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Okay","type":"process","command":"/bin/true"},
  {"label":"Cleanup","type":"process","command":"/bin/echo",
   "args":["CLEANUP_ON_SUCCESS"],"dependsOn":"Okay",
   "taskdeck":{"workflow":{"condition":"always"}}}
 ]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Cleanup")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if status:=awaitRemoteWorkflow(t,session);status!="completed"{
  t.Fatalf("cleanup after success failed: %s",status)
 }
 job:=session.longTask
 job.mu.Lock();history:=job.workflowStepHistory;job.mu.Unlock()
 if history!="CC"{t.Fatalf("invalid normal cleanup history: %s",history)}
}

func TestRemoteWorkflowFinalCleanupDoesNotBypassEarlierFailures(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"EarlyFail","type":"process","command":"/bin/false"},
  {"label":"Middle","type":"process","command":"/bin/echo",
   "args":["MIDDLE_MUST_NOT_RUN"],"dependsOn":"EarlyFail"},
  {"label":"Cleanup","type":"process","command":"/bin/echo",
   "args":["CLEANUP_MUST_NOT_RUN"],"dependsOn":"Middle",
   "taskdeck":{"workflow":{"condition":"always"}}}
 ]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Cleanup")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if status:=awaitRemoteWorkflow(t,session);status!="failed"{
  t.Fatalf("early failure did not abort workflow: %s",status)
 }
 job:=session.longTask
 job.mu.Lock();history:=job.workflowStepHistory;job.mu.Unlock()
 if history!="F--"{t.Fatalf("early failure bypassed middle gate: %q",history)}
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Contains(output,"MIDDLE_MUST_NOT_RUN")||
  strings.Contains(output,"CLEANUP_MUST_NOT_RUN"){
  t.Fatalf("skipped tasks executed despite early failure: %q",output)
 }
}

func TestRemoteWorkflowConditionRejectsUnsupportedPoliciesAndPositions(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","1")
 for _,scenario:=range []struct{name,first,last string;safe bool}{
  {"safe-success", "",`"taskdeck":{"workflow":{"condition":"success"}}`,true},
  {"always-first", `"taskdeck":{"workflow":{"condition":"always"}}`, "",false},
  {"failure-condition","",`"taskdeck":{"workflow":{"condition":"failure"}}`,true},
  {"failure-intermediate",`"taskdeck":{"workflow":{"condition":"failure"}}`,"",false},
  {"continue-on-error","",`"taskdeck":{"workflow":{"continueOnError":true}}`,false},
  {"output-export","",`"taskdeck":{"workflow":{"condition":"always","outputs":["key"]}}`,false},
  {"noncanonical","",`"taskdeck":{"workflow":{"condition":"ALWAYS"}}`,false},
 }{
  t.Run(scenario.name,func(t *testing.T){
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   first:=""
   if scenario.first!=""{first=","+scenario.first}
   last:=""
   if scenario.last!=""{last=","+scenario.last}
   source:=fmt.Sprintf(`{"tasks":[
    {"label":"First","type":"process","command":"/bin/true"%s},
    {"label":"Last","type":"process","command":"/bin/true",
     "dependsOn":"First"%s}
   ]}`,first,last)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Last")
   deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
   if err!=nil||deps.RunAvailable!=scenario.safe{
    t.Fatalf("conditional policy GUI mismatch: %+v %v",deps,err)
   }
   _,_,err=s.prepareRemoteLinearWorkflow(context.Background(),session,root)
   if (err==nil)!=scenario.safe{
    t.Fatalf("executor admitted unsafe conditional policy: %v",err)
   }
  })
 }
}

type remoteWorkflowCleanupFlagRevoker struct{identity.Store}
func (r remoteWorkflowCleanupFlagRevoker)AppendAudit(
 ctx context.Context,event identity.AuditEvent)error{
 err:=r.Store.AppendAudit(ctx,event)
 if err==nil&&event.Action=="remote.action.tasks_workflow_step"&&
  event.Result=="failed"&&strings.Contains(event.Details,`"step":1`){
  os.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","0")
 }
 return err
}
func TestRemoteWorkflowCleanupDoesNotRunWhenHostRevokesCondition(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Failed","type":"process","command":"/bin/false"},
  {"label":"Cleanup","type":"process","command":"/bin/echo",
   "args":["CLEANUP_MUST_NOT_RUN"],"dependsOn":"Failed",
   "taskdeck":{"workflow":{"condition":"always"}}}
 ]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Cleanup")
 s.Identity=remoteWorkflowCleanupFlagRevoker{Store:s.Identity}
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if status:=awaitRemoteWorkflow(t,session);status!="failed"{
  t.Fatalf("revocation changed failure to success: %s",status)
 }
 job:=session.longTask
 job.mu.Lock();history:=job.workflowStepHistory;job.mu.Unlock()
 if history!="F-"{t.Fatalf("cleanup ran after host revocation: %q",history)}
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Contains(output,"CLEANUP_MUST_NOT_RUN"){
  t.Fatalf("cleanup output found after host revoked flag: %q",output)
 }
}

 
func TestRemoteWorkflowFinalFailureOnlyRunsAfterFailedDependency(t *testing.T){
 remoteWorkflowHostFlags(t)
 source:=`{"tasks":[
  {"label":"Failed","type":"process","command":"/bin/false"},
  {"label":"Recovery","type":"process","command":"/bin/echo",
   "args":["RECOVERY_EXECUTED"],"dependsOn":"Failed",
   "taskdeck":{"workflow":{"condition":"failure"}}}
 ]}`
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","0")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Recovery")
 deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!deps.Available||deps.RunAvailable{
  t.Fatalf("failure-only cleanup offered without host opt-in: %+v %v",deps,err)
 }
 if _,_,err:=s.prepareRemoteLinearWorkflow(context.Background(),session,root);err==nil{
  t.Fatal("failure condition execution admitted without opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","1")
 deps,err=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!deps.RunAvailable{
  t.Fatalf("failure-only recovery unavailable after opt-in: %+v %v",deps,err)
 }
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if result:=awaitRemoteWorkflow(t,session);result!="failed"{
  t.Fatalf("recovery hid earlier process failure: %s",result)
 }
 job:=session.longTask
 job.mu.Lock()
 history,exit:=job.workflowStepHistory,job.exitCode
 job.mu.Unlock()
 if history!="FC"||exit!=1{
  t.Fatalf("recovery result invalid: history=%q exit=%d",history,exit)
 }
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if !strings.Contains(output,"RECOVERY_EXECUTED"){
  t.Fatalf("failure condition did not run recovery step: %q",output)
 }
 progress,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,root,0,"",
  "66666666666666666666666666666666")
 if err!=nil||progress.Status!="failed"||progress.StepHistory!="FC"||
  progress.StepStatus!="completed"||progress.ExitCode!=1{
  t.Fatalf("E2EE recovery outcome lost: %+v %v",progress,err)
 }
}

func TestRemoteWorkflowFinalFailureOnlySkippedWhenDependenciesSucceeded(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"Success","type":"process","command":"/bin/true"},
  {"label":"Recovery","type":"process","command":"/bin/echo",
   "args":["MUST_NOT_EXECUTE_RECOVERY"],"dependsOn":"Success",
   "taskdeck":{"workflow":{"condition":"failure"}}}
 ]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Recovery")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if result:=awaitRemoteWorkflow(t,session);result!="completed"{
  t.Fatalf("skipped failure-only step failed workflow: %s",result)
 }
 job:=session.longTask
 job.mu.Lock()
 history,exit:=job.workflowStepHistory,job.exitCode
 details:=append([]string(nil),job.workflowStepDetails...)
 job.mu.Unlock()
 if history!="CS"||exit!=0||len(details)!=2||details[1]!="-"{ 
  t.Fatalf("skipped step incorrectly recorded execution: %q %d %+v",history,exit,details)
 }
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Contains(output,"MUST_NOT_EXECUTE_RECOVERY")||
  !strings.Contains(output,"skipped (condition failure not met)"){
  t.Fatalf("failure-only step executed after successful dependencies: %q",output)
 }
 progress,err:=longTaskE2EEAction(t,s,session,client,agent,
  remotetransport.RemoteActionTasksPoll,root,0,"",
  "67676767676767676767676767676767")
 if err!=nil||progress.Status!="completed"||progress.StepHistory!="CS"||
  progress.StepStatus!="skipped"||progress.ExitCode!=0{
  t.Fatalf("E2EE skipped state unavailable: %+v %v",progress,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,UserID:session.userID,
  Action:"remote.action.tasks_workflow_step",Limit:8,
 })
 if err!=nil||len(events)!=3{
  t.Fatalf("missing durable skipped result or unexpected process audit: %+v %v",events,err)
 }
 skipped:=0
 for _,event:=range events{
  if strings.Contains(event.Details,`"step":2`){
   if event.Result!="skipped"||strings.Contains(event.Details,`"exit_code":0`){
    t.Fatalf("skipped recovery invented a process or exit success: %+v",event)
   }
   skipped++
  }
 }
 if skipped!=1{t.Fatalf("expected exactly one skipped outcome, got %d",skipped)}
 t.Setenv("TASKDECK_REMOTE_TASKS_REPORT","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_REPORT","1")
 report,err:=s.readRemoteWorkflowAudit(context.Background(),session)
 if err!=nil||len(report.Items)<1||report.Items[0].Status!="skipped"||
  report.Items[0].Step!=2||report.Items[0].TaskID!=root||
  report.Items[0].ExitCode!= -1||report.Items[0].RunID!=job.workflowRunID{
  t.Fatalf("re-authenticatable audit missed skipped recovery: %+v %v",report,err)
 }
 // The pre-pagination v1 contract has no skipped status, run_id or more.
 // Refill its two-event window with supported outcomes from the snapshot.
 // Otherwise a newly recorded skipped step would crash older clients.
 legacyID:="78787878787878787878787878787878"
 legacyRequest,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:legacyID,Operation:remotetransport.RemoteActionTasksWorkflowAudit,
 })
 if err!=nil{t.Fatal(err)}
 legacyFrame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,legacyRequest)
 if err!=nil{t.Fatal(err)}
 legacy,err:=remotetransport.OpenRemoteWorkflowAudit(client,legacyFrame,legacyID)
 if err!=nil||len(legacy.Items)!=1||legacy.Items[0].Status!="completed"||
  legacy.Items[0].RunID!=""||legacy.More{
  t.Fatalf("legacy audit exposed skipped/new fields: %+v %v",legacy,err)
 }
 if session.workflowAuditPageReady||session.workflowAuditPageCursor!=0||
  len(session.workflowAuditPageItems)!=0{
  t.Fatal("v1 audit incorrectly authorized a modern continuation cursor")
 }
}

func TestRemoteWorkflowFailureRecoveryKeepsFirstExitIfRecoveryAlsoFails(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:=`{"tasks":[
  {"label":"InitialFail","type":"process","command":"/bin/false"},
  {"label":"RecoveryFail","type":"process","command":"/bin/ls",
   "args":["--unknown-taskdeck-option"],"dependsOn":"InitialFail",
   "taskdeck":{"workflow":{"condition":"failure"}}}
 ]}`
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"RecoveryFail")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatal(err)
 }
 if state:=awaitRemoteWorkflow(t,session);state!="failed"{
  t.Fatalf("failed cleanup unexpectedly changed outcome: %s",state)
 }
 job:=session.longTask
 job.mu.Lock()
 history,exit:=job.workflowStepHistory,job.exitCode
 details:=append([]string(nil),job.workflowStepDetails...)
 job.mu.Unlock()
 if history!="FF"||exit!=1||len(details)!=2||
  !strings.HasPrefix(details[1],"2:"){
  t.Fatalf("original error was overwritten: history=%q exit=%d details=%+v",
   history,exit,details)
 }
}


func TestRemoteWorkflowHostOptInTwoParallelLeavesAndJoin(t *testing.T){
 remoteWorkflowHostFlags(t)
 source:=`{"tasks":[
  {"label":"Independent A","type":"process","command":"/bin/sleep","args":["1"]},
  {"label":"Independent B","type":"process","command":"/bin/sleep","args":["1"]},
  {"label":"Join","type":"process","command":"/bin/echo","args":["JOIN-ONLY-AFTER-BOTH"],
   "dependsOn":["Independent A","Independent B"],"dependsOrder":"parallel"}
 ]}`
 t.Run("denied without separate opt-in",func(t *testing.T){
  t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","0")
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
  deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
  if err!=nil||!deps.Available||deps.RunAvailable{
   t.Fatalf("parallel join unexpectedly offered without host consent: %+v %v",deps,err)
  }
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
   t.Fatal("parallel worker started without host opt-in")
  }
 })
 t.Run("two simultaneous children then single root",func(t *testing.T){
  t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
  deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
  if err!=nil||!deps.Available||!deps.RunAvailable||len(deps.Order)!=3{
   t.Fatalf("host-approved parallel join not offered: %+v %v",deps,err)
  }
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
   t.Fatalf("parallel join start denied: %v",err)
  }
  job:=session.longTask
  concurrentlyRunning:=false
  until:=time.Now().Add(900*time.Millisecond)
  for time.Now().Before(until){
   job.mu.Lock()
   state:=job.workflowStepHistory
   job.mu.Unlock()
   if state=="RR-"{concurrentlyRunning=true;break}
   time.Sleep(10*time.Millisecond)
  }
  if !concurrentlyRunning{t.Fatal("parallel leaves did not overlap in RUNNING state")}
  if got:=awaitRemoteWorkflow(t,session);got!="completed"{
   t.Fatalf("join status %q",got)
  }
  job.mu.Lock()
  history:=job.workflowStepHistory
  job.mu.Unlock()
  if history!="CCC"{t.Fatalf("parallel join did not audit all three outcomes: %q",history)}
  job.capture.mu.Lock()
  transcript:=string(append([]byte(nil),job.capture.data...))
  job.capture.mu.Unlock()
  if !strings.Contains(transcript,"JOIN-ONLY-AFTER-BOTH")||
   strings.Count(transcript,"parallel attempt 1/1")!=3{
   t.Fatalf("missing exactly-once steps: %q",transcript)
  }
 })
}
func TestRemoteWorkflowParallelFailFastAndUnsupportedShapes(t *testing.T){
 remoteWorkflowHostFlags(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
 source:=`{"tasks":[
  {"label":"Failure","type":"process","command":"/bin/false"},
  {"label":"Sibling","type":"process","command":"/bin/sleep","args":["1"]},
  {"label":"Join","type":"process","command":"/bin/echo","args":["MUST-NOT-RUN"],
   "dependsOn":["Failure","Sibling"],"dependsOrder":"parallel"}
 ]}`
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
 if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
  t.Fatalf("host opt-in did not admit safe graph: %v",err)
 }
 if got:=awaitRemoteWorkflow(t,session);got!="failed"{
  t.Fatalf("failed sibling incorrectly completed parallel join: %s",got)
 }
 job:=session.longTask
 job.capture.mu.Lock()
 output:=string(append([]byte(nil),job.capture.data...))
 job.capture.mu.Unlock()
 if strings.Contains(output,"MUST-NOT-RUN"){
  t.Fatalf("join ran even though a dependency failed: %q",output)
 }
 for label,source:=range map[string]string{
  "nested dependency":`{"tasks":[
    {"label":"A","type":"process","command":"/bin/echo"},
    {"label":"B","type":"process","command":"/bin/echo","dependsOn":"A"},
    {"label":"C","type":"process","command":"/bin/echo","dependsOn":["A","B"],
     "dependsOrder":"parallel"}]}`,
  "retry policy":`{"tasks":[
    {"label":"A","type":"process","command":"/bin/echo",
     "taskdeck":{"workflow":{"retry":1}}},
    {"label":"B","type":"process","command":"/bin/echo"},
    {"label":"C","type":"process","command":"/bin/echo",
     "dependsOn":["A","B"],"dependsOrder":"parallel"}]}`,
 }{
  t.Run(label,func(t *testing.T){
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"C")
   deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
   if err!=nil||deps.RunAvailable{
    t.Fatalf("unsupported parallel DAG was offered: %+v %v",deps,err)
   }
  })
 }
}


 
// A host-local audit storage hook exercises the exact gap between the
// second durable "attempted" row and launching two sibling processes.
// No browser request can choose this mutation or the workflow commands.
type remoteParallelChangeDuringAttemptAudit struct{
 identity.Store
 filename,source,flag string
 rejectSecond bool
}
func(store remoteParallelChangeDuringAttemptAudit)AppendAudit(
 ctx context.Context,event identity.AuditEvent,
)error{
 if event.Action!="remote.action.tasks_workflow_step"||
  event.Result!="attempted"||
  !strings.Contains(event.Details,`"step":2`){
  return store.Store.AppendAudit(ctx,event)
 }
 if store.rejectSecond{return errors.New("second attempted audit unavailable")}
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if store.filename!=""{
  // Keep the same source file and content semantics but change the
  // authenticated bytes so its source digest no longer matches.
  return os.WriteFile(store.filename,[]byte(store.source+" "),0600)
 }
 if store.flag!=""{return os.Setenv(store.flag,"0")}
 return nil
}

func TestRemoteParallelJoinNeverStartsEitherChildAfterAttemptAuditDrift(t *testing.T){
 const source=`{"tasks":[
  {"label":"Fork A","type":"process","command":"/bin/echo","args":["NEVER-CHILD-A"]},
  {"label":"Fork B","type":"process","command":"/bin/echo","args":["NEVER-CHILD-B"]},
  {"label":"Join","type":"process","command":"/bin/echo","args":["NEVER-JOIN"],
   "dependsOn":["Fork A","Fork B"],"dependsOrder":"parallel"}
 ]}`
 for _,tc:=range []struct{
  name,flag,want,history string
  drift,reject bool
 }{
  {"source digest drift","","failed","FF-",true,false},
  {"parallel host opt-in revoked",
   "TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","cancelled","XX-",false,false},
  {"second attempted audit denied","","failed","F--",false,true},
 }{
  t.Run(tc.name,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
   original:=s.Identity
   hook:=remoteParallelChangeDuringAttemptAudit{
    Store:original,source:source,flag:tc.flag,rejectSecond:tc.reject,
   }
   if tc.drift{
    hook.filename=filepath.Join(s.Workspace,".vscode","tasks.json")
   }
   s.Identity=hook
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
    t.Fatalf("parallel fork not admitted before audit mutation: %v",err)
   }
   if state:=awaitRemoteWorkflow(t,session);state!=tc.want{
    t.Fatalf("post-audit mutation returned %q, expected %q",state,tc.want)
   }
   job:=session.longTask
   job.mu.Lock()
   history,code:=job.workflowStepHistory,job.exitCode
   job.mu.Unlock()
   if history!=tc.history||code!= -1{
    t.Fatalf("unlaunched fork status=%q exit=%d, expected %q/-1",
     history,code,tc.history)
   }
   job.capture.mu.Lock()
   output:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if strings.Contains(output,"NEVER-CHILD-A")||
    strings.Contains(output,"NEVER-CHILD-B")||
    strings.Contains(output,"NEVER-JOIN")||
    strings.Contains(output,"parallel attempt 1/1"){
    t.Fatalf("a child process started after audit mutation: %q",output)
   }
   project,err:=original.ProjectByKey(context.Background(),session.projectKey)
   if err!=nil{t.Fatal(err)}
   rows,err:=original.ListAudit(context.Background(),identity.AuditQuery{
    ProjectID:project.ID,UserID:session.userID,
    Action:"remote.action.tasks_workflow_step",Limit:12,
   })
   if err!=nil{t.Fatal(err)}
   attempts,finals:=0,0
   for _,row:=range rows{
    if strings.Contains(row.Details,`"step":3`){
     t.Fatalf("root task audited without both children completing: %+v",row)
    }
    if row.Result=="attempted"{attempts++}
    if row.Result=="failed"||row.Result=="cancelled"{finals++}
   }
   expected:=2
   if tc.reject{expected=1}
   if attempts!=expected||finals!=expected{
    t.Fatalf("orphan attempted audits after denied fork: attempts=%d finals=%d",
     attempts,finals)
   }
   // The final long-job slot is still readable and explicitly clearable.
   if err:=session.clearRemoteLongTask(root);err!=nil{
    t.Fatalf("failed fork leaked its host long-job slot: %v",err)
   }
  })
 }
}

 
// Three independent siblings are the maximum host-approved parallel
// fanout. An authenticated ID-only request cannot set worker count or a
// dynamic dependency graph; the same trusted tasks.json snapshot is used.
func TestRemoteWorkflowThreeParallelLeavesSingleJoin(t *testing.T){
 remoteWorkflowHostFlags(t)
 source:=`{"tasks":[
  {"label":"Leaf 1","type":"process","command":"/bin/sleep","args":["2"]},
  {"label":"Leaf 2","type":"process","command":"/bin/sleep","args":["2"]},
  {"label":"Leaf 3","type":"process","command":"/bin/sleep","args":["2"]},
  {"label":"Join","type":"process","command":"/bin/echo","args":["JOIN-THREE-LEAVES"],
   "dependsOn":["Leaf 1","Leaf 2","Leaf 3"],"dependsOrder":"parallel"}
 ]}`
 t.Run("requires separate opt-in",func(t *testing.T){
  t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","0")
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
  deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
  if err!=nil||!deps.Available||deps.RunAvailable{
   t.Fatalf("unapproved three-way fork appeared executable: %+v %v",deps,err)
  }
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err==nil{
   t.Fatal("unapproved three-way fork started")
  }
 })
 t.Run("three workers overlap and root waits",func(t *testing.T){
  t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
  s,session,client,agent:=remoteTaskOperatorFixture(t)
  root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
  deps,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
  if err!=nil||!deps.RunAvailable||len(deps.Order)!=4||len(deps.Edges)!=3{
   t.Fatalf("valid three-worker join was not offered: %+v %v",deps,err)
  }
  if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
   t.Fatalf("three-worker fork start: %v",err)
  }
  job:=session.longTask
  concurrent:=false
  deadline:=time.Now().Add(1500*time.Millisecond)
  for time.Now().Before(deadline){
   job.mu.Lock()
   current:=job.workflowStepHistory
   parallel:=job.workflowParallel
   job.mu.Unlock()
   if parallel&&current=="RRR-"{concurrent=true;break}
   time.Sleep(10*time.Millisecond)
  }
  if !concurrent{t.Fatal("three independent native processes did not overlap")}
  if got:=awaitRemoteWorkflow(t,session);got!="completed"{
   t.Fatalf("three-way root did not finish: %s",got)
  }
  job.mu.Lock()
  history:=job.workflowStepHistory
  code:=job.exitCode
  job.mu.Unlock()
  if history!="CCCC"||code!=0{
   t.Fatalf("four authenticated steps were not completed: %q exit=%d",history,code)
  }
  job.capture.mu.Lock()
  transcript:=string(append([]byte(nil),job.capture.data...))
  job.capture.mu.Unlock()
  if !strings.Contains(transcript,"JOIN-THREE-LEAVES")||
   strings.Count(transcript,"parallel attempt 1/1")!=4{
   t.Fatalf("root or exactly-once children missing: %q",transcript)
  }
  progress,err:=longTaskE2EEAction(t,s,session,client,agent,
   remotetransport.RemoteActionTasksPoll,root,0,"",
   "33333333333333333333333333333333")
  if err!=nil||!progress.Parallel||progress.StepTotal!=4||
   progress.StepHistory!="CCCC"||len(progress.Output)>48{
   t.Fatalf("four-step fork E2EE status exceeded bounds: %+v %v",progress,err)
  }
 })
}

 
// Turning off a host-owned opt-in AFTER all three processes started must
// terminate their process groups, release the exclusive workspace lease,
// and never launch the dependent root. Browser/relay callers cannot toggle
// this flag: only the host administrator can revoke it.
func TestRemoteParallelHostRevocationCancelsActiveWorkers(t *testing.T){
 const source=`{"tasks":[
  {"label":"Leaf A","type":"process","command":"/bin/sleep","args":["4"]},
  {"label":"Leaf B","type":"process","command":"/bin/sleep","args":["4"]},
  {"label":"Leaf C","type":"process","command":"/bin/sleep","args":["4"]},
  {"label":"Join","type":"process","command":"/bin/echo","args":["ROOT-MUST-NOT-RUN"],
   "dependsOn":["Leaf A","Leaf B","Leaf C"],"dependsOrder":"parallel"}
 ]}`
 for _,flag:=range []string{
  "TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL",
  "TASKDECK_REMOTE_TASKS_RUN",
  "TASKDECK_REMOTE_TASKS_WORKFLOW_RUN",
 }{
  t.Run(flag,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
    t.Fatalf("approved fork not admitted: %v",err)
   }
   job:=session.longTask
   seenThree:=false
   deadline:=time.Now().Add(2*time.Second)
   for time.Now().Before(deadline){
    job.mu.Lock()
    state:=job.workflowStepHistory
    job.mu.Unlock()
    if state=="RRR-"{seenThree=true;break}
    time.Sleep(10*time.Millisecond)
   }
   if !seenThree{t.Fatal("did not observe all three workers running")}
   if err:=os.Setenv(flag,"0");err!=nil{t.Fatal(err)}
   begin:=time.Now()
   if got:=awaitRemoteWorkflow(t,session);got!="cancelled"{
    t.Fatalf("host revocation was not sticky: status=%q",got)
   }
   if time.Since(begin)>2*time.Second{
    t.Fatal("host revocation did not promptly terminate active workers")
   }
   job.mu.Lock()
   history:=job.workflowStepHistory
   job.mu.Unlock()
   if strings.Contains(history,"R")||strings.HasSuffix(history,"C")||
    len(history)!=4||history[3]!='-'{
    t.Fatalf("revoked fork leaked an active worker or ran the root: %q",history)
   }
   job.capture.mu.Lock()
   output:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if strings.Contains(output,"ROOT-MUST-NOT-RUN")||
    !strings.Contains(output,"Parallel host consent revoked; workers stopped"){
    t.Fatalf("revoked fork had unsafe output: %q",output)
   }
   if err:=session.clearRemoteLongTask(root);err!=nil{
    t.Fatalf("cancelled workflow did not release long-task slot: %v",err)
   }
  })
 }
}


// A successful fork never grants persistent authority to the root: the
// host can mutate the TaskDeck source, withdraw consent or fail its audit
// write at the exact root-attempt boundary. No root process may execute.
type remoteParallelRootAuditMutation struct{
 identity.Store
 sourcePath,source,flag string
 reject bool
}
func(store remoteParallelRootAuditMutation)AppendAudit(
 ctx context.Context,event identity.AuditEvent,
)error{
 if event.Action!="remote.action.tasks_workflow_step"||
  event.Result!="attempted"||
  !strings.Contains(event.Details,`"step":3`){
  return store.Store.AppendAudit(ctx,event)
 }
 if store.reject{
  return errors.New("injected unavailable root attempt audit")
 }
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if store.sourcePath!=""{
  return os.WriteFile(store.sourcePath,[]byte(store.source+" "),0600)
 }
 if store.flag!=""{return os.Setenv(store.flag,"0")}
 return nil
}
func TestRemoteParallelJoinRootAuditBarrierPreventsStaleExecution(t *testing.T){
 const source=`{"tasks":[
  {"label":"A","type":"process","command":"/bin/echo","args":["ALLOWED-LEAF-A"]},
  {"label":"B","type":"process","command":"/bin/echo","args":["ALLOWED-LEAF-B"]},
  {"label":"Join","type":"process","command":"/bin/echo","args":["DENIED-ROOT-PROCESS"],
   "dependsOn":["A","B"],"dependsOrder":"parallel"}
 ]}`
 for _,tc:=range []struct{
  name,flag,status,history string
  mutate,reject bool
 }{
  {"tasks.json changed after root attempted","","failed","CCF",true,false},
  {"host opt-in revoked at root audit",
   "TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","cancelled","CCX",false,false},
  {"root attempted audit unavailable","","failed","CC-",false,true},
 }{
  t.Run(tc.name,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
   original:=s.Identity
   hook:=remoteParallelRootAuditMutation{
    Store:original,source:source,flag:tc.flag,reject:tc.reject,
   }
   if tc.mutate{
    hook.sourcePath=filepath.Join(s.Workspace,".vscode","tasks.json")
   }
   s.Identity=hook
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
    t.Fatalf("parallel workflow denied before injected root mutation: %v",err)
   }
   if result:=awaitRemoteWorkflow(t,session);result!=tc.status{
    t.Fatalf("root audit gap yielded %q, expected %q",result,tc.status)
   }
   job:=session.longTask
   job.mu.Lock()
   history,exit:=job.workflowStepHistory,job.exitCode
   job.mu.Unlock()
   if history!=tc.history||exit!= -1{
    t.Fatalf("root ran or lost attempted audit: history=%q exit=%d",history,exit)
   }
   job.capture.mu.Lock()
   output:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if !strings.Contains(output,"ALLOWED-LEAF-A")||
    !strings.Contains(output,"ALLOWED-LEAF-B")||
    strings.Contains(output,"DENIED-ROOT-PROCESS")||
    strings.Contains(output,"Step 3/3: Task #"){
    t.Fatalf("root process crossed an invalid audit barrier: %q",output)
   }
   project,err:=original.ProjectByKey(context.Background(),session.projectKey)
   if err!=nil{t.Fatal(err)}
   rows,err:=original.ListAudit(context.Background(),identity.AuditQuery{
    ProjectID:project.ID,UserID:session.userID,
    Action:"remote.action.tasks_workflow_step",Limit:12,
   })
   if err!=nil{t.Fatal(err)}
   rootAttempted,rootFinal:=0,0
   for _,row:=range rows{
    if !strings.Contains(row.Details,`"step":3`){continue}
    if row.Result=="attempted"{rootAttempted++}
    if row.Result=="failed"||row.Result=="cancelled"{rootFinal++}
    if row.Result=="completed"{
     t.Fatal("unexecuted root was recorded as successfully completed")
    }
   }
   expected:=1
   if tc.reject{expected=0}
   if rootAttempted!=expected||rootFinal!=expected{
    t.Fatalf("root attempted/final audit mismatch: %d/%d expected %d",
     rootAttempted,rootFinal,expected)
   }
   if err:=session.clearRemoteLongTask(root);err!=nil{
    t.Fatalf("failed root left occupied long-task slot: %v",err)
   }
  })
 }
}


// A process identity includes every argument AND every inherited
// environment entry. In particular tasks.ResolveExecutionWithInputs
// refreshes os.Environ() each time, independent of tasks.json's hash.
func TestRemoteParallelSameExecutionRejectsEnvironmentAndArgDrift(t *testing.T){
 original:=remoteParallelPrepared{
  executable:"/usr/bin/echo",
  execution:tasks.Execution{
   TaskID:17,Command:"/usr/bin/echo",Cwd:"/workspace",
   Args:[]string{"hello","world"},
   Env:[]string{"A=one","B=two"},
  },
 }
 if !remoteParallelSameExecution(original,original){
  t.Fatal("unchanged host-issued process rejected")
 }
 clone:=func()remoteParallelPrepared{
  item:=original
  item.execution.Args=append([]string(nil),original.execution.Args...)
  item.execution.Env=append([]string(nil),original.execution.Env...)
  return item
 }
 for name,change:=range map[string]func(*remoteParallelPrepared){
  "executable":func(x *remoteParallelPrepared){x.executable="/usr/bin/false"},
  "task ID":func(x *remoteParallelPrepared){x.execution.TaskID=19},
  "command":func(x *remoteParallelPrepared){x.execution.Command="/usr/bin/true"},
  "cwd":func(x *remoteParallelPrepared){x.execution.Cwd="/tmp"},
  "argument":func(x *remoteParallelPrepared){x.execution.Args[1]="changed"},
  "argument count":func(x *remoteParallelPrepared){
   x.execution.Args=append(x.execution.Args,"extra")
  },
  "environment value":func(x *remoteParallelPrepared){x.execution.Env[0]="A=changed"},
  "environment count":func(x *remoteParallelPrepared){
   x.execution.Env=append(x.execution.Env,"C=three")
  },
 }{
  t.Run(name,func(t *testing.T){
   item:=clone()
   change(&item)
   if remoteParallelSameExecution(original,item){
    t.Fatal("prelaunch identity silently ignored host process changes")
   }
  })
 }
}

// Revoke a process's inherited environment after the attempted audit,
// without touching its stable ID or tasks.json. The executable, argument
// list and working directory all remain the same: only the NEW stricter
// prelaunch identity comparison can stop the unsafe root execution.
type remoteParallelEnvDriftAudit struct{
 identity.Store
 afterStep int
 envName string
}
func(store remoteParallelEnvDriftAudit)AppendAudit(
 ctx context.Context,event identity.AuditEvent,
)error{
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action=="remote.action.tasks_workflow_step"&&
  event.Result=="attempted"&&
  strings.Contains(event.Details,fmt.Sprintf(`"step":%d`,store.afterStep)){
  return os.Setenv(store.envName,"changed-after-attempted-audit")
 }
 return nil
}
func TestRemoteParallelPrelaunchDeniesInheritedEnvironmentDrift(t *testing.T){
 const source=`{"tasks":[
  {"label":"A","type":"process","command":"/bin/echo","args":["FIRST-CHILD-OUTPUT"]},
  {"label":"B","type":"process","command":"/bin/echo","args":["SECOND-CHILD-OUTPUT"]},
  {"label":"Join","type":"process","command":"/bin/echo","args":["ROOT-MUST-NOT-RUN"],
   "dependsOn":["A","B"],"dependsOrder":"parallel"}
 ]}`
 for _,tc:=range []struct{
  name string
  afterStep int
  wantedHistory string
  childOutput bool
 }{
  {"sibling drift after second audit",2,"FF-",false},
  {"root drift after all leaves succeeded",3,"CCF",true},
 }{
  t.Run(tc.name,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
   t.Setenv("TASKDECK_TEST_PARALLEL_ENV_DRIFT","before")
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
   previous:=s.Identity
   s.Identity=remoteParallelEnvDriftAudit{
    Store:previous,afterStep:tc.afterStep,
    envName:"TASKDECK_TEST_PARALLEL_ENV_DRIFT",
   }
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
    t.Fatalf("host approved initial fork: %v",err)
   }
   if got:=awaitRemoteWorkflow(t,session);got!="failed"{
    t.Fatalf("changed process environment did not fail closed: %s",got)
   }
   job:=session.longTask
   job.mu.Lock()
   history:=job.workflowStepHistory
   job.mu.Unlock()
   if history!=tc.wantedHistory{
    t.Fatalf("env drift history=%q want=%q",history,tc.wantedHistory)
   }
   job.capture.mu.Lock()
   output:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if strings.Contains(output,"ROOT-MUST-NOT-RUN"){
    t.Fatalf("stale root ran despite environment drift: %q",output)
   }
   if tc.childOutput{
    if !strings.Contains(output,"FIRST-CHILD-OUTPUT")||
     !strings.Contains(output,"SECOND-CHILD-OUTPUT"){
     t.Fatalf("prepared independent siblings did not run: %q",output)
    }
   }else if strings.Contains(output,"FIRST-CHILD-OUTPUT")||
    strings.Contains(output,"SECOND-CHILD-OUTPUT"){
    t.Fatalf("sibling fork ran despite prelaunch env drift: %q",output)
   }
   if err:=session.clearRemoteLongTask(root);err!=nil{
    t.Fatalf("denied run retained an active job: %v",err)
   }
  })
 }
}


// Revocation of a host-owned Remote flag must terminate the actual final
// root process too, not only its completed dependencies. The root is the
// step that may mutate the workspace after the fork; a new E2EE Poll or
// Cancel request is not required for host-initiated revocation.
func TestRemoteParallelHostRevocationStopsActiveRoot(t *testing.T){
 source:=`{"tasks":[
  {"label":"Leaf A","type":"process","command":"/bin/echo","args":["LEAF-A-OK"]},
  {"label":"Leaf B","type":"process","command":"/bin/echo","args":["LEAF-B-OK"]},
  {"label":"Join","type":"process","command":"/bin/sleep","args":["4"],
   "dependsOn":["Leaf A","Leaf B"],"dependsOrder":"parallel"}
 ]}`
 for _,flag:=range []string{
  "TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL",
  "TASKDECK_REMOTE_TASKS_RUN",
  "TASKDECK_REMOTE_TASKS_WORKFLOW_RUN",
 }{
  t.Run(flag,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL","1")
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,source,"Join")
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
    t.Fatalf("approved fork rejected before root: %v",err)
   }
   job:=session.longTask
   seenRoot:=false
   waitUntil:=time.Now().Add(2*time.Second)
   for time.Now().Before(waitUntil){
    job.mu.Lock()
    history:=job.workflowStepHistory
    job.mu.Unlock()
    if history=="CCR"{seenRoot=true;break}
    time.Sleep(10*time.Millisecond)
   }
   if !seenRoot{t.Fatal("did not observe active root after completed fork")}
   if err:=os.Setenv(flag,"0");err!=nil{t.Fatal(err)}
   start:=time.Now()
   if got:=awaitRemoteWorkflow(t,session);got!="cancelled"{
    t.Fatalf("revoked host flag did not cancel active root: %s",got)
   }
   if time.Since(start)>2*time.Second{
    t.Fatal("active root ignored host revocation and ran to completion")
   }
   job.mu.Lock()
   history,exit:=job.workflowStepHistory,job.exitCode
   job.mu.Unlock()
   if history!="CCX"||exit!= -1{
    t.Fatalf("root not recorded cancelled: history %q exit %d",history,exit)
   }
   job.capture.mu.Lock()
   text:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if !strings.Contains(text,"Parallel host consent revoked; root stopped"){
    t.Fatalf("root revocation was not visible in authenticated transcript: %q",text)
   }
   if err:=session.clearRemoteLongTask(root);err!=nil{
    t.Fatalf("revoked root left the long task slot occupied: %v",err)
   }
  })
 }
}


// Host-owned audit hooks inject a local mutation exactly after the durable
// step "attempted" row. These are not client-supplied commands or values.
func TestRemoteSequentialPostAuditPrelaunchRejectsDrift(t *testing.T){
 for _,tc:=range []struct{
  name string
  step int
  sourceChange bool
  wantHistory string
  wantLeafOutput bool
 }{
  {"environment changes at first step",1,false,"F--",false},
  {"tasks.json changes before root",3,true,"CCF",true},
 }{
  t.Run(tc.name,func(t *testing.T){
   remoteWorkflowHostFlags(t)
   t.Setenv("TASKDECK_TEST_SEQUENTIAL_AUDIT_ENV","original")
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   root:=remoteWorkflowIssuedRoot(t,s,session,client,agent,
    remoteWorkflowLinearSource,"Step C")
   original:=s.Identity
   if tc.sourceChange{
    s.Identity=remoteParallelRootAuditMutation{
     Store:original,
     sourcePath:filepath.Join(s.Workspace,".vscode","tasks.json"),
     source:remoteWorkflowLinearSource,
    }
   }else{
    s.Identity=remoteParallelEnvDriftAudit{
     Store:original,afterStep:tc.step,
     envName:"TASKDECK_TEST_SEQUENTIAL_AUDIT_ENV",
    }
   }
   if err:=s.startRemoteLinearWorkflow(context.Background(),session,root);err!=nil{
    t.Fatalf("approved sequential workflow could not start: %v",err)
   }
   if got:=awaitRemoteWorkflow(t,session);got!="failed"{
    t.Fatalf("stale sequential workflow continued: status=%q",got)
   }
   job:=session.longTask
   job.mu.Lock()
   history,exit:=job.workflowStepHistory,job.exitCode
   job.mu.Unlock()
   if history!=tc.wantHistory||exit!= -1{
    t.Fatalf("stale step ran or history lost: got %q exit %d, want %q",
     history,exit,tc.wantHistory)
   }
   job.capture.mu.Lock()
   output:=string(append([]byte(nil),job.capture.data...))
   job.capture.mu.Unlock()
   if !strings.Contains(output,"post-audit prelaunch denied; not started"){
    t.Fatalf("missing fail-closed transcript: %q",output)
   }
   if tc.wantLeafOutput{
    if !strings.Contains(output,"\nONE\n")||
     !strings.Contains(output,"\nTWO\n")||
     strings.Contains(output,"\nTHREE\n"){
     t.Fatalf("completed dependencies were lost or stale root executed: %q",output)
    }
   }else if strings.Contains(output,"\nONE\n")||
    strings.Contains(output,"\nTWO\n")||
    strings.Contains(output,"\nTHREE\n"){
    t.Fatalf("unapproved sequential process executed: %q",output)
   }
   project,err:=original.ProjectByKey(context.Background(),session.projectKey)
   if err!=nil{t.Fatal(err)}
   audits,err:=original.ListAudit(context.Background(),identity.AuditQuery{
    ProjectID:project.ID,UserID:session.userID,
    Action:"remote.action.tasks_workflow_step",Limit:12,
   })
   if err!=nil{t.Fatal(err)}
   attempted,closed:=0,0
   for _,entry:=range audits{
    if !strings.Contains(entry.Details,
     fmt.Sprintf(`"step":%d`,tc.step)){continue}
    if entry.Result=="attempted"{attempted++}
    if entry.Result=="failed"{closed++}
    if entry.Result=="completed"{
     t.Fatalf("stale process recorded success: %+v",entry)
    }
   }
   if attempted!=1||closed!=1{
    t.Fatalf("stale attempted audit not closed: attempted %d, failed %d",
     attempted,closed)
   }
   if err:=session.clearRemoteLongTask(root);err!=nil{
    t.Fatalf("failed workflow leaked host long-job slot: %v",err)
   }
  })
 }
}
