package server

import (
 "context"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A compromised PHP Portal cannot erase TaskDeck-local identity audit.
// Record locally authorized action attempts using the real TaskDeck principal,
// without request IDs, plaintext credentials, Portal tickets or ciphertext.
// A failed audit is a hard refusal; do not produce an encrypted "success".
func (s *Server) auditRemoteReadOnlyPing(session *remoteTaskDeckSession) error {
 return s.auditRemoteReadOnlyAction(session,"session_ping")
}
func (s *Server) auditRemoteReadOnlyAction(session *remoteTaskDeckSession,action string) error {
 if action!="session_ping" && action!="project_file_read" {
  return remotetransport.ErrRemoteActionDenied
 }
 if s==nil || session==nil || session.userID=="" || s.Identity==nil {
  return remotetransport.ErrRemoteActionDenied
 }
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
 defer cancel()
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil{return remotetransport.ErrRemoteActionDenied}
 eventID,err:=identity.NewID()
 if err!=nil{return remotetransport.ErrRemoteActionDenied}
 userID:=session.userID
 projectID:=project.ID
 err=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),
  UserID:&userID,ProjectID:&projectID,
  Action:"remote.action."+action,ResourceType:"session",
  Result:"authorized",
  Details:`{"transport":"e2ee","taskdeck_auth":true,"remote_control":false}`,
 })
 if err!=nil{return remotetransport.ErrRemoteActionDenied}
 return nil
}

// executeRemoteE2EEPing is the first deliberately UNMOUNTED typed remote
// application operation. It is a one-packet, read-only authenticated ping,
// NOT an HTTP/WS/terminal/file/Git/Patch dispatcher and grants no session.
//
// A forged or stolen PHP Portal admin ticket cannot manufacture session:
// only authenticateRemoteTaskDeckCredentials can create its in-memory
// TaskDeck-local password principal. The exact E2EE identity and channel,
// local per-action permission, account status and Portal live role are
// rechecked both before and after running this fixed operation.
//
// Arbitrary commands/URLs/paths/user-provided callbacks are not accepted.
// The caller MUST forward a sealed response only while its Agent Grant
// and browser connection are still live and must retire the session on
// logout, disconnect, revocation or the 5-minute local deadline.
func (s *Server) executeRemoteE2EEPing(
 ctx context.Context, session *remoteTaskDeckSession,
 channel *remotetransport.Channel, frame []byte,
) ([]byte,error) {
 if s==nil || ctx==nil || session==nil || channel==nil ||
  !channel.Established() || channel!=session.channel ||
  session.closed.Load() || session.portalGrant==nil ||
  !session.portalGrant.ActiveForSession(channel) {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Validate the encrypted schema only AFTER checking that this exact
 // TaskDeck-local password session owns this authenticated channel.
 // A badly typed but authenticated record permanently poisons the E2EE key.
 request,err:=remotetransport.OpenRemoteAction(channel,frame)
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 // Dispatch exactly the two reviewed typed read-only operations.
 // Never proxy an HTTP handler: such a handler trusts web cookies/CSRF state,
 // not this in-process E2EE principal. Execute through the same local
 // TaskDeck RBAC/revocation/cancellation boundary as session.ping.
 var content []byte
 permission:=identity.PermissionTasksView
 auditAction:="session_ping"
 var run func(context.Context)error=func(c context.Context)error{return c.Err()}
 switch request.Operation {
 case remotetransport.RemoteActionSessionPing:
 case remotetransport.RemoteActionProjectFileRead:
  permission=identity.PermissionFilesRead
  auditAction="project_file_read"
  run=func(actionCtx context.Context)error{
   if actionCtx.Err()!=nil{return remotetransport.ErrRemoteActionDenied}
   relative,pathErr:=cleanProjectRelativePath(request.Path,false)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   path,pathErr:=s.resolveProjectPath(relative,false,false)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   rootView,_,pathErr:=s.projectRootForVirtualPath(relative)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   pinned,pathErr:=openProjectPinnedFile(rootView.Path,path)
   if pathErr!=nil{return remotetransport.ErrRemoteActionDenied}
   defer pinned.close()
   data,info,readErr:=pinned.readCurrent(192)
   if readErr!=nil || info==nil || !info.Mode().IsRegular() ||
    !projectTextBytesValid(data) || actionCtx.Err()!=nil {
    return remotetransport.ErrRemoteActionDenied
   }
   content=append([]byte(nil),data...)
   return actionCtx.Err()
  }
 default:
  return nil,remotetransport.ErrRemoteActionDenied
 }
 defer func(){for i:=range content{content[i]=0}}()
 err=s.executeRemoteAuthenticatedAction(ctx,session,channel,permission,run)
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 // Local audit must succeed BEFORE any confidential file data leaves host.
 // No content/path is recorded in Portal or the TaskDeck audit details.
 if err:=s.auditRemoteReadOnlyAction(session,auditAction);err!=nil {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Serialize final sealing with local session Close, which cancels active
 // work and destroys the Channel. The outer Agent must still check grant
 // liveness immediately before forwarding the encrypted response.
 session.executionMu.Lock()
 defer session.executionMu.Unlock()
 if session.closed.Load() || !session.portalGrant.ActiveForSession(channel) ||
  ctx.Err()!=nil {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 var sealed []byte
 if request.Operation==remotetransport.RemoteActionProjectFileRead{
  sealed,err=remotetransport.SealRemoteFileReadResult(channel,request.RequestID,content)
 } else {
  sealed,err=remotetransport.SealRemoteActionPingResult(channel,request.RequestID)
 }
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 return sealed,nil
}
