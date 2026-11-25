package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

const agentSessionLimit = 32

// agentChatStore is private to this TaskDeck installation, not the project
// checkout. Its directory is keyed to the authoritative workspace path.
func (s *Server) agentChatStore() (*agentchat.Store, error) {
	ownerWorkspace := s.agentOwnerWorkspace()
	privateFile, err := aiPrivatePath(ownerWorkspace)
	if err != nil {
		return nil, err
	}
	identity, err := patchtool.ReadProjectIdentity(ownerWorkspace)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	store,err:=agentchat.NewProjectStore(filepath.Join(filepath.Dir(privateFile), "agent-sessions-"+aiWorkspaceHash(ownerWorkspace)), ownerWorkspace, identity.ID)
 if err!=nil{return nil,err}
 // Recover abandoned immutable plan segments once per private directory on
 // daemon startup. A failed audit blocks use; never prune from a partial
 // view of any owner/project's canonical session journal.
 if err=store.EnsureTaskPlanOrphanRecovery();err!=nil{return nil,err}
 return store,nil
}

// agentChatSessionsAPI implements only create/list in this milestone. A
// session is NOT an authorization to execute tools or contact an AI endpoint.
func (s *Server) agentChatSessionsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "session storage unavailable", http.StatusInternalServerError)
		return
	}
	owner := aiPlanOwner(r)
	if r.Method == http.MethodGet {
		items, err := store.List(owner)
		if err != nil {
			http.Error(w, "cannot list agent sessions", http.StatusInternalServerError)
			return
		}
		// Never send all transcripts in a bulk listing. Include archived
		// sessions so the UI can offer restore, but label them explicitly.
		summaries := make([]map[string]any, 0, len(items))
		for _, item := range items {
			summaries = append(summaries, map[string]any{
				"id": item.ID, "project_id": item.ProjectID, "model_id": item.ModelID,
				"provider_id": item.ProviderID, "mode": item.Mode, "status": item.Status,
				"title": item.Title, "archived": item.Archived, "target": item.Target,
				"reasoning_effort": item.ReasoningEffort, "retry_mode": item.RetryMode,
				"revision": item.Revision, "updated_at": item.UpdatedAt,
				"unresolved": agentchat.HasUnresolvedWork(item),
			})
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"sessions": summaries, "execution_enabled": false})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var req struct {
		Mode            agentchat.ContextMode `json:"mode"`
		ProviderID      string                `json:"provider_id"`
		ModelID         string                `json:"model_id"`
		ReasoningEffort string                `json:"reasoning_effort"`
		// Target must be empty or "local". Remote Agent Patch has no typed
		// E2EE operation yet, so pretending to bind a remote target here would
		// be a false capability claim.
		Target string `json:"target"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&req); err != nil {
		http.Error(w, "invalid agent session request", http.StatusBadRequest)
		return
	}
	var extra any
	if err = decoder.Decode(&extra); err == nil || !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one JSON object is required", http.StatusBadRequest)
		return
	}
	if req.Mode == "" {
		req.Mode = agentchat.Stateless
	}
	sessions, err := store.List(owner)
	if err != nil {
		http.Error(w, "session storage unavailable", http.StatusInternalServerError)
		return
	}
	// Archived sessions do not consume the active-session budget; otherwise a
	// user could be permanently blocked after archiving 32 chats.
	active := 0
	for _, sess := range sessions {
		if !sess.Archived {
			active++
		}
	}
	if active >= agentSessionLimit {
		http.Error(w, "too many active sessions", http.StatusTooManyRequests)
		return
	}
	providerID, modelID := req.ProviderID, req.ModelID
	if providerID != "" {
		registry, err := s.agentChatProviders()
		if err != nil {
			http.Error(w, "provider store unavailable", 500)
			return
		}
		p, err := registry.Get(owner, providerID)
		if err != nil {
			http.Error(w, "provider not configured", 400)
			return
		}
		if modelID == "" {
			modelID = p.Models[0].ID
		}
		if _, ok := p.FindModel(modelID); !ok {
			http.Error(w, "model not available in provider", 400)
			return
		}
	} else {
		if modelID != "" {
			http.Error(w, "provider_id required when model_id is set", 400)
			return
		}
		// Legacy default is kept for existing Smart Terminal users, but
		// explicitly configured Agent providers are fully independent.
		cfg, err := s.readAISettings()
		if err != nil {
			http.Error(w, "AI settings unavailable", 500)
			return
		}
		if !cfg.Enabled || !cfg.Consent {
			http.Error(w, "AI default provider disabled", 403)
			return
		}
		providerID, modelID = cfg.Provider, cfg.Model
	}
	identity, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil {
		http.Error(w, "project identity unavailable", http.StatusConflict)
		return
	}
	store, err = s.agentChatStore()
	if err != nil {
		http.Error(w, "session storage unavailable", http.StatusInternalServerError)
		return
	}
	if target := strings.TrimSpace(req.Target); target != "" && target != "local" {
		http.Error(w, "remote execution target is not supported for Agent Chat", 400)
		return
	}
	// Chat Completions cannot send server-side Responses references, but the
	// user may choose either context preference. The transport replays local
	// canonical history when that provider lacks a compatible stateful API.
	item, err := agentchat.NewSession(owner, s.Workspace, identity.ID, providerID, modelID, req.Mode)
	if err != nil {
		http.Error(w, "invalid session mode or identity", http.StatusBadRequest)
		return
	}
	// A reasoning effort must be advertised by the chosen model; an unknown
	// value would otherwise be silently forwarded to the provider.
	if effort := strings.TrimSpace(req.ReasoningEffort); effort != "" {
		// A configured model's explicit reasoning_efforts is the opt-in for
		// sending Chat Completions' reasoning_effort extension as well.
		registry, err := s.agentChatProviders()
		if err != nil {
			http.Error(w, "provider store unavailable", 500)
			return
		}
		p, err := registry.Get(owner, providerID)
		if err != nil {
			http.Error(w, "provider not configured", 400)
			return
		}
		model, ok := p.FindModel(modelID)
		if !ok {
			http.Error(w, "model not available in provider", 400)
			return
		}
		advertised := false
		for _, v := range model.ReasoningEfforts {
			if v == effort {
				advertised = true
				break
			}
		}
		if !advertised {
			http.Error(w, "reasoning effort is not advertised by this model", 400)
			return
		}
		item.ReasoningEffort = effort
	}
	item, err = store.Save(item, 0)
	if err != nil {
		http.Error(w, "cannot save agent session", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"session": item, "execution_enabled": false})
}

func (s *Server) agentChatSessionItemAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/agent/sessions/")
	segments := strings.Split(path, "/")
	if len(segments) == 2 && segments[0] != "" && segments[1] == "messages" {
		s.agentChatMessageAPI(w, r, segments[0])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "sync" {
		s.agentChatSyncAPI(w, r, segments[0])
		return
	}
	if len(segments) == 4 && segments[0] != "" && segments[1] == "tools" && segments[2] != "" && segments[3] == "decision" {
		s.agentChatToolDecisionAPI(w, r, segments[0], segments[2])
		return
	}
	if len(segments) == 4 && segments[0] != "" && segments[1] == "tools" && segments[2] != "" && segments[3] == "recover" {
		s.agentChatPatchRecoveryAPI(w, r, segments[0], segments[2])
		return
	}
	if len(segments) == 4 && segments[0] != "" && segments[1] == "tools" && segments[2] != "" && segments[3] == "continue" {
		s.agentChatToolContinueAPI(w, r, segments[0], segments[2])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "metadata" {
		s.agentChatSessionMetadataAPI(w, r, segments[0])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "events" {
		s.agentChatEventsAPI(w, r, segments[0])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "handoff" {
		s.agentChatHandoffAPI(w, r, segments[0])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "plan" {
		s.agentChatTaskPlanAPI(w, r, segments[0])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "retry" {
		s.agentChatRetryAPI(w, r, segments[0])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "automation" {
		s.agentChatAutomationAPI(w, r, segments[0])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "auto-feedback" {
		s.agentChatAutoFeedbackAPI(w, r, segments[0])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "pack-preview" {
		s.agentChatPackPreviewAPI(w, r, segments[0])
		return
	}
	if len(segments) == 2 && segments[0] != "" && segments[1] == "cancel" {
		s.agentChatSessionCancelAPI(w, r, segments[0])
		return
	}
	if len(segments) == 4 && segments[0] != "" && segments[1] == "tools" && segments[3] == "execution" {
		s.agentChatExecutionAPI(w, r, segments[0], segments[2])
		return
	}

	if r.Method == http.MethodDelete && len(segments) == 1 && segments[0] != "" {
		s.agentChatDeleteSessionAPI(w, r, segments[0])
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/agent/sessions/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "invalid agent session", http.StatusBadRequest)
		return
	}
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "session storage unavailable", http.StatusInternalServerError)
		return
	}
	item, err := store.Get(id, aiPlanOwner(r))
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "agent session not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "cannot read agent session", http.StatusInternalServerError)
		return
	}
	// A normal in-flight SENDING request is not a reconciliation incident.
	// Heartbeats are owner-checked liveness HINTS, never provider delivery
	// evidence or authorization to replay a request. A missing or stale
	// heartbeat eventually restores the warning.
	var heartbeat *agentchat.ProviderHeartbeat
	if item.ProviderRun!=nil && item.ProviderRun.Phase=="sending" && item.Status=="requesting_ai" {
		if h,e:=store.LatestProviderHeartbeat(id,aiPlanOwner(r));e==nil {heartbeat=&h}
	}
	review:=agentchat.ProviderEgressReviewWithLiveness(item,heartbeat,time.Now().UTC())
 // Provider attempts contain ONLY pre-send ID/digest/time hints, never model
 // text, credentials or an assertion that any network request arrived.
 // If the sidecar is damaged, explicitly surface an integrity warning.
 attempts,attemptErr:=store.ProviderAttempts(id,aiPlanOwner(r))
	// The API currently exposes a transcript only to its authenticated owner.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"session": item, "execution_enabled": false,
		"retry_runtime": agentRetrySnapshot(s.Workspace,aiPlanOwner(r),id),
		"provider_review": review,
		"auto_feedback_diagnostic":agentAutoFeedbackDiagnostic(item),
  "provider_attempts":attempts,
  "provider_attempts_integrity_error":attemptErr!=nil,
		"patch_tool_status": s.agentPatchToolStatus(aiPlanOwner(r), item.ProviderID, item.ModelID)})
}

// agentChatSessionMetadataAPI renames or archives a session. These are the
// only mutable metadata fields; owner, workspace, project, provider, model and
// mode stay immutable so an existing approval can never be re-pointed.
// Archiving an active or unresolved tool call is refused rather than hiding a
// job that may still be executing.
func (s *Server) agentChatSessionMetadataAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Title            *string `json:"title"`
		Archived         *bool   `json:"archived"`
		ExpectedRevision uint64  `json:"expected_revision"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid session metadata request", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one JSON object required", 400)
		return
	}
	if req.Title == nil && req.Archived == nil {
		http.Error(w, "title or archived required", 400)
		return
	}
	if req.Title != nil && req.Archived != nil {
		http.Error(w, "rename and archive must be separate operations", 400)
		return
	}
	owner := aiPlanOwner(r)
	if req.Title != nil {
		// Rename only changes display metadata. Do NOT acquire lockAgentTurn:
		// active provider/native turns and their CAS revision continue
		// unmodified. Store.Rename serializes the on-disk write and Save
		// carries the fresh title forward from the committed journal.
		title := strings.TrimSpace(*req.Title)
		if title == "" || len(title) > 120 {
			http.Error(w, "invalid conversation title", 400)
			return
		}
		store, err := s.agentChatStore()
		if err != nil {
			http.Error(w, "agent journal unavailable", 500)
			return
		}
		x, err := store.Rename(id, owner, title, req.ExpectedRevision)
		if errors.Is(err, agentchat.ErrNotFound) {
			http.Error(w, "Agent session not found", 404)
			return
		}
		if errors.Is(err, agentchat.ErrStaleRevision) {
			http.Error(w, "future session revision", 409)
			return
		}
		if err != nil {
			http.Error(w, "cannot persist conversation title", 500)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"session": map[string]any{
			"id": x.ID, "title": x.Title, "archived": x.Archived,
			"status": x.Status, "revision": x.Revision,
			"provider_id": x.ProviderID, "model_id": x.ModelID,
			"mode": x.Mode, "target": x.Target,
		}})
		return
	}
	release, ok := lockAgentTurn(s.Workspace, owner, id)
	if !ok {
		http.Error(w, "Agent session is busy", 409)
		return
	}
	defer release()
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	x, err := store.Get(id, owner)
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	if x.Workspace != s.Workspace {
		http.Error(w, "workspace mismatch", 403)
		return
	}
	if x.Revision != req.ExpectedRevision {
		http.Error(w, "stale session revision", 409)
		return
	}
	if req.Archived != nil {
		if *req.Archived {
			for _, call := range x.ToolCalls {
				switch call.Status {
				case "queuing", "queued", "dispatching", "running", "unknown":
					http.Error(w, "cannot archive a session with an unresolved patch call; reconcile it first", 409)
					return
				}
			}
		}
		x.Archived = *req.Archived
	}
	x, err = store.Save(x, x.Revision)
	if err != nil {
		http.Error(w, "cannot persist session metadata", 409)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"session": map[string]any{
		"id": x.ID, "title": x.Title, "archived": x.Archived, "status": x.Status,
		"revision": x.Revision, "provider_id": x.ProviderID, "model_id": x.ModelID,
		"mode": x.Mode, "target": x.Target,
	}})
}

