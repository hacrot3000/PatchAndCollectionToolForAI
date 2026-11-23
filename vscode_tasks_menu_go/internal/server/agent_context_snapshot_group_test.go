package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestContextSnapshotPinsGroupInstructionsOnlyForMembers(t *testing.T) {
	s, memberID := newAgentConversationFixture(t, "stateless")
	project, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := s.agentConversationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	group, err := registry.CreateGroup(ownerID, project.ID, "Shared build contract")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AddMember(group.ID, ownerID, project.ID, memberID); err != nil {
		t.Fatal(err)
	}
	group, err = registry.UpdateGroupInstructions(group.ID, ownerID, project.ID, group.SharedRevision, "Use API contract revision 4 only.")
	if err != nil {
		t.Fatal(err)
	}
	conversationStore, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	independent, err := agentchat.NewSession("local", s.Workspace, project.ID, "provider", "model", agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	independent, err = conversationStore.Save(independent, 0)
	if err != nil {
		t.Fatal(err)
	}
	createSnapshot := func(conversationID, prompt string) agentrun.ContextSnapshot {
		t.Helper()
		response := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{
			"conversation_id": conversationID, "prompt": prompt,
		})
		if response.Code != http.StatusCreated {
			t.Fatalf("snapshot for %s: HTTP %d: %s", conversationID, response.Code, response.Body)
		}
		var body struct {
			ID        string `json:"context_snapshot_id"`
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		store, err := s.agentContextSnapshotStore("local")
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Get(body.ID, ownerID, conversationID, body.ProjectID)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}

	memberSnapshot := createSnapshot(memberID, "member prompt")
	independentSnapshot := createSnapshot(independent.ID, "independent prompt")
	wantSource := "conversation-group:" + group.ID + "@" + "2" + ":" + group.InstructionsSHA256
	if memberSnapshot.Policy == nil || !strings.Contains(memberSnapshot.Policy.SystemInstructions, "Use API contract revision 4 only.") || !containsString(memberSnapshot.SourceRefs, wantSource) {
		t.Fatalf("member snapshot omitted shared instructions or revision provenance: policy=%+v refs=%v", memberSnapshot.Policy, memberSnapshot.SourceRefs)
	}
	if independentSnapshot.Policy == nil || strings.Contains(independentSnapshot.Policy.SystemInstructions, "Use API contract revision 4 only.") || containsString(independentSnapshot.SourceRefs, wantSource) {
		t.Fatalf("independent conversation received group context: policy=%+v refs=%v", independentSnapshot.Policy, independentSnapshot.SourceRefs)
	}

	updated, err := registry.UpdateGroupInstructions(group.ID, ownerID, project.ID, group.SharedRevision, "Use API contract revision 5 only.")
	if err != nil {
		t.Fatal(err)
	}
	nextSnapshot := createSnapshot(memberID, "member prompt after revision")
	if !strings.Contains(nextSnapshot.Policy.SystemInstructions, "Use API contract revision 5 only.") || strings.Contains(memberSnapshot.Policy.SystemInstructions, "revision 5") {
		t.Fatalf("new group revision did not preserve snapshot immutability: old=%q new=%q", memberSnapshot.Policy.SystemInstructions, nextSnapshot.Policy.SystemInstructions)
	}
	if !containsString(nextSnapshot.SourceRefs, "conversation-group:"+updated.ID+"@3:"+updated.InstructionsSHA256) {
		t.Fatalf("new snapshot did not pin the updated group revision: %v", nextSnapshot.SourceRefs)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
