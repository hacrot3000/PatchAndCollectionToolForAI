package server

import (
 "context"
 "time"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A compromised PHP Portal cannot erase TaskDeck-local identity audit.
// Record locally authorized action attempts using the real TaskDeck principal,
// without request IDs, plaintext credentials, Portal tickets or ciphertext.
// A failed audit is a hard refusal; do not produce an encrypted "success".
func (s *Server) auditRemoteReadOnlyAction(session *remoteTaskDeckSession, operation string) error {
 if operation!=remotetransport.RemoteActionSessionPing &&
    operation!=remotetransport.RemoteActionGitStatus &&
    operation!=remotetransport.RemoteActionProjectInfo &&
    operation!=remotetransport.RemoteActionTasksPreview {
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
 resourceType:="session"
 if operation==remotetransport.RemoteActionGitStatus {resourceType="git"}
 if operation==remotetransport.RemoteActionProjectInfo {resourceType="project"}
 if operation==remotetransport.RemoteActionTasksPreview {resourceType="tasks"}
 err=s.Identity.AppendAudit(ctx,identity.AuditEvent{
  ID:eventID,Timestamp:time.Now().UTC(),
  UserID:&userID,ProjectID:&projectID,
  Action:"remote.action."+strings.ReplaceAll(operation,".","_"),ResourceType:resourceType,
  Result:"authorized",
  Details:`{"transport":"e2ee","taskdeck_auth":true,"remote_control":false}`,
 })
 if err!=nil{return remotetransport.ErrRemoteActionDenied}
 return nil
}

// executeRemoteE2EEPing admits only the reviewed, bounded encrypted
// session.ping, git.status, project.info and tasks.preview operations. Git reads only
// the exact host TaskDeck workspace root; project.info exposes short
// TaskDeck-owned metadata, never the full host path. No arbitrary HTTP/shell,
// PTY, file or TaskDeck command dispatcher here.
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
 var permission string
 switch request.Operation {
 case remotetransport.RemoteActionSessionPing:
  permission=identity.PermissionTasksView
 case remotetransport.RemoteActionGitStatus:
  permission=identity.PermissionGitStatus
 case remotetransport.RemoteActionProjectInfo:
  permission=identity.PermissionTasksView
 case remotetransport.RemoteActionTasksPreview:
  permission=identity.PermissionTasksView
 default:
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Never authorize based on a Portal role alone. Every call performs
 // fresh local TaskDeck RBAC + client-key + live Portal checks, before and
 // after the bounded callback. No remote-supplied Git path is accepted.
 var status string
 var projectInfo remotetransport.RemoteProjectInfo
 var tasksPreview remotetransport.RemoteTasksPreview
 err=s.executeRemoteAuthenticatedAction(ctx,session,channel,
  permission,func(actionCtx context.Context)error {
   switch request.Operation{
   case remotetransport.RemoteActionSessionPing:
    return actionCtx.Err()
   case remotetransport.RemoteActionGitStatus:
    var gitErr error
    status,gitErr=s.readRemoteGitStatus(actionCtx)
    return gitErr
   case remotetransport.RemoteActionProjectInfo:
    var projectErr error
    projectInfo,projectErr=s.readRemoteProjectInfo(actionCtx,session)
    return projectErr
   case remotetransport.RemoteActionTasksPreview:
    var previewErr error
    tasksPreview,previewErr=s.readRemoteTasksPreview(actionCtx,session)
    return previewErr
   default:
    return remotetransport.ErrRemoteActionDenied
   }
  })
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 // Audit can block on storage. It MUST NOT hold executionMu: local
 // Close() must promptly revoke a session even when its database is slow.
 // The event is "authorized", not "successfully delivered": revocation
 // may occur before the final encrypted response reaches its recipient.
 if err:=s.auditRemoteReadOnlyAction(session,request.Operation);err!=nil {
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
 switch request.Operation{
 case remotetransport.RemoteActionSessionPing:
  sealed,err=remotetransport.SealRemoteActionPingResult(channel,request.RequestID)
 case remotetransport.RemoteActionGitStatus:
  sealed,err=remotetransport.SealRemoteActionGitStatusResult(channel,request.RequestID,status)
 case remotetransport.RemoteActionProjectInfo:
  sealed,err=remotetransport.SealRemoteActionProjectInfoResult(channel,request.RequestID,projectInfo)
 case remotetransport.RemoteActionTasksPreview:
  sealed,err=remotetransport.SealRemoteActionTasksPreviewResult(channel,request.RequestID,tasksPreview)
 default:
  return nil,remotetransport.ErrRemoteActionDenied
 }
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 return sealed,nil
}
