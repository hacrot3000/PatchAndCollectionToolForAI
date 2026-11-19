package server

import (
	"encoding/json"
	"io"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

const agentSessionLimit = 32

// agentChatStore is private to this TaskDeck installation, not the project
// checkout. Its directory is keyed to the authoritative workspace path.
func (s *Server) agentChatStore() (*agentchat.Store,error) {
	privateFile,err:=aiPrivatePath(s.Workspace)
	if err!=nil{return nil,err}
	return agentchat.NewStore(filepath.Join(filepath.Dir(privateFile),"agent-sessions-"+aiWorkspaceHash(s.Workspace)))
}

// agentChatSessionsAPI implements only create/list in this milestone. A
// session is NOT an authorization to execute tools or contact an AI endpoint.
func (s *Server) agentChatSessionsAPI(w http.ResponseWriter,r *http.Request) {
	if r.Method!=http.MethodGet&&r.Method!=http.MethodPost{
		http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
	}
	cfg,err:=s.readAISettings()
	if err!=nil{http.Error(w,"AI settings unavailable",http.StatusInternalServerError);return}
	if !cfg.Enabled||!cfg.Consent{
		http.Error(w,"AI is disabled or external provider consent is missing",http.StatusForbidden);return
	}
	store,err:=s.agentChatStore()
	if err!=nil{http.Error(w,"session storage unavailable",http.StatusInternalServerError);return}
	owner:=aiPlanOwner(r)
	if r.Method==http.MethodGet{
		items,err:=store.List(owner)
		if err!=nil{http.Error(w,"cannot list agent sessions",http.StatusInternalServerError);return}
		// Never send all transcripts in a bulk listing.
		summaries:=make([]map[string]any,0,len(items))
		for _,item:=range items{
			summaries=append(summaries,map[string]any{
				"id":item.ID,"project_id":item.ProjectID,"model_id":item.ModelID,
				"provider_id":item.ProviderID,"mode":item.Mode,"status":item.Status,
				"revision":item.Revision,"updated_at":item.UpdatedAt,
			})
		}
		w.Header().Set("Cache-Control","no-store")
		writeJSON(w,http.StatusOK,map[string]any{"sessions":summaries,"execution_enabled":false})
		return
	}
	r.Body=http.MaxBytesReader(w,r.Body,2048)
	var req struct{Mode agentchat.ContextMode `json:"mode"`}
	decoder:=json.NewDecoder(r.Body);decoder.DisallowUnknownFields()
	if err=decoder.Decode(&req);err!=nil{http.Error(w,"invalid agent session request",http.StatusBadRequest);return}
	var extra any
	if err=decoder.Decode(&extra);err==nil || !errors.Is(err,io.EOF){
		http.Error(w,"exactly one JSON object is required",http.StatusBadRequest);return
	}
	if req.Mode==""{req.Mode=agentchat.Stateless}
	sessions,err:=store.List(owner)
	if err!=nil{http.Error(w,"session storage unavailable",http.StatusInternalServerError);return}
	if len(sessions)>=agentSessionLimit{http.Error(w,"too many active/archived sessions",http.StatusTooManyRequests);return}
	identity,err:=patchtool.EnsureProjectIdentity(s.Workspace)
	if err!=nil{http.Error(w,"project identity unavailable",http.StatusConflict);return}
	item,err:=agentchat.NewSession(owner,s.Workspace,identity.ID,cfg.Provider,cfg.Model,req.Mode)
	if err!=nil{http.Error(w,"invalid session mode or identity",http.StatusBadRequest);return}
	item,err=store.Save(item,0)
	if err!=nil{http.Error(w,"cannot save agent session",http.StatusInternalServerError);return}
	w.Header().Set("Cache-Control","no-store")
	writeJSON(w,http.StatusCreated,map[string]any{"session":item,"execution_enabled":false})
}

func (s *Server) agentChatSessionItemAPI(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodGet{http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
	id:=strings.TrimPrefix(r.URL.Path,"/api/agent/sessions/")
	if id==""||strings.Contains(id,"/"){http.Error(w,"invalid agent session",http.StatusBadRequest);return}
	store,err:=s.agentChatStore()
	if err!=nil{http.Error(w,"session storage unavailable",http.StatusInternalServerError);return}
	item,err:=store.Get(id,aiPlanOwner(r))
	if errors.Is(err,agentchat.ErrNotFound){http.Error(w,"agent session not found",http.StatusNotFound);return}
	if err!=nil{http.Error(w,"cannot read agent session",http.StatusInternalServerError);return}
	// The API currently exposes a transcript only to its authenticated owner.
	w.Header().Set("Cache-Control","no-store")
	writeJSON(w,http.StatusOK,map[string]any{"session":item,"execution_enabled":false})
}
