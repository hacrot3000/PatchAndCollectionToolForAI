package server

import "testing"

func TestFileTransferServerQueuePauseAndPriority(t *testing.T) {
	q := &fileTransferServerQueue{
		ProfileID: "p1",
		Paused:    true,
		Jobs:      map[string]*fileTransferServerJob{},
		wake:      make(chan struct{}, 1),
		Items: []*fileTransferServerItem{
			{ID: "a", JobID: "j", Status: "queued"},
			{ID: "b", JobID: "j", Status: "queued"},
		},
	}
	q.mu.Lock()
	if got := q.nextRunnableLocked(); got != nil {
		q.mu.Unlock()
		t.Fatalf("paused queue unexpectedly returned %#v", got)
	}
	q.Items[1].Priority = true
	got := q.nextRunnableLocked()
	q.mu.Unlock()
	if got == nil || got.ID != "b" {
		t.Fatalf("priority item should run through global pause, got %#v", got)
	}
	if got.Priority {
		t.Fatal("priority flag should be consumed when item is selected")
	}
}

func TestFileTransferServerQueueSnapshotPersistsState(t *testing.T) {
	q := &fileTransferServerQueue{
		ProfileID:   "profile",
		Paused:      true,
		ActiveScans: 2,
		Revision:    9,
		Jobs: map[string]*fileTransferServerJob{
			"job": {ID: "job", ProfileID: "profile", Kind: fileTransferJobHostUpload, Status: "running"},
		},
		Items: []*fileTransferServerItem{
			{ID: "one", JobID: "job", Kind: "Upload", Status: "running", Source: "a", Target: "/a"},
			{ID: "two", JobID: "job", Kind: "Upload", Status: "success", Source: "b", Target: "/b"},
			{ID: "hidden", JobID: "job", Kind: "Upload", Status: "success", Removed: true},
		},
		wake: make(chan struct{}, 1),
	}
	snapshot := q.snapshot()
	if !snapshot.Paused || snapshot.ActiveScans != 2 || snapshot.Revision != 9 {
		t.Fatalf("unexpected queue snapshot: %#v", snapshot)
	}
	if len(snapshot.Items) != 2 {
		t.Fatalf("removed items must stay hidden, got %d items", len(snapshot.Items))
	}
	if len(snapshot.Jobs) != 1 || snapshot.Jobs[0].ID != "job" {
		t.Fatalf("job snapshot missing: %#v", snapshot.Jobs)
	}
	// Snapshot values must be detached from manager state so an HTTP encoder or
	// caller cannot mutate the live background queue.
	snapshot.Items[0].Status = "failed"
	if q.Items[0].Status != "running" {
		t.Fatal("snapshot item aliases live queue state")
	}
}

func TestFileTransferServerQueueJobCompletionWaitsForScanAndItems(t *testing.T) {
	q := &fileTransferServerQueue{
		ProfileID: "profile",
		Jobs: map[string]*fileTransferServerJob{
			"job": {ID: "job", ProfileID: "profile", Kind: fileTransferJobRemoteDelete, Status: "scanning"},
		},
		Items: []*fileTransferServerItem{
			{ID: "one", JobID: "job", Status: "success"},
			{ID: "two", JobID: "job", Status: "queued"},
		},
		wake: make(chan struct{}, 1),
	}
	q.mu.Lock()
	q.recomputeJobLocked("job")
	if got := q.Jobs["job"].Status; got != "scanning" {
		q.mu.Unlock()
		t.Fatalf("scan still active: status=%q", got)
	}
	q.Jobs["job"].ScanDone = true
	q.recomputeJobLocked("job")
	if got := q.Jobs["job"].Status; got != "queued" {
		q.mu.Unlock()
		t.Fatalf("pending item should keep job queued: status=%q", got)
	}
	q.Items[1].Status = "success"
	q.recomputeJobLocked("job")
	got := q.Jobs["job"].Status
	q.mu.Unlock()
	if got != "done" {
		t.Fatalf("completed scan/items should finish job, status=%q", got)
	}
}
