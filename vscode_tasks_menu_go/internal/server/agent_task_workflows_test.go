package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func TestManifestRunAllHonorsDAGAndRequiresTaskReview(t *testing.T) {
	s, _, provider := prepareRunAPIServer(t, true)
	version := agentProviderConfigVersionForTest(t, s, provider.ID, provider.Models[0].ID)
	markdown := "# Three task workflow\n" +
		"## Task A — foundation\nAcceptance:\n- A complete\n" +
		"## Task B — independent\nAcceptance:\n- B complete\n" +
		"## Task C — dependent\nDepends on: A\nAcceptance:\n- C complete\n"
	response := invokeAI(t, s.agentTaskManifestRunsAPI, http.MethodPost, "/api/agent/task-manifests/runs", map[string]any{
		"markdown": markdown, "provider_id": provider.ID, "model_id": provider.Models[0].ID,
		"provider_config_version": version, "concurrency": 2, "confirm_external": true,
	})
	if response.Code != http.StatusAccepted {
		t.Fatalf("Run All returned %d: %s", response.Code, response.Body)
	}
	var created struct {
		Workflow agentrun.WorkflowRecord `json:"workflow"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	owner := "local"
	coordinator, err := s.workflowCoordinator(owner, created.Workflow.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := s.agentRunManager(owner)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	waitWorkflowTaskStatuses(t, coordinator, map[string]agentrun.TaskStatus{"A": agentrun.TaskReview, "B": agentrun.TaskReview, "C": agentrun.TaskQueued})
	states := coordinator.dag.Snapshot().States
	if states["C"].WaitReason != "dependency" {
		t.Fatalf("C wait reason=%q, want dependency", states["C"].WaitReason)
	}
	if _, ok := manager.Get(states["C"].RunID); ok {
		t.Fatal("dependent task was dispatched before its dependency was approved")
	}
	if err := coordinator.decision("A", states["A"].Fence, true, "reviewed"); err != nil {
		t.Fatal(err)
	}
	waitWorkflowTaskStatuses(t, coordinator, map[string]agentrun.TaskStatus{"A": agentrun.TaskSucceeded, "B": agentrun.TaskReview, "C": agentrun.TaskReview})
	states = coordinator.dag.Snapshot().States
	for _, id := range []string{"B", "C"} {
		if err := coordinator.decision(id, states[id].Fence, true, "reviewed"); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		coordinator.mu.Lock()
		status := coordinator.record.Status
		coordinator.mu.Unlock()
		if status == agentrun.WorkflowCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	coordinator.mu.Lock()
	finalStatus := coordinator.record.Status
	coordinator.mu.Unlock()
	if finalStatus != agentrun.WorkflowCompleted {
		t.Fatalf("workflow status=%s, want completed", finalStatus)
	}
	for _, state := range coordinator.dag.Snapshot().States {
		if state.Status != agentrun.TaskSucceeded || state.RunID == "" || state.ContextSnapshotID == "" || state.ExecutionWorkspaceID == "" {
			t.Fatalf("task binding/review missing: %+v", state)
		}
	}
}

func TestWorkflowRejectsInvalidProviderVersionAndMissingConsent(t *testing.T) {
	s, _, provider := prepareRunAPIServer(t, true)
	base := map[string]any{"markdown": "# P\n## Task A — task\nAcceptance:\n- ok\n", "provider_id": provider.ID,
		"model_id": provider.Models[0].ID, "provider_config_version": "stale", "concurrency": 1, "confirm_external": true}
	w := invokeAI(t, s.agentTaskManifestRunsAPI, http.MethodPost, "/api/agent/task-manifests/runs", base)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale provider config accepted: %d %s", w.Code, w.Body)
	}
	base["confirm_external"] = false
	base["provider_config_version"] = agentProviderConfigVersionForTest(t, s, provider.ID, provider.Models[0].ID)
	w = invokeAI(t, s.agentTaskManifestRunsAPI, http.MethodPost, "/api/agent/task-manifests/runs", base)
	if w.Code != http.StatusForbidden {
		t.Fatalf("workflow started without egress consent: %d %s", w.Code, w.Body)
	}
}

func waitWorkflowTaskStatuses(t *testing.T, coordinator *taskWorkflowCoordinator, expected map[string]agentrun.TaskStatus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		states := coordinator.dag.Snapshot().States
		all := true
		for id, status := range expected {
			if states[id].Status != status {
				all = false
				break
			}
		}
		if all {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	states := coordinator.dag.Snapshot().States
	t.Fatalf("workflow states did not converge: %+v", states)
}

func agentProviderConfigVersionForTest(t *testing.T, s *Server, providerID, modelID string) string {
	t.Helper()
	store, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := store.Get("local", providerID)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := provider.FindModel(modelID)
	if !ok {
		t.Fatal("provider model missing")
	}
	return agentchat.RunProviderConfigVersion(provider, model)
}
