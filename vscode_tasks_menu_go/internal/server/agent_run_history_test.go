package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func TestCompletedAgentRunsMergeIntoSnapshotInRequestOrder(t *testing.T) {
	base := []agentchat.HistoryItem{{Type: "user", Content: "legacy prompt"}, {Type: "assistant", Content: "legacy answer"}}
	firstAt := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Second)
	makeRecord := func(runID, provider, model string, created time.Time, status agentrun.Status, events ...agentrun.Event) agentrun.Record {
		return agentrun.Record{
			Spec:   agentchat.RunSpec{RunID: runID, ConversationID: "conversation", ProviderID: provider, ModelID: model, ParentTurnID: "parent"},
			Status: status, CreatedAt: created,
			History: []agentchat.HistoryItem{{Type: "user", Content: "prompt " + runID}}, Events: events,
		}
	}
	records := []agentrun.Record{
		makeRecord("run-2", "gpt", "gpt-model", secondAt, agentrun.Completed,
			agentrun.Event{RunID: "run-2", AttemptID: "a1", Sequence: 1, Type: agentchat.RunEventTextDelta, Payload: []byte("discarded")},
			agentrun.Event{RunID: "run-2", AttemptID: "a1", Sequence: 2, Type: agentchat.RunEventRetry, Payload: []byte(`{"reset_partial_output":true}`)},
			agentrun.Event{RunID: "run-2", AttemptID: "a1", Sequence: 3, Type: agentchat.RunEventTextDelta, Payload: []byte("GPT answer")},
		),
		makeRecord("run-1", "composer", "composer-model", firstAt, agentrun.Completed,
			agentrun.Event{RunID: "run-1", AttemptID: "a1", Sequence: 1, Type: agentchat.RunEventTextDelta, Payload: []byte("Composer answer")},
		),
		makeRecord("pending", "gpt", "gpt-model", firstAt.Add(2*time.Second), agentrun.Running,
			agentrun.Event{RunID: "pending", AttemptID: "a1", Sequence: 1, Type: agentchat.RunEventTextDelta, Payload: []byte("partial pending")},
		),
	}
	merged := completedAgentRunHistory(base, records, "conversation")
	if len(merged) != 7 {
		t.Fatalf("merged history length=%d, want legacy pair + 2 completed run pairs + 1 pending prompt", len(merged))
	}
	if merged[2].OriginRunID != "run-1" || merged[3].Content != "Composer answer" || merged[3].ProviderID != "composer" ||
		merged[4].OriginRunID != "run-2" || merged[5].Content != "GPT answer" || merged[5].ProviderID != "gpt" {
		t.Fatalf("run history order/provenance incorrect: %+v", merged)
	}
	if merged[6].Type != "user" || merged[6].OriginRunID != "pending" {
		t.Fatalf("pending request prompt was not preserved: %+v", merged[6])
	}
	for _, item := range merged {
		if item.Content == "discarded" || item.Content == "partial pending" {
			t.Fatalf("pending/partial output leaked into a new snapshot: %+v", item)
		}
	}
}

type blockingAgentRunExecutor struct{ started chan struct{} }

func (e *blockingAgentRunExecutor) Execute(ctx context.Context, spec agentchat.RunSpec, _ []agentchat.HistoryItem, sink agentchat.EventSink) error {
	if err := sink.Publish(ctx, agentchat.RunEvent{RunID: spec.RunID, AttemptID: spec.AttemptID, ProviderID: spec.ProviderID,
		Type: agentchat.RunEventTextDelta, Payload: []byte("unfinished Composer output")}); err != nil {
		return err
	}
	close(e.started)
	<-ctx.Done()
	return ctx.Err()
}
func (*blockingAgentRunExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }

func TestContextSnapshotIncludesActiveRunPromptButExcludesItsPartialOutput(t *testing.T) {
	s, sessionID := newAgentConversationFixture(t, "stateless")
	executor := &blockingAgentRunExecutor{started: make(chan struct{})}
	s.AgentRunExecutor = executor
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providers.Get("local", "custom-r2d")
	if err != nil {
		t.Fatal(err)
	}
	model := provider.Models[0]
	spec := agentchat.RunSpec{RunID: "composer-run", ConversationID: sessionID, OwnerWorkspaceID: ownerWorkspaceID,
		ExecutionWorkspaceID: ownerWorkspaceID, ProviderID: provider.ID, ModelID: model.ID,
		ProviderConfigVersion: agentchat.RunProviderConfigVersion(provider, model), ContextSnapshotID: "snapshot-101",
		AttemptID: "composer-run-a1", IdempotencyKey: "request-101"}
	if _, err := manager.Submit(spec, []agentchat.HistoryItem{{Type: "user", Content: "Request #101 from Composer"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("Composer run did not start")
	}
	response := invokeAI(t, s.agentContextSnapshotAPI, "POST", "/api/agent/context-snapshots", map[string]any{
		"conversation_id": sessionID, "prompt": "Request #102 from GPT", "run_id": "gpt-run-102", "provider_id": provider.ID, "model_id": model.ID,
	})
	if response.Code != 201 {
		t.Fatalf("snapshot while prior run is active: %d %s", response.Code, response.Body)
	}
	var responseBody struct {
		ID                    string   `json:"context_snapshot_id"`
		ProjectID             string   `json:"project_id"`
		ParentTurnID          string   `json:"parent_turn_id"`
		ExcludedPendingRunIDs []string `json:"excluded_pending_run_ids"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &responseBody); err != nil {
		t.Fatal(err)
	}
	if responseBody.ParentTurnID != spec.RunID || len(responseBody.ExcludedPendingRunIDs) != 1 || responseBody.ExcludedPendingRunIDs[0] != spec.RunID {
		t.Fatalf("snapshot did not pin parent/excluded pending run IDs: %+v", responseBody)
	}
	store, err := s.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Get(responseBody.ID, ownerWorkspaceID, sessionID, responseBody.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.History) != 2 || snapshot.History[0].Content != "Request #101 from Composer" || snapshot.History[1].Content != "Request #102 from GPT" ||
		snapshot.History[1].ParentTurnID != spec.RunID || snapshot.History[1].ExcludedPendingRunIDs != spec.RunID {
		t.Fatalf("active-run snapshot did not preserve prompts/parent/exclusion metadata: %+v", snapshot.History)
	}
	for _, item := range snapshot.History {
		if item.Content == "unfinished Composer output" {
			t.Fatal("partial response from #101 leaked into #102 context")
		}
	}
	if err := manager.Stop(context.Background(), spec.RunID); err != nil {
		t.Fatal(err)
	}
}

func TestCompletedToolRunAddsNonExecutionNoticeToCanonicalHistory(t *testing.T) {
	record := agentrun.Record{
		Spec:   agentchat.RunSpec{RunID: "run-tool", ConversationID: "conversation", ProviderID: "gpt", ModelID: "model"},
		Status: agentrun.Completed, CreatedAt: time.Now(),
		History: []agentchat.HistoryItem{{Type: "user", Content: "inspect files"}},
		Events:  []agentrun.Event{{RunID: "run-tool", AttemptID: "a1", Sequence: 1, Type: agentchat.RunEventToolCall, Payload: []byte(`{"call_id":"c1"}`)}},
	}
	merged := completedAgentRunHistory(nil, []agentrun.Record{record}, "conversation")
	if len(merged) != 2 || merged[1].Type != "assistant" || merged[1].OriginRunID != "run-tool" ||
		!containsAll(merged[1].Content, "tool request", "not executed") {
		t.Fatalf("tool output safety notice missing: %+v", merged)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
