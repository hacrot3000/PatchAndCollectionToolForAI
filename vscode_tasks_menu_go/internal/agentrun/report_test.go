package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestIntegrationReportCapturesDiffWithoutMutatingOwner(t *testing.T) {
	owner, child, projectID := makeTestWorktree(t)
	registry, err := NewWorkspaceRegistry(filepath.Join(t.TempDir(), "registry", "workspaces.json"), owner)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := registry.Register(context.Background(), child, projectID, "conversation-1", "run-1", "provider@config-v1", []string{"files.read", "patch.propose"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "source.txt"), []byte("child change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "new.txt"), []byte("untracked\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{RunID: "run-1", ConversationID: "conversation-1", OwnerWorkspaceID: binding.OwnerWorkspaceID,
		ExecutionWorkspaceID: binding.ExecutionWorkspaceID, ProviderID: "provider", ModelID: "model",
		ProviderConfigVersion: "config-v1", ContextSnapshotID: "snapshot", AttemptID: "attempt", IdempotencyKey: "idem"}
	report, err := registry.IntegrationReport(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if report.BaseHEAD != binding.BaseHEAD || report.HEAD == "" || report.DiffSHA256 == "" || report.StatusSHA256 == "" || !strings.Contains(report.DiffPreview, "child change") || !report.ApprovalRequired || report.BranchMutationPerformed {
		t.Fatalf("incomplete or unsafe integration report: %+v", report)
	}
	foundSource, foundUntracked := false, false
	for _, file := range report.ChangedFiles {
		foundSource = foundSource || file == "source.txt"
		foundUntracked = foundUntracked || file == "new.txt"
	}
	if !foundSource || !foundUntracked || !strings.Contains(report.DiffPreview, "+untracked") {
		t.Fatalf("changed files/diff preview missing from report: files=%v preview=%q", report.ChangedFiles, report.DiffPreview)
	}
	root, err := os.ReadFile(filepath.Join(owner, "source.txt"))
	if err != nil || string(root) != "source\n" {
		t.Fatal("worktree report mutated owner source")
	}
}

func TestIntegrationReportRejectsOwnerWorkspace(t *testing.T) {
	owner, _, _ := makeTestWorktree(t)
	registry, err := NewWorkspaceRegistry(filepath.Join(t.TempDir(), "registry", "workspaces.json"), owner)
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := WorkspaceID(owner)
	if err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{RunID: "run-1", ConversationID: "conversation-1", OwnerWorkspaceID: ownerID,
		ExecutionWorkspaceID: ownerID, ProviderID: "provider", ModelID: "model", ProviderConfigVersion: "config-v1",
		ContextSnapshotID: "snapshot", AttemptID: "attempt", IdempotencyKey: "idem"}
	if _, err := registry.IntegrationReport(context.Background(), spec); err == nil {
		t.Fatal("owner working directory accepted as isolated integration target")
	}
}
