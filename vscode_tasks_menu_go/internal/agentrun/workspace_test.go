package agentrun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func makeTestWorktree(t *testing.T) (string, string, string) {
	t.Helper()
	base := t.TempDir()
	owner := filepath.Join(base, "owner")
	child := filepath.Join(base, "child")
	if err := os.Mkdir(owner, 0700); err != nil {
		t.Fatal(err)
	}
	identity, err := patchtool.EnsureProjectIdentity(owner)
	if err != nil {
		t.Fatal(err)
	}
	gitTest(t, owner, "init", "-q")
	gitTest(t, owner, "config", "user.email", "tests@example.invalid")
	gitTest(t, owner, "config", "user.name", "TaskDeck Test")
	if err := os.WriteFile(filepath.Join(owner, "source.txt"), []byte("source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, owner, "add", ".")
	gitTest(t, owner, "commit", "-qm", "initial")
	gitTest(t, owner, "worktree", "add", "--detach", child, "HEAD")
	return owner, child, identity.ID
}

func TestWorkspaceRegistryRequiresRegisteredOwnedWorktree(t *testing.T) {
	owner, child, projectID := makeTestWorktree(t)
	registry, err := NewWorkspaceRegistry(filepath.Join(t.TempDir(), "registry", "workspaces.json"), owner)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := registry.Register(context.Background(), child, projectID, "conversation-1", "run-1", "provider-config-1", []string{"files.read", "patch.propose"})
	if err != nil {
		t.Fatal(err)
	}
	if binding.OwnerWorkspaceID != workspaceID(owner) || binding.ExecutionWorkspaceID != workspaceID(child) || binding.LifecycleStatus != "active" {
		t.Fatalf("unexpected binding: %+v", binding)
	}
	resolved, err := registry.Resolve(context.Background(), binding.ExecutionWorkspaceID, "conversation-1", "run-1", projectID)
	if err != nil || resolved.WorktreePath != child {
		t.Fatalf("resolve failed: %+v, %v", resolved, err)
	}
	if _, err := registry.Resolve(context.Background(), binding.ExecutionWorkspaceID, "conversation-other", "run-1", projectID); err == nil {
		t.Fatal("binding was accepted for another conversation")
	}
}

func TestWorkspaceRegistryCreatesDetachedWorktreeWithoutCommitOrSecretCopy(t *testing.T) {
	base := t.TempDir()
	owner := filepath.Join(base, "owner")
	if err := os.Mkdir(owner, 0700); err != nil {
		t.Fatal(err)
	}
	identity, err := patchtool.EnsureProjectIdentity(owner)
	if err != nil {
		t.Fatal(err)
	}
	gitTest(t, owner, "init", "-q")
	gitTest(t, owner, "config", "user.email", "tests@example.invalid")
	gitTest(t, owner, "config", "user.name", "TaskDeck Test")
	if err := os.WriteFile(filepath.Join(owner, "source.txt"), []byte("source\\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, owner, "add", "source.txt")
	gitTest(t, owner, "commit", "-qm", "initial")
	registry, err := NewWorkspaceRegistry(filepath.Join(t.TempDir(), "registry", "workspaces.json"), owner)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "runs", "task-one")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	binding, err := registry.CreateDetached(context.Background(), target, identity.ID, "conversation-1", "run-1", "provider-config-1", []string{"files.read", "patch.propose"})
	if err != nil {
		t.Fatal(err)
	}
	if binding.BaseHEAD == "" || binding.WorktreePath != target {
		t.Fatalf("unexpected worktree binding: %+v", binding)
	}
	if _, err := os.Stat(filepath.Join(target, ".python_patch_tool.json")); err != nil {
		t.Fatal("owner project identity was not installed in worktree")
	}
	if status := strings.TrimSpace(gitTest(t, target, "status", "--porcelain")); status != "?? .python_patch_tool.json" {
		t.Fatalf("unexpected worktree status: %q", status)
	}
	if _, err := os.Stat(filepath.Join(target, ".env")); !os.IsNotExist(err) {
		t.Fatal("owner secrets were copied into worktree")
	}
}

func TestWorkspaceRegistryRejectsUnrelatedDirectory(t *testing.T) {
	owner, _, projectID := makeTestWorktree(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := patchtool.EnsureProjectIdentity(outside); err != nil {
		t.Fatal(err)
	}
	registry, err := NewWorkspaceRegistry(filepath.Join(t.TempDir(), "registry", "workspaces.json"), owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Register(context.Background(), outside, projectID, "conversation-1", "run-1", "provider-config-1", []string{"files.read"}); err == nil {
		t.Fatal("unrelated workspace was registered")
	}
}
