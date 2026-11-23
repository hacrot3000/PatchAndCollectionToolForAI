package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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

type patchRunExecutor struct {
	calls       atomic.Int32
	patchPython string
}

func (e *patchRunExecutor) Execute(ctx context.Context, _ agentchat.RunSpec, history []agentchat.HistoryItem, sink agentchat.EventSink) error {
	if e.calls.Add(1) == 1 {
		arguments, err := json.Marshal(map[string]string{"name": "run patch", "patch_python": e.patchPython})
		if err != nil {
			return err
		}
		call := agentchat.FunctionCall{CallID: "run-patch-call", Name: "taskdeck_submit_patch", Arguments: string(arguments)}
		payload, err := json.Marshal(call)
		if err != nil {
			return err
		}
		return sink.Publish(ctx, agentchat.RunEvent{Type: agentchat.RunEventToolCall, Payload: payload})
	}
	if len(history) == 0 || history[len(history)-1].Type != "function_call_output" || history[len(history)-1].CallID != "run-patch-call" {
		return context.Canceled
	}
	return sink.Publish(ctx, agentchat.RunEvent{Type: agentchat.RunEventTextDelta, Payload: []byte("Patch result reviewed")})
}
func (*patchRunExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }

func TestAgentRunPatchApprovalUsesExecutionWorktreeAndContinuesAfterNativeResult(t *testing.T) {
	s, session, provider := prepareRunAPIServer(t, true)
	if err := os.WriteFile(filepath.Join(s.Workspace, ".gitignore"), []byte("patchs/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", ".gitignore"}, {"commit", "-qm", "ignore Patch queue"}} {
		cmd := exec.Command("git", append([]string{"-C", s.Workspace}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	project, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	patchSource := fmt.Sprintf("# TASKDECK_PROJECT_KEY: %s\n# TASKDECK_PROJECT_ID: %s\n# reviewed patch\n", project.Key, project.ID)
	executor := &patchRunExecutor{patchPython: patchSource}
	s.AgentRunExecutor = executor
	runID := "run-patch-1"
	configVersion := agentchat.RunProviderConfigVersion(provider, provider.Models[0])
	worktreeResponse := invokeAI(t, func(w http.ResponseWriter, r *http.Request) { s.agentRunWorktreeAPI(w, r, "local") }, http.MethodPost, "/api/agent/runs/worktrees", map[string]any{
		"conversation_id": session.ID, "run_id": runID, "provider_id": provider.ID, "model_id": provider.Models[0].ID,
		"provider_config_version": configVersion, "permission_scope": []string{"files.read", "patch.propose", "patch.execute.approval_required"},
	})
	if worktreeResponse.Code != http.StatusCreated {
		t.Fatalf("worktree creation: %d %s", worktreeResponse.Code, worktreeResponse.Body)
	}
	var worktree struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(worktreeResponse.Body.Bytes(), &worktree); err != nil {
		t.Fatal(err)
	}
	registry, err := s.agentWorkspaceRegistryForOwner("local")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := registry.Resolve(context.Background(), worktree.WorkspaceID, session.ID, runID, session.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	snapshotResponse := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{"conversation_id": session.ID, "prompt": "Make the reviewed patch"})
	if snapshotResponse.Code != http.StatusCreated {
		t.Fatalf("snapshot: %d %s", snapshotResponse.Code, snapshotResponse.Body)
	}
	var snapshot struct {
		ID string `json:"context_snapshot_id"`
	}
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	ownerID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{RunID: runID, ConversationID: session.ID, OwnerWorkspaceID: ownerID, ExecutionWorkspaceID: binding.ExecutionWorkspaceID,
		ProviderID: provider.ID, ModelID: provider.Models[0].ID, ProviderConfigVersion: configVersion,
		ContextSnapshotID: snapshot.ID, AttemptID: "attempt-1", IdempotencyKey: "request-1"}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	contextStore, err := s.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	storedSnapshot, err := contextStore.Get(snapshot.ID, ownerID, session.ID, session.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit(spec, storedSnapshot.History); err != nil {
		t.Fatal(err)
	}
	waitForAgentRunStatus(t, manager, runID, agentrun.WaitingTool)

	runtimeDir := t.TempDir()
	entry, python := filepath.Join(runtimeDir, "python_patch_entry.py"), filepath.Join(runtimeDir, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)
	fakeNative := &fakeAgentNativeBridge{workspace: binding.WorktreePath}
	s.Sessions = fakeNative
	decision := invokeAI(t, func(w http.ResponseWriter, r *http.Request) {
		s.agentRunToolDecisionAPI(w, r, "local", runID, "run-patch-call")
	}, http.MethodPost,
		"/api/agent/runs/"+runID+"/tools/run-patch-call/decision", map[string]any{"action": "approve", "confirm_execution": true})
	if decision.Code != http.StatusOK || fakeNative.started != 1 || len(fakeNative.selectedIndexes) != 1 || fakeNative.selectedIndexes[0] != 2 {
		ownerStore, storeErr := s.agentChatStore()
		var call agentchat.ToolCallRecord
		if storeErr == nil {
			if current, getErr := ownerStore.Get(session.ID, "local"); getErr == nil && len(current.ToolCalls) > 0 {
				call = current.ToolCalls[0]
			}
		}
		fresh, _ := agentWorkspaceSnapshotOf(context.Background(), binding.WorktreePath)
		status, _, _, _ := runGitInDirectory(context.Background(), time.Second, binding.WorktreePath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
		t.Fatalf("approved Patch was not dispatched once in execution worktree: %d %s starts=%d selected=%v expected=(%s,%s,%s) fresh=(%s,%s,%s) status=%q", decision.Code, decision.Body, fakeNative.started, fakeNative.selectedIndexes, call.ExpectedHead, call.ExpectedBranch, call.WorkspaceStatusSHA256, fresh.Head, fresh.Branch, fresh.StatusSHA256, status)
	}
	ownerSource, err := os.ReadFile(filepath.Join(s.Workspace, "source.txt"))
	if err != nil || string(ownerSource) != "source\n" {
		t.Fatal("Agent Patch touched owner workspace instead of child worktree")
	}
	ownerStore, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := ownerStore.Get(session.ID, "local")
	if err != nil {
		t.Fatal(err)
	}
	if len(conversation.ToolCalls) != 1 || conversation.ToolCalls[0].Status != "running" || conversation.ToolCalls[0].PatchSessionID != fakeNative.meta.ID {
		t.Fatalf("native Patch was not durably correlated to run conversation: %+v", conversation.ToolCalls)
	}
	call := conversation.ToolCalls[0]
	rc := 7
	fakeNative.state.Items = []sessionpkg.ProtocolItemState{{Index: call.SelectedIndex, Name: filepath.Base(call.QueuePath), Kind: "PATCH", Status: "FAIL", RunID: "native-run-1", RC: &rc, FailureReason: "controlled test failure"}}
	continued := invokeAI(t, func(w http.ResponseWriter, r *http.Request) { s.agentRunToolContinueAPI(w, r, "local", runID) }, http.MethodPost, "/api/agent/runs/"+runID+"/tools/run-patch-call/continue", map[string]any{})
	if continued.Code != http.StatusAccepted {
		t.Fatalf("native result continuation: %d %s", continued.Code, continued.Body)
	}
	waitForAgentRunStatus(t, manager, runID, agentrun.Completed)
	record, _ := manager.Get(runID)
	if len(record.History) < 3 || record.History[len(record.History)-1].Type != "function_call_output" || record.History[len(record.History)-1].CallID != "run-patch-call" {
		t.Fatalf("native result was not continued in the same Agent Run: %+v", record.History)
	}
	if s.AgentRunExecutor.(*patchRunExecutor).calls.Load() != 2 {
		t.Fatal("Patch continuation did not invoke exactly one follow-up model request")
	}
	if strings.Contains(string(record.Events[len(record.Events)-1].Payload), "TEST-KEY") {
		t.Fatal("provider secret leaked into run events")
	}
}

func TestAgentRunPatchRejectContinuesWithoutNativeQueueSideEffect(t *testing.T) {
	s, conversation, provider := prepareRunAPIServer(t, true)
	project, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	patchSource := fmt.Sprintf("# TASKDECK_PROJECT_KEY: %s\n# TASKDECK_PROJECT_ID: %s\n# rejected\n", project.Key, project.ID)
	executor := &patchRunExecutor{patchPython: patchSource}
	s.AgentRunExecutor = executor
	ownerID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	configVersion := agentchat.RunProviderConfigVersion(provider, provider.Models[0])
	snapshotResponse := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{"conversation_id": conversation.ID, "prompt": "propose a patch"})
	if snapshotResponse.Code != http.StatusCreated {
		t.Fatalf("snapshot: %d %s", snapshotResponse.Code, snapshotResponse.Body)
	}
	var snapshot struct {
		ID string `json:"context_snapshot_id"`
	}
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{RunID: "run-patch-reject", ConversationID: conversation.ID, OwnerWorkspaceID: ownerID, ExecutionWorkspaceID: ownerID,
		ProviderID: provider.ID, ModelID: provider.Models[0].ID, ProviderConfigVersion: configVersion,
		ContextSnapshotID: snapshot.ID, AttemptID: "attempt-1", IdempotencyKey: "request-1"}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	snapshotStore, err := s.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := snapshotStore.Get(snapshot.ID, ownerID, conversation.ID, conversation.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit(spec, stored.History); err != nil {
		t.Fatal(err)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.WaitingTool)
	response := invokeAI(t, func(w http.ResponseWriter, r *http.Request) {
		s.agentRunToolDecisionAPI(w, r, "local", spec.RunID, "run-patch-call")
	}, http.MethodPost,
		"/api/agent/runs/"+spec.RunID+"/tools/run-patch-call/decision", map[string]any{"action": "reject"})
	if response.Code != http.StatusOK {
		t.Fatalf("tool rejection: %d %s", response.Code, response.Body)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.Completed)
	record, _ := manager.Get(spec.RunID)
	if executor.calls.Load() != 2 || record.History[len(record.History)-1].Type != "function_call_output" || !strings.Contains(record.History[len(record.History)-1].Content, "rejected") {
		t.Fatalf("rejection was not returned to the run: calls=%d history=%+v", executor.calls.Load(), record.History)
	}
	if _, err := os.Stat(filepath.Join(s.Workspace, "patchs")); !os.IsNotExist(err) {
		t.Fatal("rejected Patch created a native queue package")
	}
}
