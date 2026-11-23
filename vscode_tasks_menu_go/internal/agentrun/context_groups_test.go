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