// agentChatSessionCancelAPI cancels the ACTIVE model request for a session.
// Browser aborts are not trusted as proof of server cancellation: this
// endpoint cancels the in-flight provider context and records the intent.
// A native Patch execution already started is NOT killed here; it remains
// governed by the existing Patch Tool session and is reported as needing
// reconciliation rather than silently cancelled.
func (s *Server) agentChatSessionCancelAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	owner := aiPlanOwner(r)
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	x, err := store.Get(id, owner)
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	if x.Workspace != s.Workspace {
		http.Error(w, "workspace mismatch", 403)
		return
	}
	cancelled := cancelAgentRequest(s.Workspace, owner, id)
	// A PREPARED request has not crossed the network and may be waiting in
	// the recovery queue with no active request cancellation handle yet.
	// Honor Stop by revision-fencing the journal under the normal turn lock.
	if !cancelled && x.Status=="requesting_ai" && x.ProviderRun!=nil && x.ProviderRun.Phase=="prepared" {
		if unlock,acquired:=lockAgentTurn(s.Workspace,owner,id);acquired{
			fresh,e:=store.Get(id,owner)
			if e==nil && fresh.Status=="requesting_ai" && fresh.ProviderRun!=nil && fresh.ProviderRun.Phase=="prepared"{
				fresh.Status="cancelled"
				markAgentProviderOutcome(&fresh,"blocked")
				fresh.AppendEvent("turn_cancelled","cancelled",fresh.ProviderRun.CallID,"user_stop_prepared")
				if saved,e:=store.Save(fresh,fresh.Revision);e==nil{
					x=saved;cancelled=true
					_=store.DeleteProviderPayload(id)
				}
			}
			unlock()
		}
	}
	// A tool call that already left the proposal stage cannot be declared
	// cancelled from here; the native Patch session owns that execution.
	unresolved := false
	for _, call := range x.ToolCalls {
		switch call.Status {
		case "queuing", "queued", "dispatching", "running", "unknown":
			unresolved = true
		}
	}
	if cancelled {
		// A currently executing provider turn owns the journal CAS until it
		// observes the cancelled context. Do not race it with a second Save:
		// that could prevent the worker from persisting its final cancellation.
		// A detached/manual registered request without a turn lock is still
		// marked cancelled immediately for crash/reconnect recovery.
		if unlock, acquired := lockAgentTurn(s.Workspace, owner, id); acquired {
			if x.Status == "requesting_ai" {
				x.Status = "cancelled"
			}
			x.AppendEvent("turn_cancel_requested", x.Status, "", "user_stop")
			if changed, saveErr := store.Save(x, x.Revision); saveErr == nil {
				x = changed
			}
			unlock()
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"session_id": id, "cancelled": cancelled, "status": x.Status,
		"patch_execution_untouched": unresolved,
		"note": func() string {
			if unresolved {
				return "A native Patch execution may still be running. Reconcile it before assuming the work stopped."
			}
			if !cancelled {
				return "No active model request was registered for this session."
			}
			return "The active model request was cancelled. A late provider response will not be recorded as final."
		}(),
	})
}

