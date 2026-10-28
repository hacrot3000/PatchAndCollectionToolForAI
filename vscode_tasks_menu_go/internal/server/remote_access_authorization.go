package server

import (
 "net/http"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// authorizeRemoteTransportAction is an internal, fail-closed authorization
// boundary for a FUTURE encrypted TaskDeck request dispatcher.
// This does not mount a remote HTTP endpoint or authorize a Portal ticket.
//
// Shared-server authentication and project-scoped RBAC must produce an actual
// TaskDeck principal before a single application action can be attempted.
// Single-password legacy mode deliberately remains unimplemented for remote
// commands until it has an equally strong principal/session boundary.
func (s *Server) authorizeRemoteTransportAction(r *http.Request, portalRole remotetransport.Role, permission string) error {
 if s==nil || r==nil || !s.Config.RemoteAccessEnabled ||
  !s.Config.SharedServerEnabled || s.Config.SharedProjectID=="" ||
  r.TLS==nil {
  return remotetransport.ErrRemoteActionDenied
 }
 principal,ok:=PrincipalFromContext(r.Context())
 if !ok {
  return remotetransport.ErrRemoteActionDenied
 }
 return remotetransport.AuthorizeAction(true,portalRole,
  &principal,s.Config.SharedProjectID,permission)
}
