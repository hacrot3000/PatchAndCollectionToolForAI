package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
	sessionpkg "bletonfc/vscode_tasks_menu/internal/session"
)

// providerToolCallInvoker answers the first provider request with a real
// taskdeck_submit_patch function call and the continuation with plain text.
type providerToolCallInvoker struct {
	calls       atomic.Int32
	patchPython string
	toolSchemas []string
	payloads    [][]byte
}

func (i *providerToolCallInvoker) Invoke(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
	i.calls.Add(1)
	i.payloads = append(i.payloads, append([]byte(nil), payload...))
	i.recordTools(payload)
	if i.calls.Load() == 1 {
		arguments, err := json.Marshal(map[string]string{"name": "provider patch", "patch_python": i.patchPython})
		if err != nil {
			return agentchat.ResponsesResult{}, err
		}
		return agentchat.ResponsesResult{ResponseID: "resp-1", Calls: []agentchat.FunctionCall{{
			CallID: "provider-call-1", Name: "taskdeck_submit_patch", Arguments: string(arguments),
		}}}, nil
	}
	return agentchat.ResponsesResult{ResponseID: "resp-2", Text: "Patch result reviewed"}, nil
}

func (i *providerToolCallInvoker) recordTools(payload []byte) {
	var body struct {
		Tools []map[string]any `json:"tools"`
	}
	if json.Unmarshal(payload, &body) != nil {
		return
	}
	for _, tool := range body.Tools {
		if name, ok := tool["name"].(string); ok {
			i.toolSchemas = append(i.toolSchemas, name)
		}
	}
}

// providerToolRunFixture prepares a server whose provider transport returns a
// real Native Patch function call and returns the pinned run binding.
func providerToolRunFixture(t *testing.T, runID string) (*Server, string, *providerToolCallInvoker, agentchat.RunSpec) {
	t.Helper()
	s, sessionID := newAgentConversationFixture(t, "stateless")
	project, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	patchSource := fmt.Sprintf("# TASKDECK_PROJECT_KEY: %s\n# TASKDECK_PROJECT_ID: %s\n# provider proposed patch\n", project.Key, project.ID)
	invoker := &providerToolCallInvoker{patchPython: patchSource}
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		return invoker.Invoke(ctx, p, key, payload)
	}
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Get(sessionID, "local")
	if err != nil {
		t.Fatal(err)
	}
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providers.Get("local", session.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := provider.FindModel(session.ModelID)
	if !ok {
		t.Fatal("session model is missing")
	}
	ownerID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	configVersion := agentchat.RunProviderConfigVersion(provider, model)
	credentialFingerprint := agentchat.RunCredentialFingerprint([]byte("TEST-KEY"))
	parentTurnID := "turn-" + sessionID + "-" + runID
	snapshotStore, err := s.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	packRef, err := agentRunWorkspacePackSourceRef(s.Workspace, session.ProjectID, s.AgentChatPack)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotStore.CreateWithPolicy(ownerID, sessionID, session.ProjectID,
		[]agentchat.HistoryItem{{Type: "user", Content: "propose the reviewed patch", ParentTurnID: parentTurnID}}, []string{packRef},
		&agentrun.ContextSnapshotPolicy{
			SystemInstructions: "Pinned Agent Run system policy.", RetryMode: agentchat.RetryOff,
			RunBinding: &agentrun.RunBindingSnapshot{RunID: runID, ConversationID: sessionID,
				OwnerWorkspaceID: ownerID, ExecutionWorkspaceID: ownerID, ParentTurnID: parentTurnID,
				ProviderID: provider.ID, ModelID: model.ID, ProviderConfigVersion: configVersion,
				CredentialFingerprint: credentialFingerprint, RetryMode: agentchat.RetryOff},
		})
	if err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{RunID: runID, ConversationID: sessionID, OwnerWorkspaceID: ownerID,
		ExecutionWorkspaceID: ownerID, ProviderID: provider.ID, ModelID: model.ID,
		ProviderConfigVersion: configVersion, CredentialFingerprint: credentialFingerprint,
		ParentTurnID: parentTurnID, RetryMode: agentchat.RetryOff, ContextSnapshotID: snapshot.ID,
		AttemptID: "attempt-1", IdempotencyKey: "request-" + runID}
	return s, sessionID, invoker, spec
}

