package server

import (
 "bytes"
 "context"
 "encoding/json"
 "os"
 "io"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// readRemoteTaskReport is deliberately read-only. Its source is the local
// immutable TaskDeck audit store, scoped to BOTH the exact authenticated
// project and current user. PHP/Relay never sees output or audit details.
func(s *Server)readRemoteTaskReport(ctx context.Context,
 session *remoteTaskDeckSession)(remotetransport.RemoteTaskReport,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteTaskReport
 if s==nil||s.Identity==nil||session==nil||ctx==nil||
  ctx.Err()!=nil||session.closed.Load()||
  session.userID==""||session.projectKey==""||
  session.projectKey!=s.Config.SharedProjectID||
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_REPORT")!="1"||
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
  Action:"remote.action.tasks_run_finished",
  Limit:16,
 })
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){return empty,denied}
 report:=remotetransport.RemoteTaskReport{
  Items:make([]remotetransport.RemoteTaskReportEntry,0,remotetransport.MaxRemoteTaskReportEntries),
 }
 for _,event:=range events{
  if len(report.Items)==remotetransport.MaxRemoteTaskReportEntries{break}
  if event.UserID==nil||*event.UserID!=session.userID||
   event.ProjectID==nil||*event.ProjectID!=project.ID||
   event.Action!="remote.action.tasks_run_finished"||
   event.ResourceType!="tasks"||event.Timestamp.IsZero(){
   continue
  }
  // An untrusted or legacy audit detail must not be forwarded verbatim.
  // Parse only the exact known schema and emit independently validated
  // numeric outcome fields; skip malformed or unexpected metadata.
  dec:=json.NewDecoder(bytes.NewBufferString(event.Details))
  dec.DisallowUnknownFields()
  var detail struct{
   Transport string `json:"transport"`
   TaskID int `json:"task_id"`
   ExitCode int `json:"exit_code"`
   DurationMS int64 `json:"duration_ms"`
  }
  if dec.Decode(&detail)!=nil{continue}
  var trailing any
  if dec.Decode(&trailing)!=io.EOF{continue}
  item:=remotetransport.RemoteTaskReportEntry{
   TaskID:detail.TaskID,Status:event.Result,
   ExitCode:detail.ExitCode,DurationMS:detail.DurationMS,
   Timestamp:event.Timestamp.UTC().Unix(),
  }
  if detail.Transport!="e2ee"||
   !remotetransport.ValidRemoteTaskReportEntry(item){continue}
  if len(report.Items)>0&&
   item.Timestamp>report.Items[len(report.Items)-1].Timestamp{continue}
  report.Items=append(report.Items,item)
 }
 if ctx.Err()!=nil||session.closed.Load()||
  !session.portalGrant.ActiveForSession(session.channel)||
  !remotetransport.ValidRemoteTaskReport(report){return empty,denied}
 return report,nil
}
