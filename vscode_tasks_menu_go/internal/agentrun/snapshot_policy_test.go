package agentrun

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestContextSnapshotPinsPromptPolicyWithoutSecrets(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	store, err := NewSnapshotStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	autoCollection := false
	feedbackUntil := time.Now().UTC().Add(time.Hour)
	policy := &ContextSnapshotPolicy{
		SystemInstructions: "Pinned safe system instructions",
		RetryMode:          agentchat.RetryLimited,
		ReasoningEffort:    "high",
		Automation: AutomationSnapshot{AutoPatch: true, AutoCollection: &autoCollection, AutoFeedback: true,
			AutoFeedbackPersistent: true, AutoFeedbackUntil: &feedbackUntil},
	}
	snapshot, err := store.CreateWithPolicy("owner", "conversation", "project",
		[]agentchat.HistoryItem{{Type: "user", Content: "prompt"}}, nil, policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.SystemInstructions = "changed after send"
	*policy.Automation.AutoCollection = true
	loaded, err := store.Get(snapshot.ID, "owner", "conversation", "project")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Policy == nil || loaded.Policy.SystemInstructions != "Pinned safe system instructions" ||
		loaded.Policy.RetryMode != agentchat.RetryLimited || loaded.Policy.ReasoningEffort != "high" ||
		loaded.Policy.Automation.AutoCollection == nil || *loaded.Policy.Automation.AutoCollection {
		t.Fatalf("snapshot policy changed after send: %+v", loaded.Policy)
	}
}

func TestContextSnapshotPolicyIsIncludedInIntegrityDigest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	store, err := NewSnapshotStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.CreateWithPolicy("owner", "conversation", "project",
		[]agentchat.HistoryItem{{Type: "user", Content: "prompt"}}, nil,
		&ContextSnapshotPolicy{SystemInstructions: "pinned instructions", RetryMode: agentchat.RetryOff})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, snapshot.ID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	modified := []byte(string(body))
	for i := 0; i+len("pinned instructions") <= len(modified); i++ {
		if string(modified[i:i+len("pinned instructions")]) == "pinned instructions" {
			copy(modified[i:], "changed instructions!!")
			break
		}
	}
	if err := os.WriteFile(path, modified, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(snapshot.ID, "owner", "conversation", "project"); err == nil {
		t.Fatal("modified prompt policy passed snapshot integrity validation")
	}
}
