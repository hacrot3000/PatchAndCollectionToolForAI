package server

import(
 "context"
 "fmt"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func recordWorkflowAuditTestRow(t *testing.T,s *Server,project,user identity.ID,
 taskID,rootID,step int,status string,when time.Time,details string){
 t.Helper()
 id,err:=identity.NewID()
 if err!=nil{t.Fatal(err)}
 if details==""{
  details=fmt.Sprintf(`{"transport":"e2ee","root_task_id":%d,"task_id":%d,"step":%d,"total":3,"attempt":1,"exit_code":%d}`,
   rootID,taskID,step,func()int{if status=="completed"{return 0};return 1}())
 }
 if err:=s.Identity.AppendAudit(context.Background(),identity.AuditEvent{
  ID:id,Timestamp:when,UserID:&user,ProjectID:&project,
  Action:"remote.action.tasks_workflow_step",
  ResourceType:"tasks",Result:status,Details:details,
 });err!=nil{t.Fatal(err)}
}

func TestRemoteWorkflowAuditPrincipalIsolationDurabilityAndHostOptIn(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_REPORT","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_REPORT","1")
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Truncate(time.Second)
 principal,err:=s.Identity.UserByID(context.Background(),session.userID)
 if err!=nil{t.Fatal(err)}
 outsider,err:=identity.NewID()
 if err!=nil{t.Fatal(err)}
 if err:=s.Identity.CreateUser(context.Background(),identity.User{
  ID:outsider,Username:"workflow-audit-other",DisplayName:"Outsider",
  PasswordHash:principal.PasswordHash,Enabled:true,
  CreatedAt:now,UpdatedAt:now,
 });err!=nil{t.Fatal(err)}
 recordWorkflowAuditTestRow(t,s,project.ID,outsider,90,90,1,
  "completed",now.Add(3*time.Second),"")
 recordWorkflowAuditTestRow(t,s,project.ID,session.userID,22,24,1,
  "completed",now.Add(-5*time.Second),"")
 recordWorkflowAuditTestRow(t,s,project.ID,session.userID,23,24,2,
  "failed",now.Add(-3*time.Second),"")
 recordWorkflowAuditTestRow(t,s,project.ID,session.userID,23,24,2,
  "attempted",now.Add(-2*time.Second),"")
 recordWorkflowAuditTestRow(t,s,project.ID,session.userID,23,24,2,
  "failed",now.Add(-1*time.Second),
  `{"transport":"e2ee","root_task_id":24,"task_id":23,"step":2,
    "total":3,"attempt":1,"exit_code":1,"output":"HOST SECRET"}`)
 result,err:=s.readRemoteWorkflowAudit(context.Background(),session)
 if err!=nil||len(result.Items)!=2||
  result.Items[0].TaskID!=23||result.Items[0].Status!="failed"||
  result.Items[1].TaskID!=22||result.Items[1].RootTaskID!=24{
  t.Fatalf("local durable scoped step report incorrect: %+v %v",result,err)
 }
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksWorkflowAudit,
 })
 if err!=nil{t.Fatal(err)}
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{t.Fatalf("viewer audit action denied: %v",err)}
 decoded,err:=remotetransport.OpenRemoteWorkflowAudit(client,response,remoteDispatchTestID)
 if err!=nil||len(decoded.Items)!=2||decoded.Items[0].RootTaskID!=24{
  t.Fatalf("E2EE durable workflow audit incorrect: %+v %v",decoded,err)
 }
 for _,item:=range decoded.Items{
  if item.TaskID==90{t.Fatal("cross-user audit row disclosed")}
  if item.Status=="attempted"{t.Fatal("unfinished attempt shown as final result")}
 }
 if strings.Contains(fmt.Sprintf("%+v",decoded),"HOST SECRET"){
  t.Fatal("malicious audit content was disclosed")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_REPORT","0")
 if _,err:=s.readRemoteWorkflowAudit(context.Background(),session);err==nil{
  t.Fatal("new report bypassed dedicated host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_WORKFLOW_REPORT","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_REPORT","0")
 if _,err:=s.readRemoteWorkflowAudit(context.Background(),session);err==nil{
  t.Fatal("workflow report bypassed existing report opt-in")
 }
}

func TestRemoteWorkflowAuditStrictStoredSchema(t *testing.T){
 valid:=`{"transport":"e2ee","root_task_id":3,"task_id":2,"step":1,"total":2,"attempt":1,"exit_code":0}`
 if _,ok:=decodeRemoteWorkflowStepAuditDetails(valid);!ok{
  t.Fatal("valid host audit rejected")
 }
 for _,source:=range []string{
  strings.Replace(valid,`"step":1`,`"step":1,"step":2`,1),
  strings.Replace(valid,`"exit_code":0`,`"exit_code":null`,1),
  strings.Replace(valid,`"exit_code":0`,`"exit_code":-0`,1),
  strings.Replace(valid,`"step":1`,`"step":1,"command":"/bin/sh"`,1),
  strings.Replace(valid,`"step":1`,`"step":3`,1),
  strings.Replace(valid,`"transport":"e2ee"`,`"transport":"php"`,1),
 }{
  if _,ok:=decodeRemoteWorkflowStepAuditDetails(source);ok{
   t.Fatalf("invalid durable audit schema accepted: %s",source)
  }
 }
}
