package server

import (
 "context"
 "os"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// readRemoteFeatures is only an advisory snapshot for the native GUI.
// It must never be used in place of the final per-action TaskDeck
// password/key/RBAC/grant and host-opt-in checks.
func (s *Server) readRemoteFeatures(
 ctx context.Context,session *remoteTaskDeckSession,
)(remotetransport.RemoteFeatures,error){
 var denied remotetransport.RemoteFeatures
 if s==nil || ctx==nil || ctx.Err()!=nil || session==nil ||
  s.Identity==nil || session.projectKey=="" ||
  session.projectKey!=s.Config.SharedProjectID {
  return denied,remotetransport.ErrRemoteActionDenied
 }
 principal,err:=s.verifyRemoteSessionLocalPrincipal(ctx,session)
 if err!=nil || ctx.Err()!=nil{
  return denied,remotetransport.ErrRemoteActionDenied
 }
 allowed:=func(permission string) bool {
  return remotetransport.AuthorizeAction(true,session.role,
   &principal,session.projectKey,permission)==nil
 }
 view:=allowed(identity.PermissionTasksView)
 git:=allowed(identity.PermissionGitStatus)
 return remotetransport.RemoteFeatures{
  SessionPing:view,
  ProjectInfo:view && os.Getenv("TASKDECK_REMOTE_PROJECT_INFO")=="1",
  TasksPreview:view && os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")=="1",
  TaskRun:view && allowed(identity.PermissionTasksRun) &&
   os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")=="1" &&
   os.Getenv("TASKDECK_REMOTE_TASKS_RUN")=="1",
  TaskLong:view && allowed(identity.PermissionTasksRun) &&
   os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")=="1"&&
   os.Getenv("TASKDECK_REMOTE_TASKS_RUN")=="1"&&
   os.Getenv("TASKDECK_REMOTE_TASKS_LONG")=="1",
  GitStatus:git && os.Getenv("TASKDECK_REMOTE_GIT_STATUS")=="1",
  GitLog:allowed(identity.PermissionGitLog) && os.Getenv("TASKDECK_REMOTE_GIT_LOG")=="1",
  GitDiff:allowed(identity.PermissionGitDiff) && os.Getenv("TASKDECK_REMOTE_GIT_DIFF")=="1",
  Terminal:allowed(identity.PermissionTerminalCreate) &&
   allowed(identity.PermissionTerminalViewOwn) &&
   allowed(identity.PermissionTerminalControlOwn) &&
   os.Getenv("TASKDECK_REMOTE_TERMINAL")=="1",
  GitPull:allowed(identity.PermissionGitWrite) && os.Getenv("TASKDECK_REMOTE_GIT_PULL")=="1",
  GitFetch:allowed(identity.PermissionGitWrite) && os.Getenv("TASKDECK_REMOTE_GIT_FETCH")=="1",
  GitStageTracked:allowed(identity.PermissionGitWrite) && os.Getenv("TASKDECK_REMOTE_GIT_STAGE_TRACKED")=="1",
  GitUnstageAll:allowed(identity.PermissionGitWrite) && os.Getenv("TASKDECK_REMOTE_GIT_UNSTAGE_ALL")=="1",
  GitCommit:allowed(identity.PermissionGitWrite) && os.Getenv("TASKDECK_REMOTE_GIT_COMMIT")=="1",
  GitPush:allowed(identity.PermissionGitWrite) && allowed(identity.PermissionGitPush) &&
   os.Getenv("TASKDECK_REMOTE_GIT_PUSH")=="1",
 },nil
}