// agentChatPackPreviewAPI renders the exact bounded first-turn AI Pack text
// that WOULD be sent, after the same scrubbing, without contacting a provider
// and without persisting a session turn. It lets the user inspect egress
// before granting consent; redaction remains best-effort, never a guarantee.
func (s *Server) agentChatPackPreviewAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	owner := aiPlanOwner(r)
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	x, err := store.Get(id, owner)
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	if x.Workspace != s.Workspace {
		http.Error(w, "workspace mismatch", 403)
		return
	}
	identity, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil || identity.ID != x.ProjectID {
		http.Error(w, "project identity drift", 409)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		http.Error(w, "TaskDeck runtime unavailable", 500)
		return
	}
	builder := patchtool.BuildAIPack
	if s.AgentChatPack != nil {
		builder = s.AgentChatPack
	}
	pack, err := builder(s.Workspace, executable)
	if err != nil {
		http.Error(w, "AI Pack unavailable: "+err.Error(), 409)
		return
	}
	if pack.ProjectIdentity.ID != x.ProjectID {
		http.Error(w, "AI Pack project identity mismatch", 409)
		return
	}
	oversize := len(pack.TextContent) > 400<<10
	preview := scrubAIContext(pack.TextContent, 400<<10)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"session_id": id, "fingerprint": pack.Fingerprint,
		"doc_count": pack.DocCount, "project_id": pack.ProjectIdentity.ID,
		"pack_bytes": len(pack.TextContent), "oversize": oversize,
		"preview": preview, "truncated": oversize,
		"redaction": "best_effort",
		"note":      "Preview only. No provider request was made. Redaction cannot guarantee every secret is removed; review before sending.",
	})
}

