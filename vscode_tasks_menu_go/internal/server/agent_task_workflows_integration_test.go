package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

type workflowScriptExecutor struct {
	mu         sync.Mutex
	calls      map[string]int
	started    chan agentchat.RunSpec
	failFirst  bool
	failedOnce bool
	block      bool
}

func (e *workflowScriptExecutor) Execute(ctx context.Context, spec agentchat.RunSpec, _ []agentchat.HistoryItem, _ agentchat.EventSink) error {
	e.mu.Lock()
	if e.calls == nil {
		e.calls = map[string]int{}
	}
	e.calls[spec.RunID]++
	fail := e.failFirst && !e.failedOnce
	if fail {
		e.failedOnce = true
	}
	e.mu.Unlock()
	if e.started != nil {
		e.started <- spec
	}
	if e.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if fail {
		return agentrun.RetrySafeError{Err: errors.New("pre-egress validation failure")}
	}
	return nil
}
func (*workflowScriptExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }
func (e *workflowScriptExecutor) count(runID string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls[runID]
}

func submitTestWorkflow(t *testing.T, s *Server, provider agentchat.ProviderConfig, markdown string, concurrency int) agentrun.WorkflowRecord {
	t.Helper()
	response := invokeAI(t, s.agentTaskManifestRunsAPI, http.MethodPost, "/api/agent/task-manifests/runs", map[string]any{
		"markdown": markdown, "provider_id": provider.ID, "model_id": provider.Models[0].ID,
		"provider_config_version": agentProviderConfigVersionForTest(t, s, provider.ID, provider.Models[0].ID),
		"concurrency":             concurrency, "confirm_external": true,
	})
	if response.Code != http.StatusAccepted {
		t.Fatalf("workflow start returned %d: %s", response.Code, response.Body)
	}
	var result struct {
		Workflow agentrun.WorkflowRecord `json:"workflow"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Workflow
}

func TestWorkflowQueueDistinguishesDependencyAndGlobalSlotWait(t *testing.T) {
	s, _, provider := prepareRunAPIServer(t, true)
	executor := &workflowScriptExecutor{block: true, started: make(chan agentchat.RunSpec, 3)}
	s.AgentRunExecutor = executor
	workflow := submitTestWorkflow(t, s, provider, "# Queues\n## Task A — active\nAcceptance:\n- active\n## Task B — slot queued\nAcceptance:\n- queued\n## Task C — dependency queued\nDepends on: A\nAcceptance:\n- dependent\n", 1)
	coordinator, err := s.workflowCoordinator("local", workflow.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	select {
	case <-executor.started:
	case <-time.After(5 * time.Second):
		t.Fatal("root task did not start")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		states := coordinator.dag.Snapshot().States
		if states["A"].Status == agentrun.TaskRunning && states["B"].WaitReason == "concurrency_slot" && states["C"].WaitReason == "dependency" {
			if err := coordinator.stop(); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("workflow did not distinguish wait states: %+v", coordinator.dag.Snapshot().States)
}

func TestWorkflowPinsSharedContextGroupRevisionPerTaskSnapshot(t *testing.T) {
	s, _, provider := prepareRunAPIServer(t, true)
	groupResponse := invokeAI(t, s.agentContextGroupsAPI, http.MethodPost, "/api/agent/context-groups", map[string]any{"title": "Shared", "instructions": "Use the shared API contract v1."})
	if groupResponse.Code != http.StatusCreated {
		t.Fatalf("context group create: %d %s", groupResponse.Code, groupResponse.Body)
	}
	var groupResult struct {
		Group agentrun.ContextGroup `json:"group"`
	}
	if err := json.Unmarshal(groupResponse.Body.Bytes(), &groupResult); err != nil {
		t.Fatal(err)
	}
	executor := &fakeAgentRunExecutor{started: make(chan agentchat.RunSpec, 2), owners: make(chan string, 2)}
	s.AgentRunExecutor = executor
	markdown := "# Shared workflow\n## Task A — consumer\nAcceptance:\n- use contract\n"
	response := invokeAI(t, s.agentTaskManifestRunsAPI, http.MethodPost, "/api/agent/task-manifests/runs", map[string]any{
		"markdown": markdown, "context_group_id": groupResult.Group.ID, "provider_id": provider.ID,
		"model_id": provider.Models[0].ID, "provider_config_version": agentProviderConfigVersionForTest(t, s, provider.ID, provider.Models[0].ID),
		"concurrency": 1, "confirm_external": true,
	})
	if response.Code != http.StatusAccepted {
		t.Fatalf("workflow create: %d %s", response.Code, response.Body)
	}
	var plan struct {
		Workflow agentrun.WorkflowRecord `json:"workflow"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Workflow.ContextGroupID != groupResult.Group.ID || plan.Workflow.ContextGroupRevision != groupResult.Group.Revision || plan.Workflow.ContextGroupSHA256 != groupResult.Group.InstructionsSHA256 {
		t.Fatalf("workflow did not pin group revision: %+v", plan.Workflow)
	}
	coordinator, err := s.workflowCoordinator("local", plan.Workflow.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	waitWorkflowTaskStatuses(t, coordinator, map[string]agentrun.TaskStatus{"A": agentrun.TaskReview})
	state := coordinator.dag.Snapshot().States["A"]
	snapshotStore, err := s.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotStore.Get(state.ContextSnapshotID, plan.Workflow.OwnerWorkspaceID, state.ConversationID, plan.Workflow.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot.History[0].Content, "Use the shared API contract v1.") {
		t.Fatal("shared instructions missing from task snapshot")
	}
	wantSource := fmt.Sprintf("context-group:%s@%d:%s", groupResult.Group.ID, groupResult.Group.Revision, groupResult.Group.InstructionsSHA256)
	found := false
	for _, source := range snapshot.SourceRefs {
		found = found || source == wantSource
	}
	if !found {
		t.Fatalf("snapshot provenance missing pinned group: %v", snapshot.SourceRefs)
	}
	groups, _, _, err := s.agentContextGroupStore("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := groups.Update(groupResult.Group.ID, plan.Workflow.OwnerWorkspaceID, plan.Workflow.ProjectID, groupResult.Group.Revision, "Shared", "Use contract v2."); err != nil {
		t.Fatal(err)
	}
	unchanged, err := snapshotStore.Get(state.ContextSnapshotID, plan.Workflow.OwnerWorkspaceID, state.ConversationID, plan.Workflow.ProjectID)
	if err != nil || !strings.Contains(unchanged.History[0].Content, "contract v1") || strings.Contains(unchanged.History[0].Content, "contract v2") {
		t.Fatal("pinned run context changed after group update")
	}
}

func TestWorkflowFailureRetryAndDependencyChain(t *testing.T) {
	s, _, provider := prepareRunAPIServer(t, true)
	executor := &workflowScriptExecutor{failFirst: true, started: make(chan agentchat.RunSpec, 8)}
	s.AgentRunExecutor = executor
	workflow := submitTestWorkflow(t, s, provider, "# Retry plan\n## Task A — root\nAcceptance:\n- root done\n## Task B — child\nDepends on: A\nAcceptance:\n- child done\n", 2)
	coordinator, err := s.workflowCoordinator("local", workflow.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	waitWorkflowTaskStatuses(t, coordinator, map[string]agentrun.TaskStatus{"A": agentrun.TaskFailed, "B": agentrun.TaskBlocked})
	before := coordinator.dag.Snapshot().States["A"]
	if err := coordinator.retryTask("A"); err != nil {
		t.Fatal(err)
	}
	waitWorkflowTaskStatuses(t, coordinator, map[string]agentrun.TaskStatus{"A": agentrun.TaskReview, "B": agentrun.TaskQueued})
	after := coordinator.dag.Snapshot().States["A"]
	if executor.count(before.RunID) != 2 || after.RunID != before.RunID || after.ExecutionWorkspaceID != before.ExecutionWorkspaceID {
		t.Fatalf("retry changed run/worktree binding or did not create a fresh attempt: before=%+v after=%+v calls=%d", before, after, executor.count(before.RunID))
	}
	if executor.count("not-started") != 0 {
		t.Fatal("dependent task ran before review approval")
	}
	approvalResponse := workflowTaskAction(t, s, workflow.ID, "A", "decision", map[string]any{"approve": true, "fence": after.Fence, "reason": "root reviewed"})
	if approvalResponse.Code != http.StatusAccepted {
		t.Fatalf("root review failed: %d %s", approvalResponse.Code, approvalResponse.Body)
	}
	var approvalState struct {
		Workflow agentrun.WorkflowRecord `json:"workflow"`
	}
	if err := json.Unmarshal(approvalResponse.Body.Bytes(), &approvalState); err != nil {
		t.Fatal(err)
	}
	if approvalState.Workflow.DAG.States["A"].Status != agentrun.TaskSucceeded {
		t.Fatalf("approval API did not persist A success: %s", approvalResponse.Body)
	}
	waitWorkflowTaskStatuses(t, coordinator, map[string]agentrun.TaskStatus{"A": agentrun.TaskSucceeded, "B": agentrun.TaskReview})
	b := coordinator.dag.Snapshot().States["B"]
	if response := workflowTaskAction(t, s, workflow.ID, "B", "decision", map[string]any{"approve": true, "fence": b.Fence, "reason": "child reviewed"}); response.Code != http.StatusAccepted {
		t.Fatalf("child review failed: %d %s", response.Code, response.Body)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		coordinator.mu.Lock()
		status := coordinator.record.Status
		coordinator.mu.Unlock()
		if status == agentrun.WorkflowCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("workflow did not complete after both reviews: %+v", coordinator.dag.Snapshot().States)
}

func TestWorkflowStopCancelsAllActiveRuns(t *testing.T) {
	s, _, provider := prepareRunAPIServer(t, true)
	executor := &workflowScriptExecutor{block: true, started: make(chan agentchat.RunSpec, 4)}
	s.AgentRunExecutor = executor
	workflow := submitTestWorkflow(t, s, provider, "# Stop plan\n## Task A — root\nAcceptance:\n- finish\n## Task B — parallel\nAcceptance:\n- finish\n", 2)
	coordinator, err := s.workflowCoordinator("local", workflow.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	for i := 0; i < 2; i++ {
		select {
		case <-executor.started:
		case <-time.After(5 * time.Second):
			t.Fatal("workflow did not start both root tasks")
		}
	}
	if err := coordinator.stop(); err != nil {
		t.Fatal(err)
	}
	states := coordinator.dag.Snapshot().States
	for id, state := range states {
		if state.Status != agentrun.TaskCancelled {
			t.Fatalf("task %s status=%s, want cancelled", id, state.Status)
		}
	}
	for _, state := range states {
		if record, ok := manager.Get(state.RunID); !ok || record.Status != agentrun.Stopped {
			t.Fatalf("run %s not stopped", state.RunID)
		}
	}
}

func TestWorkflowRestartRestoresActiveTaskAsInterruptedWithoutAutoResume(t *testing.T) {
	s, _, provider := prepareRunAPIServer(t, true)
	firstExecutor := &workflowScriptExecutor{block: true, started: make(chan agentchat.RunSpec, 2)}
	s.AgentRunExecutor = firstExecutor
	workflow := submitTestWorkflow(t, s, provider, "# Restart plan\n## Task A — root\nAcceptance:\n- finish\n", 1)
	oldCoordinator, err := s.workflowCoordinator("local", workflow.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	oldManager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	var originalSpec agentchat.RunSpec
	select {
	case originalSpec = <-firstExecutor.started:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not start")
	}
	oldCoordinator.cancel()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		oldCoordinator.mu.Lock()
		running := oldCoordinator.running
		oldCoordinator.mu.Unlock()
		if !running {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	workflowKey := workflowCoordinatorKey(s.Workspace, "local", workflow.ID)
	workflowCoordinators.Delete(workflowKey)
	secondExecutor := &workflowScriptExecutor{started: make(chan agentchat.RunSpec, 2)}
	restarted := &Server{Workspace: s.Workspace, Config: s.Config, ConnectionSecrets: s.ConnectionSecrets, AgentRunExecutor: secondExecutor}
	recovered, err := restarted.workflowCoordinator("local", workflow.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	states := recovered.dag.Snapshot().States
	if state := states["A"]; state.Status != agentrun.TaskInterrupted || state.RunID != originalSpec.RunID {
		t.Fatalf("active task was not recovered as interrupted: %+v", state)
	}
	select {
	case spec := <-secondExecutor.started:
		t.Fatalf("interrupted run auto-resumed: %+v", spec)
	case <-time.After(50 * time.Millisecond):
	}
	if manager, err := restarted.agentRunManager("local"); err == nil {
		manager.Close()
	}
	oldManager.Close()
}

func workflowTaskAction(t *testing.T, s *Server, workflowID, taskID, action string, body any) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/agent/task-manifests/runs/" + workflowID + "/tasks/" + taskID + "/" + action
	return invokeAI(t, s.agentTaskManifestRunItemAPI, http.MethodPost, path, body)
}
