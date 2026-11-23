package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

type hotSwitchRunExecutor struct {
	started   chan agentchat.RunSpec
	failFirst chan struct{}
	mu        sync.Mutex
	calls     map[string]int
	cancelled []string
}

func (e *hotSwitchRunExecutor) Execute(ctx context.Context, spec agentchat.RunSpec, _ []agentchat.HistoryItem, sink agentchat.EventSink) error {
	e.mu.Lock()
	if e.calls == nil {
		e.calls = make(map[string]int)
	}
	e.calls[spec.RunID]++
	attempt := e.calls[spec.RunID]
	e.mu.Unlock()
	e.started <- spec
	if spec.RunID == "run-102" {
		return sink.Publish(ctx, agentchat.RunEvent{RunID: spec.RunID, AttemptID: spec.AttemptID, ProviderID: spec.ProviderID,
			Type: agentchat.RunEventTextDelta, Payload: []byte("GPT #102 response")})
	}
	if spec.RunID == "run-101" && attempt == 1 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.failFirst:
			return agentrun.RetrySafeError{Err: errors.New("provider was not invoked")}
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func (e *hotSwitchRunExecutor) Cancel(_ context.Context, spec agentchat.RunSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancelled = append(e.cancelled, spec.RunID)
	return nil
}

func TestComposerRunCanOverlapGPTRunRetryAndStopIndependently(t *testing.T) {
	s, session, _ := prepareRunAPIServer(t, false)
	projectIdentity, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	s.AgentChatPack = func(string, string) (patchtool.AIPackResult, error) {
		return patchtool.AIPackResult{TextContent: "pinned test workspace pack", ProjectIdentity: projectIdentity}, nil
	}
	composer := agentchat.ProviderConfig{ID: "composer", Name: "Composer", APIType: "responses", BaseURL: "https://composer.example.test/v1",
		Models: []agentchat.ModelConfig{{ID: "composer-2.5", Name: "Composer 2.5"}}}
	gpt := agentchat.ProviderConfig{ID: "gpt", Name: "GPT", APIType: "responses", BaseURL: "https://gpt.example.test/v1",
		Models: []agentchat.ModelConfig{{ID: "gpt-6.1-sol", Name: "GPT 6.1"}}}
	providerStore, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []agentchat.ProviderConfig{composer, gpt} {
		if err := providerStore.Put("local", provider); err != nil {
			t.Fatal(err)
		}
		if err := s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace, "local", provider.ID), []byte("test-secret-"+provider.ID)); err != nil {
			t.Fatal(err)
		}
	}
	executor := &hotSwitchRunExecutor{started: make(chan agentchat.RunSpec, 4), failFirst: make(chan struct{}, 1)}
	s.AgentRunExecutor = executor
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	makeSpec := func(runID string, provider agentchat.ProviderConfig, prompt string) agentchat.RunSpec {
		model := provider.Models[0]
		snapshotResponse := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{
			"conversation_id": session.ID, "prompt": prompt, "run_id": runID, "provider_id": provider.ID, "model_id": model.ID,
		})
		if snapshotResponse.Code != http.StatusCreated {
			t.Fatalf("snapshot %s: %d %s", runID, snapshotResponse.Code, snapshotResponse.Body)
		}
		var snapshot struct {
			ID                    string              `json:"context_snapshot_id"`
			ParentTurnID          string              `json:"parent_turn_id"`
			ExcludedPendingRunIDs []string            `json:"excluded_pending_run_ids"`
			RetryMode             agentchat.RetryMode `json:"retry_mode"`
			ProviderVersion       string              `json:"provider_config_version"`
			CredentialFingerprint string              `json:"credential_fingerprint"`
		}
		if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
			t.Fatal(err)
		}
		return agentchat.RunSpec{
			RunID: runID, ConversationID: session.ID, ParentTurnID: snapshot.ParentTurnID,
			OwnerWorkspaceID: ownerWorkspaceID, ExecutionWorkspaceID: ownerWorkspaceID,
			ProviderID: provider.ID, ModelID: model.ID, ProviderConfigVersion: snapshot.ProviderVersion,
			CredentialFingerprint: snapshot.CredentialFingerprint,
			ContextSnapshotID:     snapshot.ID, ExcludedPendingRunIDs: strings.Join(snapshot.ExcludedPendingRunIDs, ","),
			AttemptID: runID + "-a1", IdempotencyKey: "idem-" + runID, RetryMode: snapshot.RetryMode,
		}
	}
	submit := func(spec agentchat.RunSpec) *httptest.ResponseRecorder {
		return invokeAI(t, s.agentRunAPI, http.MethodPost, "/api/agent/runs", map[string]any{"spec": spec, "confirm_external": true})
	}
	spec101 := makeSpec("run-101", composer, "Request #101 via Composer")
	if response := submit(spec101); response.Code != http.StatusAccepted {
		t.Fatalf("submit #101: %d %s", response.Code, response.Body)
	}
	manager := s.agentRunManagers["local"]
	defer manager.Close()
	started101 := <-executor.started
	if started101.ProviderID != "composer" || started101.ModelID != "composer-2.5" {
		t.Fatalf("#101 binding: %+v", started101)
	}

	spec102 := makeSpec("run-102", gpt, "Request #102 via GPT")
	if spec102.ExcludedPendingRunIDs != spec101.RunID || spec102.ParentTurnID != spec101.RunID {
		t.Fatalf("#102 did not pin its pending-run exclusion and parent: %+v", spec102)
	}
	snapshotStore, err := s.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot102 agentrun.ContextSnapshot
	for _, record := range manager.List() {
		if record.Spec.RunID == spec102.RunID {
			t.Fatal("#102 was unexpectedly admitted before submit")
		}
	}
	if response := submit(spec102); response.Code != http.StatusAccepted {
		t.Fatalf("submit #102: %d %s", response.Code, response.Body)
	}
	started102 := <-executor.started
	if started102.ProviderID != "gpt" || started102.ModelID != "gpt-6.1-sol" {
		t.Fatalf("#102 binding: %+v", started102)
	}
	var errGet error
	snapshot102, errGet = snapshotStore.Get(spec102.ContextSnapshotID, ownerWorkspaceID, session.ID, session.ProjectID)
	if errGet != nil {
		t.Fatal(errGet)
	}
	if len(snapshot102.History) != 2 || snapshot102.History[0].Content != "Request #101 via Composer" || snapshot102.History[1].Content != "Request #102 via GPT" {
		t.Fatalf("#102 snapshot included the wrong pending-run context: %+v", snapshot102.History)
	}
	waitRunStatus(t, manager, spec102.RunID, agentrun.Completed)
	if record, ok := manager.Get(spec101.RunID); !ok || record.Status != agentrun.Running {
		t.Fatalf("#101 stopped when #102 completed: %+v", record)
	}
	if record, _ := manager.Get(spec102.RunID); len(record.Events) == 0 || string(record.Events[0].Payload) != "GPT #102 response" {
		t.Fatalf("#102 output was not pinned to its run: %+v", record.Events)
	}

	executor.failFirst <- struct{}{}
	waitRunStatus(t, manager, spec101.RunID, agentrun.Failed)
	if record, ok := manager.Get(spec101.RunID); !ok || !record.RetrySafe {
		t.Fatalf("#101 failure was not safely retryable: %+v", record)
	}
	retryResponse := invokeAI(t, s.agentRunItemAPI, http.MethodPost, "/api/agent/runs/"+spec101.RunID+"/retry", map[string]any{})
	if retryResponse.Code != http.StatusAccepted {
		t.Fatalf("retry #101: %d %s", retryResponse.Code, retryResponse.Body)
	}
	retried101 := <-executor.started
	if retried101.ProviderID != spec101.ProviderID || retried101.ModelID != spec101.ModelID || retried101.ProviderConfigVersion != spec101.ProviderConfigVersion || retried101.AttemptID == spec101.AttemptID {
		t.Fatalf("retry #101 changed pinned binding: %+v", retried101)
	}
	stopResponse := invokeAI(t, s.agentRunItemAPI, http.MethodPost, "/api/agent/runs/"+spec101.RunID+"/stop", map[string]any{})
	if stopResponse.Code != http.StatusOK {
		t.Fatalf("stop retried #101: %d %s", stopResponse.Code, stopResponse.Body)
	}
	waitRunStatus(t, manager, spec101.RunID, agentrun.Stopped)
	if record, ok := manager.Get(spec102.RunID); !ok || record.Status != agentrun.Completed {
		t.Fatalf("Stop #101 affected #102: %+v", record)
	}
	executor.mu.Lock()
	cancelled := append([]string(nil), executor.cancelled...)
	executor.mu.Unlock()
	if len(cancelled) != 1 || cancelled[0] != spec101.RunID {
		t.Fatalf("Stop routed to the wrong run: %+v", cancelled)
	}
}

func waitRunStatus(t *testing.T, manager *agentrun.Manager, runID string, expected agentrun.Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if record, ok := manager.Get(runID); ok && record.Status == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	record, _ := manager.Get(runID)
	t.Fatalf("run %s status=%s, want %s", runID, record.Status, expected)
}
