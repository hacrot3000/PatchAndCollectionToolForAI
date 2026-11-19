package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// agentChatToolContinueAPI submits an authenticated, evidence-backed
// function_call_output using the SAME call_id returned by the provider.
// It does not rerun, replay or requeue any patch; feedback is once-only.
func (s *Server) agentChatToolContinueAPI(w http.ResponseWriter, r *http.Request, id, callID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		ExpectedRevision uint64 `json:"expected_revision"`
		ConfirmExternal  bool   `json:"confirm_external"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil {
		http.Error(w, "invalid tool continuation request", 400)
		return
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one continuation object required", 400)
		return
	}
	if !req.ConfirmExternal {
		http.Error(w, "explicit external-provider sharing consent required", 403)
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
	if x.Revision != req.ExpectedRevision || x.Workspace != s.Workspace {
		http.Error(w, "Agent session revision/workspace mismatch", 409)
		return
	}
	if x.Status != "waiting_tool" && x.Status != "tool_rejected" {
		http.Error(w, "Agent session not awaiting a final tool result", 409)
		return
	}
	idx := -1
	for i, item := range x.ToolCalls {
		if item.CallID == callID {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Error(w, "tool result not pending", 409)
		return
	}
	res, err := s.agentDeliverToolResult(s.agentServerContext(), store, x, idx, true)
	if err != nil {
		writeAgentContinueError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"session_id": id, "revision": res.Session.Revision, "status": res.Session.Status,
		"answer": res.Result.Text, "tool_calls": res.Result.Calls,
		"response_id": res.Result.ResponseID, "input_tokens": res.Result.InputTokens,
		"output_tokens": res.Result.OutputTokens, "execution_enabled": false,
	})
}

// agentContinueError carries an HTTP status alongside a safe message so the
// same delivery path can serve the manual endpoint and the opt-in scheduler.
type agentContinueError struct {
	status  int
	message string
}

func (e agentContinueError) Error() string { return e.message }

func writeAgentContinueError(w http.ResponseWriter, err error) {
	var sc agentContinueError
	if errors.As(err, &sc) {
		http.Error(w, sc.message, sc.status)
		return
	}
	http.Error(w, "tool continuation failed", 500)
}

// agentDeliverResult is the shared outcome of a tool-result continuation.
type agentDeliverResult struct {
	Session agentchat.Session
	Result  agentchat.ResponsesResult
}

// agentDeliverToolResult performs the once-only function_call_output delivery.
// The caller MUST hold the session turn lock and have already re-authorized
// the owner, project identity and provider. When consent is true the caller
// has an explicit user grant for this egress; otherwise no request is made.
func (s *Server) agentDeliverToolResult(ctx context.Context, store *agentchat.Store, x agentchat.Session, idx int, consent bool) (agentDeliverResult, error) {
	owner := x.Owner
	call := x.ToolCalls[idx]
	if call.Status != "done" && call.Status != "error" && call.Status != "rejected" {
		return agentDeliverResult{}, agentContinueError{409, "Patch result has no verified final status; do not send success prematurely"}
	}
	if call.FeedbackSent {
		return agentDeliverResult{}, agentContinueError{409, "tool result not pending"}
	}
	if _, found := findAgentToolCall(x.History, call.CallID); !found {
		return agentDeliverResult{}, agentContinueError{409, "source tool call not in transcript"}
	}
	identity, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil || identity.ID != x.ProjectID {
		return agentDeliverResult{}, agentContinueError{409, "project identity drift"}
	}
	registry, err := s.agentChatProviders()
	if err != nil {
		return agentDeliverResult{}, agentContinueError{500, "provider store unavailable"}
	}
	p, err := registry.Get(owner, x.ProviderID)
	if err != nil {
		return agentDeliverResult{}, agentContinueError{409, "provider no longer configured"}
	}
	model, ok := p.FindModel(x.ModelID)
	if !ok {
		return agentDeliverResult{}, agentContinueError{409, "model no longer configured"}
	}
	if s.ConnectionSecrets == nil {
		return agentDeliverResult{}, agentContinueError{503, "secret store unavailable"}
	}
	secret, err := s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace, owner, p.ID))
	if err != nil || len(secret) == 0 {
		return agentDeliverResult{}, agentContinueError{503, "provider credential unavailable"}
	}
	releaseSlot, admitted := acquireAgentSlot(ctx, aiWorkspaceHash(s.Workspace)+":"+owner)
	if !admitted {
		return agentDeliverResult{}, agentContinueError{429, "Agent concurrency limit reached; retry shortly"}
	}
	defer releaseSlot()
	releaseQuota, allowed := s.reserveAIRequest(aiWorkspaceHash(s.Workspace)+":"+owner, 20)
	if !allowed {
		return agentDeliverResult{}, agentContinueError{429, "AI request rate limit exceeded"}
	}
	defer releaseQuota()
	if len(x.History) > 200 {
		compacted := agentchat.CompactHistory(x.History, agentchat.CompactionConfig{KeepTail: 120, MaxDropped: 120})
		if compacted.Dropped == 0 {
			return agentDeliverResult{}, agentContinueError{413, "agent context limit reached; start a new session"}
		}
		x.History = compacted.History
	}
	output := strings.TrimSpace(call.Result)
	if output == "" {
		return agentDeliverResult{}, agentContinueError{409, "tool result is empty"}
	}
	if len(output) > 4000 {
		return agentDeliverResult{}, agentContinueError{413, "tool result exceeds limit"}
	}
	next := agentchat.HistoryItem{Type: "function_call_output", CallID: call.CallID, Content: output}
	limit := 4096
	if model.MaxOutputTokens > 0 && model.MaxOutputTokens < limit {
		limit = model.MaxOutputTokens
	}
	toolAvailable := model.ToolCalling && s.agentToolCapabilityVerified(owner, p, model)
	mode := x.Mode
	conversationID := x.ConversationID
	if p.APIType == "chat_completions" {
		mode = agentchat.Stateless
		conversationID = ""
	} else if mode == agentchat.Conversation && !s.agentConversationCapabilityVerified(owner, p, model) {
		mode = agentchat.Stateless
		conversationID = ""
	}
	payload, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
		Model: x.ModelID, Instructions: agentChatInstructions(x.ProjectID, toolAvailable),
		Mode: mode, PreviousResponseID: x.PreviousResponseID, ConversationID: conversationID,
		History: x.History, Next: []agentchat.HistoryItem{next},
		MaxOutputTokens: limit, ReasoningEffort: agentProviderReasoningEffort(p, x.ReasoningEffort, model),
		EnablePatchTool: toolAvailable,
	})
	if err != nil {
		return agentDeliverResult{}, agentContinueError{413, "cannot format tool-result model request"}
	}
	// Persist output and mark delivered BEFORE crossing the network; a timeout
	// cannot result in accidental duplicate tool output or patch replay.
	x.History = append(x.History, next)
	x.ToolCalls[idx].FeedbackSent = true
	x.ToolCalls[idx].FeedbackStatus = "prepared"
	x.Status = "requesting_ai"
	x.AppendEvent("feedback_prepared", "requesting_ai", call.CallID, "")
	x, err = store.Save(x, x.Revision)
	if err != nil {
		return agentDeliverResult{}, agentContinueError{409, "cannot persist once-only tool output"}
	}
	x.ToolCalls[idx].FeedbackStatus = "sending"
	if x, err = store.Save(x, x.Revision); err != nil {
		return agentDeliverResult{}, agentContinueError{409, "cannot record tool-output delivery attempt"}
	}
	if !consent {
		// The scheduler must never send without a recorded user grant. Restore the
		// pending state so a later explicit continuation remains possible.
		x.ToolCalls[idx].FeedbackStatus = ""
		x.History = x.History[:len(x.History)-1]
		x.ToolCalls[idx].FeedbackSent = false
		x.Status = "waiting_tool"
		_, _ = store.Save(x, x.Revision)
		return agentDeliverResult{}, agentContinueError{403, "explicit external-provider consent is required"}
	}
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	releaseRequest := registerAgentRequest(agentRequestKey(s.Workspace, owner, x.ID), cancel)
	result, err := s.agentChatCallForOwner(callCtx, owner, p, string(secret), payload)
	releaseRequest()
	if err != nil {
		if errors.Is(callCtx.Err(), context.Canceled) {
			x.Status = "cancelled"
			x.ToolCalls[idx].FeedbackStatus = "unknown"
			_, _ = store.Save(x, x.Revision)
			return agentDeliverResult{}, agentContinueError{409, "continuation cancelled; the tool result was already recorded and will not be resent"}
		}
		x.Status = "failed"
		x.ToolCalls[idx].FeedbackStatus = "unknown"
		_, _ = store.Save(x, x.Revision)
		return agentDeliverResult{}, agentContinueError{502, "provider continuation failed; tool result was saved and will not be resent automatically"}
	}
	x.ToolCalls[idx].FeedbackStatus = "acknowledged"
	x.AppendEvent("feedback_acknowledged", x.Status, call.CallID, "")
	appendAgentProviderResult(&x, result, toolAvailable)
	x, err = store.Save(x, x.Revision)
	if err != nil {
		return agentDeliverResult{}, agentContinueError{409, "provider responded but journal write failed; inspect history before retry"}
	}
	return agentDeliverResult{Session: x, Result: result}, nil
}
