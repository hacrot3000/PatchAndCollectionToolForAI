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

func TestCoordinationAPIIsolatesGroupsOwnersAndUnscopedQueries(t *testing.T) {
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
	createConversation := func(owner string) agentchat.Session {
		t.Helper()
		session, err := agentchat.NewSession(owner, s.Workspace, project.ID, "provider", "model", agentchat.Stateless)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := store.Save(session, 0)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	ownerA, ownerB := "user:alice", "user:bob"
	a1, a2, a3 := createConversation(ownerA), createConversation(ownerA), createConversation(ownerA)
	b1, b2 := createConversation(ownerB), createConversation(ownerB)
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := s.agentConversationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	groupA, err := groups.CreateGroupForPrincipal(ownerWorkspaceID, project.ID, "Shared group A", "local")
	if err != nil {
		t.Fatal(err)
	}
	groupB, err := groups.CreateGroupForPrincipal(ownerWorkspaceID, project.ID, "Group B", "local")
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range []string{a1.ID, a2.ID, b1.ID, b2.ID} {
		if _, err := groups.AddMember(groupA.ID, ownerWorkspaceID, project.ID, member); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := groups.AddMember(groupB.ID, ownerWorkspaceID, project.ID, a3.ID); err != nil {
		t.Fatal(err)
	}

	call := func(owner string, method, path string, payload any) *httptest.ResponseRecorder {
		t.Helper()
		var body []byte
		if payload != nil {
			body, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		principal := identity.Principal{
			UserID: identity.ID(strings.TrimPrefix(owner, "user:")), ProjectID: identity.ID(project.ID),
			Permissions: map[string]bool{identity.PermissionSettingsRead: true, identity.PermissionSettingsWrite: true},
		}
		request = request.WithContext(context.WithValue(request.Context(), sharedPrincipalContextKey{}, principal))
		response := httptest.NewRecorder()
		s.agentCoordinationAPI(response, request)
		return response
	}
	publish := func(owner, conversationID, groupID, signature, artifactRef, artifactID string) {
		t.Helper()
		progress := call(owner, http.MethodPost, "/api/agent/coordination", map[string]any{
			"kind": "peer_progress", "conversation_id": conversationID, "group_id": groupID, "status": "running", "body": owner,
		})
		if progress.Code != http.StatusOK {
			t.Fatalf("publish scoped progress for %s: HTTP %d %s", owner, progress.Code, progress.Body)
		}
		contract := call(owner, http.MethodPost, "/api/agent/coordination", map[string]any{
			"kind": "contract_publish", "conversation_id": conversationID, "group_id": groupID,
			"contract": map[string]any{"key": "api.auth", "signature": signature, "status": "planned"},
		})
		if contract.Code != http.StatusOK {
			t.Fatalf("publish scoped contract for %s/%s: HTTP %d %s", owner, groupID, contract.Code, contract.Body)
		}
		artifact := call(owner, http.MethodPost, "/api/agent/coordination", map[string]any{
			"kind": "artifact_publish", "conversation_id": conversationID, "group_id": groupID,
			"artifact": map[string]any{"id": artifactID, "kind": "diff", "ref": artifactRef,
				"sha256": strings.Repeat("a", 64), "conversation_id": conversationID},
		})
		if artifact.Code != http.StatusOK {
			t.Fatalf("publish scoped artifact for %s/%s: HTTP %d %s", owner, groupID, artifact.Code, artifact.Body)
		}
		dependency := call(owner, http.MethodPost, "/api/agent/coordination", map[string]any{
			"kind": "dependency_register", "conversation_id": conversationID, "group_id": groupID,
			"dependency": map[string]any{"consumer_task": "consumer", "producer_task": "producer", "contract_key": "api.auth"},
		})
		if dependency.Code != http.StatusOK {
			t.Fatalf("register scoped dependency for %s/%s: HTTP %d %s", owner, groupID, dependency.Code, dependency.Body)
		}
	}
	publish(ownerA, a1.ID, groupA.ID, "auth(owner A, group A)", "diff:owner-a-group-a", "same-artifact-id")
	publish(ownerA, a3.ID, groupB.ID, "auth(owner A, group B)", "diff:owner-a-group-b", "artifact-group-b")
	publish(ownerB, b1.ID, groupA.ID, "auth(owner B, group A)", "diff:owner-b-group-a", "same-artifact-id")
	askA := call(ownerA, http.MethodPost, "/api/agent/coordination", map[string]any{
		"kind": "peer_message", "message_kind": "ask", "conversation_id": a1.ID,
		"to_conversation_id": a2.ID, "group_id": groupA.ID, "correlation_id": "same-correlation", "body": "A only",
	})
	askB := call(ownerB, http.MethodPost, "/api/agent/coordination", map[string]any{
		"kind": "peer_message", "message_kind": "ask", "conversation_id": b1.ID,
		"to_conversation_id": b2.ID, "group_id": groupA.ID, "correlation_id": "same-correlation", "body": "B only",
	})
	if askA.Code != http.StatusOK || askB.Code != http.StatusOK {
		t.Fatalf("owner-scoped messages rejected: A=%d %s B=%d %s", askA.Code, askA.Body, askB.Code, askB.Body)
	}

	type coordinationView struct {
		Peers        []agentrun.Peer              `json:"peers"`
		Messages     []agentrun.PeerMessage       `json:"messages"`
		Contracts    []agentrun.Contract          `json:"contracts"`
		Artifacts    []agentrun.Artifact          `json:"artifacts"`
		Dependencies []agentrun.Dependency        `json:"dependencies"`
		Events       []agentrun.CoordinationEvent `json:"events"`
	}
	get := func(owner, groupID, conversationID string) coordinationView {
		t.Helper()
		path := "/api/agent/coordination?group_id=" + groupID + "&conversation_id=" + conversationID
		response := call(owner, http.MethodGet, path, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("get scoped coordination for %s/%s: HTTP %d %s", owner, groupID, response.Code, response.Body)
		}
		var view coordinationView
		if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	viewA := get(ownerA, groupA.ID, a1.ID)
	viewB := get(ownerB, groupA.ID, b1.ID)
	viewAGroupB := get(ownerA, groupB.ID, a3.ID)
	for name, view := range map[string]coordinationView{"A/group A": viewA, "B/group A": viewB, "A/group B": viewAGroupB} {
		if len(view.Contracts) != 1 || len(view.Artifacts) != 1 || len(view.Dependencies) != 1 || len(view.Events) == 0 {
			t.Fatalf("incomplete scoped data for %s: %+v", name, view)
		}
	}
	if viewA.Contracts[0].Signature != "auth(owner A, group A)" || viewB.Contracts[0].Signature != "auth(owner B, group A)" || viewAGroupB.Contracts[0].Signature != "auth(owner A, group B)" {
		t.Fatalf("contract key collision crossed group/owner scope: A=%+v B=%+v A2=%+v", viewA.Contracts, viewB.Contracts, viewAGroupB.Contracts)
	}
	if len(viewA.Peers) != 1 || viewA.Peers[0].ConversationID != a1.ID || len(viewB.Peers) != 1 || viewB.Peers[0].ConversationID != b1.ID {
		t.Fatalf("peer records crossed owner scope: A=%+v B=%+v", viewA.Peers, viewB.Peers)
	}
	if len(viewA.Messages) != 1 || viewA.Messages[0].Body != "A only" || len(viewB.Messages) != 1 || viewB.Messages[0].Body != "B only" {
		t.Fatalf("peer messages crossed owner scope: A=%+v B=%+v", viewA.Messages, viewB.Messages)
	}
	for _, scoped := range []struct {
		name  string
		view  coordinationView
		group string
	}{{"A/group A", viewA, groupA.ID}, {"B/group A", viewB, groupA.ID}, {"A/group B", viewAGroupB, groupB.ID}} {
		for _, event := range scoped.view.Events {
			if event.GroupID != scoped.group {
				t.Fatalf("event escaped %s: %+v", scoped.name, event)
			}
		}
	}
	if viewA.Artifacts[0].Ref != "diff:owner-a-group-a" || viewB.Artifacts[0].Ref != "diff:owner-b-group-a" || viewAGroupB.Artifacts[0].Ref != "diff:owner-a-group-b" {
		t.Fatalf("artifacts crossed scope: A=%+v B=%+v A2=%+v", viewA.Artifacts, viewB.Artifacts, viewAGroupB.Artifacts)
	}
	if viewA.Dependencies[0].GroupID != groupA.ID || viewB.Dependencies[0].GroupID != groupA.ID || viewAGroupB.Dependencies[0].GroupID != groupB.ID ||
		viewA.Dependencies[0].ConversationID != a1.ID || viewB.Dependencies[0].ConversationID != b1.ID || viewAGroupB.Dependencies[0].ConversationID != a3.ID {
		t.Fatalf("dependency ownership was not persisted: A=%+v B=%+v A2=%+v", viewA.Dependencies, viewB.Dependencies, viewAGroupB.Dependencies)
	}
	if response := getUnauthorizedGroupScope(t, s, ownerA, groupB.ID, b1.ID); response != http.StatusNotFound {
		t.Fatalf("owner A read owner B conversation: HTTP %d", response)
	}
	if response := call(ownerA, http.MethodGet, "/api/agent/coordination", nil); response.Code != http.StatusBadRequest {
		t.Fatalf("unscoped coordination query returned HTTP %d: %s", response.Code, response.Body)
	}
	if response := call(ownerB, http.MethodGet, "/api/agent/coordination?group_id="+groupB.ID, nil); response.Code != http.StatusNotFound {
		t.Fatalf("owner B read a group with no owned conversation: HTTP %d %s", response.Code, response.Body)
	}
}

func getUnauthorizedGroupScope(t *testing.T, s *Server, owner, groupID, conversationID string) int {
	t.Helper()
	userID := strings.TrimPrefix(owner, "user:")
	principal := identity.Principal{UserID: identity.ID(userID), Permissions: map[string]bool{identity.PermissionSettingsRead: true, identity.PermissionSettingsWrite: true}}
	request := httptest.NewRequest(http.MethodGet, "/api/agent/coordination?group_id="+groupID+"&conversation_id="+conversationID, nil)
	request = request.WithContext(context.WithValue(request.Context(), sharedPrincipalContextKey{}, principal))
	response := httptest.NewRecorder()
	s.agentCoordinationAPI(response, request)
	return response.Code
}
