package server

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/restrictedpolicy"
)

// restrictedCommand deliberately does NOT create PTYs or launch a shell.
// Its capabilities are limited to audited metadata-only operations.
// All other commands, including source-exposing Git subcommands, are denied.
func (s *Server) restrictedCommand(w http.ResponseWriter,r *http.Request) {
 if s.Config.Restricted==nil||r.Method!=http.MethodPost {
  http.Error(w,"restricted command unavailable",http.StatusForbidden);return
 }
 // An authenticated shared user is scoped to a project, not necessarily the
 // workspace root. Until project-root binding is audited, deny this gateway
 // rather than list metadata from another project via the shared workspace.
 if s.Config.SharedServerEnabled {
  http.Error(w,"shared project-bound command gateway is not ready",http.StatusForbidden);return
 }
 if !s.Config.SharedServerEnabled&&!s.Config.AuthEnabled&&!loopbackRemote(r.RemoteAddr) {
  http.Error(w,"restricted command requires loopback or authentication",http.StatusForbidden);return
 }
 if !strings.HasPrefix(r.Header.Get("Content-Type"),"application/json") {
  http.Error(w,"JSON required",http.StatusUnsupportedMediaType);return
 }
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 defer r.Body.Close()
 var req restrictedpolicy.CommandRequest
 dec:=json.NewDecoder(r.Body)
 dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil {
  http.Error(w,"invalid command request",http.StatusBadRequest);return
 }
 var trailing any
 if err:=dec.Decode(&trailing);err!=io.EOF {
  http.Error(w,"unexpected trailing input",http.StatusBadRequest);return
 }
 result,err:=restrictedpolicy.RunCommand(s.Workspace,*s.Config.Restricted,req)
 if err!=nil {
  if errors.Is(err,restrictedpolicy.ErrCommandDenied) {
   http.Error(w,"command not approved for restricted mode",http.StatusForbidden);return
  }
  http.Error(w,"restricted operation failed",http.StatusBadRequest);return
 }
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,http.StatusOK,result)
}
