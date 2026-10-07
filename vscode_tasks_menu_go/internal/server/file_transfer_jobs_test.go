package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStopFileTransferScansCancelsActiveAndDropsPending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	q := &fileTransferServerQueue{
		ProfileID: "p1",
		Jobs: map[string]*fileTransferServerJob{
			"active": {ID: "active", Status: "scanning"},
			"queued": {ID: "queued", Status: "scan_queued"},
		},
		PendingScans: []fileTransferPendingScan{{JobID: "queued"}},
		QueuedScans:  1,
		ScanCancels:  map[string]context.CancelFunc{"active": cancel},
	}
	q.mu.Lock()
	stopFileTransferScansLocked(q)
	q.mu.Unlock()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("active scan context was not canceled")
	}
	if len(q.PendingScans) != 0 || q.QueuedScans != 0 {
		t.Fatalf("pending scans not cleared: pending=%d queued=%d", len(q.PendingScans), q.QueuedScans)
	}
	for id, job := range q.Jobs {
		if !job.ScanDone || job.Status != "stopped" {
			t.Fatalf("job %s not marked stopped: %#v", id, job)
		}
	}
}

func TestClearFileTransferQueueDropsLargePendingStateButKeepsRunningItem(t *testing.T) {
	q := &fileTransferServerQueue{
		ProfileID: "p1",
		Jobs: map[string]*fileTransferServerJob{
			"queued":  {ID: "queued", Status: "queued", ScanDone: true},
			"running": {ID: "running", Status: "running", ScanDone: true},
		},
		Items: []*fileTransferServerItem{
			{ID: "q", JobID: "queued", Status: "queued"},
			{ID: "f", JobID: "queued", Status: "failed"},
			{ID: "r", JobID: "running", Status: "running"},
		},
		PendingScans: []fileTransferPendingScan{{JobID: "queued"}},
		QueuedScans:  1,
	}
	q.mu.Lock()
	clearFileTransferQueueLocked(q)
	q.mu.Unlock()
	if len(q.Items) != 1 || q.Items[0].ID != "r" || !q.Items[0].RemoveAfterRun {
		t.Fatalf("clear queue must retain only running item with remove-after-run: %#v", q.Items)
	}
	if len(q.Jobs) != 1 || q.Jobs["running"] == nil {
		t.Fatalf("clear queue retained stale jobs: %#v", q.Jobs)
	}
	if len(q.PendingScans) != 0 || q.QueuedScans != 0 {
		t.Fatalf("clear queue retained scan backlog: pending=%d queued=%d", len(q.PendingScans), q.QueuedScans)
	}
}

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

func TestFileTransferConnectionBudgetCountsScansTransfersAndBrowse(t *testing.T) {
	q := &fileTransferServerQueue{MaxConnections: 3}
	cases := []struct {
		scans, transfers, browses int
		wantActive                int
		wantCapacity              bool
	}{
		{scans: 1, transfers: 1, wantActive: 2, wantCapacity: true},
		{scans: 1, transfers: 2, wantActive: 3, wantCapacity: false},
		{scans: 2, transfers: 1, wantActive: 3, wantCapacity: false},
		{scans: 2, browses: 1, wantActive: 3, wantCapacity: false},
	}
	for _, tc := range cases {
		q.ActiveScans, q.ActiveTransfers, q.ActiveBrowses = tc.scans, tc.transfers, tc.browses
		if got := q.activeConnectionsLocked(); got != tc.wantActive {
			t.Fatalf("scans=%d transfers=%d browses=%d active=%d want=%d", tc.scans, tc.transfers, tc.browses, got, tc.wantActive)
		}
		if got := q.hasConnectionCapacityLocked(); got != tc.wantCapacity {
			t.Fatalf("scans=%d transfers=%d browses=%d capacity=%v want=%v", tc.scans, tc.transfers, tc.browses, got, tc.wantCapacity)
		}
	}
}

