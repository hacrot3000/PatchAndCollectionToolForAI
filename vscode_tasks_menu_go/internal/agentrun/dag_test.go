package agentrun

import (
	"path/filepath"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/taskmanifest"
)

func testManifest(t *testing.T) taskmanifest.Manifest {
	t.Helper()
	manifest, err := taskmanifest.ParseMarkdown("# Plan\n## Task A — base\nAcceptance:\n- base ready\n## Task B — dependent\nDepends on: A\nAcceptance:\n- dependent ready\n## Task C — downstream\nDepends on: B\nAcceptance:\n- downstream ready\n", "project-1")
	if err != nil || !manifest.Valid {
		t.Fatalf("manifest invalid: %v %+v", err, manifest.Issues)
	}
	return manifest
}

func TestDAGDependencyMustSucceedBeforeClaim(t *testing.T) {
	scheduler, err := NewDAGScheduler(testManifest(t), 2)
	if err != nil {
		t.Fatal(err)
	}
	first, task, ok, err := scheduler.ClaimReady()
	if err != nil || !ok || task.ID != "A" {
		t.Fatalf("first claim=%+v %q %v %v", first, task.ID, ok, err)
	}
	if _, _, ok, err := scheduler.ClaimReady(); err != nil || ok {
		t.Fatal("dependent task unlocked before prerequisite success")
	}
	if got := scheduler.Snapshot().States["B"].WaitReason; got != "dependency" {
		t.Fatalf("blocked reason=%q, want dependency", got)
	}
	if err := scheduler.Complete("A", first.Fence, TaskFailed, "build failed"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := scheduler.ClaimReady(); err != nil || ok {
		t.Fatal("failed prerequisite opened the dependent")
	}
	if scheduler.Snapshot().States["B"].Status != TaskBlocked || scheduler.Snapshot().States["C"].Status != TaskBlocked {
		t.Fatal("dependent chain was not reported blocked")
	}
	if err := scheduler.Retry("A"); err != nil {
		t.Fatal(err)
	}
	if scheduler.Snapshot().States["B"].Status != TaskQueued || scheduler.Snapshot().States["C"].Status != TaskQueued {
		t.Fatal("retry did not reopen the full downstream chain")
	}
	second, _, ok, err := scheduler.ClaimReady()
	if err != nil || !ok || second.Attempt != 2 {
		t.Fatalf("retry claim=%+v %v %v", second, ok, err)
	}
	if err := scheduler.Complete("A", second.Fence, TaskSucceeded, "accepted"); err != nil {
		t.Fatal(err)
	}
	third, task, ok, err := scheduler.ClaimReady()
	if err != nil || !ok || task.ID != "B" {
		t.Fatalf("dependent claim=%+v %q %v %v", third, task.ID, ok, err)
	}
	if err := scheduler.Complete("B", third.Fence, TaskSucceeded, "dependent accepted"); err != nil {
		t.Fatal(err)
	}
	fourth, task, ok, err := scheduler.ClaimReady()
	if err != nil || !ok || task.ID != "C" {
		t.Fatalf("downstream claim=%+v %q %v %v", fourth, task.ID, ok, err)
	}
	if err := scheduler.Complete("A", first.Fence, TaskSucceeded, "stale completion"); err == nil {
		t.Fatal("stale worker completion accepted")
	}
}

func TestDAGConcurrencyIsCappedAndSnapshotResumesSafely(t *testing.T) {
	manifest, err := taskmanifest.ParseMarkdown("# Plan\n## Task A — one\nAcceptance:\n- one\n## Task B — two\nAcceptance:\n- two\n## Task C — three\nAcceptance:\n- three\n", "project-1")
	if err != nil {
		t.Fatal(err)
	}
	scheduler, err := NewDAGScheduler(manifest, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxConcurrentRuns+1; i++ {
		if _, _, ok, err := scheduler.ClaimReady(); err != nil {
			t.Fatal(err)
		} else if ok != (i < 2) {
			t.Fatalf("claim %d ok=%v", i, ok)
		}
	}
	claimed, _, _, _ := scheduler.ClaimReady()
	if claimed.Status != "" {
		t.Fatal("unexpected claim after all root tasks started")
	}
	if got := scheduler.Snapshot().States["C"].WaitReason; got != "concurrency_slot" {
		t.Fatalf("queued reason=%q, want concurrency_slot", got)
	}
	path := filepath.Join(t.TempDir(), "dag.json")
	if err := SaveDAGSnapshot(path, scheduler.Snapshot()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := LoadDAGSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreDAGScheduler(manifest, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restoredStates := restored.Snapshot().States
	interrupted, queued := 0, 0
	for _, state := range restoredStates {
		if state.Status == TaskInterrupted {
			interrupted++
		}
		if state.Status == TaskQueued {
			queued++
		}
	}
	if interrupted != 2 || queued != 1 || restoredStates["C"].WaitReason != "ready" {
		t.Fatalf("restart statuses lost active/waiting distinction: %+v", restoredStates)
	}
}

func TestDAGRejectsConcurrencyAboveGlobalLimit(t *testing.T) {
	if _, err := NewDAGScheduler(testManifest(t), MaxConcurrentRuns+1); err == nil {
		t.Fatal("DAG accepted concurrency above five")
	}
}
