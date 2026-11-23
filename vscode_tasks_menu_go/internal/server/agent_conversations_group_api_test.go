package server

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestCreatingConversationWithGroupFailsClosedWhenMembershipIsRejected(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := patchtool.EnsureProjectIdentity(s.Workspace); err != nil {
		t.Fatal(err)
	}
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	response := invokeAI(t, s.agentConversationsAPI, http.MethodPost, "/api/agent/conversations", map[string]any{
		"provider_id": "provider", "model_id": "model", "group_id": "missing-group",
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("invalid group membership returned HTTP %d: %s", response.Code, response.Body)
	}
	conversations, err := store.List("local")
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 0 {
		t.Fatalf("conversation persisted despite rejected group membership: %+v", conversations)
	}
}

func TestCreatingConversationAddsItToExistingGroup(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := patchtool.EnsureProjectIdentity(s.Workspace); err != nil {
		t.Fatal(err)
	}
	groupResponse := invokeAI(t, s.agentConversationGroupsAPI, http.MethodPost, "/api/agent/conversation-groups", map[string]any{"title": "Review"})
	if groupResponse.Code != http.StatusCreated {
		t.Fatalf("create group: HTTP %d: %s", groupResponse.Code, groupResponse.Body)
	}
	var groupBody struct {
		Group agentrun.ConversationGroup `json:"group"`
	}
	if err := json.Unmarshal(groupResponse.Body.Bytes(), &groupBody); err != nil {
		t.Fatal(err)
	}
	response := invokeAI(t, s.agentConversationsAPI, http.MethodPost, "/api/agent/conversations", map[string]any{
		"provider_id": "provider", "model_id": "model", "group_id": groupBody.Group.ID,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create grouped conversation: HTTP %d: %s", response.Code, response.Body)
	}
	var conversationBody struct {
		Conversation struct {
			ID string `json:"id"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &conversationBody); err != nil {
		t.Fatal(err)
	}
	registry, err := s.agentConversationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	groups, err := registry.Groups(groupBody.Group.OwnerWorkspaceID, groupBody.Group.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Members) != 1 || groups[0].Members[0] != conversationBody.Conversation.ID {
		t.Fatalf("conversation missing from its group: %+v", groups)
	}
}
