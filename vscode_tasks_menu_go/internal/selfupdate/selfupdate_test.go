package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequestLifecycle(t *testing.T) {
	workspace := t.TempDir()
	req, err := CreateRequest(workspace, "0123456789abcdef", "http://127.0.0.1:1234", false)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != "awaiting_confirmation" || req.ID == "" {
		t.Fatalf("unexpected request: %#v", req)
	}
	loaded, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != req.ID || loaded.Revision != req.Revision {
		t.Fatalf("loaded request mismatch: %#v", loaded)
	}
	confirmed, err := Update(workspace, req.ID, "confirmed", "ok", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "confirmed" || confirmed.ConfirmedAt == "" {
		t.Fatalf("confirmation not recorded: %#v", confirmed)
	}
	completed, err := Update(workspace, req.ID, "completed", "done", "http://127.0.0.1:1234", "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.CompletedAt == "" || completed.TargetURL == "" {
		t.Fatalf("completion not recorded: %#v", completed)
	}
}

func TestInstallReplacesBinaryAndWritesRevision(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "vscode_tasks_menu")
	staged := filepath.Join(dir, "staged")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(staged, target, "abcdef0123456789"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("target=%q want new", got)
	}
	if revision := InstalledRevision(target); revision != "abcdef0123456789" {
		t.Fatalf("revision=%q", revision)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged binary still exists: %v", err)
	}
}
