package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// agentGroupFixture builds a shared-mode server with two independent owners,
// each with their own conversations, plus a conversation group per owner.
type agentGroupFixture struct {
	server   *Server
	project  patchtool.ProjectIdentity
	registry *agentrun.ConversationRegistry
	ownerID  string
	convs    map[string][]agentchat.Session
	groups   map[string]agentrun.ConversationGroup
}

func newAgentGroupFixture(t *testing.T) *agentGroupFixture {
	t.Helper()
	s := newAIServerForTest(t)
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
	fixture := &agentGroupFixture{server: s, project: project, convs: map[string][]agentchat.Session{}, groups: map[string]agentrun.ConversationGroup{}}
	fixture.registry, err = s.agentConversationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	fixture.ownerID, err = agentrunWorkspaceID(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"user:alice", "user:bob"} {
		for i := 0; i < 2; i++ {
			session, err := agentchat.NewSession(owner, s.Workspace, project.ID, "provider", "model", agentchat.Stateless)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := store.Save(session, 0)
			if err != nil {
				t.Fatal(err)
			}
			fixture.convs[owner] = append(fixture.convs[owner], saved)
		}
	}
	// Group A is owned by alice, group B is owned by bob.
	groupA, err := fixture.registry.CreateGroupForPrincipal(fixture.ownerID, project.ID, "Group A", "user:alice")
	if err != nil {
		t.Fatal(err)
	}
	groupB, err := fixture.registry.CreateGroupForPrincipal(fixture.ownerID, project.ID, "Group B", "user:bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.registry.AddMember(groupA.ID, fixture.ownerID, project.ID, fixture.convs["user:alice"][0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.registry.AddMember(groupB.ID, fixture.ownerID, project.ID, fixture.convs["user:bob"][0].ID); err != nil {
		t.Fatal(err)
	}
	fixture.groups["user:alice"] = groupA
	fixture.groups["user:bob"] = groupB
	return fixture
}

// call performs a request as the given owner with the given permissions.
func (f *agentGroupFixture) call(t *testing.T, owner, method, path string, payload any, permissions ...string) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	allowed := map[string]bool{}
	for _, permission := range permissions {
		allowed[permission] = true
	}
	principal := identity.Principal{
		UserID:      identity.ID(strings.TrimPrefix(owner, "user:")),
		ProjectID:   identity.ID(f.project.ID),
		Permissions: allowed,
	}
	request = request.WithContext(context.WithValue(request.Context(), sharedPrincipalContextKey{}, principal))
	response := httptest.NewRecorder()
	f.server.agentConversationGroupItemAPI(response, request)
	return response
}

// listGroups lists the groups visible to the given owner.
func (f *agentGroupFixture) listGroups(t *testing.T, owner string, permissions ...string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/agent/conversation-groups", nil)
	principal := identity.Principal{
		UserID:      identity.ID(strings.TrimPrefix(owner, "user:")),
		ProjectID:   identity.ID(f.project.ID),
		Permissions: map[string]bool{identity.PermissionSettingsRead: true},
	}
	request = request.WithContext(context.WithValue(request.Context(), sharedPrincipalContextKey{}, principal))
	response := httptest.NewRecorder()
	f.server.agentConversationGroupsAPI(response, request)
	return response
}

