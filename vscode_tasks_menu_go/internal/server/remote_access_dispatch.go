package server

import (
 "context"
 "os"
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
    operation!=remotetransport.RemoteActionSessionFeatures &&
    operation!=remotetransport.RemoteActionGitStatus &&
    operation!=remotetransport.RemoteActionProjectInfo &&
    operation!=remotetransport.RemoteActionTasksPreview &&
    operation!=remotetransport.RemoteActionTasksPreviewNext &&
    operation!=remotetransport.RemoteActionTasksInspect {
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
 if operation==remotetransport.RemoteActionTasksPreview || operation==remotetransport.RemoteActionTasksPreviewNext || operation==remotetransport.RemoteActionTasksInspect {resourceType="tasks"}
 if operation==remotetransport.RemoteActionSessionFeatures {resourceType="session"}
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

// executeRemoteE2EEPing admits explicitly enumerated, bounded encrypted
// read actions plus independently host-opted-in tasks.run by Task.ID only.
// Git reads only the host TaskDeck workspace root; no arbitrary HTTP/shell,
// PTY, file or Git write dispatcher is exposed.
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
 // Page cursor is host-owned, and serialized across authenticated task
 // requests; Close() never takes taskPageMu and remains nonblocking.
 paging:=request.Operation==remotetransport.RemoteActionTasksPreview ||
  request.Operation==remotetransport.RemoteActionTasksPreviewNext
 // Inspection consumes an immutable host-issued task ID, protected by
 // the same in-memory per-session snapshot lock as the page cursor.
 if paging || request.Operation==remotetransport.RemoteActionTasksInspect ||
  request.Operation==remotetransport.RemoteActionTasksRun {
  session.taskPageMu.Lock()
  defer session.taskPageMu.Unlock()
 }
 var permission string
 switch request.Operation {
 case remotetransport.RemoteActionSessionPing:
  permission=identity.PermissionTasksView
 case remotetransport.RemoteActionSessionFeatures:
  // A restricted TaskDeck role may legitimately hold git.status WITHOUT
  // tasks.view. Pick one of those two fixed read permissions from the
  // current host-local principal. executeRemoteAuthenticatedAction then
  // RECHECKS the chosen permission and live Portal grant before/after
  // reading; this snapshot cannot grant extra privileges.
  principal,principalErr:=s.verifyRemoteSessionLocalPrincipal(ctx,session)
  if principalErr!=nil {
   return nil,remotetransport.ErrRemoteActionDenied
  }
  permission=identity.PermissionTasksView
  if remotetransport.AuthorizeAction(true,session.role,
   &principal,session.projectKey,permission)!=nil {
   permission=identity.PermissionGitStatus
  }
 case remotetransport.RemoteActionGitStatus:
  permission=identity.PermissionGitStatus
 case remotetransport.RemoteActionProjectInfo:
  permission=identity.PermissionTasksView
 case remotetransport.RemoteActionTasksPreview,remotetransport.RemoteActionTasksPreviewNext,
  remotetransport.RemoteActionTasksInspect:
  permission=identity.PermissionTasksView
 case remotetransport.RemoteActionTasksRun:
  permission=identity.PermissionTasksRun
 default:
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Never authorize based on a Portal role alone. Every call performs
 // fresh local TaskDeck RBAC + client-key + live Portal checks, before and
 // after the bounded callback. No remote-supplied Git path is accepted.
 var status string
 var projectInfo remotetransport.RemoteProjectInfo
 var tasksPreview remotetransport.RemoteTasksPreview
 var inspected remotetransport.RemoteTaskInspect
 var taskRun remotetransport.RemoteTaskRun
 var nextTaskCursor int
 var taskPageHash [32]byte
 var features remotetransport.RemoteFeatures
 err=s.executeRemoteAuthenticatedAction(ctx,session,channel,
  permission,func(actionCtx context.Context)error {
   switch request.Operation{
   case remotetransport.RemoteActionSessionPing:
    return actionCtx.Err()
   case remotetransport.RemoteActionSessionFeatures:
    var featuresErr error
    features,featuresErr=s.readRemoteFeatures(actionCtx,session)
    return featuresErr
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
    tasksPreview,nextTaskCursor,taskPageHash,previewErr=
     s.readRemoteTaskPage(actionCtx,session,0,nil,request.RequestID)
    return previewErr
   case remotetransport.RemoteActionTasksPreviewNext:
    if !session.taskPageReady{
     return remotetransport.ErrRemoteActionDenied
    }
    var previewErr error
    tasksPreview,nextTaskCursor,taskPageHash,previewErr=
     s.readRemoteTaskPage(actionCtx,session,session.taskPageCursor,
      &session.taskPageHash,request.RequestID)
    return previewErr
   case remotetransport.RemoteActionTasksRun:
    var runErr error
    taskRun,runErr=s.runRemoteTaskBounded(actionCtx,session,request.TaskID,request.Choice)
    return runErr
   case remotetransport.RemoteActionTasksInspect:
    // Display labels alone cannot select a task: this ID must have been
    // emitted by this very authenticated session and the host-owned task
    // source must remain byte-for-byte the same.
    if os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1" ||
     !session.taskPageReady || session.taskIssuedIDs==nil ||
     !session.taskIssuedIDs[request.TaskID] {
     return remotetransport.ErrRemoteActionDenied
    }
    var inspectErr error
    inspected,inspectErr=inspectRemoteTaskSelection(
     actionCtx,s.Workspace,session.taskPageHash,request.TaskID)
    if inspectErr!=nil || inspected.TaskID!=request.TaskID {
     return remotetransport.ErrRemoteActionDenied
    }
    return nil
   default:
    return remotetransport.ErrRemoteActionDenied
   }
  })
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 // Audit can block on storage. It MUST NOT hold executionMu: local
 // Close() must promptly revoke a session even when its database is slow.
 // The event is "authorized", not "successfully delivered": revocation
 // may occur before the final encrypted response reaches its recipient.
 if request.Operation!=remotetransport.RemoteActionTasksRun {
  if err:=s.auditRemoteReadOnlyAction(session,request.Operation);err!=nil {
   return nil,remotetransport.ErrRemoteActionDenied
  }
 } // Task runner wrote its durable attempted audit before starting.
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
 case remotetransport.RemoteActionSessionFeatures:
  sealed,err=remotetransport.SealRemoteActionFeaturesResult(channel,request.RequestID,features)
 case remotetransport.RemoteActionGitStatus:
  sealed,err=remotetransport.SealRemoteActionGitStatusResult(channel,request.RequestID,status)
 case remotetransport.RemoteActionProjectInfo:
  sealed,err=remotetransport.SealRemoteActionProjectInfoResult(channel,request.RequestID,projectInfo)
 case remotetransport.RemoteActionTasksPreview,remotetransport.RemoteActionTasksPreviewNext:
  sealed,err=remotetransport.SealRemoteActionTasksPreviewResult(channel,request.RequestID,tasksPreview)
 case remotetransport.RemoteActionTasksInspect:
  sealed,err=remotetransport.SealRemoteActionTasksInspectResult(channel,request.RequestID,inspected)
 case remotetransport.RemoteActionTasksRun:
  sealed,err=remotetransport.SealRemoteActionTaskRunResult(channel,request.RequestID,taskRun)
 default:
  return nil,remotetransport.ErrRemoteActionDenied
 }
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 if paging {
  // Only commit host-owned cursor and published selection IDs after the
  // authenticated E2EE response has been successfully sealed.
  if request.Operation==remotetransport.RemoteActionTasksPreview {
   session.taskIssuedIDs=make(map[int]bool)
  }
  if session.taskIssuedIDs==nil{session.taskIssuedIDs=make(map[int]bool)}
  for _,taskID:=range tasksPreview.IDs{
   if taskID>0 {session.taskIssuedIDs[taskID]=true}
  }
  session.taskPageHash=taskPageHash
  session.taskPageCursor=nextTaskCursor
  session.taskPageReady=true
 }
 return sealed,nil
}
