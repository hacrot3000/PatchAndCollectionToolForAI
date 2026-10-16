package server

import (
 "net/http"
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
  if (r.URL.Path=="/"||r.URL.Path=="/index.html")&&r.Method==http.MethodGet {
   s.restrictedPage(w,r);return
  }
  if r.URL.Path=="/api/health" && r.Method==http.MethodGet {
   next.ServeHTTP(w,r);return
  }
  if r.URL.Path=="/api/browser/lease" &&
   (r.Method==http.MethodGet||r.Method==http.MethodPost) {
   // Single-user mutation requests need the existing browser-lease token;
   // keeping this narrow control endpoint available is not permission to
   // restore any legacy file, terminal or workspace mutation route.
   next.ServeHTTP(w,r);return
  }
  if r.Method==http.MethodPost && (r.URL.Path=="/api/restricted/command" || r.URL.Path=="/api/restricted/file-window" || r.URL.Path=="/api/restricted/file-edit" || r.URL.Path=="/api/restricted/compare" || r.URL.Path=="/api/restricted/diff" || r.URL.Path=="/api/restricted/search") {
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
  // The restricted page is entirely self-contained except for its one
  // JavaScript module. Never allow arbitrary GET routes merely because they
  // lack the /api/ prefix: plugins or future endpoints could serve raw files.
  if r.URL.Path=="/featuremods/restricted.js" &&
   (r.Method==http.MethodGet||r.Method==http.MethodHead){
   next.ServeHTTP(w,r);return
  }
  w.Header().Set("Cache-Control","no-store")
  http.Error(w,"restricted policy: this capability is unavailable",http.StatusForbidden)
 })
}
