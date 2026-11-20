package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestCurrentCwdTracksTerminalShell(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager(64 << 10)
	meta, err := m.Start(tasks.Execution{
		TaskID:  0,
		Label:   "Terminal",
		Command: "/bin/sh",
		Cwd:     root,
		Env:     os.Environ(),
		Preview: "/bin/sh",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(time.Second)
	if err := m.Input(meta.ID, []byte("cd sub\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		cwd, err := m.CurrentCwd(meta.ID)
		if err == nil && cwd == sub {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("cwd did not become %q; last=%q err=%v", sub, cwd, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