func TestConversationGroupListHidesGroupsOwnedByAnotherPrincipal(t *testing.T) {
	fixture := newAgentGroupFixture(t)
	response := fixture.listGroups(t, "user:alice")
	if response.Code != http.StatusOK {
		t.Fatalf("list groups: HTTP %d %s", response.Code, response.Body)
	}
	var view struct {
		Groups []agentrun.ConversationGroup `json:"groups"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Groups) != 1 || view.Groups[0].ID != fixture.groups["user:alice"].ID {
		t.Fatalf("owner saw a group it does not own: %+v", view.Groups)
	}
	bobResponse := fixture.listGroups(t, "user:bob")
	if err := json.Unmarshal(bobResponse.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Groups) != 1 || view.Groups[0].ID != fixture.groups["user:bob"].ID {
		t.Fatalf("owner B saw a group it does not own: %+v", view.Groups)
	}
}

func TestConversationGroupItemIsNotReadableAcrossOwners(t *testing.T) {
	fixture := newAgentGroupFixture(t)
	groupA := fixture.groups["user:alice"]
	// Bob must not read the group owned by Alice.
	response := fixture.call(t, "user:bob", http.MethodGet, "/api/agent/conversation-groups/"+groupA.ID, nil,
		identity.PermissionSettingsRead)
	if response.Code != http.StatusNotFound {
		t.Fatalf("owner B read owner A group: HTTP %d %s", response.Code, response.Body)
	}
	// Alice may read her own group and learns her access level.
	own := fixture.call(t, "user:alice", http.MethodGet, "/api/agent/conversation-groups/"+groupA.ID, nil,
		identity.PermissionSettingsRead)
	if own.Code != http.StatusOK {
		t.Fatalf("owner A read own group: HTTP %d %s", own.Code, own.Body)
	}
	var view struct {
		Group  agentrun.ConversationGroup `json:"group"`
		Access agentrun.GroupAccess       `json:"access"`
	}
	if err := json.Unmarshal(own.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Access.Read || !view.Access.Manage {
		t.Fatalf("group owner lost management rights: %+v", view.Access)
	}
}

func TestConversationGroupManagementRequiresGroupOwnership(t *testing.T) {
	fixture := newAgentGroupFixture(t)
	groupA := fixture.groups["user:alice"]
	bobConversation := fixture.convs["user:bob"][0].ID
	write := []string{identity.PermissionSettingsRead, identity.PermissionSettingsWrite}

	// Bob cannot add his own conversation to Alice's group.
	response := fixture.call(t, "user:bob", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "add_member", "conversation_id": bobConversation,
	}, write...)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-owner added a member to another owner's group: HTTP %d %s", response.Code, response.Body)
	}
	// Bob cannot change the shared instructions of Alice's group.
	response = fixture.call(t, "user:bob", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "update_instructions", "expected_revision": groupA.SharedRevision,
		"instructions": "instructions injected by a foreign owner",
	}, write...)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-owner changed shared instructions: HTTP %d %s", response.Code, response.Body)
	}
	// Bob cannot remove members of Alice's group.
	response = fixture.call(t, "user:bob", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "remove_member", "conversation_id": fixture.convs["user:alice"][0].ID,
	}, write...)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-owner removed a member: HTTP %d %s", response.Code, response.Body)
	}
	// The group is unchanged.
	group, ok := fixture.registry.Group(groupA.ID)
	if !ok {
		t.Fatal("group disappeared")
	}
	if len(group.Members) != 1 || group.Members[0] != fixture.convs["user:alice"][0].ID {
		t.Fatalf("membership changed by a foreign owner: %+v", group.Members)
	}
	if group.SharedInstructions != "" {
		t.Fatalf("shared instructions changed by a foreign owner: %q", group.SharedInstructions)
	}
}

func TestConversationGroupMemberWithoutMembershipIsRejected(t *testing.T) {
	fixture := newAgentGroupFixture(t)
	groupA := fixture.groups["user:alice"]
	write := []string{identity.PermissionSettingsRead, identity.PermissionSettingsWrite}
	// A principal owning a member conversation does not automatically manage a
	// group that is explicitly owned by somebody else.
	response := fixture.call(t, "user:bob", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "add_member", "conversation_id": fixture.convs["user:bob"][0].ID,
	}, write...)
	if response.Code != http.StatusForbidden {
		t.Fatalf("member owner managed a group owned by another principal: HTTP %d %s", response.Code, response.Body)
	}
	// Alice owns the group, so she may add her second conversation.
	response = fixture.call(t, "user:alice", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "add_member", "conversation_id": fixture.convs["user:alice"][1].ID,
	}, write...)
	if response.Code != http.StatusOK {
		t.Fatalf("group owner could not add a member: HTTP %d %s", response.Code, response.Body)
	}
	// A member conversation from another owner is still rejected.
	response = fixture.call(t, "user:alice", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "add_member", "conversation_id": fixture.convs["user:bob"][1].ID,
	}, write...)
	if response.Code != http.StatusNotFound {
		t.Fatalf("foreign conversation joined the group: HTTP %d %s", response.Code, response.Body)
	}
}

func TestConversationGroupSharedPrincipalGetsReadOnlyAccess(t *testing.T) {
	fixture := newAgentGroupFixture(t)
	groupA := fixture.groups["user:alice"]
	write := []string{identity.PermissionSettingsRead, identity.PermissionSettingsWrite}
	// Alice deliberately shares the group with Bob for reading.
	response := fixture.call(t, "user:alice", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "share", "shared_principals": []string{"user:bob"},
	}, write...)
	if response.Code != http.StatusOK {
		t.Fatalf("share group: HTTP %d %s", response.Code, response.Body)
	}
	// Bob may now read the group...
	read := fixture.call(t, "user:bob", http.MethodGet, "/api/agent/conversation-groups/"+groupA.ID, nil,
		identity.PermissionSettingsRead)
	if read.Code != http.StatusOK {
		t.Fatalf("shared principal could not read the group: HTTP %d %s", read.Code, read.Body)
	}
	var view struct {
		Access agentrun.GroupAccess `json:"access"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Access.Read || view.Access.Manage {
		t.Fatalf("shared principal received management rights: %+v", view.Access)
	}
	// ...but may not change the shared instructions.
	mutate := fixture.call(t, "user:bob", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "update_instructions", "expected_revision": groupA.SharedRevision,
		"instructions": "instructions injected by a read-only principal",
	}, write...)
	if mutate.Code != http.StatusForbidden {
		t.Fatalf("read-only principal changed shared instructions: HTTP %d %s", mutate.Code, mutate.Body)
	}
	// The group list now shows the shared group to Bob.
	listed := fixture.listGroups(t, "user:bob")
	var listView struct {
		Groups []agentrun.ConversationGroup `json:"groups"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &listView); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, group := range listView.Groups {
		found = found || group.ID == groupA.ID
	}
	if !found {
		t.Fatalf("shared group is missing from the owner's list: %+v", listView.Groups)
	}
}

func TestConversationGroupManagerMayManageWithoutOwningMembers(t *testing.T) {
	fixture := newAgentGroupFixture(t)
	groupA := fixture.groups["user:alice"]
	write := []string{identity.PermissionSettingsRead, identity.PermissionSettingsWrite}
	// Alice appoints Bob as a manager of her group.
	response := fixture.call(t, "user:alice", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "set_managers", "managers": []string{"user:bob"},
	}, write...)
	if response.Code != http.StatusOK {
		t.Fatalf("set managers: HTTP %d %s", response.Code, response.Body)
	}
	// The manager may read the group even without owning a member conversation.
	read := fixture.call(t, "user:bob", http.MethodGet, "/api/agent/conversation-groups/"+groupA.ID, nil,
		identity.PermissionSettingsRead)
	if read.Code != http.StatusOK {
		t.Fatalf("manager could not read the group: HTTP %d %s", read.Code, read.Body)
	}
	// The manager may update the shared instructions.
	update := fixture.call(t, "user:bob", http.MethodPost, "/api/agent/conversation-groups/"+groupA.ID, map[string]any{
		"action": "update_instructions", "expected_revision": groupA.SharedRevision,
		"instructions": "instructions published by an appointed manager",
	}, write...)
	if update.Code != http.StatusOK {
		t.Fatalf("manager could not update shared instructions: HTTP %d %s", update.Code, update.Body)
	}
}

func TestConversationGroupLegacyGroupWithoutPrincipalOwnerStaysUsable(t *testing.T) {
	fixture := newAgentGroupFixture(t)
	// A group created before principal ownership was recorded has no owner.
	legacy, err := fixture.registry.CreateGroup(fixture.ownerID, fixture.project.ID, "Legacy group")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.registry.AddMember(legacy.ID, fixture.ownerID, fixture.project.ID, fixture.convs["user:alice"][0].ID); err != nil {
		t.Fatal(err)
	}
	write := []string{identity.PermissionSettingsRead, identity.PermissionSettingsWrite}
	// The owner of a member conversation may still manage the legacy group.
	response := fixture.call(t, "user:alice", http.MethodPost, "/api/agent/conversation-groups/"+legacy.ID, map[string]any{
		"action": "update_instructions", "expected_revision": legacy.SharedRevision,
		"instructions": "legacy group instructions",
	}, write...)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy group was not manageable by its member owner: HTTP %d %s", response.Code, response.Body)
	}
	// A principal without a member conversation cannot manage it.
	response = fixture.call(t, "user:bob", http.MethodPost, "/api/agent/conversation-groups/"+legacy.ID, map[string]any{
		"action": "update_instructions", "expected_revision": legacy.SharedRevision + 1,
		"instructions": "instructions from a principal without a member conversation",
	}, write...)
	if response.Code != http.StatusForbidden {
		t.Fatalf("principal without members managed the legacy group: HTTP %d %s", response.Code, response.Body)
	}
}

func TestCreatingConversationCannotJoinInaccessibleGroup(t *testing.T) {
	fixture := newAgentGroupFixture(t)
	groupA := fixture.groups["user:alice"]
	body, err := json.Marshal(map[string]any{
		"provider_id": "provider", "model_id": "model", "group_id": groupA.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/agent/conversations", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	principal := identity.Principal{
		UserID:      identity.ID("bob"),
		ProjectID:   identity.ID(fixture.project.ID),
		Permissions: map[string]bool{identity.PermissionSettingsRead: true, identity.PermissionSettingsWrite: true},
	}
	request = request.WithContext(context.WithValue(request.Context(), sharedPrincipalContextKey{}, principal))
	response := httptest.NewRecorder()
	fixture.server.agentConversationsAPI(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("conversation joined an inaccessible group: HTTP %d %s", response.Code, response.Body)
	}
	store, err := fixture.server.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := store.List("user:bob")
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		for _, member := range groupA.Members {
			if member == session.ID {
				t.Fatalf("conversation was persisted into an inaccessible group: %+v", session.ID)
			}
		}
	}
}
