package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConversationGroupMemberMustBelongToOwnerProject(t *testing.T) {
	s, conversationID := newAgentConversationFixture(t, "stateless")
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.Get(conversationID, "local")
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
	group, err := registry.CreateGroupForPrincipal(ownerID, conversation.ProjectID, "Member validation", "local")
	if err != nil {
		t.Fatal(err)
	}
	add := func(id string) *httptest.ResponseRecorder {
		return invokeAI(t, s.agentConversationGroupItemAPI, http.MethodPost, "/api/agent/conversation-groups/"+group.ID, map[string]any{
			"action": "add_member", "conversation_id": id,
		})
	}
	if response := add("not-an-owner-conversation"); response.Code != http.StatusNotFound {
		t.Fatalf("unknown conversation joined group: HTTP %d %s", response.Code, response.Body)
	}
	if response := add(conversationID); response.Code != http.StatusOK {
		t.Fatalf("owner conversation could not join group: HTTP %d %s", response.Code, response.Body)
	}
}
