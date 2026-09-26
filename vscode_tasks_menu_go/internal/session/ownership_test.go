package session

import (
	"os"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestOwnedSessionMetadata(t *testing.T) {
	manager := NewManager(64 << 10)
	meta, err := manager.Start(tasks.Execution{
		TaskID: 11, Label: "owned", Command: "/bin/sh", Args: []string{"-c", "exit 0"},
		Cwd: t.TempDir(), Env: os.Environ(), Preview: "exit 0",
		SessionKind: tasks.SessionKindTask, OwnerUserID: "user-1", ProjectID: "project-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Kind != tasks.SessionKindTask || meta.OwnerUserID != "user-1" || meta.ProjectID != "project-1" {
		t.Fatalf("unexpected ownership metadata: %#v", meta)
	}
	manager.Shutdown(2 * time.Second)
}

func TestOwnedSessionRequiresCompleteValidIdentity(t *testing.T) {
	manager := NewManager(64 << 10)
	tests := []tasks.Execution{
		{SessionKind: tasks.SessionKindTask, OwnerUserID: "user-1"},
		{SessionKind: "unknown", OwnerUserID: "user-1", ProjectID: "project-1"},
		{OwnerUserID: "user-1", ProjectID: "project-1"},
	}
	for _, spec := range tests {
		if _, err := manager.Start(spec); err == nil {
			t.Fatalf("Start(%#v) unexpectedly accepted incomplete ownership", spec)
		}
	}
}
