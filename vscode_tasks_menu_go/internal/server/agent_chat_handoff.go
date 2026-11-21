package server

import (
 "errors"
 "net/http"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// A portable, always-current checkpoint for transferring an interrupted
// multi-day Agent task to another browser/session/AI. This is read-only,
// owner/project scoped and never invokes any native tool or model.
func (s *Server) agentChatHandoffAPI(w http.ResponseWriter,r *http.Request,id string){
 if r.Method!=http.MethodGet {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 store,err:=s.agentChatStore()
 if err!=nil{http.Error(w,"agent journal unavailable",http.StatusInternalServerError);return}
 content,revision,err:=store.Handoff(id,aiPlanOwner(r))
 if errors.Is(err,agentchat.ErrNotFound){http.Error(w,"agent session not found",http.StatusNotFound);return}
 if err!=nil{http.Error(w,"agent handoff unavailable",http.StatusInternalServerError);return}
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 if r.URL.Query().Get("format")=="markdown"{
  w.Header().Set("Content-Type","text/markdown; charset=utf-8")
  _,_=w.Write([]byte(content))
  return
 }
 writeJSON(w,http.StatusOK,map[string]any{"revision":revision,"handoff":content})
}
