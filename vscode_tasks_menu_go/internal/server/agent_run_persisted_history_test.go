package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

type persistedHistoryExecutor struct{}

func (persistedHistoryExecutor) Execute(ctx context.Context, _ agentchat.RunSpec, _ []agentchat.HistoryItem, sink agentchat.EventSink) error {
	return sink.Publish(ctx, agentchat.RunEvent{Type: agentchat.RunEventTextDelta, Payload: []byte("persisted answer")})
}
func (persistedHistoryExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }

func TestContextSnapshotReadsPersistedRunHistoryWithoutOpeningManager(t *testing.T) {
	s, session, provider := prepareRunAPIServer(t, false)
	s.AgentRunExecutor = persistedHistoryExecutor{}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	spec := runSpecFor(s, session, provider)
	spec.RunID = "persisted-run"
	if _, err := manager.Submit(spec, []agentchat.HistoryItem{{Type: "user", Content: "persisted prompt"}}); err != nil {
		t.Fatal(err)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.Completed)
	manager.Close()

	restarted := &Server{Workspace: s.Workspace, Config: s.Config, ConnectionSecrets: s.ConnectionSecrets}
	if records, err := restarted.completedAgentRuns("local", session.ID); err != nil || len(records) != 1 || records[0].Status != agentrun.Completed {
		t.Fatalf("persisted run history unavailable without manager: records=%+v err=%v", records, err)
	}
	response := invokeAI(t, restarted.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{
		"conversation_id": session.ID, "prompt": "continue after restart",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("snapshot after restart: HTTP %d: %s", response.Code, response.Body)
	}
	var body struct {
		ID string `json:"context_snapshot_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ownerID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	project, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	store, err := restarted.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Get(body.ID, ownerID, session.ID, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.History) < 3 || snapshot.History[0].Content != "persisted prompt" || snapshot.History[1].Content != "persisted answer" || snapshot.History[len(snapshot.History)-1].Content != "continue after restart" {
		t.Fatalf("snapshot did not include durable run turn: %+v", snapshot.History)
	}
	if restarted.agentRunManagers != nil {
		t.Fatal("reading persisted history unexpectedly opened the run manager")
	}
}

func TestChildWorkspaceSnapshotResponseCanSubmitBoundRun(t *testing.T) {
	s, session, provider := prepareRunAPIServer(t, true)
	const runID = "snapshot-child-run"
	configVersion := agentchat.RunProviderConfigVersion(provider, provider.Models[0])
	worktreeResponse := invokeAI(t, func(w http.ResponseWriter, r *http.Request) {
		s.agentRunWorktreeAPI(w, r, "local")
	}, http.MethodPost, "/api/agent/runs/worktrees", map[string]any{
		"conversation_id": session.ID, "run_id": runID, "provider_id": provider.ID,
		"model_id": provider.Models[0].ID, "provider_config_version": configVersion,
		"permission_scope": []string{"files.read", "patch.propose", "patch.execute.approval_required"},
	})
	if worktreeResponse.Code != http.StatusCreated {
		t.Fatalf("create child workspace: HTTP %d: %s", worktreeResponse.Code, worktreeResponse.Body)
	}
	var worktree struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(worktreeResponse.Body.Bytes(), &worktree); err != nil {
		t.Fatal(err)
	}
	s.AgentChatPack = func(workspace, _ string) (patchtool.AIPackResult, error) {
		project, err := patchtool.ReadProjectIdentity(workspace)
		return patchtool.AIPackResult{TextContent: "child workspace context", ProjectIdentity: project}, err
	}
	snapshotResponse := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{
		"conversation_id": session.ID, "prompt": "inspect the child worktree",
		"provider_id": provider.ID, "model_id": provider.Models[0].ID,
		"run_id": runID, "execution_workspace_id": worktree.WorkspaceID,
	})
	if snapshotResponse.Code != http.StatusCreated {
		t.Fatalf("create child workspace snapshot: HTTP %d: %s", snapshotResponse.Code, snapshotResponse.Body)
	}
	var snapshot struct {
		ID                    string   `json:"context_snapshot_id"`
		OwnerWorkspaceID      string   `json:"owner_workspace_id"`
		ExecutionWorkspaceID  string   `json:"execution_workspace_id"`
		ParentTurnID          string   `json:"parent_turn_id"`
		ProviderConfigVersion string   `json:"provider_config_version"`
		CredentialFingerprint string   `json:"credential_fingerprint"`
		ExcludedPendingRunIDs []string `json:"excluded_pending_run_ids"`
		RetryMode             string   `json:"retry_mode"`
	}
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ExecutionWorkspaceID != worktree.WorkspaceID || snapshot.ExecutionWorkspaceID == snapshot.OwnerWorkspaceID {
		t.Fatalf("snapshot returned the wrong execution workspace: %+v", snapshot)
	}
	spec := runSpecFor(s, session, provider)
	spec.RunID = runID
	spec.OwnerWorkspaceID = snapshot.OwnerWorkspaceID
	spec.ExecutionWorkspaceID = snapshot.ExecutionWorkspaceID
	spec.ParentTurnID = snapshot.ParentTurnID
	spec.ProviderConfigVersion = snapshot.ProviderConfigVersion
	spec.CredentialFingerprint = snapshot.CredentialFingerprint
	spec.ContextSnapshotID = snapshot.ID
	spec.ExcludedPendingRunIDs = strings.Join(snapshot.ExcludedPendingRunIDs, ",")
	spec.RetryMode = agentchat.RetryMode(snapshot.RetryMode)
	response := invokeAI(t, s.agentRunAPI, http.MethodPost, "/api/agent/runs", map[string]any{"spec": spec, "confirm_external": true})
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit using snapshot response: HTTP %d: %s", response.Code, response.Body)
	}
	waitForAgentRunStatus(t, s.agentRunManagers["local"], runID, agentrun.Completed)
	select {
	case got := <-s.AgentRunExecutor.(*fakeAgentRunExecutor).started:
		if got.ExecutionWorkspaceID != worktree.WorkspaceID {
			t.Fatalf("submitted run used workspace %q, want %q", got.ExecutionWorkspaceID, worktree.WorkspaceID)
		}
	default:
		t.Fatal("run executor did not receive the snapshot-bound child workspace")
	}
}
