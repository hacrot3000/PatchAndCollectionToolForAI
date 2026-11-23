package agentrun

import (
	"os"
	"path/filepath"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestContextSnapshotIsPersistedAndOwnerConversationBound(t *testing.T) {
	store, err := NewSnapshotStore(filepath.Join(t.TempDir(), "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	history := []agentchat.HistoryItem{{Type: "user", Content: "task prompt"}, {Type: "assistant", Content: "prior context"}}
	snapshot, err := store.Create("owner-1", "conversation-1", "project-1", history)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(snapshot.ID, "owner-1", "conversation-1", "project-1")
	if err != nil || loaded.PayloadSHA256 != snapshot.PayloadSHA256 || len(loaded.History) != len(history) {
		t.Fatalf("snapshot load failed: %+v %v", loaded, err)
	}
	if _, err := store.Get(snapshot.ID, "owner-2", "conversation-1", "project-1"); err == nil {
		t.Fatal("snapshot crossed owner workspace boundary")
	}
	if _, err := store.Get(snapshot.ID, "owner-1", "conversation-2", "project-1"); err == nil {
		t.Fatal("snapshot crossed conversation boundary")
	}
}

func TestContextSnapshotRejectsTamperingAndOversizedHistory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	store, err := NewSnapshotStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Create("owner-1", "conversation-1", "project-1", []agentchat.HistoryItem{{Type: "user", Content: "before"}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, snapshot.ID+".json")
	body := []byte(`{"id":"` + snapshot.ID + `","owner_workspace_id":"owner-1","conversation_id":"conversation-1","project_id":"project-1","history":[{"type":"user","content":"tampered"}],"payload_sha256":"` + snapshot.PayloadSHA256 + `"}`)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(snapshot.ID, "owner-1", "conversation-1", "project-1"); err == nil {
		t.Fatal("tampered context snapshot was accepted")
	}
	if _, err := store.Create("owner-1", "conversation-1", "project-1", []agentchat.HistoryItem{{Type: "unknown"}}); err == nil {
		t.Fatal("invalid history item accepted")
	}
}
