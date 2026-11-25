package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// A run's original provider binding is not the legacy Session.ProviderID.
// Fail closed when the durable run ledger is unavailable. This fence
// separates the legacy scheduler from provider-bound run continuation.
func (s *Server) agentConversationRequiresRunContinuation(owner, id string) bool {
	records, err := s.completedAgentRuns(owner, id)
	return err != nil || len(records) != 0
}

func (s *Server) agentRunCollectionAllowed(owner string, spec agentchat.RunSpec, inputs AgentRunInputs) bool {
	if s.Config.SharedServerEnabled || owner != "local" ||
		(s.Sessions == nil && s.AgentCollectionCall == nil) ||
		!scopeIncludes(inputs.Binding.PermissionScope, "files.read") || inputs.Snapshot.Policy == nil {
		return false
	}
	// Only new snapshots with explicit pinned consent may enable this bridge.
	pinned := inputs.Snapshot.Policy.Automation.AutoCollection
	if pinned == nil || !*pinned {
		return false
	}
	providers, err := s.agentChatProviders()
	if err != nil { return false }
	provider, model, err := providers.GetVersion(owner, spec.ProviderID, spec.ModelID, spec.ProviderConfigVersion)
	if err != nil || !model.ToolCalling || !s.agentToolCapabilityVerified(owner, provider, model) { return false }
	store, err := s.agentChatStore()
	if err != nil {
		return false
	}
	x, err := store.Get(spec.ConversationID, owner)
	return err == nil && !x.Archived && x.ProjectID == inputs.ProjectID &&
		(x.AutoCollection == nil || *x.AutoCollection)
}

func pendingRunCollection(history []agentchat.HistoryItem) (agentchat.HistoryItem, bool) {
	pending := map[string]agentchat.HistoryItem{}
	for _, item := range history {
		switch item.Type {
		case "function_call":
			pending[item.CallID] = item
		case "function_call_output":
			delete(pending, item.CallID)
		}
	}
	if len(pending) != 1 {
		return agentchat.HistoryItem{}, false
	}
	for _, call := range pending {
		if call.Name == "taskdeck_collect_workspace" {
			return call, true
		}
	}
	return agentchat.HistoryItem{}, false
}

func runCollectionDiagnostic(code, message string, retryAllowed bool) string {
	body, _ := json.Marshal(map[string]any{"error": map[string]any{
		"code": code, "message": message, "retry_allowed": retryAllowed,
	}})
	return string(body)
}

// Called only by the active run worker after a complete provider turn has
// persisted its canonical call. The manager persists the returned result
// and new snapshot before issuing another provider request. Interrupted
// reads are not automatically replayed. No synthetic success receipt is made.
func (e ownerBoundAgentRunExecutor) ResolveWaitingTool(ctx context.Context, run agentrun.Record) ([]agentchat.HistoryItem, string, error) {
	call, ok := pendingRunCollection(run.History)
	if !ok {
		return e.resolveRunDeniedTools(ctx, run) // Native Patch approval remains manual.
	}
	if call.OriginRunID != run.Spec.RunID { return nil, "", errors.New("Collection call has no exact run provenance") }
	s := e.server
	inputs, err := s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(ctx, e.owner), run.Spec)
	if err != nil {
		return nil, "", err
	}
	identity, err := patchtool.ReadProjectIdentity(inputs.ExecutionWorkspacePath)
	if err != nil || identity.ID != inputs.ProjectID {
		return nil, "", errors.New("native Collection workspace identity mismatch")
	}
	release, acquired := lockAgentTurn(inputs.OwnerWorkspacePath, e.owner, run.Spec.ConversationID)
	if !acquired {
		return nil, "", errors.New("conversation is busy; native Collection was not started")
	}
	defer release()
	observed := ""
	request, validationErr := agentNativeCollectionRequest(call.Arguments, identity, false)
	if !s.agentRunCollectionAllowed(e.owner, run.Spec, inputs) {
		observed = runCollectionDiagnostic("COLLECTION_NOT_AUTHORIZED", "Native Collection is disabled or outside the approved run scope; no files were read.", false)
	} else if validationErr != nil {
		reason := scrubAIContext(validationErr.Error(), 512)
		if strings.Contains(strings.ToLower(reason), "identity") || strings.Contains(strings.ToLower(reason), "project.key") {
			return nil, "", errors.New("native Collection project identity rejected")
		}
		observed = runCollectionDiagnostic("INVALID_COLLECTION_REQUEST", reason, true)
	} else {
		repeats := 0
		for _, item := range run.History {
			if item.Type == "function_call" && item.Name == call.Name {
				previous, parseErr := agentNativeCollectionRequest(item.Arguments, identity, false)
				if parseErr == nil && string(previous) == string(request) {
					repeats++
				}
			}
		}
		if repeats > 3 {
			return nil, "", errors.New("repeated identical Collection without progress; review required")
		}
		if repeats == 3 {
			observed = runCollectionDiagnostic("COLLECTION_NO_PROGRESS", "Choose a narrower native read-only action instead of repeating the same request.", true)
		} else if s.AgentCollectionCall != nil {
			// Same test-only seam as the legacy native Collection flow.
			observed = s.AgentCollectionCall(inputs.ExecutionWorkspacePath, identity, request)
		} else {
			child := s.agentExecutionServer(inputs.ExecutionWorkspacePath)
			observed, err = child.agentRunNativeCollection(ctx, e.owner, request, identity)
			if err != nil {
				var schemaErr *agentNativeCollectValidationError
				repairable := errors.As(err, &schemaErr)
				code := "NATIVE_COLLECTION_FAILED"
				if repairable {
					code = "COLLECT_SCHEMA_INVALID"
				}
				observed = runCollectionDiagnostic(code, scrubAIContext(err.Error(), 700), repairable)
			}
		}
	}
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	if len(observed) == 0 || len(observed) > 512<<10 || !json.Valid([]byte(observed)) {
		observed = runCollectionDiagnostic("COLLECTION_RESULT_UNAVAILABLE", "No bounded verified native observation was returned. Inspect native history; do not claim success or replay an ambiguous operation.", false)
	}
	result := agentchat.HistoryItem{Type: "function_call_output", CallID: call.CallID, Content: observed,
		OriginRunID: run.Spec.RunID, ProviderID: run.Spec.ProviderID, ModelID: run.Spec.ModelID, ParentTurnID: run.Spec.ParentTurnID}
	history := append(append([]agentchat.HistoryItem(nil), run.History...), result)
	store, err := s.agentContextSnapshotStore(e.owner)
	if err != nil {
		return nil, "", err
	}
	// Preserve the original pinned prompt and workspace binding. Do not
	// silently rebase onto a changed AI Pack after a tool operation.
	snapshot, err := store.CreateWithPolicy(run.Spec.OwnerWorkspaceID, run.Spec.ConversationID, inputs.ProjectID, history, inputs.Snapshot.SourceRefs, inputs.Snapshot.Policy)
	if err != nil {
		return nil, "", err
	}
	return []agentchat.HistoryItem{result}, snapshot.ID, nil
}

