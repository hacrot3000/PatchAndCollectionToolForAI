package server

import (
	"encoding/json"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestSharedPatchMutationModeSelection(t *testing.T) {
	for _, mode := range []string{"", "queue", "resume"} {
		if sharedPatchMutationRequired(mode, "native") {
			t.Fatalf("native mode %q must defer workspace mutation lock until PATCH execution", mode)
		}
		if !sharedPatchMutationRequired(mode, "terminal") {
			t.Fatalf("terminal mode %q must conservatively hold workspace mutation lock", mode)
		}
	}
	for _, mode := range []string{"history", "plan", "health"} {
		if sharedPatchMutationRequired(mode, "native") || sharedPatchMutationRequired(mode, "terminal") {
			t.Fatalf("read-only mode %q must not hold workspace mutation lock", mode)
		}
	}
}

func TestNativePatchCommandLockClassification(t *testing.T) {
	state := session.ProtocolState{Prompt: json.RawMessage(`{
		"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"p1","prompt_kind":"queue_selection",
		"actions":["select","cancel"],
		"items":[{"index":1,"kind":"PATCH"},{"index":2,"kind":"COLLECT"}]
	}`)}
	if !patchPromptResponseRequiresWorkspaceMutation(state, patchPromptResponseRequest{PromptID: "p1", Action: "select", Indexes: []int{1}}) {
		t.Fatal("PATCH selection must acquire workspace mutation lock")
	}
	if patchPromptResponseRequiresWorkspaceMutation(state, patchPromptResponseRequest{PromptID: "p1", Action: "select", Indexes: []int{2}}) {
		t.Fatal("COLLECT-only selection must not acquire workspace mutation lock")
	}
	if patchPromptResponseRequiresWorkspaceMutation(state, patchPromptResponseRequest{PromptID: "p1", Action: "cancel"}) {
		t.Fatal("queue cancel must not acquire workspace mutation lock")
	}
	for _, action := range []string{"all", "failed", "remaining"} {
		if !patchResumeActionRequiresWorkspaceMutation(patchResumeActionRequest{Action: action}) {
			t.Fatalf("Resume action %q must acquire workspace mutation lock", action)
		}
	}
	for _, action := range []string{"collect_failed", "delete_failed", "history", "normal"} {
		if patchResumeActionRequiresWorkspaceMutation(patchResumeActionRequest{Action: action}) {
			t.Fatalf("Resume action %q must not hold source workspace mutation lock", action)
		}
	}
}

func TestNativePatchQueueLeavesWorkspaceAvailableForOtherMutations(t *testing.T) {
	if sharedPatchMutationRequired("queue", "native") {
		t.Fatal("native Queue must not reserve the workspace")
	}
	lock := &sharedMutationLock{}
	principal := identity.Principal{UserID: "alice", Username: "alice"}
	now := time.Now().UTC()

	gitLease, conflict, err := lock.acquire(principal, "git.write", "main", now)
	if err != nil || conflict != nil || gitLease.token == "" {
		t.Fatalf("Git mutation should be available while native Queue is idle: lease=%+v conflict=%+v err=%v", gitLease, conflict, err)
	}
	if !lock.release(gitLease.token) {
		t.Fatal("failed to release Git mutation test lease")
	}

	updateLease, conflict, err := lock.acquire(principal, "selfupdate.run", "", now.Add(time.Second))
	if err != nil || conflict != nil || updateLease.token == "" {
		t.Fatalf("self-update should be available while native Queue is idle: lease=%+v conflict=%+v err=%v", updateLease, conflict, err)
	}
}

func TestActivePatchMutationStillBlocksOtherWorkspaceMutations(t *testing.T) {
	lock := &sharedMutationLock{}
	patchUser := identity.Principal{UserID: "alice", Username: "alice"}
	otherUser := identity.Principal{UserID: "bob", Username: "bob"}
	lease, conflict, err := lock.acquire(patchUser, "patch.run", "patch-session", time.Now().UTC())
	if err != nil || conflict != nil || lease.token == "" {
		t.Fatalf("prepare Patch mutation lease=%+v conflict=%+v err=%v", lease, conflict, err)
	}
	if _, conflict, err := lock.acquire(otherUser, "selfupdate.run", "", time.Now().UTC()); err != nil || conflict == nil || conflict.Operation != "patch.run" {
		t.Fatalf("active Patch must block self-update: conflict=%+v err=%v", conflict, err)
	}
	if _, conflict, err := lock.acquire(otherUser, "git.write", "main", time.Now().UTC()); err != nil || conflict == nil || conflict.ResourceID != "patch-session" {
		t.Fatalf("active Patch must block Git mutation: conflict=%+v err=%v", conflict, err)
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
