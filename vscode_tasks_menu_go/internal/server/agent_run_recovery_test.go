package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

// The recovery tests kill a real Agent Run daemon process while a run is
// active and then verify that the next daemon instance recovers the durable
// state without replaying a side effect that was never reconciled.

const (
	agentRunRecoveryHelperEnv  = "TASKDECK_AGENT_RUN_RECOVERY_HELPER"
	agentRunRecoveryWorkdirEnv = "TASKDECK_AGENT_RUN_RECOVERY_WORKDIR"
	agentRunRecoveryRunIDEnv   = "TASKDECK_AGENT_RUN_RECOVERY_RUN_ID"
	agentRunRecoveryMarkerEnv  = "TASKDECK_AGENT_RUN_RECOVERY_MARKER"
)

// recoveryRunExecutor simulates a provider that keeps streaming until the
// process is killed. It records the attempts it made so the parent process
// can prove that a recovered run is not replayed without a review.
type recoveryRunExecutor struct {
	attempts atomic.Int32
}

func (e *recoveryRunExecutor) Execute(ctx context.Context, _ agentchat.RunSpec, _ []agentchat.HistoryItem, sink agentchat.EventSink) error {
	e.attempts.Add(1)
	if err := sink.Publish(ctx, agentchat.RunEvent{Type: agentchat.RunEventTextDelta, Payload: []byte("partial answer")}); err != nil {
		return err
	}
	// Block until the process is killed so the run stays active.
	<-ctx.Done()
	return ctx.Err()
}

func (*recoveryRunExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }

// recoveryRunSpec returns a valid run binding for the recovery tests.
func recoveryRunSpec(runID string) agentchat.RunSpec {
	return agentchat.RunSpec{RunID: runID, ConversationID: "recovery-conversation",
		OwnerWorkspaceID: "recovery-workspace", ExecutionWorkspaceID: "recovery-workspace",
		ProviderID: "recovery-provider", ModelID: "recovery-model",
		ProviderConfigVersion: "recovery-config-v1", ContextSnapshotID: "recovery-snapshot",
		ParentTurnID: "recovery-turn", AttemptID: runID + "-a1", IdempotencyKey: runID}
}

