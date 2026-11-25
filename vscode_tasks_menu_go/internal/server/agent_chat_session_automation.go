package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

// The user controls automation per conversation. A toggle is an explicit
// setting change, NOT implicit authority supplied by an AI prompt.
func (s *Server) agentChatAutomationAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var req struct {
		Option string `json:"option"`
		Enabled bool `json:"enabled"`
		ExpectedRevision uint64 `json:"expected_revision"`
		ConfirmExecution bool `json:"confirm_execution,omitempty"`
		ConfirmExternal bool `json:"confirm_external,omitempty"`
	}
	dec := json.NewDecoder(r.Body); dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil { http.Error(w, "invalid automation settings",400);return }
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err,io.EOF) { http.Error(w,"exactly one automation setting required",400);return }
	if req.Option!="patch" && req.Option!="collection" && req.Option!="feedback" && req.Option!="continue" {
		http.Error(w,"unknown automation setting",400);return
	}
	if req.Option=="patch" && req.Enabled {
		// Automatic source changes are currently supported only for local
		// single-user projects, never unauthenticated shared-server sessions.
		if s.Config.SharedServerEnabled || aiPlanOwner(r)!="local" {
			http.Error(w,"automatic Patch execution is local-owner only",403);return
		}
		if !req.ConfirmExecution {
			http.Error(w,"explicit session-level automatic Patch authorization required",403);return
		}
		if !s.requireSharedActionPermission(w,r,identity.PermissionPatchRun,"agent.auto_patch_setting",id) {return}
	}
	if req.Option=="continue" && req.Enabled &&
  (s.Config.SharedServerEnabled||aiPlanOwner(r)!="local"||!req.ConfirmExternal){
  http.Error(w,"explicit local-owner provider consent required for standalone continuation",403);return
 }
 if req.Option=="feedback" && req.Enabled && !req.ConfirmExternal {
		http.Error(w,"explicit provider sharing authorization required",403);return
	}
	owner:=aiPlanOwner(r)
	store,err:=s.agentChatStore()
	if err!=nil {http.Error(w,"session storage unavailable",500);return}
	x,err:=store.UpdateAutomation(id,owner,s.Workspace,req.ExpectedRevision,req.Option,req.Enabled,"")
	if errors.Is(err,agentchat.ErrNotFound){http.Error(w,"conversation not found",404);return}
	if err!=nil {http.Error(w,"cannot save automation preference; check conversation or Task plan",409);return}
	w.Header().Set("Cache-Control","no-store")
	writeJSON(w,200,map[string]any{"session":x,"execution_enabled":false})
}

func (s *Server) agentChatDeleteSessionAPI(w http.ResponseWriter,r *http.Request,id string) {
	if r.Method!=http.MethodDelete {http.Error(w,"method not allowed",405);return}
	r.Body=http.MaxBytesReader(w,r.Body,2048)
	var req struct {
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
	if err:=dec.Decode(&req);err!=nil {http.Error(w,"invalid delete request",400);return}
	var extra any
	if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){http.Error(w,"exactly one delete request required",400);return}
	owner:=aiPlanOwner(r)
	release,ok:=lockAgentTurn(s.Workspace,owner,id)
	if !ok {http.Error(w,"conversation busy",409);return}
	defer release()
	store,err:=s.agentChatStore()
	if err!=nil {http.Error(w,"session storage unavailable",500);return}
	err=store.Delete(id,owner,req.ExpectedRevision)
	if errors.Is(err,agentchat.ErrNotFound) {http.Error(w,"conversation not found",404);return}
	if errors.Is(err,agentchat.ErrStaleRevision) {http.Error(w,"stale conversation revision",409);return}
	if err!=nil {http.Error(w,strings.TrimSpace(err.Error()),409);return}
	w.Header().Set("Cache-Control","no-store")
	writeJSON(w,200,map[string]any{"deleted":true,"session_id":id})
}