// An unsupported/mixed batch never reaches the native Patch gateway. Return
// exact call-ID denials through the already authorized active run, using
// the same snapshot and provider binding as Collection. No command runs.
func (e ownerBoundAgentRunExecutor) resolveRunDeniedTools(ctx context.Context, run agentrun.Record) ([]agentchat.HistoryItem, string, error) {
	pending := make(map[string]agentchat.HistoryItem)
	for _, item := range run.History {
		if item.Type == "function_call" { pending[item.CallID] = item }
		if item.Type == "function_call_output" { delete(pending, item.CallID) }
	}
	if len(pending) == 0 { return nil, "", nil }
	if len(pending) == 1 {
		for _, call := range pending {
			if call.Name == "taskdeck_submit_patch" { return nil, "", nil }
		}
	}
	if agentConsecutiveDenialOutputs(run.History) >= agentMaxConsecutiveDenials {
		return nil, "", errors.New("repeated unsupported tool requests; no tool was executed; manual review required")
	}
	if ctx.Err() != nil { return nil, "", ctx.Err() }
	s := e.server
	inputs, err := s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(ctx, e.owner), run.Spec)
	if err != nil { return nil, "", err }
	if inputs.Snapshot.Policy == nil { return nil, "", errors.New("run policy unavailable") }
	code := "UNSUPPORTED_TOOL"
	if len(pending) > 1 { code = "MULTIPLE_TOOL_CALLS" }
	results := make([]agentchat.HistoryItem, 0, len(pending))
	for _, call := range run.History {
		if call.Type != "function_call" { continue }
		if _, ok := pending[call.CallID]; !ok { continue }
		if call.OriginRunID != run.Spec.RunID || call.ProviderID != run.Spec.ProviderID ||
			call.ModelID != run.Spec.ModelID || call.CallID == "" {
			return nil, "", errors.New("unsupported tool call is not bound to the active run")
		}
		results = append(results, agentchat.HistoryItem{Type:"function_call_output",
			CallID:call.CallID, Content:agentRejectedUnsupportedToolFeedback(call.Name, code),
			OriginRunID:run.Spec.RunID, ProviderID:run.Spec.ProviderID, ModelID:run.Spec.ModelID,
			ParentTurnID:run.Spec.ParentTurnID})
		delete(pending,call.CallID)
	}
	history := append(append([]agentchat.HistoryItem(nil),run.History...),results...)
	store,err := s.agentContextSnapshotStore(e.owner)
	if err != nil { return nil,"",err }
	snapshot,err := store.CreateWithPolicy(run.Spec.OwnerWorkspaceID,run.Spec.ConversationID,inputs.ProjectID,history,inputs.Snapshot.SourceRefs,inputs.Snapshot.Policy)
	if err != nil { return nil,"",err }
	return results,snapshot.ID,nil
}