func TestFileTransferScanQueueWaitsWhenConnectionBudgetIsFull(t *testing.T) {
	q := &fileTransferServerQueue{
		ProfileID: "p1", MaxConnections: 3,
		Jobs: map[string]*fileTransferServerJob{}, wake: make(chan struct{}, 1),
	}
	for i := 1; i <= 4; i++ {
		id := string(rune('a' + i - 1))
		req := fileTransferJobCreateRequest{ProfileID: "p1", Kind: fileTransferJobHostUpload, HostPaths: []string{"file-" + id}}
		q.Jobs[id] = &fileTransferServerJob{ID: id, ProfileID: "p1", Kind: fileTransferJobHostUpload, Status: "scan_queued", Request: cloneFileTransferJobRequest(&req)}
		q.enqueueScanLocked(id, req)
	}
	if q.QueuedScans != 4 {
		t.Fatalf("queued scans=%d want=4", q.QueuedScans)
	}
	for i := 0; i < 3; i++ {
		if !q.hasConnectionCapacityLocked() {
			t.Fatalf("budget filled too early at scan %d", i+1)
		}
		scan := q.nextPendingScanLocked()
		if scan == nil {
			t.Fatalf("missing queued scan %d", i+1)
		}
		q.ActiveScans++
	}
	if q.hasConnectionCapacityLocked() {
		t.Fatal("three active scans must consume max_connections=3")
	}
	if q.QueuedScans != 1 || len(q.PendingScans) != 1 {
		t.Fatalf("fourth scan should stay queued: queued=%d pending=%d", q.QueuedScans, len(q.PendingScans))
	}
	if got := q.PendingScans[0].JobID; got != "d" {
		t.Fatalf("queued scan=%q want d", got)
	}
}

