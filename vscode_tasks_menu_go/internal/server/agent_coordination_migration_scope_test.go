package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// agentLegacyCoordinationFixture prepares a shared server with two owners, two
// conversation groups and a legacy coordination journal that also holds records
// without any owner scope.
func agentLegacyCoordinationFixture(t *testing.T) (*Server, map[string]agentchat.Session, map[string]agentrun.ConversationGroup, string) {
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
	conversations := map[string]agentchat.Session{}
	for _, owner := range []string{"user:alice", "user:bob"} {
		session, err := agentchat.NewSession(owner, s.Workspace, project.ID, "provider", "model", agentchat.Stateless)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := store.Save(session, 0)
		if err != nil {
			t.Fatal(err)
		}
		conversations[owner] = saved
	}
	ownerID, err := agentrunWorkspaceID(s)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := s.agentConversationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	groupA, err := registry.CreateGroupForPrincipal(ownerID, project.ID, "Group A", "user:alice")
	if err != nil {
		t.Fatal(err)
	}
	groupB, err := registry.CreateGroupForPrincipal(ownerID, project.ID, "Group B", "user:bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AddMember(groupA.ID, ownerID, project.ID, conversations["user:alice"].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AddMember(groupB.ID, ownerID, project.ID, conversations["user:bob"].ID); err != nil {
		t.Fatal(err)
	}
	groups := map[string]agentrun.ConversationGroup{"user:alice": groupA, "user:bob": groupB}
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
			"alice-peer": map[string]any{"conversation_id": conversations["user:alice"].ID, "group_id": groupA.ID, "status": "idle"},
			"bob-peer":   map[string]any{"conversation_id": conversations["user:bob"].ID, "group_id": groupB.ID, "status": "idle"},
			"scopeless":  map[string]any{"status": "idle"},
		},
		"messages": []any{
			map[string]any{"id": "m-alice", "group_id": groupA.ID, "from_conversation": conversations["user:alice"].ID, "to_conversation": "other", "kind": "ask", "body": "alice message"},
			map[string]any{"id": "m-bob", "group_id": groupB.ID, "from_conversation": conversations["user:bob"].ID, "to_conversation": "other", "kind": "ask", "body": "bob message"},
			map[string]any{"id": "m-scopeless", "from_conversation": "", "to_conversation": "", "kind": "ask", "body": "scopeless message"},
		},
		"contracts": map[string]any{
			"alice.api": map[string]any{"key": "alice.api", "owner_group": groupA.ID, "signature": "alice(): Result", "revision": 1, "status": "planned"},
			"bob.api":   map[string]any{"key": "bob.api", "owner_group": groupB.ID, "signature": "bob(): Result", "revision": 1, "status": "planned"},
			"loose.api": map[string]any{"key": "loose.api", "signature": "loose(): Result", "revision": 1, "status": "planned"},
		},
		"artifacts": map[string]any{
			"artifact-alice": map[string]any{"id": "artifact-alice", "kind": "diff", "ref": "diff:alice", "sha256": repeatHex('a'), "conversation_id": conversations["user:alice"].ID},
			"artifact-bob":   map[string]any{"id": "artifact-bob", "kind": "diff", "ref": "diff:bob", "sha256": repeatHex('b'), "conversation_id": conversations["user:bob"].ID},
			"artifact-loose": map[string]any{"id": "artifact-loose", "kind": "diff", "ref": "diff:loose", "sha256": repeatHex('c')},
		},
		"dependencies": map[string]any{
			"dep-alice": map[string]any{"id": "dep-alice", "consumer_task": "consumer", "producer_task": "producer", "contract_key": "alice.api", "group_id": groupA.ID},
			"dep-bob":   map[string]any{"id": "dep-bob", "consumer_task": "consumer", "producer_task": "producer", "contract_key": "bob.api", "group_id": groupB.ID},
			"dep-loose": map[string]any{"id": "dep-loose", "consumer_task": "consumer", "producer_task": "producer", "contract_key": "loose.api"},
		},
	}
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	return s, conversations, groups, legacyPath
}

func repeatHex(character rune) string {
	out := make([]rune, 64)
	for i := range out {
		out[i] = character
	}
	return string(out)
}

