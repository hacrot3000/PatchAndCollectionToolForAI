package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
	sessionpkg "bletonfc/vscode_tasks_menu/internal/session"
)

func TestAgentRunNativePatchRoundTripExecutesOnceAcrossRestart(t *testing.T) {
	s, conversation, provider := prepareRunAPIServer(t, true)
	if err := os.WriteFile(filepath.Join(s.Workspace, ".gitignore"), []byte("patchs/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", ".gitignore"}, {"commit", "-qm", "ignore native Patch queue"}} {
		cmd := exec.Command("git", append([]string{"-C", s.Workspace}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository Patch runtime")
	}
	patchEntry := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", "..", "python_patch_entry.py"))
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("actual Patch execution requires python3: %v", err)
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", patchEntry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)

	project, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	patchSource := fmt.Sprintf("# TASKDECK_PROJECT_KEY: %s\n# TASKDECK_PROJECT_ID: %s\nimport time\nfrom pathlib import Path\ntime.sleep(2)\nPath('source.txt').write_text('native Patch executed once\\n')\n", project.Key, project.ID)
	executor := &patchRunExecutor{patchPython: patchSource}
	s.AgentRunExecutor = executor
	nativeSessions := sessionpkg.NewManager(1 << 20)
	s.Sessions = nativeSessions
	t.Cleanup(func() { nativeSessions.Shutdown(2 * time.Second) })

	const runID = "native-patch-restart"
	configVersion := agentchat.RunProviderConfigVersion(provider, provider.Models[0])
	worktreeResponse := invokeAI(t, func(w http.ResponseWriter, r *http.Request) {
		s.agentRunWorktreeAPI(w, r, "local")
	}, http.MethodPost, "/api/agent/runs/worktrees", map[string]any{
		"conversation_id": conversation.ID, "run_id": runID, "provider_id": provider.ID,
		"model_id": provider.Models[0].ID, "provider_config_version": configVersion,
		"permission_scope": []string{"files.read", "patch.propose", "patch.execute.approval_required"},
	})
	if worktreeResponse.Code != http.StatusCreated {
		t.Fatalf("create execution worktree: HTTP %d: %s", worktreeResponse.Code, worktreeResponse.Body)
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
	binding, err := registry.Resolve(context.Background(), worktree.WorkspaceID, conversation.ID, runID, conversation.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	s.AgentChatPack = func(workspace, _ string) (patchtool.AIPackResult, error) {
		identity, err := patchtool.ReadProjectIdentity(workspace)
		return patchtool.AIPackResult{TextContent: "real native Patch execution context", ProjectIdentity: identity}, err
	}
	snapshotResponse := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{
		"conversation_id": conversation.ID, "prompt": "apply the reviewed native patch",
		"provider_id": provider.ID, "model_id": provider.Models[0].ID,
		"run_id": runID, "execution_workspace_id": worktree.WorkspaceID,
	})
	if snapshotResponse.Code != http.StatusCreated {
		t.Fatalf("create run snapshot: HTTP %d: %s", snapshotResponse.Code, snapshotResponse.Body)
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
	if snapshot.ExecutionWorkspaceID != worktree.WorkspaceID {
		t.Fatalf("snapshot did not bind the child workspace: %+v", snapshot)
	}
	spec := runSpecFor(s, conversation, provider)
	spec.RunID = runID
	spec.OwnerWorkspaceID = snapshot.OwnerWorkspaceID
	spec.ExecutionWorkspaceID = snapshot.ExecutionWorkspaceID
	spec.ParentTurnID = snapshot.ParentTurnID
	spec.ProviderConfigVersion = snapshot.ProviderConfigVersion
	spec.CredentialFingerprint = snapshot.CredentialFingerprint
	spec.ContextSnapshotID = snapshot.ID
	spec.ExcludedPendingRunIDs = strings.Join(snapshot.ExcludedPendingRunIDs, ",")
	spec.RetryMode = agentchat.RetryMode(snapshot.RetryMode)
	submit := invokeAI(t, func(w http.ResponseWriter,r *http.Request){s.agentRunAPIWithCreationPolicy(w,r,true)}, http.MethodPost, "/api/agent/runs", map[string]any{"spec": spec, "confirm_external": true})
	if submit.Code != http.StatusAccepted {
		t.Fatalf("submit Agent Run: HTTP %d: %s", submit.Code, submit.Body)
	}
	manager := s.agentRunManagers["local"]
	if manager == nil {
		t.Fatal("Agent Run Manager was not initialized")
	}
	waitForAgentRunStatus(t, manager, runID, agentrun.WaitingTool)

	decision := invokeAI(t, func(w http.ResponseWriter, r *http.Request) {
		s.agentRunToolDecisionAPI(w, r, "local", runID, "run-patch-call")
	}, http.MethodPost, "/api/agent/runs/"+runID+"/tools/run-patch-call/decision", map[string]any{"action": "approve", "confirm_execution": true})
	if decision.Code != http.StatusOK || !strings.Contains(decision.Body.String(), `"status":"running"`) {
		t.Fatalf("actual native Patch approval was not dispatched: HTTP %d: %s", decision.Code, decision.Body)
	}
	cancelRequest := invokeAI(t, s.agentRunItemAPI, http.MethodPost, "/api/agent/runs/"+runID+"/stop", map[string]any{})
	if cancelRequest.Code != http.StatusOK {
		t.Fatalf("stop Agent Run while native Patch is active: HTTP %d: %s", cancelRequest.Code, cancelRequest.Body)
	}
	waitForAgentRunStatus(t, manager, runID, agentrun.Stopped)

	conversationStore, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	nativeCall, err := conversationStore.Get(conversation.ID, "local")
	if err != nil || len(nativeCall.ToolCalls) != 1 {
		t.Fatalf("native Patch ledger unavailable: %+v %v", nativeCall.ToolCalls, err)
	}
	call := nativeCall.ToolCalls[0]
	if call.Status != "running" || call.PatchSessionID == "" {
		t.Fatalf("approved Patch is not linked to a native session: %+v", call)
	}
	deadline := time.Now().Add(90 * time.Second)
	var final sessionpkg.ProtocolItemState
	for time.Now().Before(deadline) {
		state, stateErr := nativeSessions.ProtocolState(call.PatchSessionID)
		if stateErr == nil {
			for _, item := range state.Items {
				if item.Index == call.SelectedIndex && item.Name == filepath.Base(call.QueuePath) && (item.Status == "PASS" || item.Status == "FAIL" || item.Status == "PREFLIGHT_FAIL" || item.Status == "INCOMPLETE") {
					final = item
					break
				}
			}
		}
		if final.RunID != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if final.RunID == "" {
		state, _ := nativeSessions.ProtocolState(call.PatchSessionID)
		t.Fatalf("actual native Patch produced no final per-item result: %+v", state)
	}
	if final.Status != "PASS" || final.RC == nil || *final.RC != 0 {
		t.Fatalf("actual native Patch failed: %+v", final)
	}
	childSource, err := os.ReadFile(filepath.Join(binding.WorktreePath, "source.txt"))
	if err != nil || string(childSource) != "native Patch executed once\n" {
		t.Fatalf("native Patch did not change the execution worktree: %q %v", childSource, err)
	}
	ownerSource, err := os.ReadFile(filepath.Join(s.Workspace, "source.txt"))
	if err != nil || string(ownerSource) != "source\n" {
		t.Fatalf("native Patch changed the owner workspace: %q %v", ownerSource, err)
	}
	if _, ok := nativeSessions.Metadata(call.PatchSessionID); !ok || executor.calls.Load() != 1 {
		t.Fatalf("approved Patch session or single provider proposal was not preserved: native_session=%v provider_calls=%d", ok, executor.calls.Load())
	}
	queuedPackages, err := os.ReadDir(filepath.Join(binding.WorktreePath, "patchs"))
	if err != nil || len(queuedPackages) != 1 {
		t.Fatalf("expected exactly one native Patch queue package, entries=%d err=%v", len(queuedPackages), err)
	}
	replayedApproval := invokeAI(t, func(w http.ResponseWriter, r *http.Request) {
		s.agentRunToolDecisionAPI(w, r, "local", runID, "run-patch-call")
	}, http.MethodPost, "/api/agent/runs/"+runID+"/tools/run-patch-call/decision", map[string]any{"action": "approve", "confirm_execution": true})
	if replayedApproval.Code < 400 || executor.calls.Load() != 1 {
		t.Fatalf("replayed approval started duplicate work: HTTP %d: %s; provider_calls=%d", replayedApproval.Code, replayedApproval.Body, executor.calls.Load())
	}

	manager.Close()
	restarted := &Server{Workspace: s.Workspace, OwnerWorkspace: s.OwnerWorkspace, Config: s.Config,
		Sessions: nativeSessions, ConnectionSecrets: s.ConnectionSecrets, AgentRunExecutor: executor, AgentChatPack: s.AgentChatPack}
	restartManager, err := restarted.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	if record, ok := restartManager.Get(runID); !ok || record.Status != agentrun.Stopped {
		t.Fatalf("stopped run state was not recovered after restart: %+v, found=%v", record, ok)
	}
	continued := invokeAI(t, func(w http.ResponseWriter, r *http.Request) {
		restarted.agentRunToolContinueAPI(w, r, "local", runID)
	}, http.MethodPost, "/api/agent/runs/"+runID+"/tools/run-patch-call/continue", map[string]any{})
	if continued.Code != http.StatusAccepted {
		t.Fatalf("native Patch result did not resume after restart: HTTP %d: %s", continued.Code, continued.Body)
	}
	waitForAgentRunStatus(t, restartManager, runID, agentrun.Completed)
	record, _ := restartManager.Get(runID)
	if executor.calls.Load() != 2 || len(record.History) < 3 || record.History[len(record.History)-1].Type != "function_call_output" || record.History[len(record.History)-1].CallID != "run-patch-call" {
		t.Fatalf("native Patch result was not continued exactly once: calls=%d history=%+v", executor.calls.Load(), record.History)
	}
}