func TestProviderToolCallReachesRunJournalWithProvenance(t *testing.T) {
	s, _, invoker, spec := providerToolRunFixture(t, "run-provider-tool")
	options := serverAgentRunPromptOptions{server: s, owner: "local"}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providers.Get("local", spec.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := provider.FindModel(spec.ModelID)
	if !ok {
		t.Fatal("session model is missing")
	}
	support, err := options.ToolSupport(spec, provider, model)
	if err != nil {
		t.Fatal(err)
	}
	if !support.PatchTool {
		t.Fatal("expected the Native Patch tool to be advertised for this verified model")
	}
	history := []agentchat.HistoryItem{{Type: "user", Content: "propose the reviewed patch", ParentTurnID: spec.ParentTurnID}}
	if _, err := manager.Submit(spec, history); err != nil {
		t.Fatal(err)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.WaitingTool)
	record, _ := manager.Get(spec.RunID)
	if len(record.History) == 0 {
		t.Fatal("run history is empty after the provider tool call")
	}
	last := record.History[len(record.History)-1]
	if last.Type != "function_call" || last.CallID != "provider-call-1" || last.Name != "taskdeck_submit_patch" {
		t.Fatalf("provider tool call was not persisted in the run: %+v", last)
	}
	if last.OriginRunID != spec.RunID || last.ProviderID != spec.ProviderID {
		t.Fatalf("tool call lost its run/provider provenance: %+v", last)
	}
	if len(invoker.toolSchemas) == 0 || invoker.toolSchemas[0] != "taskdeck_submit_patch" {
		t.Fatalf("provider did not receive the advertised tool schema: %+v", invoker.toolSchemas)
	}
}

func TestProviderToolCallExecutesNativePatchOnceAndContinuesWithToolResult(t *testing.T) {
	s, sessionID, invoker, spec := providerToolRunFixture(t, "run-provider-native")
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	history := []agentchat.HistoryItem{{Type: "user", Content: "propose the reviewed patch", ParentTurnID: spec.ParentTurnID}}
	if _, err := manager.Submit(spec, history); err != nil {
		t.Fatal(err)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.WaitingTool)

	runtimeDir := t.TempDir()
	entry, python := filepath.Join(runtimeDir, "python_patch_entry.py"), filepath.Join(runtimeDir, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)
	fakeNative := &fakeAgentNativeBridge{workspace: s.Workspace}
	s.Sessions = fakeNative

	decision := invokeAI(t, func(w http.ResponseWriter, r *http.Request) {
		s.agentRunToolDecisionAPI(w, r, "local", spec.RunID, "provider-call-1")
	}, http.MethodPost, "/api/agent/runs/"+spec.RunID+"/tools/provider-call-1/decision",
		map[string]any{"action": "approve", "confirm_execution": true})
	if decision.Code != http.StatusOK {
		t.Fatalf("approved provider Patch was not dispatched: %d %s", decision.Code, decision.Body)
	}
	if fakeNative.started != 1 {
		t.Fatalf("native Patch did not run exactly once: %d", fakeNative.started)
	}
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.Get(sessionID, "local")
	if err != nil {
		t.Fatal(err)
	}
	if len(conversation.ToolCalls) != 1 || conversation.ToolCalls[0].CallID != "provider-call-1" {
		t.Fatalf("provider tool call was not bridged to the conversation ledger: %+v", conversation.ToolCalls)
	}
	call := conversation.ToolCalls[0]
	rc := 0
	fakeNative.state.Items = []sessionpkg.ProtocolItemState{{Index: call.SelectedIndex, Name: filepath.Base(call.QueuePath), Kind: "PATCH", Status: "PASS", RunID: "native-provider-run", RC: &rc}}
	continued := invokeAI(t, func(w http.ResponseWriter, r *http.Request) { s.agentRunToolContinueAPI(w, r, "local", spec.RunID) },
		http.MethodPost, "/api/agent/runs/"+spec.RunID+"/tools/provider-call-1/continue", map[string]any{})
	if continued.Code != http.StatusAccepted {
		t.Fatalf("native result continuation: %d %s", continued.Code, continued.Body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if current, ok := manager.Get(spec.RunID); ok && current.Status != agentrun.Running && current.Status != agentrun.Queued {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	record, _ := manager.Get(spec.RunID)
	if record.Status != agentrun.Completed {
		t.Fatalf("Agent Run status=%s want completed; last error=%q history=%+v", record.Status, record.LastError, record.History)
	}
	if len(record.History) < 3 || record.History[len(record.History)-1].Type != "function_call_output" ||
		record.History[len(record.History)-1].CallID != "provider-call-1" {
		t.Fatalf("native result was not returned to the provider history: %+v", record.History)
	}
	if invoker.calls.Load() != 2 {
		t.Fatalf("expected exactly one follow-up provider request, got %d", invoker.calls.Load())
	}
	if len(invoker.payloads) != 2 {
		t.Fatalf("captured %d provider payloads, want initial and continuation", len(invoker.payloads))
	}
	var continuation struct {
		Input []struct {
			Role    string `json:"role"`
			Type    string `json:"type"`
			Content string `json:"content"`
			Output  string `json:"output"`
			CallID  string `json:"call_id"`
		} `json:"input"`
	}
	if err := json.Unmarshal(invoker.payloads[1], &continuation); err != nil {
		t.Fatalf("decode actual continuation provider payload: %v", err)
	}
	if len(continuation.Input) != 3 || continuation.Input[0].Role != "user" ||
		continuation.Input[1].Type != "function_call" || continuation.Input[1].CallID != "provider-call-1" ||
		continuation.Input[2].Type != "function_call_output" || continuation.Input[2].CallID != "provider-call-1" ||
		!strings.Contains(continuation.Input[2].Output, "PASS") {
		t.Fatalf("second provider request omitted/reordered the Native Patch result: %s", invoker.payloads[1])
	}
	if !strings.Contains(record.History[len(record.History)-1].Content, "PASS") {
		t.Fatalf("tool result did not carry the native outcome: %+v", record.History[len(record.History)-1])
	}
}
