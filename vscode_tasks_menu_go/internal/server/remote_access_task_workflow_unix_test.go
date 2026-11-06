//go:build linux || darwin

package server

import(
 "context"
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
