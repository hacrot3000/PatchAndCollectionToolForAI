package server

import (
 "net/http"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// attestAITerminal derives the target solely from authoritative session
// metadata, not from user-controlled terminal labels or request JSON.
// This checks visibility, not execution permission: execution remains disabled.
func (s *Server) attestAITerminal(w http.ResponseWriter,r *http.Request,sessionID string)(aiPlanTarget,bool){
 sessionID=strings.TrimSpace(sessionID)
 if sessionID==""||len(sessionID)>128{
  http.Error(w,"valid terminal session_id required",http.StatusBadRequest)
  return aiPlanTarget{},false
 }
 if s.Sessions==nil{
  http.Error(w,"terminal session service unavailable",http.StatusServiceUnavailable)
  return aiPlanTarget{},false
 }
 if s.Config.SharedServerEnabled&&!s.sharedSessionOwnershipReady(w){
  return aiPlanTarget{},false
 }
 principal,hasPrincipal:=PrincipalFromContext(r.Context())
 if s.Config.SharedServerEnabled&&!hasPrincipal{
  http.Error(w,"authentication required",http.StatusUnauthorized)
  return aiPlanTarget{},false
 }
 meta,ok:=s.Sessions.Metadata(sessionID)
 if !ok||meta.Kind!=tasks.SessionKindTerminal||meta.ID!=sessionID{
  http.Error(w,"terminal session not found",http.StatusNotFound)
  return aiPlanTarget{},false
 }
 if meta.Status!="running"{
  http.Error(w,"terminal session is no longer running",http.StatusConflict)
  return aiPlanTarget{},false
 }
 if hasPrincipal&&!sharedSessionVisible(principal,meta){
  http.Error(w,"terminal session access denied",http.StatusForbidden)
  return aiPlanTarget{},false
 }
 remote:=meta.TargetType=="ssh"
 host:="local"
 if remote {
  host=strings.TrimSpace(meta.TargetProfileID)
  if host==""{
   http.Error(w,"remote terminal profile unavailable",http.StatusConflict)
   return aiPlanTarget{},false
  }
 }
 return aiPlanTarget{
  SessionID:sessionID,
  Host:host,
  CWD:meta.Cwd,
  Remote:remote,
 },true
}

// Ensure current implementations expose the server-side owner/project fields.
var _ = session.Metadata{}
