package agentrun

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

type toolCallExecutor struct{}

func (toolCallExecutor) Execute(ctx context.Context, _ agentchat.RunSpec, _ []agentchat.HistoryItem, sink agentchat.EventSink) error {
	return sink.Publish(ctx, agentchat.RunEvent{Type: agentchat.RunEventToolCall, Payload: json.RawMessage(`{"call_id":"call-1","name":"taskdeck_submit_patch","arguments":"{}"}`)})
}
func (toolCallExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }

type continuingToolExecutor struct{ calls int }

func (e *continuingToolExecutor) Execute(ctx context.Context, spec agentchat.RunSpec, history []agentchat.HistoryItem, sink agentchat.EventSink) error {
	e.calls++
	if e.calls == 1 {
		return sink.Publish(ctx, agentchat.RunEvent{Type: agentchat.RunEventToolCall, Payload: json.RawMessage(`{"call_id":"call-1","name":"taskdeck_submit_patch","arguments":"{}"}`)})
	}
	if spec.ContextSnapshotID != "snapshot-after-tool" || len(history) < 2 || history[len(history)-1].Type != "function_call_output" || history[len(history)-1].CallID != "call-1" {
		return context.Canceled
	}
	return sink.Publish(ctx, agentchat.RunEvent{Type: agentchat.RunEventTextDelta, Payload: []byte("completed")})
}
func (*continuingToolExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }

func TestWaitingToolRetainsGlobalSlotUntilToolResultContinuation(t *testing.T) {
	resetRunSlots()
	var held []func()
	for i := 0; i < MaxConcurrentRuns-1; i++ {
		release, ok := AcquireGlobalSlot(context.Background())
		if !ok {
			t.Fatal("failed to reserve a global run slot")
		}
		held = append(held, release)
	}
	defer func() {
		for _, release := range held {
			release()
		}
	}()
	executor := &continuingToolExecutor{}
	manager, err := NewManager(filepath.Join(t.TempDir(), "runs"), 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	spec := testSpec(t, "run-tool-continue")
	if _, err := manager.Submit(spec, []agentchat.HistoryItem{{Type: "user", Content: "do work"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		record, _ := manager.Get(spec.RunID)
		if record.Status == WaitingTool {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if record, _ := manager.Get(spec.RunID); record.Status != WaitingTool {
		t.Fatalf("status=%s, want waiting_tool", record.Status)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if release, ok := AcquireGlobalSlot(waitCtx); ok {
		release()
		t.Fatal("waiting tool run released its global slot")
	}
	result := agentchat.HistoryItem{Type: "function_call_output", CallID: "call-1", Content: "Patch PASS"}
	continued, err := manager.ContinueWithToolResults(spec.RunID, []agentchat.HistoryItem{result}, "snapshot-after-tool")
	if err != nil {
		t.Fatal(err)
	}
	if continued.Attempt != 2 || continued.Spec.OwnerWorkspaceID != spec.OwnerWorkspaceID || continued.Spec.ExecutionWorkspaceID != spec.ExecutionWorkspaceID || continued.Spec.ProviderID != spec.ProviderID || continued.Spec.ModelID != spec.ModelID {
		t.Fatalf("continuation changed immutable run binding: %+v", continued.Spec)
	}
	for time.Now().Before(deadline) {
		record, _ := manager.Get(spec.RunID)
		if record.Status == Completed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if record, _ := manager.Get(spec.RunID); record.Status != Completed {
		t.Fatalf("continued run status=%s, want completed", record.Status)
	}
	release, ok := AcquireGlobalSlot(context.Background())
	if !ok {
		t.Fatal("completed run did not release its global slot")
	}
	release()
}

func TestStoppedWaitingToolCanContinueOnlyFromReconciledResult(t *testing.T) {
	resetRunSlots()
	var held []func()
	for i := 0; i < MaxConcurrentRuns-1; i++ {
		release, ok := AcquireGlobalSlot(context.Background())
		if !ok {
			for _, prior := range held {
				prior()
			}
			t.Fatalf("reserve slot %d", i)
		}
		held = append(held, release)
	}
	defer func() {
		for _, release := range held {
			release()
		}
	}()
	executor := &continuingToolExecutor{}
	manager, err := NewManager(filepath.Join(t.TempDir(), "runs"), 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	spec := testSpec(t, "run-tool-stopped")
	if _, err := manager.Submit(spec, []agentchat.HistoryItem{{Type: "user", Content: "do work"}}); err != nil {
		t.Fatal(err)
	}
	awaitRunStatus(t, manager, spec.RunID, WaitingTool)
	if err := manager.Stop(context.Background(), spec.RunID); err != nil {
		t.Fatal(err)
	}
	awaitRunStatus(t, manager, spec.RunID, Stopped)
	released := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		release, ok := AcquireGlobalSlot(ctx)
		cancel()
		if ok {
			release()
			released = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !released {
		t.Fatal("stopped waiting tool run retained global slot")
	}
	continued, err := manager.ContinueWithToolResults(spec.RunID, []agentchat.HistoryItem{{Type: "function_call_output", CallID: "call-1", Content: "verified native result"}}, "snapshot-after-tool")
	if err != nil {
		t.Fatal(err)
	}
	if continued.Status != Queued || continued.Attempt != 2 {
		t.Fatalf("stopped continuation state: %+v", continued)
	}
	awaitRunStatus(t, manager, spec.RunID, Completed)
}

func TestStoppingWaitingToolReleasesGlobalSlot(t *testing.T) {
	resetRunSlots()
	var held []func()
	for i := 0; i < MaxConcurrentRuns-1; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		release, ok := AcquireGlobalSlot(ctx)
		cancel()
		if !ok {
			for _, prior := range held {
				prior()
			}
			t.Fatalf("failed to reserve slot %d", i)
		}
		held = append(held, release)
	}
	defer func() {
		for _, release := range held {
			release()
		}
	}()
	manager, err := NewManager(filepath.Join(t.TempDir(), "runs"), 1, toolCallExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	spec := testSpec(t, "run-tool-stop")
	if _, err := manager.Submit(spec, []agentchat.HistoryItem{{Type: "user", Content: "wait"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if record, _ := manager.Get(spec.RunID); record.Status == WaitingTool {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := manager.Stop(context.Background(), spec.RunID); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(deadline) {
		if record, _ := manager.Get(spec.RunID); record.Status == Stopped {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if record, _ := manager.Get(spec.RunID); record.Status != Stopped {
		t.Fatalf("status=%s, want stopped", record.Status)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, ok := AcquireGlobalSlot(ctx)
	if !ok {
		t.Fatal("stopped waiting tool run leaked its global slot")
	}
	release()
}

func awaitRunStatus(t *testing.T, manager *Manager, runID string, want Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if record, ok := manager.Get(runID); ok && record.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	record, _ := manager.Get(runID)
	t.Fatalf("run status=%s, want %s", record.Status, want)
}

func TestToolCallLeavesRunWaitingAndEventsInheritImmutableBinding(t *testing.T) {
	resetRunSlots()
	manager, err := NewManager(filepath.Join(t.TempDir(), "runs"), 1, toolCallExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	spec := testSpec(t, "run-tool-wait")
	if _, err := manager.Submit(spec, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		record, ok := manager.Get(spec.RunID)
		if ok && record.Status == WaitingTool {
			if len(record.Events) != 1 {
				t.Fatalf("events=%d, want 1", len(record.Events))
			}
			event := record.Events[0]
			if event.RunID != spec.RunID || event.AttemptID != spec.AttemptID || event.ProviderID != spec.ProviderID || event.Sequence != 1 {
				t.Fatalf("event missing immutable binding: %+v", event)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	record, _ := manager.Get(spec.RunID)
	t.Fatalf("tool call run status=%s, want waiting_tool", record.Status)
}