func TestLegacyCoordinationMigrationIsolatesTwoOwnersAndQuarantinesUnscopedRecords(t *testing.T) {
	s, conversations, groups, legacyPath := agentLegacyCoordinationFixture(t)

	aliceHub, err := s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	bobHub, err := s.agentCoordinationHub("user:bob")
	if err != nil {
		t.Fatal(err)
	}

	alicePeers, err := aliceHub.Peers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(alicePeers) != 1 || alicePeers[0].ConversationID != conversations["user:alice"].ID {
		t.Fatalf("owner A migrated foreign peers: %+v", alicePeers)
	}
	bobPeers, err := bobHub.Peers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(bobPeers) != 1 || bobPeers[0].ConversationID != conversations["user:bob"].ID {
		t.Fatalf("owner B migrated foreign peers: %+v", bobPeers)
	}

	aliceMessages, err := aliceHub.PeerMessages("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceMessages) != 1 || aliceMessages[0].Body != "alice message" {
		t.Fatalf("owner A migrated foreign messages: %+v", aliceMessages)
	}
	bobMessages, err := bobHub.PeerMessages("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(bobMessages) != 1 || bobMessages[0].Body != "bob message" {
		t.Fatalf("owner B migrated foreign messages: %+v", bobMessages)
	}

	aliceContracts, err := aliceHub.Contracts("")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceContracts) != 1 || aliceContracts[0].Key != "alice.api" {
		t.Fatalf("owner A migrated foreign contracts: %+v", aliceContracts)
	}
	bobContracts, err := bobHub.Contracts("")
	if err != nil {
		t.Fatal(err)
	}
	if len(bobContracts) != 1 || bobContracts[0].Key != "bob.api" {
		t.Fatalf("owner B migrated foreign contracts: %+v", bobContracts)
	}

	aliceArtifacts, err := aliceHub.Artifacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceArtifacts) != 1 || aliceArtifacts[0].Ref != "diff:alice" {
		t.Fatalf("owner A migrated foreign artifacts: %+v", aliceArtifacts)
	}
	bobArtifacts, err := bobHub.Artifacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(bobArtifacts) != 1 || bobArtifacts[0].Ref != "diff:bob" {
		t.Fatalf("owner B migrated foreign artifacts: %+v", bobArtifacts)
	}

	aliceDependencies, err := aliceHub.Dependencies()
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceDependencies) != 1 || aliceDependencies[0].GroupID != groups["user:alice"].ID {
		t.Fatalf("owner A migrated foreign dependencies: %+v", aliceDependencies)
	}
	bobDependencies, err := bobHub.Dependencies()
	if err != nil {
		t.Fatal(err)
	}
	if len(bobDependencies) != 1 || bobDependencies[0].GroupID != groups["user:bob"].ID {
		t.Fatalf("owner B migrated foreign dependencies: %+v", bobDependencies)
	}

	// The unscoped legacy records are quarantined, not silently dropped and
	// never attributed to an owner.
	aliceHubPath := agentOwnerHubPath(t, s, "user:alice")
	unclaimed, err := s.agentCoordinationUnclaimedRecords(aliceHubPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(unclaimed) == 0 {
		t.Fatal("unscoped legacy records were not quarantined")
	}
	kinds := map[string]bool{}
	for _, record := range unclaimed {
		kinds[record.Kind] = true
		if record.Reason == "" {
			t.Fatalf("quarantined record has no reason: %+v", record)
		}
	}
	for _, kind := range []string{"peer", "message", "contract", "artifact", "dependency"} {
		if !kinds[kind] {
			t.Fatalf("quarantine is missing a %s record: %+v", kind, unclaimed)
		}
	}
	// The legacy journal is preserved for a rollback.
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy coordination journal was removed: %v", err)
	}
}

func TestLegacyCoordinationMigrationIsIdempotent(t *testing.T) {
	s, conversations, _, legacyPath := agentLegacyCoordinationFixture(t)
	hub, err := s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	hubPath := agentOwnerHubPath(t, s, "user:alice")
	first, err := os.ReadFile(hubPath)
	if err != nil {
		t.Fatal(err)
	}
	// Running the migration again must not change the migrated journal.
	for i := 0; i < 3; i++ {
		if _, err := s.agentCoordinationHub("user:alice"); err != nil {
			t.Fatalf("repeated migration failed: %v", err)
		}
	}
	second, err := os.ReadFile(hubPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeated migration changed the journal:\nfirst=%s\nsecond=%s", first, second)
	}
	peers, err := hub.Peers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].ConversationID != conversations["user:alice"].ID {
		t.Fatalf("repeated migration duplicated peers: %+v", peers)
	}
	// The legacy journal is untouched by repeated migrations.
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy coordination journal was removed: %v", err)
	}
}

// agentOwnerHubPath returns the owner-scoped coordination journal path.
func agentOwnerHubPath(t *testing.T, s *Server, owner string) string {
	t.Helper()
	privatePath, err := aiPrivatePath(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(filepath.Dir(privatePath), "agent-coordination-"+aiWorkspaceHash(s.Workspace)+"-"+aiOwnerHash(owner))
	return filepath.Join(dir, "coordination.json")
}
