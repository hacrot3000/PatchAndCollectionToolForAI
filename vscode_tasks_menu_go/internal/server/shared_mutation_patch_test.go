package server

import (
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestSharedPatchMutationModeSelection(t *testing.T) {
	for _, mode := range []string{"", "queue", "resume"} {
		if !sharedPatchMutationRequired(mode) {
			t.Fatalf("mode %q must hold workspace mutation lock", mode)
		}
	}
	for _, mode := range []string{"history", "plan", "health"} {
		if sharedPatchMutationRequired(mode) {
			t.Fatalf("read-only mode %q must not hold workspace mutation lock", mode)
		}
	}
}

func TestSharedPatchMutationLockReleasesStoppedAndCompletedSession(t *testing.T) {
	principal := identity.Principal{UserID: "alice", Username: "alice"}
	service := &ownershipTestService{supported: true, items: []session.Metadata{{
		ID: "patch-1",
		Kind: tasks.SessionKindPatch,
		Status: "running",
	}}}
	s := &Server{Sessions: service}
	lease, conflict, err := s.sharedMutation.acquire(principal, "patch.run", "", time.Now().UTC())
	if err != nil || conflict != nil {
		t.Fatalf("acquire conflict=%+v err=%v", conflict, err)
	}
	if !s.sharedMutation.bindResource(lease.token, "patch-1") {
		t.Fatal("bind patch session failed")
	}
	s.releaseSharedMutationForSession("patch-1")
	if _, ok := s.sharedMutation.snapshot(); ok {
		t.Fatal("explicitly stopped patch session kept mutation lock")
	}

	lease, conflict, err = s.sharedMutation.acquire(principal, "patch.run", "", time.Now().UTC())
	if err != nil || conflict != nil {
		t.Fatalf("reacquire conflict=%+v err=%v", conflict, err)
	}
	if !s.sharedMutation.bindResource(lease.token, "patch-1") {
		t.Fatal("second bind failed")
	}
	service.items[0].Status = "success"
	s.refreshSharedMutationLock()
	if _, ok := s.sharedMutation.snapshot(); ok {
		t.Fatal("completed patch session kept stale mutation lock")
	}
}
