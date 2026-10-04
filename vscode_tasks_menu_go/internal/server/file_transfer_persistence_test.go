package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

func TestFileTransferQueuePersistsAndRestoresRunningItemPaused(t *testing.T) {
	workspace := t.TempDir()
	s1 := &Server{Workspace: workspace}
	q1 := s1.fileTransferServerQueue("profile-a")
	now := time.Now().UTC()
	job := &fileTransferServerJob{
		ID: "job-a", ProfileID: "profile-a", Kind: fileTransferJobHostDownload,
		Status: "running", CreatedAt: now, UpdatedAt: now, ScanDone: true,
		Request: &fileTransferJobCreateRequest{ProfileID: "profile-a", Kind: fileTransferJobHostDownload, HostDir: "downloads"},
	}
	q1.mu.Lock()
	q1.Jobs[job.ID] = job
	q1.Sequence = 1
	q1.Items = append(q1.Items, &fileTransferServerItem{
		ID: "item-a", JobID: job.ID, Kind: "Download", Direction: "←",
		Source: "/remote/a.bin", Target: "downloads/a.bin", Size: 123,
		Status: "running", Operation: "host_download", Overwrite: true,
	})
	q1.touchLocked()
	q1.mu.Unlock()
	if err := s1.persistFileTransferQueues(); err != nil {
		t.Fatal(err)
	}

	statePath, err := projectfiles.Resolve(workspace, fileTransferQueueStateFile)
	if err != nil { t.Fatal(err) }
	info, err := os.Stat(statePath)
	if err != nil { t.Fatal(err) }
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}

	s2 := &Server{Workspace: workspace}
	q2 := s2.fileTransferServerQueue("profile-a")
	snap := q2.snapshot()
	if !snap.Paused {
		t.Fatalf("restored queue must be paused: %+v", snap)
	}
	if len(snap.Items) != 1 || snap.Items[0].Status != "queued" {
		t.Fatalf("restored items=%+v", snap.Items)
	}
	if !strings.Contains(snap.Items[0].Error, "daemon restart") {
		t.Fatalf("recovery explanation missing: %+v", snap.Items[0])
	}
	q2.mu.Lock()
	restored := q2.Items[0]
	if restored.Operation != "host_download" || !restored.Overwrite {
		t.Fatalf("internal item fields not restored: %+v", restored)
	}
	q2.mu.Unlock()
}

func TestFileTransferQueueRestoresInterruptedScanForExplicitResume(t *testing.T) {
	workspace := t.TempDir()
	s1 := &Server{Workspace: workspace}
	q1 := s1.fileTransferServerQueue("profile-b")
	now := time.Now().UTC()
	request := &fileTransferJobCreateRequest{
		ProfileID: "profile-b", Kind: fileTransferJobHostUpload,
		HostPaths: []string{"build/output.bin"}, RemoteDir: "/uploads",
	}
	q1.mu.Lock()
	q1.Jobs["job-scan"] = &fileTransferServerJob{
		ID: "job-scan", ProfileID: "profile-b", Kind: fileTransferJobHostUpload,
		Status: "scanning", CreatedAt: now, UpdatedAt: now, ScanDone: false,
		Request: cloneFileTransferJobRequest(request),
	}
	q1.Items = append(q1.Items, &fileTransferServerItem{
		ID: "known", JobID: "job-scan", Kind: "Upload", Direction: "→",
		Source: "build/already.bin", Target: "/uploads/already.bin",
		Status: "success", Operation: "host_upload",
	})
	q1.touchLocked()
	q1.mu.Unlock()
	if err := s1.persistFileTransferQueues(); err != nil { t.Fatal(err) }

	s2 := &Server{Workspace: workspace}
	q2 := s2.fileTransferServerQueue("profile-b")
	q2.mu.Lock()
	job := q2.Jobs["job-scan"]
	if job == nil || !job.NeedsRescan || !job.Recovered || job.Request == nil {
		t.Fatalf("restored job=%+v", job)
	}
	if !q2.Paused {
		t.Fatal("interrupted scan queue must restore paused")
	}
	q2.mu.Unlock()
}

func TestFileTransferQueueDeduplicatesRecoveredScanItems(t *testing.T) {
	q := &fileTransferServerQueue{Jobs: map[string]*fileTransferServerJob{}, wake: make(chan struct{}, 1)}
	first := q.addItem("job", "Upload", "→", "a.bin", "/a.bin", "host_upload", 10, false)
	first.Status = "success"
	second := q.addItem("job", "Upload", "→", "a.bin", "/a.bin", "host_upload", 10, false)
	if first != second || len(q.Items) != 1 {
		t.Fatalf("dedup failed: first=%p second=%p items=%d", first, second, len(q.Items))
	}
}

func TestFileTransferQueueStateRejectsSymlink(t *testing.T) {
	workspace := t.TempDir()
	path, err := projectfiles.Resolve(workspace, fileTransferQueueStateFile)
	if err != nil { t.Fatal(err) }
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"version":1,"queues":[]}`), 0o600); err != nil { t.Fatal(err) }
	if err := os.Symlink(outside, path); err != nil { t.Fatal(err) }
	s := &Server{Workspace: workspace}
	if _, err := s.readFileTransferQueueState(); err == nil || !strings.Contains(err.Error(), "safe regular") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestFileTransferQueueStateMigratesLegacyRootFile(t *testing.T) {
	workspace := t.TempDir()
	legacy := filepath.Join(workspace, fileTransferQueueStateFile)
	if err := os.WriteFile(legacy, []byte(`{"version":1,"queues":[]}`), 0o600); err != nil { t.Fatal(err) }
	s := &Server{Workspace: workspace}
	state, err := s.readFileTransferQueueState()
	if err != nil { t.Fatal(err) }
	if state.Version != 1 { t.Fatalf("state=%+v", state) }
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file still exists: %v", err)
	}
	migrated := projectfiles.Path(workspace, fileTransferQueueStateFile)
	if _, err := os.Stat(migrated); err != nil {
		t.Fatalf("migrated state missing: %v", err)
	}
}
