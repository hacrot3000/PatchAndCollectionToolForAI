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
 _,resumeEligible:=agentchat.CollectionResumeReceipt(item)
 if s.Config.SharedServerEnabled||attemptErr!=nil{resumeEligible=false}
 recoveryState:=agentUnknownCollectionRecoveryState(item,time.Now().UTC())
 if recoveryState!=""&&(s.Config.SharedServerEnabled||s.agentConversationRequiresRunContinuation(item.Owner,id)||attemptErr!=nil){
  recoveryState="manual_review_required"
 }
	// Poll through the gap between completed and the next Auto Continue.
	// This is a read-only monitoring hint, not execution admission.
	autoContinueMonitor:=false
	if !s.Config.SharedServerEnabled&&attemptErr==nil&&!s.agentConversationRequiresRunContinuation(item.Owner,id){
		_,autoContinueMonitor=agentPatchFeedbackResumeReceipt(item)
	}
	if !s.Config.SharedServerEnabled && attemptErr==nil && item.Status=="completed" {
		candidate:=item
		if item.Continuation!=nil{
			c:=*item.Continuation;c.NextRetryAt=nil;candidate.Continuation=&c
		}
		if item.TaskPlan!=nil{
			p:=*item.TaskPlan;p.NextRetryAt=nil;candidate.TaskPlan=&p
		}
		autoContinueMonitor=(agentchat.StandaloneAutoContinueCandidate(candidate)||agentchat.TaskPlanAutoContinueCandidate(candidate))&&
			!s.agentConversationRequiresRunContinuation(item.Owner,id)
	}
	// The API currently exposes a transcript only to its authenticated owner.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"session": item, "execution_enabled": false,
		"retry_runtime": agentRetrySnapshot(s.Workspace,aiPlanOwner(r),id),
		"live_preview": agentLivePreviewForSession(s.Workspace, item),
		"native_work_wait":agentNativeWaitSnapshot(s.Workspace,item.Owner,id),
		"provider_review": review,
		"collection_resume_eligible": resumeEligible,
		"collection_progress": agentchat.CollectionProgressViewForSession(item),
		"collection_auto_recovery": recoveryState,
		"auto_feedback_diagnostic":agentAutoFeedbackDiagnostic(item),
		"auto_continue_monitor":autoContinueMonitor,
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
	recoveredCollection := false
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
	// A worker may exit after native COLLECT but fail to commit the
	// observation. Without a live cancellation handle, only the exact turn
	// lease and an acknowledged local read checkpoint permit closing it.
	if !cancelled && !agentRequestActive(s.Workspace,owner,id) {
		if unlock,acquired:=lockAgentTurn(s.Workspace,owner,id);acquired{
			fresh,e:=store.Get(id,owner)
			if e==nil && fresh.Workspace==s.Workspace && agentCancelOrphanedCollection(&fresh){
				if saved,e:=store.Save(fresh,fresh.Revision);e==nil{
					x=saved;cancelled=true;recoveredCollection=true
				}else{
					unlock()
					http.Error(w,"Cannot persist interrupted Collection cancellation; existing journal retained",409)
					return
				}
			}
			unlock()
		}
	}
	// Cancel may also explicitly request Stop for a linked Legacy native
	// Patch. Reconcile first; never cancel by rewriting a live result.
	if !cancelled && r.URL.Query().Get("stop_native")=="1" {
		if owner!="local"||s.Config.SharedServerEnabled {
			http.Error(w,"Native Agent Stop is local-only; use the authorized native Patch console",403);return
		}
		unlock,acquired:=lockAgentTurn(s.Workspace,owner,id)
		if !acquired{http.Error(w,"Agent conversation is busy; retry Stop",409);return}
		defer unlock()
		fresh,e:=store.Get(id,owner)
		if e!=nil||fresh.Workspace!=s.Workspace||fresh.Archived||s.agentConversationRequiresRunContinuation(owner,id){
			http.Error(w,"Native Agent Stop requires the current Legacy conversation",409);return
		}
		identity,e:=patchtool.ReadProjectIdentity(s.Workspace)
		if e!=nil||identity.ID!=fresh.ProjectID{http.Error(w,"project identity mismatch",409);return}
		idx:=-1
		for i,c:=range fresh.ToolCalls {
			switch c.Status {
			case "queuing","queued","dispatching","running","unknown":
				if idx!=-1||c.Name!="taskdeck_submit_patch"||c.AgentRunID!=""||c.FeedbackSent {
					http.Error(w,"ambiguous native Patch ownership; inspect native console",409);return
				}
				idx=i
			}
		}
		if idx<0{writeJSON(w,200,map[string]any{"cancelled":false,"note":"No unresolved native Patch remains. Refresh conversation."});return}
		call:=fresh.ToolCalls[idx]
		view:=agentToolReconcileView{Record:call}
		s.agentReconcilePatchRecord(&view)
		if view.Changed {
			fresh.ToolCalls[idx]=view.Record
			if view.Record.Status=="done"||view.Record.Status=="error"{fresh.Status="waiting_tool"}else{fresh.Status="tool_unknown"}
			fresh.AppendEvent("tool_recovered",view.Record.Status,call.CallID,"user_stop_reconcile")
			fresh,e=store.Save(fresh,fresh.Revision)
			if e!=nil{http.Error(w,"Cannot persist native reconciliation",409);return}
			call=view.Record
		}
		if call.Status=="done"||call.Status=="error"{
			writeJSON(w,200,map[string]any{"cancelled":false,"status":fresh.Status,"note":"Native Patch already has a verified terminal outcome. Send that result to AI; no execution was stopped or repeated."});return
		}
		if s.Sessions==nil||call.PatchSessionID==""{http.Error(w,"Native session unavailable; execution outcome unverified. Inspect exact Patch History.",409);return}
		meta,exists:=s.Sessions.Metadata(call.PatchSessionID)
		if !exists||meta.Status=="exited"||meta.Status=="stopped"{
			writeJSON(w,200,map[string]any{"cancelled":false,"status":fresh.Status,"note":"Native session is no longer active or available; final outcome remains UNKNOWN. No result was fabricated. Inspect the exact Patch History."});return
		}
		if meta.Status!="running"||filepath.Clean(meta.Cwd)!=filepath.Clean(s.Workspace)||
			(!strings.EqualFold(meta.Kind,"patch")&&!strings.HasPrefix(meta.Label,"Patch Tool"))||
			(meta.ProjectID!=""&&meta.ProjectID!=fresh.ProjectID){
			http.Error(w,"Linked native session identity or liveness could not be verified",409);return
		}
		if !s.agentNativeStopMatches(call){http.Error(w,"Exact native selection could not be verified; use the linked native console",409);return}
		if e=s.Sessions.Stop(call.PatchSessionID);e!=nil{http.Error(w,"Native Stop request failed; inspect linked console",409);return}
		fresh.AppendEvent("tool_stop_requested",fresh.Status,call.CallID,"user_stop_native")
		if _,e=store.Save(fresh,fresh.Revision);e!=nil{http.Error(w,"Native Stop requested, journal update failed; inspect linked console",409);return}
		writeJSON(w,200,map[string]any{"cancelled":false,"native_stop_requested":true,"status":fresh.Status,"note":"Stop requested for the exact native Patch session. Waiting for terminal native evidence; no Patch result was fabricated or replayed."});return
	}
	// Without explicit native Stop, cancellation affects the provider only.
	unresolved := false
	for _, call := range x.ToolCalls {
		switch call.Status {
		case "queuing", "queued", "dispatching", "running", "unknown":
			unresolved = true
		}
	}
	if cancelled && !recoveredCollection {
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
		"recovered_local_collection": recoveredCollection,
		"patch_execution_untouched": unresolved,
		"note": func() string {
			if unresolved {
				return "A native Patch execution may still be running. Reconcile it before assuming the work stopped."
			}
			if recoveredCollection {return "Interrupted local Collection closed. Existing native History is retained; send a new message to resume the conversation."}
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
	var until *time.Time
	if req.Enabled {
		deadline := time.Now().UTC().Add(agentAutoFeedbackMaxAge)
		until = &deadline
	}
	x, err = store.UpdateLegacyAutoFeedback(id, owner, s.Workspace, req.ExpectedRevision, req.Enabled, until)
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
