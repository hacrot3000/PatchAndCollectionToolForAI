package server

import (
 "context"
 "fmt"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Cancel and clear are per-session task lifecycle mutations. Persist the
// intention before changing job state; no commands, raw input, output,
// credentials, role tickets or E2EE request IDs belong in TaskDeck audit.
func (s *Server) auditRemoteLongTaskMutation(
 session *remoteTaskDeckSession,taskID int,operation string,
)error{
 if s==nil||s.Identity==nil||session==nil||
  session.closed.Load()||taskID<1||taskID>0x7fffffff||
  (operation!="cancel"&&operation!="clear"){
  return remotetransport.ErrRemoteActionDenied
 }
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
 defer cancel()
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil||!project.Enabled{return remotetransport.ErrRemoteActionDenied}
 eventID,err:=identity.NewID()
 if err!=nil{return remotetransport.ErrRemoteActionDenied}
 user,projectID:=session.userID,project.ID
 err=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),
  UserID:&user,ProjectID:&projectID,
  Action:"remote.action.tasks_"+operation,ResourceType:"tasks",Result:"attempted",
  Details:fmt.Sprintf("{\"transport\":\"e2ee\",\"task_id\":%d}",taskID),
 })
 if err!=nil||session.closed.Load(){return remotetransport.ErrRemoteActionDenied}
 return nil
}
