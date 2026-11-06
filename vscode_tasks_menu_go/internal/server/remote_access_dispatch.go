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
    operation!=remotetransport.RemoteActionGitLog &&
    operation!=remotetransport.RemoteActionGitBranches &&
    operation!=remotetransport.RemoteActionGitRemotes &&
    operation!=remotetransport.RemoteActionGitTrackingStart &&
    operation!=remotetransport.RemoteActionGitTrackingNext &&
    operation!=remotetransport.RemoteActionGitPullReview &&
    operation!=remotetransport.RemoteActionGitDiff &&
    operation!=remotetransport.RemoteActionGitPatchStart &&
    operation!=remotetransport.RemoteActionGitPatchNext &&
    operation!=remotetransport.RemoteActionProjectInfo &&
    operation!=remotetransport.RemoteActionTasksPreview &&
    operation!=remotetransport.RemoteActionTasksPreviewNext &&
    operation!=remotetransport.RemoteActionTasksInspect &&
    operation!=remotetransport.RemoteActionTasksPoll &&
    operation!=remotetransport.RemoteActionTasksClear &&
    operation!=remotetransport.RemoteActionTerminalPoll {
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
 if operation==remotetransport.RemoteActionGitStatus||operation==remotetransport.RemoteActionGitLog||operation==remotetransport.RemoteActionGitBranches||operation==remotetransport.RemoteActionGitRemotes||operation==remotetransport.RemoteActionGitTrackingStart||operation==remotetransport.RemoteActionGitTrackingNext||operation==remotetransport.RemoteActionGitPullReview||operation==remotetransport.RemoteActionGitDiff||operation==remotetransport.RemoteActionGitPatchStart||operation==remotetransport.RemoteActionGitPatchNext {resourceType="git"}
 if operation==remotetransport.RemoteActionProjectInfo {resourceType="project"}
 if operation==remotetransport.RemoteActionTasksPreview || operation==remotetransport.RemoteActionTasksPreviewNext || operation==remotetransport.RemoteActionTasksInspect ||
  operation==remotetransport.RemoteActionTasksPoll || operation==remotetransport.RemoteActionTasksClear {resourceType="tasks"}
 if operation==remotetransport.RemoteActionSessionFeatures {resourceType="session"}
 if operation==remotetransport.RemoteActionTerminalPoll {resourceType="terminal"}
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
  request.Operation==remotetransport.RemoteActionTasksRun ||
  request.Operation==remotetransport.RemoteActionTasksStart ||
  request.Operation==remotetransport.RemoteActionTasksPoll ||
  request.Operation==remotetransport.RemoteActionTasksCancel ||
  request.Operation==remotetransport.RemoteActionTasksClear {
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
   if remotetransport.AuthorizeAction(true,session.role,
    &principal,session.projectKey,permission)!=nil {
    permission=identity.PermissionGitLog
    if remotetransport.AuthorizeAction(true,session.role,
     &principal,session.projectKey,permission)!=nil {
     permission=identity.PermissionGitDiff
     if remotetransport.AuthorizeAction(true,session.role,
      &principal,session.projectKey,permission)!=nil {
      permission=identity.PermissionGitWrite
      if remotetransport.AuthorizeAction(true,session.role,
       &principal,session.projectKey,permission)!=nil {
       permission=identity.PermissionTerminalCreate
      }
     }
    }
   }
  }
 case remotetransport.RemoteActionTerminalOpen:
  permission=identity.PermissionTerminalCreate
 case remotetransport.RemoteActionTerminalPoll:
  permission=identity.PermissionTerminalViewOwn
 case remotetransport.RemoteActionTerminalWrite,remotetransport.RemoteActionTerminalResize,
  remotetransport.RemoteActionTerminalClose:
  permission=identity.PermissionTerminalControlOwn
 case remotetransport.RemoteActionGitStatus:
  permission=identity.PermissionGitStatus
 case remotetransport.RemoteActionGitLog:
  permission=identity.PermissionGitLog
 case remotetransport.RemoteActionGitBranches,remotetransport.RemoteActionGitRemotes,
  remotetransport.RemoteActionGitPullReview,
  remotetransport.RemoteActionGitTrackingStart,remotetransport.RemoteActionGitTrackingNext:
  permission=identity.PermissionGitStatus
 case remotetransport.RemoteActionGitBranchCreate:
  permission=identity.PermissionGitWrite
 case remotetransport.RemoteActionGitDiff,
  remotetransport.RemoteActionGitPatchStart,remotetransport.RemoteActionGitPatchNext:
  permission=identity.PermissionGitDiff
 case remotetransport.RemoteActionGitPull,remotetransport.RemoteActionGitCommit,
  remotetransport.RemoteActionGitFetch,
  remotetransport.RemoteActionGitStageTracked,remotetransport.RemoteActionGitUnstageAll,
  remotetransport.RemoteActionGitStageFile,remotetransport.RemoteActionGitUnstageFile:
  permission=identity.PermissionGitWrite
 case remotetransport.RemoteActionGitPush:
  permission=identity.PermissionGitPush
 case remotetransport.RemoteActionProjectInfo:
  permission=identity.PermissionTasksView
 case remotetransport.RemoteActionTasksPreview,remotetransport.RemoteActionTasksPreviewNext,
  remotetransport.RemoteActionTasksInspect:
  permission=identity.PermissionTasksView
 case remotetransport.RemoteActionTasksRun,remotetransport.RemoteActionTasksStart,
  remotetransport.RemoteActionTasksPoll,remotetransport.RemoteActionTasksCancel,
  remotetransport.RemoteActionTasksClear:
  permission=identity.PermissionTasksRun
 default:
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Never authorize based on a Portal role alone. Every call performs
 // fresh local TaskDeck RBAC + client-key + live Portal checks, before and
 // after the bounded callback. No remote-supplied Git path is accepted.
 var status string
 var gitLog string
 var gitBranches string
 var gitRemotes string
 var gitTracking remotetransport.RemoteGitTrackingPage
 var gitBranchCreate remotetransport.RemoteGitBranchCreateResult
 var gitDiff string
 var gitPatch remotetransport.RemoteGitPatchPage
 var gitPull remotetransport.RemoteGitPullResult
 var gitPullReview remotetransport.RemoteGitPullReviewResult
 var gitCommit remotetransport.RemoteGitCommitResult
 var gitPush remotetransport.RemoteGitPushResult
 var gitFetch remotetransport.RemoteGitFetchResult
 var gitIndex remotetransport.RemoteGitIndexResult
 var terminal remotetransport.RemoteTerminalProgress
 startedTerminal:=false
 var projectInfo remotetransport.RemoteProjectInfo
 var tasksPreview remotetransport.RemoteTasksPreview
 var inspected remotetransport.RemoteTaskInspect
 var taskRun remotetransport.RemoteTaskRun
 var longTask remotetransport.RemoteLongTaskProgress
 startedLongTask:=false
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
   case remotetransport.RemoteActionTerminalOpen:
    if err:=s.startRemoteTerminal(actionCtx,session);err!=nil{return err}
    startedTerminal=true
    terminal=remotetransport.RemoteTerminalProgress{State:"running"}
    return nil
   case remotetransport.RemoteActionTerminalPoll:
    if os.Getenv("TASKDECK_REMOTE_TERMINAL")!="1"{
     return remotetransport.ErrRemoteActionDenied
    }
    if err:=s.auditRemoteReadOnlyAction(session,request.Operation);err!=nil{return err}
    var termErr error
    terminal,termErr=session.pollRemoteTerminal(actionCtx)
    return termErr
   case remotetransport.RemoteActionTerminalWrite:
    var termErr error
    terminal,termErr=s.writeRemoteTerminal(actionCtx,session,request.TerminalInput)
    return termErr
   case remotetransport.RemoteActionTerminalResize:
    var termErr error
    terminal,termErr=s.resizeRemoteTerminal(actionCtx,session,request.Cols,request.Rows)
    return termErr
   case remotetransport.RemoteActionTerminalClose:
    var termErr error
    terminal,termErr=s.closeRemoteTerminal(actionCtx,session)
    return termErr
   case remotetransport.RemoteActionGitStatus:
    var gitErr error
    status,gitErr=s.readRemoteGitStatus(actionCtx)
    return gitErr
   case remotetransport.RemoteActionGitLog:
    var logErr error
    gitLog,logErr=s.readRemoteGitLog(actionCtx)
    return logErr
   case remotetransport.RemoteActionGitBranches:
    var branchesErr error
    gitBranches,branchesErr=s.readRemoteGitBranches(actionCtx)
    return branchesErr
   case remotetransport.RemoteActionGitRemotes:
    var remotesErr error
    gitRemotes,remotesErr=s.readRemoteGitRemotes(actionCtx)
    return remotesErr
   case remotetransport.RemoteActionGitTrackingStart,remotetransport.RemoteActionGitTrackingNext:
    var trackingErr error
    gitTracking,trackingErr=s.readRemoteGitTrackingPage(actionCtx,session,
     request.Operation==remotetransport.RemoteActionGitTrackingStart,request.RequestID)
    return trackingErr
   case remotetransport.RemoteActionGitBranchCreate:
    var branchErr error
    gitBranchCreate,branchErr=s.executeRemoteGitBranchCreate(actionCtx,session)
    return branchErr
   case remotetransport.RemoteActionGitDiff:
    var diffErr error
    gitDiff,diffErr=s.readRemoteGitDiff(actionCtx)
    return diffErr
   case remotetransport.RemoteActionGitPatchStart,remotetransport.RemoteActionGitPatchNext:
    var patchErr error
    gitPatch,patchErr=s.readRemoteGitPatchPage(actionCtx,session,
     request.Operation==remotetransport.RemoteActionGitPatchStart,request.RequestID)
    return patchErr
   case remotetransport.RemoteActionGitPullReview:
    var reviewErr error
    gitPullReview,reviewErr=s.readRemoteGitPullReview(actionCtx)
    return reviewErr
   case remotetransport.RemoteActionGitPull:
    var pullErr error
    gitPull,pullErr=s.executeRemoteGitPull(actionCtx,session)
    return pullErr
   case remotetransport.RemoteActionGitCommit:
    var commitErr error
    gitCommit,commitErr=s.executeRemoteGitCommit(actionCtx,session,request.Message)
    return commitErr
   case remotetransport.RemoteActionGitPush:
    var pushErr error
    gitPush,pushErr=s.executeRemoteGitPush(actionCtx,session)
    return pushErr
   case remotetransport.RemoteActionGitFetch:
    var fetchErr error
    gitFetch,fetchErr=s.executeRemoteGitFetch(actionCtx,session)
    return fetchErr
   case remotetransport.RemoteActionGitStageTracked,remotetransport.RemoteActionGitUnstageAll:
    var indexErr error
    gitIndex,indexErr=s.executeRemoteGitIndex(actionCtx,session,request.Operation)
    return indexErr
   case remotetransport.RemoteActionGitStageFile,remotetransport.RemoteActionGitUnstageFile:
    var fileErr error
    gitIndex,fileErr=s.executeRemoteGitFileIndex(actionCtx,session,
     request.Operation,request.FileIndex,request.Snapshot)
    return fileErr
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
   case remotetransport.RemoteActionTasksStart:
    startErr:=s.startRemoteLongTask(actionCtx,session,request.TaskID,request.Choice,request.Text)
    if startErr!=nil{return startErr}
    startedLongTask=true
    longTask=remotetransport.RemoteLongTaskProgress{
     TaskID:request.TaskID,Status:"started",ExitCode:-1,
    }
    return nil
   case remotetransport.RemoteActionTasksPoll:
    if os.Getenv("TASKDECK_REMOTE_TASKS_LONG")!="1"{
     return remotetransport.ErrRemoteActionDenied
    }
    // Audit before advancing this session's incremental output cursor.
    if err:=s.auditRemoteReadOnlyAction(session,request.Operation);err!=nil{return err}
    progress,pollErr:=session.pollRemoteLongTask(actionCtx,request.TaskID)
    if pollErr!=nil{return pollErr}
    longTask=remotetransport.RemoteLongTaskProgress{
     TaskID:progress.TaskID,Status:progress.Status,ExitCode:progress.ExitCode,
     Output:progress.Output,Truncated:progress.Truncated,More:progress.More,
    }
    return nil
   case remotetransport.RemoteActionTasksCancel:
    if os.Getenv("TASKDECK_REMOTE_TASKS_LONG")!="1"{
     return remotetransport.ErrRemoteActionDenied
    }
    if err:=s.auditRemoteLongTaskMutation(session,request.TaskID,"cancel");err!=nil{
     return remotetransport.ErrRemoteActionDenied
    }
    if err:=session.cancelRemoteLongTask(request.TaskID);err!=nil{return err}
    longTask=remotetransport.RemoteLongTaskProgress{
     TaskID:request.TaskID,Status:"cancel_requested",ExitCode:-1,
    }
    return nil
   case remotetransport.RemoteActionTasksClear:
    if os.Getenv("TASKDECK_REMOTE_TASKS_LONG")!="1"{
     return remotetransport.ErrRemoteActionDenied
    }
    if err:=s.auditRemoteLongTaskMutation(session,request.TaskID,"clear");err!=nil{
     return err
    }
    if err:=session.clearRemoteLongTask(request.TaskID);err!=nil{return err}
    longTask=remotetransport.RemoteLongTaskProgress{
     TaskID:request.TaskID,Status:"cleared",ExitCode:-1,
    }
    return nil
   case remotetransport.RemoteActionTasksRun:
    var runErr error
    taskRun,runErr=s.runRemoteTaskBoundedWithInput(actionCtx,session,request.TaskID,request.Choice,request.Text)
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
 if err!=nil{
  if startedLongTask{_ = session.cancelRemoteLongTask(request.TaskID)}
  if startedTerminal{session.stopRemoteTerminal()}
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Audit can block on storage. It MUST NOT hold executionMu: local
 // Close() must promptly revoke a session even when its database is slow.
 // The event is "authorized", not "successfully delivered": revocation
 // may occur before the final encrypted response reaches its recipient.
 if request.Operation!=remotetransport.RemoteActionTerminalOpen &&
  request.Operation!=remotetransport.RemoteActionTerminalPoll &&
  request.Operation!=remotetransport.RemoteActionTerminalWrite &&
  request.Operation!=remotetransport.RemoteActionTerminalResize &&
  request.Operation!=remotetransport.RemoteActionTerminalClose &&
  request.Operation!=remotetransport.RemoteActionGitPull &&
  request.Operation!=remotetransport.RemoteActionGitCommit &&
  request.Operation!=remotetransport.RemoteActionGitPush &&
  request.Operation!=remotetransport.RemoteActionGitFetch &&
  request.Operation!=remotetransport.RemoteActionGitBranchCreate &&
  request.Operation!=remotetransport.RemoteActionGitStageTracked &&
  request.Operation!=remotetransport.RemoteActionGitUnstageAll &&
  request.Operation!=remotetransport.RemoteActionGitStageFile &&
  request.Operation!=remotetransport.RemoteActionGitUnstageFile &&
  request.Operation!=remotetransport.RemoteActionTasksRun &&
  request.Operation!=remotetransport.RemoteActionTasksStart &&
  request.Operation!=remotetransport.RemoteActionTasksCancel &&
  request.Operation!=remotetransport.RemoteActionTasksPoll &&
  request.Operation!=remotetransport.RemoteActionTasksClear {
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
 case remotetransport.RemoteActionTerminalOpen,remotetransport.RemoteActionTerminalPoll,
  remotetransport.RemoteActionTerminalWrite,remotetransport.RemoteActionTerminalResize,
  remotetransport.RemoteActionTerminalClose:
  sealed,err=remotetransport.SealRemoteTerminalProgress(channel,request.RequestID,terminal)
 case remotetransport.RemoteActionGitStatus:
  sealed,err=remotetransport.SealRemoteActionGitStatusResult(channel,request.RequestID,status)
 case remotetransport.RemoteActionGitLog:
  sealed,err=remotetransport.SealRemoteGitLogResult(channel,request.RequestID,gitLog)
 case remotetransport.RemoteActionGitBranches:
  sealed,err=remotetransport.SealRemoteGitBranchesResult(channel,request.RequestID,gitBranches)
 case remotetransport.RemoteActionGitRemotes:
  sealed,err=remotetransport.SealRemoteGitRemotesResult(channel,request.RequestID,gitRemotes)
 case remotetransport.RemoteActionGitTrackingStart,remotetransport.RemoteActionGitTrackingNext:
  sealed,err=remotetransport.SealRemoteGitTrackingPage(channel,request.RequestID,gitTracking)
 case remotetransport.RemoteActionGitBranchCreate:
  sealed,err=remotetransport.SealRemoteGitBranchCreateResult(channel,request.RequestID,gitBranchCreate)
 case remotetransport.RemoteActionGitDiff:
  sealed,err=remotetransport.SealRemoteGitDiffResult(channel,request.RequestID,gitDiff)
 case remotetransport.RemoteActionGitPatchStart,remotetransport.RemoteActionGitPatchNext:
  sealed,err=remotetransport.SealRemoteGitPatchPage(channel,request.RequestID,gitPatch)
 case remotetransport.RemoteActionGitPullReview:
  sealed,err=remotetransport.SealRemoteGitPullReviewResult(channel,request.RequestID,gitPullReview)
 case remotetransport.RemoteActionGitPull:
  sealed,err=remotetransport.SealRemoteGitPullResult(channel,request.RequestID,gitPull)
 case remotetransport.RemoteActionGitCommit:
  sealed,err=remotetransport.SealRemoteGitCommitResult(channel,request.RequestID,gitCommit)
 case remotetransport.RemoteActionGitPush:
  sealed,err=remotetransport.SealRemoteGitPushResult(channel,request.RequestID,gitPush)
 case remotetransport.RemoteActionGitFetch:
  sealed,err=remotetransport.SealRemoteGitFetchResult(channel,request.RequestID,gitFetch)
 case remotetransport.RemoteActionGitStageTracked,remotetransport.RemoteActionGitUnstageAll,
  remotetransport.RemoteActionGitStageFile,remotetransport.RemoteActionGitUnstageFile:
  sealed,err=remotetransport.SealRemoteGitIndexResult(channel,request.RequestID,gitIndex)
 case remotetransport.RemoteActionProjectInfo:
  sealed,err=remotetransport.SealRemoteActionProjectInfoResult(channel,request.RequestID,projectInfo)
 case remotetransport.RemoteActionTasksPreview,remotetransport.RemoteActionTasksPreviewNext:
  sealed,err=remotetransport.SealRemoteActionTasksPreviewResult(channel,request.RequestID,tasksPreview)
 case remotetransport.RemoteActionTasksInspect:
  sealed,err=remotetransport.SealRemoteActionTasksInspectResult(channel,request.RequestID,inspected)
 case remotetransport.RemoteActionTasksRun:
  sealed,err=remotetransport.SealRemoteActionTaskRunResult(channel,request.RequestID,taskRun)
 case remotetransport.RemoteActionTasksStart,remotetransport.RemoteActionTasksPoll,
  remotetransport.RemoteActionTasksCancel,remotetransport.RemoteActionTasksClear:
  sealed,err=remotetransport.SealRemoteLongTaskProgress(channel,request.RequestID,longTask)
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
