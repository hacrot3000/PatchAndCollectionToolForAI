package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestCloneTerminalCopiesLaunchEnvironmentAndRestampsOwnership(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(1 << 20)
	sourceID := "source-terminal"
	sourceEnv := append(os.Environ(), "TASKDECK_CLONE_TEST=source")
	source := &managedSession{
		meta: Metadata{
			ID: sourceID, TaskID: 0, Kind: tasks.SessionKindTerminal,
			OwnerUserID: "old-user", ProjectID: "old-project",
			Label: "Terminal", Cwd: root, TargetType: "local", Status: "exited",
		},
		launch: tasks.Execution{
			Label: "Terminal", Command: "/bin/sh", Args: []string{"-c", "exit 0"},
			Cwd: root, Env: append([]string(nil), sourceEnv...),
			SessionKind: tasks.SessionKindTerminal, OwnerUserID: "old-user", ProjectID: "old-project",
			TargetType: "local",
		},
		subscribers: map[chan []byte]struct{}{},
	}
	manager.sessions[sourceID] = source

	meta, err := manager.CloneTerminal(sourceID, TerminalCloneOptions{OwnerUserID: "new-user", ProjectID: "new-project"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = manager.Stop(meta.ID)
		time.Sleep(20 * time.Millisecond)
		_ = manager.Remove(meta.ID)
	}()

	if meta.OwnerUserID != "new-user" || meta.ProjectID != "new-project" || meta.Kind != tasks.SessionKindTerminal {
		t.Fatalf("clone ownership=%+v", meta)
	}
	if filepath.Clean(meta.Cwd) != filepath.Clean(root) {
		t.Fatalf("clone cwd=%q want=%q", meta.Cwd, root)
	}
	clone, ok := manager.Get(meta.ID)
	if !ok {
		t.Fatal("cloned session missing")
	}
	clone.mu.Lock()
	launch := cloneSessionExecution(clone.launch)
	clone.mu.Unlock()
	if launch.OwnerUserID != "new-user" || launch.ProjectID != "new-project" || launch.SessionKind != tasks.SessionKindTerminal {
		t.Fatalf("clone launch ownership=%+v", launch)
	}
	if len(launch.Args) != 2 || launch.Args[0] != "-c" || launch.Args[1] != "exit 0" {
		t.Fatalf("clone args=%v", launch.Args)
	}
	if !containsEnvValue(launch.Env, "TASKDECK_CLONE_TEST=source") {
		t.Fatalf("clone env missing marker: %v", launch.Env)
	}

	launch.Args[0] = "changed"
	launch.Env[len(launch.Env)-1] = "TASKDECK_CLONE_TEST=changed"
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.launch.Args[0] != "-c" || !containsEnvValue(source.launch.Env, "TASKDECK_CLONE_TEST=source") {
		t.Fatalf("clone mutated source launch: %+v", source.launch)
	}
}

func TestCloneTerminalClearsOwnershipForSingleUserClone(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(1 << 20)
	manager.sessions["source"] = &managedSession{
		meta: Metadata{ID: "source", TaskID: 0, Kind: tasks.SessionKindTerminal, OwnerUserID: "old", ProjectID: "old-project", Label: "Terminal", Cwd: root, TargetType: "local", Status: "exited"},
		launch: tasks.Execution{
			Label: "Terminal", Command: "/bin/sh", Args: []string{"-c", "exit 0"}, Cwd: root, Env: os.Environ(),
			SessionKind: tasks.SessionKindTerminal, OwnerUserID: "old", ProjectID: "old-project", TargetType: "local",
		},
		subscribers: map[chan []byte]struct{}{},
	}

	meta, err := manager.CloneTerminal("source", TerminalCloneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Stop(meta.ID) }()
	if meta.OwnerUserID != "" || meta.ProjectID != "" || meta.Kind != "" {
		t.Fatalf("single-user clone retained ownership=%+v", meta)
	}
}

func TestCloneTerminalRejectsRemoteTerminal(t *testing.T) {
	manager := NewManager(1 << 20)
	manager.sessions["ssh"] = &managedSession{
		meta: Metadata{ID: "ssh", TaskID: 0, Kind: tasks.SessionKindTerminal, Label: "SSH", Cwd: "/tmp", TargetType: "ssh", TargetProfileID: "prod", Status: "running"},
		launch: tasks.Execution{Label: "SSH", Command: "/usr/bin/ssh", Cwd: "/tmp", TargetType: "ssh", TargetProfileID: "prod"},
		subscribers: map[chan []byte]struct{}{},
	}
	if _, err := manager.CloneTerminal("ssh", TerminalCloneOptions{}); err == nil || !strings.Contains(err.Error(), "only local terminals") {
		t.Fatalf("remote clone error=%v", err)
	}
}

func containsEnvValue(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
