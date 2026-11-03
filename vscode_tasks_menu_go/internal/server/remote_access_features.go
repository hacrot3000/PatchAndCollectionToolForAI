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
  ProjectInfo:view && os.Getenv("TASKDECK_REMOTE_PROJECT_INFO")=="1",
  TasksPreview:view && os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")=="1",
  GitStatus:git && os.Getenv("TASKDECK_REMOTE_GIT_STATUS")=="1",
 },nil
}
