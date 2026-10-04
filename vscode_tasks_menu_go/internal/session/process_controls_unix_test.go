//go:build !windows

package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func waitSessionStopped(t *testing.T, manager *Manager, id string) Metadata {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		meta, ok := manager.Metadata(id)
		if ok && meta.Status != "running" {
			return meta
		}
		time.Sleep(20 * time.Millisecond)
	}
	meta, _ := manager.Metadata(id)
	t.Fatalf("session did not stop: %+v", meta)
	return Metadata{}
}

func TestParseProcessTableBuildsOnlySessionDescendants(t *testing.T) {
	raw := strings.Join([]string{
		"10 1 10 Ss 100 shell /bin/sh",
		"11 10 10 S 90 sleep sleep 30",
		"12 11 10 S 80 helper helper --child",
		"99 1 99 S 70 other other",
	}, "\n")
	rows := parseProcessTable(raw, 10)
	if len(rows) != 3 {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].PID != 10 || rows[0].Depth != 0 || rows[1].PID != 11 || rows[1].Depth != 1 || rows[2].PID != 12 || rows[2].Depth != 2 {
		t.Fatalf("unexpected tree=%+v", rows)
	}
}

func TestManagerProcessTreeIncludesChildProcess(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil { t.Skip(err) }
	manager := NewManager(64 << 10)
	meta, err := manager.Start(tasks.Execution{
		TaskID: 0, Label: "process tree", Command: "/bin/sh",
		Args: []string{"-c", "sleep 30 & wait"}, Cwd: t.TempDir(),
		Env: os.Environ(), Preview: "sleep 30",
	})
	if err != nil { t.Fatal(err) }
	defer manager.Shutdown(time.Second)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rows, treeErr := manager.ProcessTree(meta.ID)
		if treeErr == nil && len(rows) >= 2 {
			if rows[0].Depth != 0 {
				t.Fatalf("root depth=%d rows=%+v", rows[0].Depth, rows)
			}
			foundSleep := false
			for _, row := range rows[1:] {
				if strings.Contains(row.Command, "sleep") || strings.Contains(row.Args, "sleep 30") {
					foundSleep = true
					break
				}
			}
			if foundSleep { return }
		}
		time.Sleep(30 * time.Millisecond)
	}
	rows, err := manager.ProcessTree(meta.ID)
	t.Fatalf("child process not found: rows=%+v err=%v", rows, err)
}

func TestManagerTerminateUsesGracefulTerminationState(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil { t.Skip(err) }
	manager := NewManager(64 << 10)
	meta, err := manager.Start(tasks.Execution{
		TaskID: 1, Label: "terminate", Command: "/bin/sh",
		Args: []string{"-c", "trap 'exit 0' TERM; while :; do sleep 1; done"},
		Cwd: t.TempDir(), Env: os.Environ(), Preview: "terminate test",
	})
	if err != nil { t.Fatal(err) }
	defer manager.Shutdown(time.Second)
	time.Sleep(80 * time.Millisecond)
	if err := manager.Terminate(meta.ID); err != nil { t.Fatal(err) }
	final := waitSessionStopped(t, manager, meta.ID)
	if final.Status != "stopped" {
		t.Fatalf("final=%+v", final)
	}
}
