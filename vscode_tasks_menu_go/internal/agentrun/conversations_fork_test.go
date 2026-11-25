package agentrun

import (
	"path/filepath"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestSharedSnapshotForForkUsesInclusiveTurnBoundary(t *testing.T) {
	history := []agentchat.HistoryItem{
		{Type: "user", Content: "prompt one", OriginRunID: "run-1", ParentTurnID: "root"},
		{Type: "assistant", Content: "answer one", OriginRunID: "run-1", ParentTurnID: "root"},
		{Type: "user", Content: "prompt two", OriginRunID: "run-2", ParentTurnID: "run-1"},
		{Type: "assistant", Content: "answer two", OriginRunID: "run-2", ParentTurnID: "run-1"},
		{Type: "user", Content: "prompt three", OriginRunID: "run-3", ParentTurnID: "run-2"},
		{Type: "assistant", Content: "answer three", OriginRunID: "run-3", ParentTurnID: "run-2"},
	}
	for _, test := range []struct {
		name  string
		turn  string
		items int
	}{
		{name: "start", turn: "run-1", items: 2},
		{name: "middle", turn: "run-2", items: 4},
		{name: "end", turn: "run-3", items: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry, err := NewConversationRegistry(filepath.Join(t.TempDir(), "conversations.json"))
			if err != nil {
				t.Fatal(err)
			}
			forkID := "fork-" + test.name
			if _, err := registry.RecordFork(forkID, "source", test.turn, ""); err != nil {
				t.Fatal(err)
			}
			prefix, ok := registry.SharedSnapshotFor(forkID, history)
			if !ok || len(prefix) != test.items {
				t.Fatalf("prefix length=%d, ok=%v; want %d", len(prefix), ok, test.items)
			}
			for _, item := range prefix {
				if item.OriginRunID == "run-3" && test.turn != "run-3" {
					t.Fatalf("fork at %s leaked a later turn: %+v", test.turn, item)
				}
			}
		})
	}
}

func TestSharedSnapshotForForkRejectsUnknownTurnInsteadOfCopyingFullHistory(t *testing.T) {
	registry, err := NewConversationRegistry(filepath.Join(t.TempDir(), "conversations.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.RecordFork("fork", "source", "missing-turn", ""); err != nil {
		t.Fatal(err)
	}
	if prefix, ok := registry.SharedSnapshotFor("fork", []agentchat.HistoryItem{{Type: "user", Content: "private later turn"}}); ok || prefix != nil {
		t.Fatalf("unknown turn returned a history prefix: %+v, ok=%v", prefix, ok)
	}
}
