package agentrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

type testExecutor struct {
	started   chan string
	slotHeld  chan bool
	release   <-chan struct{}
	err       error
	active    atomic.Int32
	peak      atomic.Int32
	mu        sync.Mutex
	cancelled map[string]bool
}

func (e *testExecutor) Execute(ctx context.Context, spec agentchat.RunSpec, _ []agentchat.HistoryItem, _ agentchat.EventSink) error {
	active := e.active.Add(1)
	for {
		peak := e.peak.Load()
		if active <= peak || e.peak.CompareAndSwap(peak, active) {
			break
		}
	}
	defer e.active.Add(-1)
	if e.started != nil {
		e.started <- spec.RunID
	}
	if e.slotHeld != nil {
		e.slotHeld <- GlobalSlotHeld(ctx)
	}
	if e.release == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.release:
		return e.err
	}
}

func (e *testExecutor) Cancel(_ context.Context, spec agentchat.RunSpec) error {
	e.mu.Lock()
	if e.cancelled == nil {
		e.cancelled = map[string]bool{}
	}
	e.cancelled[spec.RunID] = true
	e.mu.Unlock()
	return nil
}

func testSpec(t *testing.T, id string) agentchat.RunSpec {
	t.Helper()
	spec := agentchat.RunSpec{
		RunID: id, ConversationID: "conversation-" + id,
		OwnerWorkspaceID: "owner", ExecutionWorkspaceID: "execution-" + id,
		ProviderID: "provider-a", ModelID: "model-a", ProviderConfigVersion: "v1",
		ContextSnapshotID: "snapshot-" + id, AttemptID: "attempt-1", IdempotencyKey: "idem-" + id,
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	return spec
}

func awaitStatus(t *testing.T, managers []*Manager, ids []string, want Status) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		all := true
		for i, manager := range managers {
			record, ok := manager.Get(ids[i])
			if !ok || record.Status != want {
				all = false
				break
			}
		}
		if all {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("runs did not reach %s", want)
}

func TestGlobalConcurrencyNeverExceedsFiveAcrossManagers(t *testing.T) {
	resetRunSlots()
	release := make(chan struct{})
	executor := &testExecutor{started: make(chan string, 8), release: release}
	root := t.TempDir()
	m1, err := NewManager(filepath.Join(root, "one"), 5, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer m1.Close()
	m2, err := NewManager(filepath.Join(root, "two"), 5, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	managers := []*Manager{m1, m2, m1, m2, m1, m2, m1}
	ids := make([]string, len(managers))
	for i, manager := range managers {
		ids[i] = "run-" + string(rune('a'+i))
		if _, err := manager.Submit(testSpec(t, ids[i]), nil); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < MaxConcurrentRuns; i++ {
		select {
		case <-executor.started:
		case <-time.After(3 * time.Second):
			t.Fatal("expected five active runs")
		}
	}
	if got := executor.active.Load(); got != MaxConcurrentRuns {
		t.Fatalf("active=%d, want %d", got, MaxConcurrentRuns)
	}
	select {
	case id := <-executor.started:
		t.Fatalf("sixth run %s started before a slot was released", id)
	default:
	}
	close(release)
	for _, manager := range managers {
		manager.Close()
	}
	if got := executor.peak.Load(); got > MaxConcurrentRuns {
		t.Fatalf("peak concurrency=%d exceeds limit %d", got, MaxConcurrentRuns)
	}
}

func TestManagerMarksGlobalSlotForNestedAgentChatAdmission(t *testing.T) {
	resetRunSlots()
	executor := &testExecutor{slotHeld: make(chan bool, 1)}
	manager, err := NewManager(filepath.Join(t.TempDir(), "runs"), 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	spec := testSpec(t, "run-slot-marker")
	if _, err := manager.Submit(spec, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case held := <-executor.slotHeld:
		if !held {
			t.Fatal("run executor context did not carry its already-held global slot")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run executor did not start")
	}
}

func TestStopOnlyCancelsSelectedRun(t *testing.T) {
	resetRunSlots()
	executor := &testExecutor{started: make(chan string, 2)}
	manager, err := NewManager(filepath.Join(t.TempDir(), "runs"), 2, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	for _, id := range []string{"run-a", "run-b"} {
		if _, err := manager.Submit(testSpec(t, id), nil); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-executor.started:
			seen[id] = true
		case <-time.After(3 * time.Second):
			t.Fatal("runs did not start")
		}
	}
	if err := manager.Stop(context.Background(), "run-a"); err != nil {
		t.Fatal(err)
	}
	executor.mu.Lock()
	stoppedA, stoppedB := executor.cancelled["run-a"], executor.cancelled["run-b"]
	executor.mu.Unlock()
	if !stoppedA || stoppedB {
		t.Fatalf("cancel scope A=%v B=%v", stoppedA, stoppedB)
	}
	if record, _ := manager.Get("run-b"); record.Status != Running {
		t.Fatalf("run B state=%s, want running", record.Status)
	}
}

func TestQueuedRunRestoresAndExecutesSafeWork(t *testing.T) {
	resetRunSlots()
	dir := filepath.Join(t.TempDir(), "runs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	spec := testSpec(t, "run-queued-restart")
	seed := &Manager{dir: dir}
	queued := Record{Spec: spec, Status: Queued, Attempt: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := seed.saveLocked(queued); err != nil {
		t.Fatal(err)
	}
	executor := &testExecutor{started: make(chan string, 1)}
	manager, err := NewManager(dir, 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	select {
	case runID := <-executor.started:
		if runID != spec.RunID {
			t.Fatalf("started run %q", runID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("safe queued work did not resume after restart")
	}
	if record, ok := manager.Get(spec.RunID); !ok || record.Status != Running || record.WorkerEpoch == "" {
		t.Fatal("queued metadata was not restored into the current worker epoch")
	}
}

func TestRestartMarksInFlightInterruptedAndRetryKeepsBinding(t *testing.T) {
	resetRunSlots()
	dir := filepath.Join(t.TempDir(), "runs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	spec := testSpec(t, "run-restart")
	original := Record{Spec: spec, Status: Running, Attempt: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	seed := &Manager{dir: dir}
	if err := seed.saveLocked(original); err != nil {
		t.Fatal(err)
	}
	executor := &testExecutor{}
	manager, err := NewManager(dir, 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	loaded, ok := manager.Get(spec.RunID)
	if !ok || loaded.Status != Interrupted {
		t.Fatalf("loaded status=%s, want interrupted", loaded.Status)
	}
	if _, err := manager.Retry(spec.RunID); err == nil {
		t.Fatal("uncertain interrupted run was retried without review")
	}
	if _, err := manager.ResumeAfterReview(spec.RunID, false); err == nil {
		t.Fatal("uncertain run resumed without explicit side-effect review")
	}
	retried, err := manager.ResumeAfterReview(spec.RunID, true)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Spec.OwnerWorkspaceID != spec.OwnerWorkspaceID || retried.Spec.ExecutionWorkspaceID != spec.ExecutionWorkspaceID || retried.Spec.ProviderID != spec.ProviderID || retried.Spec.ModelID != spec.ModelID || retried.Spec.ContextSnapshotID != spec.ContextSnapshotID {
		t.Fatal("retry changed immutable run binding")
	}
	if retried.Spec.AttemptID == spec.AttemptID || retried.Spec.IdempotencyKey == spec.IdempotencyKey || retried.Attempt != 2 {
		t.Fatal("retry did not create a new attempt identity")
	}
}

func TestRetryRequiresSafeFailureClassification(t *testing.T) {
	resetRunSlots()
	release := make(chan struct{})
	close(release)
	executor := &testExecutor{release: release, err: RetrySafeError{Err: errors.New("validation failed before provider egress")}}
	manager, err := NewManager(filepath.Join(t.TempDir(), "runs"), 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	spec := testSpec(t, "run-retry-safe")
	if _, err := manager.Submit(spec, nil); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, []*Manager{manager}, []string{spec.RunID}, Failed)
	record, _ := manager.Get(spec.RunID)
	if !record.RetrySafe || record.LastError == "validation failed before provider egress" {
		t.Fatal("retryability or error redaction was not persisted safely")
	}
	if retried, err := manager.Retry(spec.RunID); err != nil || retried.Attempt != 2 {
		t.Fatalf("safe retry failed: %+v %v", retried, err)
	}
}

func resetRunSlots() {
	for {
		select {
		case <-globalRunSlots:
		default:
			return
		}
	}
}
