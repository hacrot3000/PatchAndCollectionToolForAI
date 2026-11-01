package server

import (
 "context"
 "crypto/subtle"
 "os"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Reads only a verified TaskDeck-owned child for this exact authenticated
// E2EE principal. An older broker that cannot certify monotonic output
// offsets fails closed rather than returning silently truncated scrollback.
func (s *Server) remoteManagedTaskOutput(ctx context.Context,
 owner *remoteTaskDeckSession,channel *remotetransport.Channel,
 handle string,offset int64)(remotetransport.RemoteManagedOutput,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||owner==nil||channel==nil||
  owner.channel!=channel||owner.closed.Load()||owner.portalGrant==nil||
  !owner.portalGrant.ActiveForSession(channel)||
  os.Getenv("TASKDECK_REMOTE_TASK_RUN")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASK_MANAGE")!="1"||
  !remotetransport.ValidManagedTaskHandle(handle)||
  !remotetransport.ValidManagedOutputOffset(offset){
  return remotetransport.RemoteManagedOutput{},denied
 }
 reader,ok:=s.Sessions.(session.OutputWindowReader)
 if !ok{return remotetransport.RemoteManagedOutput{},denied}
 owner.executionMu.Lock()
 defer owner.executionMu.Unlock()
 if owner.closed.Load()||owner.channel!=channel||
  !owner.portalGrant.ActiveForSession(channel)||ctx.Err()!=nil{
  return remotetransport.RemoteManagedOutput{},denied
 }
 owner.managedMu.Lock()
 defer owner.managedMu.Unlock()
 task:=owner.managedTask
 if task==nil||
  subtle.ConstantTimeCompare([]byte(task.handle),[]byte(handle))!=1||
  task.userID!=string(owner.userID){
  return remotetransport.RemoteManagedOutput{},denied
 }
 project,err:=s.Identity.ProjectByKey(ctx,owner.projectKey)
 if err!=nil||string(project.ID)!=task.projectID{
  return remotetransport.RemoteManagedOutput{},denied
 }
 meta,ok:=s.Sessions.Metadata(task.sessionID)
 if !ok||meta.OwnerUserID!=task.userID||
  meta.ProjectID!=task.projectID||meta.Kind!=tasks.SessionKindTask{
  return remotetransport.RemoteManagedOutput{},denied
 }
 window,err:=reader.ReadOutputWindow(task.sessionID,offset,
  remotetransport.RemoteManagedOutputWindowMaxBytes)
 if err!=nil||!remotetransport.ValidManagedOutputOffset(window.Total){
  // In particular, a stale cursor whose bytes have been evicted is NEVER
  // advanced to the retained suffix; client must see an explicit failure.
  return remotetransport.RemoteManagedOutput{},denied
 }
 after,ok:=s.Sessions.Metadata(task.sessionID)
 if !ok||after.OwnerUserID!=task.userID||
  after.ProjectID!=task.projectID||after.Kind!=tasks.SessionKindTask||
  (after.Status!="running"&&after.Status!="exited"&&after.Status!="stopped")||
  ctx.Err()!=nil{
  return remotetransport.RemoteManagedOutput{},denied
 }
 result:=remotetransport.RemoteManagedOutput{
  Handle:handle,Offset:window.Offset,Next:window.Next,Total:window.Total,
  State:after.Status,EOF:after.Status!="running"&&window.Next==window.Total,
  Data:string(window.Data),
 }
 // Validate again before the outer dispatcher seals with the same channel.
 if !remotetransport.ValidRemoteManagedOutput(result){
  return remotetransport.RemoteManagedOutput{},denied
 }
 return result,nil
}
