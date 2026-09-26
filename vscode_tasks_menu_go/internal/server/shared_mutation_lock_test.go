package server

import (
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedMutationLockExclusiveBindAndRelease(t *testing.T) {
	var lock sharedMutationLock
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	alice := identity.Principal{UserID: "alice-id", Username: "alice"}
	bob := identity.Principal{UserID: "bob-id", Username: "bob"}

	lease, conflict, err := lock.acquire(alice, "patch.run", "", now)
	if err != nil || conflict != nil || lease.token == "" {
		t.Fatalf("first acquire lease=%+v conflict=%+v err=%v", lease, conflict, err)
	}
	if !lock.bindResource(lease.token, "session-1") {
		t.Fatal("bind resource failed")
	}
	owner, ok := lock.snapshot()
	if !ok || owner.UserID != alice.UserID || owner.Operation != "patch.run" || owner.ResourceID != "session-1" || !owner.AcquiredAt.Equal(now) {
		t.Fatalf("unexpected owner: %+v ok=%v", owner, ok)
	}

	_, conflict, err = lock.acquire(bob, "file.write", "src/main.go", now.Add(time.Second))
	if err != nil || conflict == nil {
		t.Fatalf("conflicting acquire conflict=%+v err=%v", conflict, err)
	}
	if conflict.UserID != alice.UserID || conflict.ResourceID != "session-1" {
		t.Fatalf("conflict did not expose current owner metadata: %+v", conflict)
	}
	if lock.release("wrong-token") {
		t.Fatal("wrong token released lock")
	}
	if !lock.release(lease.token) {
		t.Fatal("lease release failed")
	}
	if _, ok := lock.snapshot(); ok {
		t.Fatal("lock still held after release")
	}

	next, conflict, err := lock.acquire(bob, "file.write", "src/main.go", now.Add(2*time.Second))
	if err != nil || conflict != nil || next.token == "" {
		t.Fatalf("second acquire lease=%+v conflict=%+v err=%v", next, conflict, err)
	}
}