// agentChatAutoFeedbackAPI records an explicit, bounded opt-in to deliver
// verified tool results to the provider without a per-result click. It does
// NOT approve patches, queue work or extend itself.
func (s *Server) agentChatAutoFeedbackAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var req struct {
		Enabled          bool   `json:"enabled"`
		ConfirmExternal  bool   `json:"confirm_external"`
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid auto-feedback request", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one JSON object required", 400)
		return
	}
	if req.Enabled && !req.ConfirmExternal {
		http.Error(w, "explicit consent is required to send tool results automatically", 403)
		return
	}
	owner := aiPlanOwner(r)
	release, ok := lockAgentTurn(s.Workspace, owner, id)
	if !ok {
		http.Error(w, "Agent session busy", 409)
		return
	}
	defer release()
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	x, err := store.Get(id, owner)
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	if x.Workspace != s.Workspace {
		http.Error(w, "workspace mismatch", 403)
		return
	}
	if x.Revision != req.ExpectedRevision {
		http.Error(w, "stale session revision", 409)
		return
	}
	x.AutoFeedback = req.Enabled
	x.AutoFeedbackPersistent = false // legacy endpoint grants only a time-bounded lease
	if req.Enabled {
		until := time.Now().UTC().Add(agentAutoFeedbackMaxAge)
		x.AutoFeedbackUntil = &until
	} else {
		x.AutoFeedbackUntil = nil
	}
	x, err = store.Save(x, x.Revision)
	if err != nil {
		http.Error(w, "cannot persist auto-feedback consent", 409)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"session_id": id, "revision": x.Revision, "auto_feedback": x.AutoFeedback,
		"auto_feedback_until": x.AutoFeedbackUntil,
		"note":                "Automatic delivery covers verified tool results only; it never approves or executes a patch.",
	})
}
