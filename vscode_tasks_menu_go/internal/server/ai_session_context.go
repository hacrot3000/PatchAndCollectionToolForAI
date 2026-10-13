package server

import (
 "encoding/json"
 "net/http"
 "strings"
)

// aiSessionContextAPI is a read-only, backend-attested context inspector.
// Terminal output and environment variables are never returned here.
func (s *Server) aiSessionContextAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet {
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
  return
 }
 cfg,err:=s.readAISettings()
 if err!=nil{http.Error(w,"AI settings unavailable",http.StatusInternalServerError);return}
 if !cfg.Enabled{http.Error(w,"AI Assistant is disabled",http.StatusForbidden);return}
 sessionID:=strings.TrimSpace(r.URL.Query().Get("session_id"))
 target,ok:=s.attestAITerminal(w,r,sessionID)
 if !ok{return}
 // Initial CWD comes from the session manager. An interactive shell may cd
 // later; do not misrepresent this value as live shell CWD.
 response:=map[string]any{
  "verified":true,
  "session_id":target.SessionID,
  "host":target.Host,
  "launch_cwd":target.CWD,
  "remote":target.Remote,
  "status":"running",
  "execution_enabled":false,
 }
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 json.NewEncoder(w).Encode(response)
}
