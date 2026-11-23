package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestContextSnapshotRejectsChildWorkspaceWithoutProviderRunBinding(t *testing.T) {
	s, conversation, provider := prepareRunAPIServer(t, true)
	const runID = "unbound-child-snapshot"
	response := invokeAI(t, func(w http.ResponseWriter, r *http.Request) {
		s.agentRunWorktreeAPI(w, r, "local")
	}, http.MethodPost, "/api/agent/runs/worktrees", map[string]any{
		"conversation_id": conversation.ID, "run_id": runID, "provider_id": provider.ID,
		"model_id":                provider.Models[0].ID,
		"provider_config_version": agentchat.RunProviderConfigVersion(provider, provider.Models[0]),
		"permission_scope":        []string{"files.read", "patch.propose", "patch.execute.approval_required"},
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create child workspace: HTTP %d: %s", response.Code, response.Body)
	}
	var worktree struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &worktree); err != nil {
		t.Fatal(err)
	}
	snapshot := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{
		"conversation_id": conversation.ID, "prompt": "unbound child context",
		"execution_workspace_id": worktree.WorkspaceID,
	})
	if snapshot.Code != http.StatusBadRequest {
		t.Fatalf("unbound child workspace returned HTTP %d: %s", snapshot.Code, snapshot.Body)
	}
}
