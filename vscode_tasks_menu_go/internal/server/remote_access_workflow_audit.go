package server

import (
 "context"
 "os"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// This is read-only local audit history, across successive E2EE logins.
// No task commands, paths, output, key material, raw audit JSON or server
// secrets can enter the response. It is NOT an execution-resume API.
func (s *Server)readRemoteWorkflowAudit(ctx context.Context,
 session *remoteTaskDeckSession)(remotetransport.RemoteWorkflowAudit,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteWorkflowAudit
 if s==nil||s.Identity==nil||session==nil||ctx==nil||
  ctx.Err()!=nil||session.closed.Load()||
  session.userID==""||session.projectKey==""||
  session.projectKey!=s.Config.SharedProjectID||
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_REPORT")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_REPORT")!="1"||
  session.channel==nil||!session.channel.Established()||
  session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return empty,denied
 }
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled||project.Key!=session.projectKey{
  return empty,denied
 }
 events,err:=s.Identity.ListAudit(ctx,identity.AuditQuery{
  ProjectID:project.ID,UserID:session.userID,
  Action:"remote.action.tasks_workflow_step",ResourceType:"tasks",
  Limit:32,
 })
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){return empty,denied}
 report:=remotetransport.RemoteWorkflowAudit{
  Items:make([]remotetransport.RemoteWorkflowAuditEntry,0,
   remotetransport.MaxRemoteWorkflowAuditEntries),
 }
 for _,event:=range events{
  if len(report.Items)==remotetransport.MaxRemoteWorkflowAuditEntries{break}
  if event.UserID==nil||*event.UserID!=session.userID||
   event.ProjectID==nil||*event.ProjectID!=project.ID||
   event.Action!="remote.action.tasks_workflow_step"||
   event.ResourceType!="tasks"||event.Timestamp.IsZero()||
   event.Result=="attempted"{continue}
  detail,valid:=decodeRemoteWorkflowStepAuditDetails(event.Details)
  if !valid{continue}
  item:=remotetransport.RemoteWorkflowAuditEntry{
   RootTaskID:detail.RootTaskID,TaskID:detail.TaskID,
   Step:detail.Step,Total:detail.Total,Attempt:detail.Attempt,
   Status:event.Result,ExitCode:detail.ExitCode,
   Timestamp:event.Timestamp.UTC().Unix(),
  }
  if !remotetransport.ValidRemoteWorkflowAuditEntry(item){continue}
  if len(report.Items)>0&&
   item.Timestamp>report.Items[len(report.Items)-1].Timestamp{continue}
  report.Items=append(report.Items,item)
 }
 if ctx.Err()!=nil||session.closed.Load()||
  !session.portalGrant.ActiveForSession(session.channel)||
  !remotetransport.ValidRemoteWorkflowAudit(report){
  return empty,denied
 }
 return report,nil
}
