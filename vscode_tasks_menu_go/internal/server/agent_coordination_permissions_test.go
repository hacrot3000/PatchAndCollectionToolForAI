package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedAgentConversationRoutesMapReadAndWritePermissions(t *testing.T) {
	cases := []struct {
		method, path string
		wantRead     bool
		wantWrite    bool
	}{
		{http.MethodGet, "/api/agent/conversations", true, false},
		{http.MethodPost, "/api/agent/conversations", true, true},
		{http.MethodGet, "/api/agent/conversations/conv-1", true, false},
		{http.MethodGet, "/api/agent/conversation-groups", true, false},
		{http.MethodPost, "/api/agent/conversation-groups", true, true},
		{http.MethodGet, "/api/agent/conversation-groups/group-1", true, false},
		{http.MethodPost, "/api/agent/conversation-groups/group-1", true, true},
		{http.MethodGet, "/api/agent/coordination", true, false},
		{http.MethodPost, "/api/agent/coordination", true, true},
		{http.MethodPost, "/api/agent/context-groups", true, true},
	}
	for _, test := range cases {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			permissions := sharedRoutePermissions(request)
			read, write := false, false
			for _, permission := range permissions {
				read = read || permission == identity.PermissionSettingsRead
				write = write || permission == identity.PermissionSettingsWrite
			}
			if read != test.wantRead || write != test.wantWrite {
				t.Fatalf("route permissions=%v, settings.read=%v settings.write=%v; want read=%v write=%v", permissions, read, write, test.wantRead, test.wantWrite)
			}
		})
	}
}

func TestCoordinationMutationRejectsReadOnlyPrincipal(t *testing.T) {
	s := newAIServerForTest(t)
	request := httptest.NewRequest(http.MethodPost, "/api/agent/coordination", nil)
	principal := identity.Principal{UserID: identity.ID("reader"), Permissions: map[string]bool{identity.PermissionSettingsRead: true}}
	request = request.WithContext(context.WithValue(request.Context(), sharedPrincipalContextKey{}, principal))
	response := httptest.NewRecorder()
	s.agentCoordinationAPI(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("read-only principal mutated coordination state: HTTP %d %s", response.Code, response.Body)
	}
}

func TestAgentMutationHandlersRejectReadOnlyPrincipal(t *testing.T) {
	s := newAIServerForTest(t)
	principal := identity.Principal{UserID: identity.ID("reader"), Permissions: map[string]bool{identity.PermissionSettingsRead: true}}
	for _, test := range []struct {
		name, path string
		handler    http.HandlerFunc
	}{
		{"conversations", "/api/agent/conversations", s.agentConversationsAPI},
		{"conversation groups", "/api/agent/conversation-groups", s.agentConversationGroupsAPI},
		{"conversation group member", "/api/agent/conversation-groups/group-1", s.agentConversationGroupItemAPI},
		{"coordination", "/api/agent/coordination", s.agentCoordinationAPI},
		{"context groups", "/api/agent/context-groups", s.agentContextGroupsAPI},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			request = request.WithContext(context.WithValue(request.Context(), sharedPrincipalContextKey{}, principal))
			response := httptest.NewRecorder()
			test.handler(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("read-only principal mutated state: HTTP %d %s", response.Code, response.Body)
			}
		})
	}
}

func TestCoordinationWritesRequireCurrentGroupMembership(t *testing.T) {
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
	group, err := registry.CreateGroup(ownerID, conversation.ProjectID, "Authorized peers")
	if err != nil {
		t.Fatal(err)
	}
	request := func(from, to, kind string) *httptest.ResponseRecorder {
		return invokeAI(t, s.agentCoordinationAPI, http.MethodPost, "/api/agent/coordination", map[string]any{
			"kind": "peer_message", "message_kind": kind, "conversation_id": from,
			"to_conversation_id": to, "group_id": group.ID, "body": "share a contract update",
		})
	}
	if response := request(conversationID, "unregistered-peer", "ask"); response.Code != http.StatusForbidden {
		t.Fatalf("non-member sent a group message: HTTP %d %s", response.Code, response.Body)
	}
	if _, err := registry.AddMember(group.ID, ownerID, conversation.ProjectID, conversationID); err != nil {
		t.Fatal(err)
	}
	if response := request(conversationID, "unregistered-peer", "ask"); response.Code != http.StatusForbidden {
		t.Fatalf("message targeted a non-member: HTTP %d %s", response.Code, response.Body)
	}
}
