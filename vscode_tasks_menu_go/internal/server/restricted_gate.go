package server

import (
 "net/http"
 "strings"
)

// restrictedGate is intentionally fail-closed until the guarded command and
// file-window endpoints are implemented and audited. It is independent of
// SharedServerEnabled: single-user must not bypass restricted policy.
//
// IMPORTANT: Never add broad /api/git/, /api/project/ or /api/sessions/
// exceptions here. Each capability needs a separately reviewed endpoint that
// bounds response content and prevents command execution by side channels.
func (s *Server) restrictedGate(next http.Handler) http.Handler {
 if s.Config.Restricted==nil{return next}
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path=="/api/health" && r.Method==http.MethodGet {
   next.ServeHTTP(w,r);return
  }
  if r.URL.Path=="/api/restricted/command" {
   // The command handler performs exact allowlist parsing and output caps.
   // Both shared auth and same-origin mutation middleware still run outside.
   next.ServeHTTP(w,r);return
  }
  if r.URL.Path=="/api/restricted/status" && r.Method==http.MethodGet {
   w.Header().Set("Cache-Control","no-store")
   writeJSON(w,http.StatusOK,map[string]any{
    "enabled":true,
    "ready":false,
    "status":"locked_until_secure_gateways_are_available",
    "max_visible_lines":s.Config.Restricted.MaxVisibleLines,
    "max_copy_lines":s.Config.Restricted.MaxCopyLines,
    "max_copy_bytes":s.Config.Restricted.MaxCopyBytes,
   })
   return
  }
  // Browser assets are application code only; all old dynamic capabilities,
  // including old SSH/SFTP, task, git, file, websocket and export endpoints,
  // are denied until their restricted paths have independent enforcement.
  if !strings.HasPrefix(r.URL.Path,"/api/") && r.URL.Path!="/admin/access" &&
   !strings.HasPrefix(r.URL.Path,"/admin/") &&
   (r.Method==http.MethodGet||r.Method==http.MethodHead) {
   next.ServeHTTP(w,r);return
  }
  w.Header().Set("Cache-Control","no-store")
  http.Error(w,"restricted policy: this capability is unavailable",http.StatusForbidden)
 })
}