// TestAgentRunRecoveryHelperProcess is not a real test. It runs an Agent Run
// in its own process so the parent can kill it while the run is active.
func TestAgentRunRecoveryHelperProcess(t *testing.T) {
	if os.Getenv(agentRunRecoveryHelperEnv) != "1" {
		t.Skip("helper process")
	}
	workdir := os.Getenv(agentRunRecoveryWorkdirEnv)
	runID := os.Getenv(agentRunRecoveryRunIDEnv)
	marker := os.Getenv(agentRunRecoveryMarkerEnv)
	if workdir == "" || runID == "" || marker == "" {
		t.Fatal("the recovery helper is missing its environment")
	}
	executor := &recoveryRunExecutor{}
	manager, err := agentrun.NewManager(filepath.Join(workdir, "runs"), 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	spec := recoveryRunSpec(runID)
	if _, err := manager.Submit(spec, []agentchat.HistoryItem{{Type: "user", Content: "long running prompt"}}); err != nil {
		t.Fatal(err)
	}
	// Report that the run is active so the parent can kill this process.
	payload, err := json.Marshal(map[string]any{"run_id": runID, "pid": os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, append(payload, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	// Keep the process alive until it is killed.
	select {}
}

// waitForDurableRunningRun waits until the durable journal of the run claims
// that the run is active. The parent process must wait for this before it
// kills the daemon, otherwise the crash happens while the run is still queued
// and the recovery would legitimately re-queue it.
func waitForDurableRunningRun(t *testing.T, journalPath string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(journalPath)
		if err == nil {
			var persisted struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(body, &persisted) == nil && persisted.Status == string(agentrun.Running) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the run journal %s did not record a running run", journalPath)
}

// startRecoveryHelper starts a daemon process that runs one Agent Run and
// reports the moment its run is active.
func startRecoveryHelper(t *testing.T, workdir, runID string) (*exec.Cmd, string, string) {
	t.Helper()
	marker := filepath.Join(workdir, "marker.json")
	command := exec.Command(os.Args[0], "-test.run=TestAgentRunRecoveryHelperProcess", "-test.v")
	command.Env = append(os.Environ(),
		agentRunRecoveryHelperEnv+"=1",
		agentRunRecoveryWorkdirEnv+"="+workdir,
		agentRunRecoveryRunIDEnv+"="+runID,
		agentRunRecoveryMarkerEnv+"="+marker,
	)
	if err := command.Start(); err != nil {
		t.Fatalf("the recovery helper could not be started: %v", err)
	}
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the recovery helper did not report an active run: %v", err)
	}
	journalPath := filepath.Join(workdir, "runs", runID+".json")
	waitForDurableRunningRun(t, journalPath)
	return command, marker, journalPath
}

// TestAgentRunRecoversAfterProcessKillWithoutReplayingSideEffects kills a real
// daemon process while its run is streaming and verifies that the next daemon
// instance recovers the run as interrupted, requires an explicit review, and
// does not replay the provider request on its own.
func TestAgentRunRecoversAfterProcessKillWithoutReplayingSideEffects(t *testing.T) {
	workdir := t.TempDir()
	runID := "recovery-killed-run"

	command, _, journalPath := startRecoveryHelper(t, workdir, runID)

	// Kill the daemon process outright: no clean shutdown, no interruption
	// bookkeeping by the manager.
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("the recovery helper could not be killed: %v", err)
	}
	_ = command.Wait()
	command.Process = nil

	journal, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("the killed run did not leave a durable journal: %v", err)
	}
	var persisted struct {
		Status  string `json:"status"`
		Attempt int    `json:"attempt"`
	}
	if err := json.Unmarshal(journal, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Status != string(agentrun.Running) {
		t.Fatalf("the killed run journal does not claim an active run: %+v", persisted)
	}

	// The next daemon instance recovers the run without executing it again.
	recovered := &recoveryRunExecutor{}
	manager, err := agentrun.NewManager(filepath.Join(workdir, "runs"), 1, recovered)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	record, ok := manager.Get(runID)
	if !ok {
		t.Fatal("the killed run was not recovered by the next daemon instance")
	}
	if record.Status != agentrun.Interrupted {
		t.Fatalf("the recovered run status is %q, want interrupted after a daemon crash", record.Status)
	}
	if recovered.attempts.Load() != 0 {
		t.Fatalf("the recovered run replayed the provider request %d times", recovered.attempts.Load())
	}
	if record.RetrySafe {
		t.Fatal("a run with an unknown provider outcome must not be marked safe to retry")
	}
	if record.LastError == "" {
		t.Fatal("the recovered run does not explain why it was interrupted")
	}

	// An explicit review is required before the run may be retried, so the
	// uncertain provider outcome is never repeated silently.
	if _, err := manager.Retry(runID); err == nil {
		t.Fatal("an unreviewed interrupted run was retried without review")
	}
	if _, err := manager.ResumeAfterReview(runID, false); err == nil {
		t.Fatal("an interrupted run was resumed without an explicit side-effect review")
	}
}

// TestAgentRunWaitingToolSurvivesCleanManagerCloseAndSyntheticJournal verifies
// the reconstruction path with a clean Manager close and a synthetic journal.
// It is NOT real process-kill evidence; see
// TestAgentRunWaitingToolSurvivesRealProcessKill below.
func TestAgentRunWaitingToolSurvivesCleanManagerCloseAndSyntheticJournal(t *testing.T) {
	workdir := t.TempDir()
	runID := "recovery-waiting-tool"
	executor := &recoveryRunExecutor{}
	manager, err := agentrun.NewManager(filepath.Join(workdir, "runs"), 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	spec := recoveryRunSpec(runID)
	if _, err := manager.Submit(spec, []agentchat.HistoryItem{{Type: "user", Content: "prompt"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if record, ok := manager.Get(runID); ok && record.Status == agentrun.Running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Stop the manager to simulate the daemon going away while the run was
	// active, then rewrite the durable journal to the waiting-tool state the
	// run had when it asked for a tool decision.
	manager.Close()
	journalPath := filepath.Join(workdir, "runs", runID+".json")
	body, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(body, &persisted); err != nil {
		t.Fatal(err)
	}
	persisted["status"] = string(agentrun.WaitingTool)
	persisted["history"] = []any{map[string]any{
		"type": "function_call", "call_id": "recovery-call-1", "name": "taskdeck_submit_patch",
		"arguments": `{"name":"run patch"}`,
	}}
	waiting, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, append(waiting, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	// The next daemon instance recovers the pending call.
	recovered := &recoveryRunExecutor{}
	restart, err := agentrun.NewManager(filepath.Join(workdir, "runs"), 1, recovered)
	if err != nil {
		t.Fatal(err)
	}
	defer restart.Close()
	record, ok := restart.Get(runID)
	if !ok {
		t.Fatal("the waiting-tool run was not recovered after the daemon restart")
	}
	if record.Status != agentrun.Interrupted {
		t.Fatalf("the recovered waiting-tool run status is %q, want interrupted", record.Status)
	}
	pending := 0
	for _, item := range record.History {
		if item.Type == "function_call" && item.CallID == "recovery-call-1" {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("the recovered run lost its pending tool call: %+v", record.History)
	}
	if recovered.attempts.Load() != 0 {
		t.Fatalf("the recovered waiting-tool run replayed the provider request %d times", recovered.attempts.Load())
	}
}

// TestAgentRunUncertainProviderResultRequiresReconciliation kills a real
// daemon process while its provider request is in flight and verifies that the
// uncertain outcome is never replayed automatically and that the run is only
// resumable after an explicit review.
func TestAgentRunUncertainProviderResultRequiresReconciliation(t *testing.T) {
	workdir := t.TempDir()
	runID := "recovery-uncertain"

	command, _, _ := startRecoveryHelper(t, workdir, runID)
	// Kill the daemon while the provider request is still in flight, so the
	// outcome of that request is unknown.
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("the recovery helper could not be killed: %v", err)
	}
	_ = command.Wait()
	command.Process = nil

	restarted := &recoveryRunExecutor{}
	restart, err := agentrun.NewManager(filepath.Join(workdir, "runs"), 1, restarted)
	if err != nil {
		t.Fatal(err)
	}
	defer restart.Close()
	record, ok := restart.Get(runID)
	if !ok {
		t.Fatal("the recovered uncertain run was not found")
	}
	if record.Status != agentrun.Interrupted {
		t.Fatalf("the recovered uncertain run status is %q, want interrupted", record.Status)
	}
	if record.RetrySafe {
		t.Fatal("a run with an uncertain provider outcome must not be marked safe to retry")
	}
	if _, err := restart.Retry(runID); err == nil {
		t.Fatal("an uncertain provider outcome was retried without an explicit review")
	}
	if _, err := restart.ResumeAfterReview(runID, false); err == nil {
		t.Fatal("an uncertain provider outcome was resumed without a side-effect review")
	}
	if restarted.attempts.Load() != 0 {
		t.Fatalf("the uncertain provider request was replayed %d times", restarted.attempts.Load())
	}
}

// TestAgentRunReloadAndRetryKeepsSingleProviderAttempt kills a real daemon
// process, reopens the run through a fresh manager as a UI reload does, and
// verifies that a reviewed retry produces exactly one additional provider
// attempt while the run history stays continuous.
func TestAgentRunReloadAndRetryKeepsSingleProviderAttempt(t *testing.T) {
	workdir := t.TempDir()
	runID := "recovery-reload-retry"

	command, _, _ := startRecoveryHelper(t, workdir, runID)
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("the recovery helper could not be killed: %v", err)
	}
	_ = command.Wait()
	command.Process = nil

	// The user reloads the UI, which reopens the run through a fresh manager.
	reloaded := &recoveryRunExecutor{}
	restart, err := agentrun.NewManager(filepath.Join(workdir, "runs"), 1, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	defer restart.Close()
	record, ok := restart.Get(runID)
	if !ok {
		t.Fatal("the run was not restored after the reload")
	}
	if record.Status != agentrun.Interrupted {
		t.Fatalf("the restored run status is %q, want interrupted", record.Status)
	}
	if reloaded.attempts.Load() != 0 {
		t.Fatalf("the reload replayed the provider request %d times", reloaded.attempts.Load())
	}
	// The history of the interrupted run is still available for review.
	if len(record.History) == 0 {
		t.Fatal("the restored run lost its history")
	}
	if !strings.Contains(record.History[len(record.History)-1].Content, "prompt") {
		t.Fatalf("the restored run history is not continuous: %+v", record.History)
	}
	// An explicit review that confirms the side effects were inspected
	// resumes the run exactly once.
	if _, err := restart.ResumeAfterReview(runID, false); err == nil {
		t.Fatal("the interrupted run was resumed without an explicit side-effect review")
	}
	if _, err := restart.ResumeAfterReview(runID, true); err != nil {
		t.Fatalf("the reviewed run could not be resumed: %v", err)
	}
	// The resumed run is queued, so the worker picks it up asynchronously.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if reloaded.attempts.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if reloaded.attempts.Load() != 1 {
		t.Fatalf("the reviewed retry produced %d provider attempts, want exactly 1", reloaded.attempts.Load())
	}
}

const agentRunWaitingToolHelperEnv = "TASKDECK_AGENT_RUN_WAITING_TOOL_HELPER"

// waitingToolRunExecutor publishes a valid tool call and then blocks so the
// helper process stays in waiting_tool until it is killed.
type waitingToolRunExecutor struct{}

func (*waitingToolRunExecutor) Execute(ctx context.Context, _ agentchat.RunSpec, _ []agentchat.HistoryItem, sink agentchat.EventSink) error {
	call, err := json.Marshal(map[string]any{"call_id": "killed-waiting-call", "name": "taskdeck_submit_patch", "arguments": `{"name":"真实 process kill"}`})
	if err != nil {
		return err
	}
	if err := sink.Publish(ctx, agentchat.RunEvent{Type: agentchat.RunEventToolCall, Payload: call}); err != nil {
		return err
	}
	select {}
}

func (*waitingToolRunExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }

// TestAgentRunWaitingToolHelperProcess is a helper process used only by the
// real process-kill test below.
func TestAgentRunWaitingToolHelperProcess(t *testing.T) {
	if os.Getenv(agentRunWaitingToolHelperEnv) != "1" {
		t.Skip("helper process")
	}
	workdir := os.Getenv(agentRunRecoveryWorkdirEnv)
	runID := os.Getenv(agentRunRecoveryRunIDEnv)
	marker := os.Getenv(agentRunRecoveryMarkerEnv)
	manager, err := agentrun.NewManager(filepath.Join(workdir, "runs"), 1, &waitingToolRunExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err := manager.Submit(recoveryRunSpec(runID), []agentchat.HistoryItem{{Type: "user", Content: "wait for tool"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if record, ok := manager.Get(runID); ok && record.Status == agentrun.WaitingTool {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	payload := []byte(runID + "\n")
	if err := os.WriteFile(marker, payload, 0600); err != nil {
		t.Fatal(err)
	}
	select {}
}

// TestAgentRunWaitingToolSurvivesRealProcessKill kills a real daemon while the
// run is in waiting_tool. It is distinct from the clean-close/synthetic-journal
// evidence test and proves the pending tool call survives an actual SIGKILL.
func TestAgentRunWaitingToolSurvivesRealProcessKill(t *testing.T) {
	workdir := t.TempDir()
	runID := "recovery-waiting-tool-real-kill"
	marker := filepath.Join(workdir, "waiting-marker.json")
	command := exec.Command(os.Args[0], "-test.run=TestAgentRunWaitingToolHelperProcess", "-test.v")
	command.Env = append(os.Environ(),
		agentRunWaitingToolHelperEnv+"=1",
		agentRunRecoveryWorkdirEnv+"="+workdir,
		agentRunRecoveryRunIDEnv+"="+runID,
		agentRunRecoveryMarkerEnv+"="+marker,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the waiting-tool helper did not become ready: %v", err)
	}
	journalPath := filepath.Join(workdir, "runs", runID+".json")
	waitForDurableActiveRun(t, journalPath, agentrun.WaitingTool)
	body, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(body, &persisted) != nil || persisted.Status != string(agentrun.WaitingTool) {
		t.Fatalf("the killed process was not in waiting_tool: %s", body)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	command.Process = nil
	restartExecutor := &recoveryRunExecutor{}
	manager, err := agentrun.NewManager(filepath.Join(workdir, "runs"), 1, restartExecutor)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	record, ok := manager.Get(runID)
	if !ok {
		t.Fatal("waiting-tool run was not recovered after real process kill")
	}
	if record.Status != agentrun.Interrupted {
		t.Fatalf("recovered status is %q, want interrupted", record.Status)
	}
	pending := 0
	for _, item := range record.History {
		if item.Type == "function_call" && item.CallID == "killed-waiting-call" {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("pending tool call was lost or duplicated after real kill: %+v", record.History)
	}
	if restartExecutor.attempts.Load() != 0 {
		t.Fatalf("recovery replayed the provider request: %d", restartExecutor.attempts.Load())
	}
}

// waitForDurableActiveRun waits until the journal records the requested active
// status. Waiting-tool runs never pass through a durable Running state in the
// helper, so the generic Running-only helper cannot be used here.
func waitForDurableActiveRun(t *testing.T, journalPath string, status agentrun.Status) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(journalPath)
		if err == nil {
			var persisted struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(body, &persisted) == nil && persisted.Status == string(status) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the run journal %s did not record %s", journalPath, status)
}
