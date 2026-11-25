package agentrun

import (
	"path/filepath"
	"testing"
)

func TestContextGroupStorePinsRevisionAndOwnerScope(t *testing.T) {
	store, err := NewContextGroupStore(filepath.Join(t.TempDir(), "groups"))
	if err != nil {
		t.Fatal(err)
	}
	group, err := store.Create("owner-1", "project-1", "Shared tasks", "Use Go 1.20; do not commit.")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(group.ID, "owner-1", "project-1")
	if err != nil || loaded.Revision != 1 || loaded.InstructionsSHA256 == "" {
		t.Fatalf("load group: %+v %v", loaded, err)
	}
	if _, err := store.Get(group.ID, "owner-2", "project-1"); err == nil {
		t.Fatal("group crossed owner boundary")
	}
	updated, err := store.Update(group.ID, "owner-1", "project-1", 1, group.Title, "New shared instruction")
	if err != nil || updated.Revision != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := store.Update(group.ID, "owner-1", "project-1", 1, group.Title, "stale update"); err == nil {
		t.Fatal("stale context group update accepted")
	}
}

func TestConversationGroupAccessIsIndependentOfReadGrants(t *testing.T) {
	aliceOwnsMember := func(conversationID string) bool { return conversationID == "conv-alice" }
	noMembers := func(string) bool { return false }
	group := ConversationGroup{
		PrincipalOwner:    "user:alice",
		SharedPrincipals:  []string{"user:bob"},
		ManagerPrincipals: []string{"user:carol"},
		Members:           []string{"conv-alice"},
	}
	if access := group.Access("user:alice", aliceOwnsMember); !access.Read || !access.Manage {
		t.Fatalf("owner access: %+v", access)
	}
	if access := group.Access("user:bob", aliceOwnsMember); !access.Read || access.Manage {
		t.Fatalf("shared principal gained management: %+v", access)
	}
	if access := group.Access("user:carol", aliceOwnsMember); !access.Read || !access.Manage {
		t.Fatalf("manager access: %+v", access)
	}
	if access := group.Access("user:dave", noMembers); access.Read || access.Manage {
		t.Fatalf("unrelated principal gained access: %+v", access)
	}
}

func TestConversationGroupOwnerlessGrantsNoImplicitRights(t *testing.T) {
	group := ConversationGroup{Members: []string{"conv-alice"}}
	ownerOf := func(conversationID string) bool { return conversationID == "conv-alice" }
	if group.ManagedBy("user:alice", ownerOf) {
		t.Fatal("member owner manages an ownerless group")
	}
	// Read access may still be granted by verified membership, but it never
	// grants management.
	access := group.Access("user:alice", ownerOf)
	if !access.Read || access.Manage {
		t.Fatalf("member owner access: %+v", access)
	}
	if (ConversationGroup{}).ManagedBy("user:alice", ownerOf) {
		t.Fatal("empty ownerless group granted management")
	}
}

func TestConversationGroupOwnerClaimRequiresEvidence(t *testing.T) {
	registry, err := NewConversationRegistry(filepath.Join(t.TempDir(), "conversations.json"))
	if err != nil {
		t.Fatal(err)
	}
	group, err := registry.CreateGroup("workspace", "project", "Legacy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AddMember(group.ID, "workspace", "project", "conv-alice"); err != nil {
		t.Fatal(err)
	}
	admin := func(principal string) bool { return principal == "user:admin" }
	owns := func(principal, conversationID string) bool {
		return principal == "user:alice" && conversationID == "conv-alice"
	}
	if _, err := registry.ClaimGroupOwner(group.ID, "workspace", "project", GroupOwnerClaim{Principal: "user:alice"}, admin, owns); err == nil {
		t.Fatal("claim without evidence was accepted")
	}
	claimed, err := registry.ClaimGroupOwner(group.ID, "workspace", "project",
		GroupOwnerClaim{Principal: "user:alice", ConversationIDs: []string{"conv-alice"}}, admin, owns)
	if err != nil || claimed.PrincipalOwner != "user:alice" {
		t.Fatalf("verified claim failed: %+v %v", claimed, err)
	}
	// A second claim by a different principal requires an administrator and
	// does not silently overwrite the recorded owner.
	if _, err := registry.ClaimGroupOwner(group.ID, "workspace", "project",
		GroupOwnerClaim{Principal: "user:bob", ConversationIDs: []string{"conv-alice"}}, admin, owns); err == nil {
		t.Fatal("non-administrator replaced an existing owner")
	}
}
