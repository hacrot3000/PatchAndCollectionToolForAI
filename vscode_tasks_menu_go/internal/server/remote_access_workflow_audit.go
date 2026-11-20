package server

import (
 "context"
 "os"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const remoteWorkflowAuditSnapshotMaxEntries=32
const remoteWorkflowAuditScanLimit=128

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
  Limit:remoteWorkflowAuditScanLimit,
 })
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){return empty,denied}
 snapshot:=make([]remotetransport.RemoteWorkflowAuditEntry,0,
  remoteWorkflowAuditSnapshotMaxEntries)
 for _,event:=range events{
  if len(snapshot)==remoteWorkflowAuditSnapshotMaxEntries{break}
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
   Timestamp:event.Timestamp.UTC().Unix(),RunID:detail.RunID,
  }
  if !remotetransport.ValidRemoteWorkflowAuditEntry(item){continue}
  if len(snapshot)>0&&
   item.Timestamp>snapshot[len(snapshot)-1].Timestamp{continue}
  snapshot=append(snapshot,item)
 }
 if ctx.Err()!=nil||session.closed.Load()||
  !session.portalGrant.ActiveForSession(session.channel)||
  !remotetransport.ValidRemoteWorkflowAuditRunMapping(snapshot){
  return empty,denied
 }
 // executeRemoteE2EEPing holds workflowAuditPageMu until the response
 // seals. An unsuccessful seal cannot advance or authorize a next page.
 session.workflowAuditPageReady=false
 session.workflowAuditPageCursor=0
 session.workflowAuditPageItems=snapshot
 end:=len(snapshot)
 if end>remotetransport.MaxRemoteWorkflowAuditEntries{
  end=remotetransport.MaxRemoteWorkflowAuditEntries
 }
 report:=remotetransport.RemoteWorkflowAudit{
  Items:append([]remotetransport.RemoteWorkflowAuditEntry{},snapshot[:end]...),
  More:end<len(snapshot),
 }
 if !remotetransport.ValidRemoteWorkflowAudit(report){return empty,denied}
 return report,nil
}

// Every Next rechecks current local host opt-ins, principal and Portal
// grant. The caller supplies no offset; authorization and sealing are
// serialized by workflowAuditPageMu at the dispatcher.
func (s *Server)readRemoteWorkflowAuditNext(ctx context.Context,
 session *remoteTaskDeckSession)(remotetransport.RemoteWorkflowAudit,error){
 var empty remotetransport.RemoteWorkflowAudit
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||s.Identity==nil||session==nil||ctx==nil||
  ctx.Err()!=nil||session.closed.Load()||
  session.channel==nil||!session.channel.Established()||
  session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel)||
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_REPORT")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_WORKFLOW_REPORT")!="1"||
  session.projectKey!=s.Config.SharedProjectID||
  !session.workflowAuditPageReady{return empty,denied}
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled{return empty,denied}
 start:=session.workflowAuditPageCursor
 if start<1||start>=len(session.workflowAuditPageItems){
  return empty,denied
 }
 end:=start+remotetransport.MaxRemoteWorkflowAuditEntries
 if end>len(session.workflowAuditPageItems){
  end=len(session.workflowAuditPageItems)
 }
 report:=remotetransport.RemoteWorkflowAudit{
  Items:append([]remotetransport.RemoteWorkflowAuditEntry{},
   session.workflowAuditPageItems[start:end]...),
  More:end<len(session.workflowAuditPageItems),
 }
 if !remotetransport.ValidRemoteWorkflowAudit(report){return empty,denied}
 return report,nil
}