func TestFileTransferQueueSnapshotReportsConnectionUsage(t *testing.T) {
	q := &fileTransferServerQueue{
		ProfileID: "profile", MaxConnections: 3,
		ActiveScans: 1, ActiveTransfers: 1, ActiveBrowses: 1, QueuedScans: 2,
		Jobs: map[string]*fileTransferServerJob{}, wake: make(chan struct{}, 1),
	}
	snapshot := q.snapshot()
	if snapshot.MaxConnections != 3 || snapshot.ActiveConnections != 3 {
		t.Fatalf("connection snapshot=%#v", snapshot)
	}
	if snapshot.ActiveScans != 1 || snapshot.ActiveTransfers != 1 || snapshot.ActiveBrowses != 1 || snapshot.QueuedScans != 2 {
		t.Fatalf("connection detail snapshot=%#v", snapshot)
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

func TestFileTransferConflictPolicyNormalization(t *testing.T) {
	for _, value := range []string{
		fileTransferConflictAsk,
		fileTransferConflictOverwrite,
		fileTransferConflictSkip,
		fileTransferConflictSizeDiff,
		fileTransferConflictSourceNewer,
		fileTransferConflictChecksumDiff,
	} {
		got, err := normalizeFileTransferConflictPolicy(value)
		if err != nil || got != value {
			t.Fatalf("policy %q => %q err=%v", value, got, err)
		}
	}
	if _, err := normalizeFileTransferConflictPolicy("invented"); err == nil {
		t.Fatal("unknown conflict policy unexpectedly accepted")
	}
	for _, value := range []string{
		fileTransferConflictScopeItem,
		fileTransferConflictScopeJob,
		fileTransferConflictScopeDirectionSession,
		fileTransferConflictScopeDirectionAlways,
	} {
		got, err := normalizeFileTransferConflictScope(value)
		if err != nil || got != value {
			t.Fatalf("scope %q => %q err=%v", value, got, err)
		}
	}
}

func TestFileTransferConflictSizePolicyQueuesOrSkipsWithoutNetwork(t *testing.T) {
	s := &Server{}
	q := &fileTransferServerQueue{
		ProfileID: "p",
		Jobs: map[string]*fileTransferServerJob{
			"job": {ID: "job", ProfileID: "p", Kind: fileTransferJobHostDownload, ScanDone: true},
		},
		wake: make(chan struct{}, 1),
	}
	conflict := &fileTransferServerItem{
		ID: "one", JobID: "job", Kind: "Download", Direction: "←",
		Source: "/remote/a.bin", Target: "a.bin", Status: "conflict",
		Operation: "host_download",
		Conflict: &fileTransferConflictMeta{SourceSize: 12, TargetSize: 9},
	}
	q.Items = []*fileTransferServerItem{conflict}
	if err := s.resolveServerConflictItem(q, conflict, fileTransferConflictSizeDiff); err != nil {
		t.Fatal(err)
	}
	if conflict.Status != "queued" || !conflict.Overwrite || conflict.Decision != fileTransferConflictSizeDiff {
		t.Fatalf("different sizes should queue overwrite: %#v", conflict)
	}

	same := &fileTransferServerItem{
		ID: "two", JobID: "job", Kind: "Download", Direction: "←",
		Source: "/remote/b.bin", Target: "b.bin", Status: "conflict",
		Operation: "host_download",
		Conflict: &fileTransferConflictMeta{SourceSize: 7, TargetSize: 7},
	}
	q.Items = append(q.Items, same)
	if err := s.resolveServerConflictItem(q, same, fileTransferConflictSizeDiff); err != nil {
		t.Fatal(err)
	}
	if same.Status != "skipped" || same.Overwrite || same.Decision != fileTransferConflictSizeDiff {
		t.Fatalf("same size should skip: %#v", same)
	}
}

func TestFileTransferConflictScopeCanApplyToWholeJobAndDirection(t *testing.T) {
	s := &Server{}
	q := &fileTransferServerQueue{
		ProfileID: "p",
		Jobs: map[string]*fileTransferServerJob{
			"job1": {ID: "job1", ProfileID: "p", Kind: fileTransferJobHostUpload, ScanDone: true},
			"job2": {ID: "job2", ProfileID: "p", Kind: fileTransferJobHostUpload, ScanDone: true},
		},
		wake: make(chan struct{}, 1),
		Items: []*fileTransferServerItem{
			{ID: "a", JobID: "job1", Kind: "Upload", Status: "conflict", Conflict: &fileTransferConflictMeta{SourceSize: 1, TargetSize: 1}},
			{ID: "b", JobID: "job1", Kind: "Upload", Status: "conflict", Conflict: &fileTransferConflictMeta{SourceSize: 2, TargetSize: 2}},
			{ID: "c", JobID: "job2", Kind: "Upload", Status: "conflict", Conflict: &fileTransferConflictMeta{SourceSize: 3, TargetSize: 3}},
		},
	}
	if err := s.resolveServerConflicts(q, fileTransferJobControlRequest{
		ProfileID: "p", Action: "resolve_conflict", ItemIDs: []string{"a"},
		ConflictPolicy: fileTransferConflictSkip, ConflictScope: fileTransferConflictScopeJob, JobID: "job1",
	}); err != nil {
		t.Fatal(err)
	}
	if q.Items[0].Status != "skipped" || q.Items[1].Status != "skipped" || q.Items[2].Status != "conflict" {
		t.Fatalf("job scope applied incorrectly: %#v", q.Items)
	}
	if got := q.Jobs["job1"].ConflictPolicy; got != fileTransferConflictSkip {
		t.Fatalf("job policy=%q", got)
	}

	if err := s.resolveServerConflicts(q, fileTransferJobControlRequest{
		ProfileID: "p", Action: "resolve_conflict", ItemIDs: []string{"c"},
		ConflictPolicy: fileTransferConflictOverwrite, ConflictScope: fileTransferConflictScopeDirectionSession,
	}); err != nil {
		t.Fatal(err)
	}
	if got := q.UploadConflictPolicy; got != fileTransferConflictOverwrite {
		t.Fatalf("upload session policy=%q", got)
	}
	if q.Items[2].Status != "queued" {
		t.Fatalf("direction conflict was not resolved: %#v", q.Items[2])
	}
}

func TestFileTransferConflictScopesApplyToFutureConflicts(t *testing.T) {
	s := &Server{}
	q := &fileTransferServerQueue{
		ProfileID: "p",
		Jobs: map[string]*fileTransferServerJob{
			"job1": {ID: "job1", ProfileID: "p", Kind: fileTransferJobHostUpload, ScanDone: false},
			"job2": {ID: "job2", ProfileID: "p", Kind: fileTransferJobHostUpload, ScanDone: false},
			"job3": {ID: "job3", ProfileID: "p", Kind: fileTransferJobHostUpload, ScanDone: false},
		},
		wake: make(chan struct{}, 8),
		Items: []*fileTransferServerItem{
			{ID: "a", JobID: "job1", Kind: "Upload", Status: "conflict", Conflict: &fileTransferConflictMeta{SourceSize: 1, TargetSize: 1}},
			{ID: "b", JobID: "job2", Kind: "Upload", Status: "conflict", Conflict: &fileTransferConflictMeta{SourceSize: 2, TargetSize: 2}},
		},
	}

	if err := s.resolveServerConflicts(q, fileTransferJobControlRequest{
		ProfileID: "p", Action: "resolve_conflict", ItemIDs: []string{"a"},
		ConflictPolicy: fileTransferConflictSkip, ConflictScope: fileTransferConflictScopeJob, JobID: "job1",
	}); err != nil {
		t.Fatal(err)
	}
	if got := q.effectiveConflictPolicy("job1", "Upload"); got != fileTransferConflictSkip {
		t.Fatalf("future job conflict policy=%q want %q", got, fileTransferConflictSkip)
	}
	if err := s.enqueueServerTransferConflictAware(
		q, "job1", "Upload", "→", "later-job.txt", "/later-job.txt", "host_upload", 3,
		&fileTransferConflictMeta{SourceSize: 3, TargetSize: 3},
	); err != nil {
		t.Fatal(err)
	}
	jobFuture := q.Items[len(q.Items)-1]
	if jobFuture.Status != "skipped" || jobFuture.Decision != fileTransferConflictSkip {
		t.Fatalf("future conflict in same transfer asked again instead of applying job policy: %#v", jobFuture)
	}

	if err := s.resolveServerConflicts(q, fileTransferJobControlRequest{
		ProfileID: "p", Action: "resolve_conflict", ItemIDs: []string{"b"},
		ConflictPolicy: fileTransferConflictOverwrite, ConflictScope: fileTransferConflictScopeDirectionSession,
	}); err != nil {
		t.Fatal(err)
	}
	if got := q.effectiveConflictPolicy("job3", "Upload"); got != fileTransferConflictOverwrite {
		t.Fatalf("future direction conflict policy=%q want %q", got, fileTransferConflictOverwrite)
	}
	if err := s.enqueueServerTransferConflictAware(
		q, "job3", "Upload", "→", "later-session.txt", "/later-session.txt", "host_upload", 4,
		&fileTransferConflictMeta{SourceSize: 4, TargetSize: 4},
	); err != nil {
		t.Fatal(err)
	}
	directionFuture := q.Items[len(q.Items)-1]
	if directionFuture.Status != "queued" || directionFuture.Decision != fileTransferConflictOverwrite {
		t.Fatalf("future upload conflict asked again instead of applying session policy: %#v", directionFuture)
	}
}

func TestParseFileTransferModifiedSupportsRFC3339AndSFTPDisplayTime(t *testing.T) {
	now := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	if got, ok := parseFileTransferModified("2026-10-02T10:00:00Z", now); !ok || got.Hour() != 10 {
		t.Fatalf("RFC3339 parse got=%v ok=%v", got, ok)
	}
	got, ok := parseFileTransferModified("Oct 2 09:30", now)
	if !ok || got.Year() != 2026 || got.Month() != time.October || got.Day() != 2 || got.Hour() != 9 {
		t.Fatalf("SFTP display parse got=%v ok=%v", got, ok)
	}
	if _, ok := parseFileTransferModified("unknown", now); ok {
		t.Fatal("unknown modified time unexpectedly parsed")
	}
}

func TestHashFileSHA256UsesRealFileContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := hashFileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("sha256=%q want %q", got, want)
	}
}

func TestFileTransferQueueSnapshotDeepCopiesConflictMetadata(t *testing.T) {
	q := &fileTransferServerQueue{
		ProfileID: "p",
		Jobs: map[string]*fileTransferServerJob{"j": {ID: "j", ProfileID: "p"}},
		Items: []*fileTransferServerItem{
			{ID: "a", JobID: "j", Status: "conflict", Conflict: &fileTransferConflictMeta{SourceSize: 11, TargetSize: 22}},
		},
		wake: make(chan struct{}, 1),
	}
	snapshot := q.snapshot()
	if len(snapshot.Items) != 1 || snapshot.Items[0].Conflict == nil {
		t.Fatalf("snapshot conflict missing: %#v", snapshot.Items)
	}
	snapshot.Items[0].Conflict.SourceSize = 999
	if q.Items[0].Conflict.SourceSize != 11 {
		t.Fatal("snapshot conflict metadata aliases live queue state")
	}
}
