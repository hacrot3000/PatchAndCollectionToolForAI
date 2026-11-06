package server

import (
 "context"
 "fmt"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func recordRemoteTaskReportAudit(t *testing.T,s *Server,projectID,userID identity.ID,
 taskID int,status string,when time.Time,details string){
 t.Helper()
 id,err:=identity.NewID()
 if err!=nil{t.Fatal(err)}
 if details==""{
  details=fmt.Sprintf(`{"transport":"e2ee","task_id":%d,"exit_code":7,"duration_ms":20}`,taskID)
 }
 err=s.Identity.AppendAudit(context.Background(),identity.AuditEvent{
  ID:id,Timestamp:when,UserID:&userID,ProjectID:&projectID,
  Action:"remote.action.tasks_run_finished",ResourceType:"tasks",
  Result:status,Details:details,
 })
 if err!=nil{t.Fatal(err)}
}
func TestRemoteTaskReportPrincipalIsolationAndOptIn(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_REPORT","1")
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 userID:=session.userID
 outsider,err:=identity.NewID()
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Truncate(time.Second)
 ownUser,err:=s.Identity.UserByID(context.Background(),session.userID)
 if err!=nil{t.Fatal(err)}
 // The SQLite audit table enforces a real user FK. A fabricated ID would
 // exercise an impossible database state rather than E2EE principal isolation.
 if err:=s.Identity.CreateUser(context.Background(),identity.User{
  ID:outsider,Username:"audit-other-user",DisplayName:"Audit outsider",
  PasswordHash:ownUser.PasswordHash,Enabled:true,CreatedAt:now,UpdatedAt:now,
 });err!=nil{t.Fatal(err)}
 recordRemoteTaskReportAudit(t,s,project.ID,outsider,99,"completed",now.Add(-2*time.Second),"")
 recordRemoteTaskReportAudit(t,s,project.ID,userID,11,"failed",now,"")
 recordRemoteTaskReportAudit(t,s,project.ID,userID,12,"completed",now.Add(-time.Second),"")
 recordRemoteTaskReportAudit(t,s,project.ID,userID,13,"completed",now.Add(-3*time.Second),
  `{"transport":"e2ee","task_id":13,"exit_code":0,"duration_ms":0,"output":"SHOULD NEVER BE READ"}`)
 report,err:=s.readRemoteTaskReport(context.Background(),session)
 if err!=nil||len(report.Items)!=2{
  t.Fatalf("wrong per-user report: %+v %v",report,err)
 }
 if report.Items[0].TaskID!=11||report.Items[1].TaskID!=12{
  t.Fatalf("report included cross-user or malformed events: %+v",report)
 }
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksReport,
 })
 if err!=nil{t.Fatal(err)}
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{t.Fatalf("viewer tasks.view denied read-only report: %v",err)}
 decoded,err:=remotetransport.OpenRemoteTaskReport(client,response,remoteDispatchTestID)
 if err!=nil||len(decoded.Items)!=2||decoded.Items[0].TaskID!=11{
  t.Fatalf("E2EE outcome report: %+v %v",decoded,err)
 }
 if strings.Contains(fmt.Sprintf("%+v",decoded),"99")||
  strings.Contains(fmt.Sprintf("%+v",decoded),"SHOULD NEVER"){
  t.Fatal("report leaked another user's run or raw audit data")
 }
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,UserID:userID,Action:"remote.action.tasks_report",Limit:5,
 })
 if err!=nil||len(audit)!=1||audit[0].Result!="authorized"||
  audit[0].ResourceType!="tasks"{
  t.Fatalf("report access missing TaskDeck audit: %+v %v",audit,err)
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_REPORT","0")
 if _,err:=s.readRemoteTaskReport(context.Background(),session);err==nil{
  t.Fatal("task report bypassed host opt-in")
 }
}
func TestRemoteTaskReportBoundsAndRejectsHostileAuditRows(t *testing.T){
 s,session,_,_:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_REPORT","1")
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 for i:=0;i<10;i++{
  recordRemoteTaskReportAudit(t,s,project.ID,session.userID,i+1,"failed",
   now.Add(-time.Duration(i)*time.Second),"")
 }
 report,err:=s.readRemoteTaskReport(context.Background(),session)
 if err!=nil||len(report.Items)!=3{
  t.Fatalf("expected capped three-entry report: %+v %v",report,err)
 }
 for i,item:=range report.Items{
  if item.TaskID!=i+1{t.Fatalf("unsorted capped task report: %+v",report)}
 }
 // No output, environment or executable is ever reconstructed from the
 // audit DB, including legacy entries with hostile JSON.
 eventID,err:=identity.NewID()
 if err!=nil{t.Fatal(err)}
 if err:=s.Identity.AppendAudit(context.Background(),identity.AuditEvent{
  ID:eventID,Timestamp:now.Add(time.Second),
  UserID:&session.userID,ProjectID:&project.ID,
  Action:"remote.action.tasks_run_finished",
  ResourceType:"tasks",Result:"completed",
  Details:`{"transport":"e2ee","task_id":123,"exit_code":0,"duration_ms":10,"command":"/bin/sh"}`,
 });err!=nil{t.Fatal(err)}
 refreshed,err:=s.readRemoteTaskReport(context.Background(),session)
 if err!=nil||len(refreshed.Items)!=3||refreshed.Items[0].TaskID!=1{
  t.Fatalf("malicious audit record poisoned user report: %+v %v",refreshed,err)
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","0")
 if _,err:=s.readRemoteTaskReport(context.Background(),session);err==nil{
  t.Fatal("report bypassed tasks.preview host gate")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 s.Workspace=""
 // Report is host-audit-only; workspace path is not part of its data.
 if _,err:=s.readRemoteTaskReport(context.Background(),session);err!=nil{
  t.Fatalf("report should not depend on cwd: %v",err)
 }
}
