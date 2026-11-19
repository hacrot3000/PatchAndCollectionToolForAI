package server

import (
 "errors"
 "net/http"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Explicit POST reconciliation is intentional. It reads authoritative native
// protocol evidence and atomically updates the local durable tool ledger.
// GET session remains side-effect-free.
func (s *Server) agentChatSyncAPI(w http.ResponseWriter,r *http.Request,id string){
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 owner:=aiPlanOwner(r)
 release,ok:=lockAgentTurn(s.Workspace,owner,id)
 if !ok{http.Error(w,"Agent session is busy",409);return}
 defer release()
 store,err:=s.agentChatStore()
 if err!=nil{http.Error(w,"agent journal unavailable",500);return}
 x,err:=store.Get(id,owner)
 if errors.Is(err,agentchat.ErrNotFound){http.Error(w,"Agent session not found",404);return}
 if err!=nil{http.Error(w,"agent journal unavailable",500);return}
 changed:=false
 for i,record:=range x.ToolCalls{
  if record.Status!="running"||record.PatchSessionID==""{continue}
  view:=agentToolReconcileView{Record:record}
  s.agentReconcilePatchRecord(&view)
  if view.Changed{
   x.ToolCalls[i]=view.Record
   changed=true
  }
 }
 if changed {
  x,err=store.Save(x,x.Revision)
  if err!=nil{http.Error(w,"agent journal revision changed during sync",409);return}
 }
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]any{
  "session_id":id,"status":x.Status,"revision":x.Revision,
  "tool_calls":x.ToolCalls,"execution_enabled":false,
 })
}
