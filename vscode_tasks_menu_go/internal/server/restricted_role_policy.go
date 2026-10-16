package server

import (
 "net/http"

 "bletonfc/vscode_tasks_menu/internal/restrictedpolicy"
)

// The CLI restricted launch remains global and immutable. In normal shared
// server mode only members assigned to the reserved restricted-developer role
// get the same fail-closed gate and server-owned default quota policy.
// No web settings, custom role name or client-supplied request field may
// activate/deactivate this security boundary.
func (s *Server) restrictedPolicyForRequest(r *http.Request) *restrictedpolicy.Policy {
 if s.Config.Restricted != nil { return s.Config.Restricted }
 if !s.Config.SharedServerEnabled { return nil }
 principal,ok:=PrincipalFromContext(r.Context())
 if !ok||!principal.Restricted { return nil }
 policy:=restrictedpolicy.Default()
 return &policy
}
