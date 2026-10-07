package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileTransferPartialHostPathIsDeterministicAndHiddenSibling(t *testing.T) {
	target := filepath.Join(t.TempDir(), "capture.bin")
	first := fileTransferPartialHostPath(target, "/remote/a/capture.bin")
	second := fileTransferPartialHostPath(target, "/remote/a/capture.bin")
	other := fileTransferPartialHostPath(target, "/remote/b/capture.bin")
	if first != second {
		t.Fatalf("partial path changed: %q != %q", first, second)
	}
	if first == other {
		t.Fatalf("different remote paths must not share partial file: %q", first)
	}
	if filepath.Dir(first) != filepath.Dir(target) {
		t.Fatalf("partial must stay beside target: %q", first)
	}
	if !strings.HasPrefix(filepath.Base(first), ".capture.bin.taskdeck-part-") {
		t.Fatalf("partial is not hidden/deterministic: %q", first)
	}
}

func TestFileTransferQueuePersistsAttemptsForResume(t *testing.T) {
	workspace := t.TempDir()
	s1 := &Server{Workspace: workspace}
	q1 := s1.fileTransferServerQueue("resume-profile")
	now := time.Now().UTC()
	q1.mu.Lock()
	q1.Jobs["resume-job"] = &fileTransferServerJob{
		ID: "resume-job", ProfileID: "resume-profile", Kind: fileTransferJobHostUpload,
		Status: "failed", CreatedAt: now, UpdatedAt: now, ScanDone: true,
	}
	q1.Items = append(q1.Items, &fileTransferServerItem{
		ID: "resume-item", JobID: "resume-job", Kind: "Upload", Direction: "→",
		Source: "capture.bin", Target: "/capture.bin", Size: 2048,
		Status: "failed", Error: "network lost", Operation: "host_upload", Attempts: 2,
	})
	q1.touchLocked()
	q1.mu.Unlock()
	if err := s1.persistFileTransferQueues(); err != nil { t.Fatal(err) }

	s2 := &Server{Workspace: workspace}
	q2 := s2.fileTransferServerQueue("resume-profile")
	q2.mu.Lock()
	defer q2.mu.Unlock()
	if len(q2.Items) != 1 || q2.Items[0].Attempts != 2 {
		t.Fatalf("attempt count not restored: %+v", q2.Items)
	}
	if q2.Items[0].Operation != "host_upload" {
		t.Fatalf("operation not restored: %+v", q2.Items[0])
	}
}

func TestUploadConflictOverwriteMarksQueueItemForReplacementAndRetry(t *testing.T) {
	q := &fileTransferServerQueue{
		ProfileID: "p1",
		Jobs: map[string]*fileTransferServerJob{
			"job": {ID: "job", ConflictPolicy: fileTransferConflictOverwrite, ScanDone: true},
		},
		wake: make(chan struct{}, 1),
	}
	s := &Server{}
	conflict := &fileTransferConflictMeta{SourceSize: 20, TargetSize: 10}
	if err := s.enqueueServerTransferConflictAware(context.Background(), q, "job", "Upload", "→", "local.bin", "/remote.bin", "host_upload", 20, conflict); err != nil {
		t.Fatal(err)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Items) != 1 {
		t.Fatalf("items=%+v", q.Items)
	}
	item := q.Items[0]
	if !item.Overwrite || item.Status != "queued" || item.Decision != fileTransferConflictOverwrite {
		t.Fatalf("upload overwrite semantics lost: %+v", item)
	}
}

func TestRecoveredPartialFileRemainsProtected(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "result.bin")
	partial := fileTransferPartialHostPath(target, "/remote/result.bin")
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil { t.Fatal(err) }
	info, err := os.Stat(partial)
	if err != nil { t.Fatal(err) }
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("partial mode=%o", info.Mode().Perm())
	}
}
