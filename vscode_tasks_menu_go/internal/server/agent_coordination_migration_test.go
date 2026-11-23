package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestLegacyCoordinationJournalMigratesOnlyOwnerRecords(t *testing.T) {
	s := newAIServerForTest(t)
	// The pre-tenant journal was shared by every shared user, so a shared
	// installation must only migrate the records this owner owns.
	s.Config.SharedServerEnabled = true
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
	ownerConversation, err := agentchat.NewSession("local", s.Workspace, project.ID, "provider", "model", agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	ownerConversation, err = store.Save(ownerConversation, 0)
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := agentrunWorkspaceID(s)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := s.agentConversationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	group, err := registry.CreateGroup(ownerID, project.ID, "Legacy group")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AddMember(group.ID, ownerID, project.ID, ownerConversation.ID); err != nil {
		t.Fatal(err)
	}
	privatePath, err := aiPrivatePath(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(filepath.Dir(privatePath), "agent-coordination-"+aiWorkspaceHash(s.Workspace), "coordination.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"version": 1,
		"peers": map[string]any{
			"owner-conv":   map[string]any{"conversation_id": ownerConversation.ID, "group_id": group.ID, "status": "idle"},
			"foreign-conv": map[string]any{"conversation_id": "foreign-conversation", "group_id": group.ID, "status": "idle"},
		},
		"messages": []any{
			map[string]any{"id": "m1", "group_id": group.ID, "from_conversation": ownerConversation.ID, "to_conversation": "other", "kind": "ask", "body": "owned", "status": "pending"},
			map[string]any{"id": "m2", "group_id": group.ID, "from_conversation": "foreign", "to_conversation": "other", "kind": "ask", "body": "foreign", "status": "pending"},
		},
		"contracts": map[string]any{
			"legacy.api":  map[string]any{"key": "legacy.api", "owner_group": group.ID, "signature": "legacy(): Result", "revision": 1, "status": "planned"},
			"foreign.api": map[string]any{"key": "foreign.api", "owner_group": "foreign-group", "signature": "foreign(): Result", "revision": 1, "status": "planned"},
		},
	}
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	hub, err := s.agentCoordinationHub("local")
	if err != nil {
		t.Fatal(err)
	}
	peers, err := hub.Peers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].ConversationID != ownerConversation.ID {
		t.Fatalf("legacy migration copied foreign peers: %+v", peers)
	}
	messages, err := hub.PeerMessages("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Body != "owned" {
		t.Fatalf("legacy migration copied foreign messages: %+v", messages)
	}
	contracts, err := hub.Contracts("")
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 1 || contracts[0].Key != "legacy.api" {
		t.Fatalf("legacy migration lost the owner's contract: %+v", contracts)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy coordination journal was removed: %v", err)
	}
}
