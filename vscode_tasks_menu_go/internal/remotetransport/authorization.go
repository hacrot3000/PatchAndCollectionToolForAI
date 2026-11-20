// Package remotetransport contains isolated encrypted transport and explicit
// remote policy gates. A Portal grant NEVER authenticates a TaskDeck user.
package remotetransport

import (
 "errors"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

// Role is the maximum authority granted by Portal for one TaskDeck instance.
// It is always intersected with the local TaskDeck principal and its project.
type Role string

const (
 RoleViewer Role = "viewer"
 RoleOperator Role = "operator"
 RoleAdmin Role = "admin"
)

var ErrRemoteActionDenied = errors.New("remote TaskDeck action denied")

// RemotePermissions is deliberately conservative. Portal "operator" does
// not automatically get terminal, file mutation, settings, Git push or Patch
// execution merely because TaskDeck permits those locally.
var viewerPermissions = map[string]bool{
 identity.PermissionTasksView:true,
 identity.PermissionPatchView:true,
 identity.PermissionPatchHistory:true,
 identity.PermissionFilesRead:true,
 identity.PermissionFilesDownload:true,
 identity.PermissionGitStatus:true,
 identity.PermissionGitLog:true,
 identity.PermissionGitDiff:true,
}
var operatorPermissions = map[string]bool{
 identity.PermissionTasksRun:true,
 identity.PermissionPatchCollect:true,
 identity.PermissionSchedulesView:true,
}

// AuthorizeAction may only be called after a separate, verified TaskDeck
// login. The Portal role by itself is NEVER sufficient. The remote session
// must match the exact TaskDeck project and an actual TaskDeck permission.
// When the authenticated TaskDeck session is absent, this fails closed.
func AuthorizeAction(remoteEnabled bool, portalRole Role, principal *identity.Principal,
 expectedProjectKey, permission string) error {
 if !remoteEnabled || principal==nil || principal.UserID=="" ||
  expectedProjectKey=="" || principal.ProjectKey!=expectedProjectKey ||
  !identity.KnownPermission(permission) || !principal.Allowed(permission) {
  return ErrRemoteActionDenied
 }
 switch portalRole {
 case RoleViewer:
  if viewerPermissions[permission] {return nil}
 case RoleOperator:
  if viewerPermissions[permission] || operatorPermissions[permission] {return nil}
 case RoleAdmin:
  // Admin here means no *additional* Portal scope restriction. It does
  // NOT grant a TaskDeck permission: principal.Allowed() already checked.
  return nil
 }
 return ErrRemoteActionDenied
}
