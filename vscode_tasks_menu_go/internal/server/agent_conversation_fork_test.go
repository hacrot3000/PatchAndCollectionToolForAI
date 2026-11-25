package server

import (
	"os"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestForkConversationWhileSourceIsActiveCreatesIndependentPrefixes(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	project, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	source, err := agentchat.NewSession("local", s.Workspace, project.ID, "provider", "model", agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	source.Status = "requesting_ai"
	source.History = []agentchat.HistoryItem{
		{Type: "user", Content: "prompt one", OriginRunID: "run-1", ParentTurnID: "root"},
		{Type: "assistant", Content: "answer one", OriginRunID: "run-1", ParentTurnID: "root"},
		{Type: "user", Content: "prompt two", OriginRunID: "run-2", ParentTurnID: "run-1"},
		{Type: "assistant", Content: "answer two", OriginRunID: "run-2", ParentTurnID: "run-1"},
		{Type: "user", Content: "prompt three", OriginRunID: "run-3", ParentTurnID: "run-2"},
		{Type: "assistant", Content: "answer three", OriginRunID: "run-3", ParentTurnID: "run-2"},
	}
	source, err = store.Save(source, 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		turn  string
		items int
	}{
		{turn: "run-1", items: 2},
		{turn: "run-2", items: 4},
		{turn: "run-3", items: 6},
	} {
		fork, err := s.ForkConversation("local", source.ID, test.turn)
		if err != nil {
			t.Fatalf("fork at %s: %v", test.turn, err)
		}
		if fork.Status != "created" || len(fork.History) != test.items {
			t.Fatalf("fork at %s has status %q and %d history items; want created and %d", test.turn, fork.Status, len(fork.History), test.items)
		}
	}
	first, err := s.ForkConversation("local", source.ID, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ForkConversation("local", source.ID, "run-2")
	if err != nil {
		t.Fatal(err)
	}
	first.History = append(first.History, agentchat.HistoryItem{Type: "user", Content: "local fork prompt"})
	if _, err := store.Save(first, first.Revision); err != nil {
		t.Fatal(err)
	}
	secondReloaded, err := store.Get(second.ID, "local")
	if err != nil {
		t.Fatal(err)
	}
	if len(secondReloaded.History) != 4 || secondReloaded.History[len(secondReloaded.History)-1].Content != "answer two" {
		t.Fatalf("first fork mutation leaked into second fork: %+v", secondReloaded.History)
	}
	sourceReloaded, err := store.Get(source.ID, "local")
	if err != nil {
		t.Fatal(err)
	}
	if sourceReloaded.Status != "requesting_ai" || len(sourceReloaded.History) != 6 {
		t.Fatalf("fork mutated active source: status=%q history=%d", sourceReloaded.Status, len(sourceReloaded.History))
	}
}
