package agentrun

import (
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/taskmanifest"
)

func TestPlanStorePersistsDAGAndUsesRevisionCAS(t *testing.T) {
	manifest, err := taskmanifest.ParseMarkdown("# Plan\n## Task A — first\nAcceptance:\n- first\n## Task B — second\nDepends on: A\nAcceptance:\n- second\n", "project-plan")
	if err != nil {
		t.Fatal(err)
	}
	dag, err := NewDAGScheduler(manifest, 2)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewPlanStore(filepath.Join(t.TempDir(), "plans"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.Create(WorkflowRecord{OwnerWorkspaceID: "owner", ProjectID: manifest.ProjectID, Manifest: manifest,
		DAG: dag.Snapshot(), ProviderID: "provider", ModelID: "model", ProviderConfigVersion: "cfg", CredentialFingerprint: strings.Repeat("a", 64), Concurrency: 2, Status: WorkflowQueued})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(plan.ID)
	if err != nil || loaded.DAG.ManifestID != manifest.ManifestID || loaded.Revision != 1 {
		t.Fatalf("load: %+v %v", loaded, err)
	}
	loaded.Status = WorkflowRunning
	updated, err := store.Update(loaded, 1)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := store.Update(loaded, 1); err == nil {
		t.Fatal("stale workflow revision was accepted")
	}
	list, err := store.List()
	if err != nil || len(list) != 1 || list[0].Status != WorkflowRunning {
		t.Fatalf("list: %+v %v", list, err)
	}
}
