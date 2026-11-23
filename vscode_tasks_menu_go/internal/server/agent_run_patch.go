package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func (s *Server) agentRunToolDecisionAPI(w http.ResponseWriter, r *http.Request, owner, runID, callID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionPatchRun, "agent.run.patch_decision", runID) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var decisionRequest struct {
		Action           string `json:"action"`
		ConfirmExecution bool   `json:"confirm_execution"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decisionRequest); err != nil || (decisionRequest.Action != "approve" && decisionRequest.Action != "reject") {
		http.Error(w, "action must be approve or reject", 400)
		return
	}
	var extra any
	if !errors.Is(dec.Decode(&extra), io.EOF) {
		http.Error(w, "exactly one decision object required", 400)
		return
	}
	if decisionRequest.Action == "approve" && !decisionRequest.ConfirmExecution {
		http.Error(w, "explicit Patch execution approval required", 403)
		return
	}
	manager, err := s.agentRunManager(owner)
	if err != nil {
		http.Error(w, "Agent Run Manager unavailable", 503)
		return
	}
	run, ok := manager.Get(runID)
	if !ok || run.Status != agentrun.WaitingTool {
		http.Error(w, "Agent Run is not awaiting tool approval", 409)
		return
	}
	inputs, err := s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(r.Context(), owner), run.Spec)
	if err != nil {
		http.Error(w, "run workspace binding is no longer valid", 409)
		return
	}
	if !scopeIncludes(inputs.Binding.PermissionScope, "patch.propose") || !scopeIncludes(inputs.Binding.PermissionScope, "patch.execute.approval_required") {
		http.Error(w, "run workspace does not grant approval-controlled Patch", 403)
		return
	}
	session, err := s.syncAgentRunConversation(owner, run, inputs.ExecutionWorkspacePath)
	if err != nil {
		http.Error(w, "Agent Run tool call could not be synchronized", 409)
		return
	}
	child := s.agentExecutionServer(inputs.ExecutionWorkspacePath)
	bodyObject := map[string]any{"action": decisionRequest.Action, "expected_revision": session.Revision}
	if decisionRequest.Action == "approve" {
		bodyObject["confirm_execution"] = true
	}
	body, _ := json.Marshal(bodyObject)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/api/agent/sessions/"+session.ID+"/tools/"+callID+"/decision", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "cannot create tool decision", 500)
		return
	}
	request = markInternalAgentRunRequest(request)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	child.agentChatToolDecisionAPI(response, request, session.ID, callID)
	if response.Code < 200 || response.Code >= 300 {
		http.Error(w, response.Body.String(), response.Code)
		return
	}
	var decision map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &decision)
	if decision["status"] == "rejected" {
		if err := s.continueAgentRunFromNativeResults(r.Context(), owner, runID, inputs, child); err != nil {
			http.Error(w, "rejected tool result could not continue run", 409)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, response.Code, decision)
}

var errAgentRunPatchPending = errors.New("Agent Run Patch is still active")

func scopeIncludes(scope []string, expected string) bool {
	for _, item := range scope {
		if item == expected {
			return true
		}
	}
	return false
}

func (s *Server) agentRunToolContinueAPI(w http.ResponseWriter, r *http.Request, owner, runID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionPatchView, "agent.run.patch_reconcile", runID) {
		return
	}
	if err := s.continueAgentRunFromNativeResults(r.Context(), owner, runID, AgentRunInputs{}, nil); err != nil {
		if errors.Is(err, errAgentRunPatchPending) {
			if manager, managerErr := s.agentRunManager(owner); managerErr == nil {
				if record, ok := manager.Get(runID); ok {
					writeJSON(w, http.StatusAccepted, map[string]any{"run": record, "status": "waiting_tool"})
					return
				}
			}
		}
		http.Error(w, "native Patch result is not ready or cannot be reconciled", 409)
		return
	}
	manager, err := s.agentRunManager(owner)
	if err != nil {
		http.Error(w, "Agent Run Manager unavailable", 503)
		return
	}
	if record, ok := manager.Get(runID); ok {
		writeJSON(w, 202, map[string]any{"run": record})
		return
	}
	http.Error(w, "Agent Run not found", 404)
}

func (s *Server) continueAgentRunFromNativeResults(ctx context.Context, owner, runID string, inputs AgentRunInputs, child *Server) error {
	manager, err := s.agentRunManager(owner)
	if err != nil {
		return err
	}
	run, ok := manager.Get(runID)
	if !ok || (run.Status != agentrun.WaitingTool && run.Status != agentrun.Stopped && run.Status != agentrun.Interrupted) {
		return errors.New("run is not waiting on or recovering a Patch")
	}
	if inputs.ExecutionWorkspacePath == "" {
		inputs, err = s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(ctx, owner), run.Spec)
		if err != nil {
			return err
		}
	}
	if child == nil {
		child = s.agentExecutionServer(inputs.ExecutionWorkspacePath)
	}
	session, err := s.syncAgentRunConversation(owner, run, inputs.ExecutionWorkspacePath)
	if err != nil {
		return err
	}
	if hasUnresolvedRunToolCalls(run) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/api/agent/sessions/"+session.ID+"/sync", nil)
		w := httptest.NewRecorder()
		child.agentChatSyncAPI(w, req, session.ID)
		if w.Code != 200 {
			return fmt.Errorf("native Patch reconciliation returned HTTP %d", w.Code)
		}
		sessionStore, storeErr := s.agentChatStore()
		if storeErr != nil {
			return storeErr
		}
		session, err = sessionStore.Get(run.Spec.ConversationID, owner)
		if err != nil {
			return err
		}
	}
	pending := map[string]bool{}
	for _, item := range run.History {
		if item.Type == "function_call" {
			pending[item.CallID] = true
		}
		if item.Type == "function_call_output" {
			delete(pending, item.CallID)
		}
	}
	results := make([]agentchat.HistoryItem, 0, len(pending))
	for callID := range pending {
		found := false
		for _, call := range session.ToolCalls {
			if call.CallID != callID {
				continue
			}
			found = true
			switch call.Status {
			case "done", "error", "rejected":
				result := call.Result
				if result == "" {
					return errors.New("final Patch result is empty")
				}
				results = append(results, agentchat.HistoryItem{Type: "function_call_output", CallID: callID, Content: result})
			case "pending", "queuing", "queued", "dispatching", "running":
				return errAgentRunPatchPending
			default:
				return errors.New("Patch result is unknown; explicit manual recovery is required")
			}
			break
		}
		if !found {
			return errors.New("run Patch call is missing from its conversation")
		}
	}
	if len(results) == 0 {
		return errors.New("no unresolved Patch calls")
	}
	history := append([]agentchat.HistoryItem(nil), run.History...)
	history = append(history, results...)
	snapshots, err := s.agentContextSnapshotStore(owner)
	if err != nil {
		return err
	}
	sources := append([]string(nil), inputs.Snapshot.SourceRefs...)
	for _, result := range results {
		sources = append(sources, "native-tool-result:"+result.CallID)
	}
	snapshot, err := snapshots.CreateWithPolicy(run.Spec.OwnerWorkspaceID, run.Spec.ConversationID, inputs.ProjectID, history, sources, inputs.Snapshot.Policy)
	if err != nil {
		return err
	}
	_, err = manager.ContinueWithToolResults(runID, results, snapshot.ID)
	if err != nil {
		return err
	}
	s.resumeWorkflowForContinuedRun(owner, runID)
	session.History = history
	session.Status = "requesting_ai"
	_, _ = s.agentChatStore()
	store, err := s.agentChatStore()
	if err == nil {
		_, _ = store.Save(session, session.Revision)
	}
	return nil
}
