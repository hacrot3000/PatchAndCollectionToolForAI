package approval

import (
	"path/filepath"
	"testing"
	"time"
)

func TestApprovalStorePolicyAndOneTimeConsume(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "approvals.json"))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := store.SetPolicy(Policy{Rules: []Rule{
		{Action: "git.remote.delete", Mode: ModeType},
		{Action: "db.production.write", Mode: ModeAdmin},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Rules) != 2 {
		t.Fatalf("rules=%d", len(policy.Rules))
	}
	mode, err := store.ModeFor("git.remote.delete")
	if err != nil || mode != ModeType {
		t.Fatalf("mode=%q err=%v", mode, err)
	}
	req, err := store.Create("git.remote.delete", "origin/feature-x", "user-1", "alice", "feature-x")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != "approved" || req.Mode != ModeType {
		t.Fatalf("request=%+v", req)
	}
	if _, err := store.Consume(req.ID, req.Action, req.Resource, req.RequesterID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Consume(req.ID, req.Action, req.Resource, req.RequesterID); err == nil {
		t.Fatal("approval grant was reusable")
	}
}

func TestAdminApprovalRequiresResolveAndMatchesRequesterActionResource(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "approvals.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetPolicy(Policy{Rules: []Rule{{Action: "db.production.write", Mode: ModeAdmin}}}); err != nil {
		t.Fatal(err)
	}
	req, err := store.Create("db.production.write", "prod/users", "user-1", "alice", "")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != "pending" {
		t.Fatalf("status=%q", req.Status)
	}
	if _, err := store.Consume(req.ID, req.Action, req.Resource, req.RequesterID); err == nil {
		t.Fatal("pending approval unexpectedly consumed")
	}
	resolved, err := store.Resolve(req.ID, "admin-1", "approved")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "approved" || resolved.ResolverID != "admin-1" {
		t.Fatalf("resolved=%+v", resolved)
	}
	if _, err := store.Consume(req.ID, req.Action, "other", req.RequesterID); err == nil {
		t.Fatal("grant unexpectedly matched different resource")
	}
	if _, err := store.Consume(req.ID, req.Action, req.Resource, "user-2"); err == nil {
		t.Fatal("grant unexpectedly matched different requester")
	}
	if _, err := store.Consume(req.ID, req.Action, req.Resource, req.RequesterID); err != nil {
		t.Fatal(err)
	}
}

func TestApprovalStoreRejectsInvalidPolicyAndExpiresGrants(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "approvals.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetPolicy(Policy{Rules: []Rule{{Action: "bad action", Mode: ModeConfirm}}}); err == nil {
		t.Fatal("invalid action unexpectedly accepted")
	}
	if _, err := store.SetPolicy(Policy{Rules: []Rule{{Action: "git.push", Mode: "unknown"}}}); err == nil {
		t.Fatal("invalid mode unexpectedly accepted")
	}
	if _, err := store.SetPolicy(Policy{Rules: []Rule{{Action: "git.push", Mode: ModeConfirm}, {Action: "git.push", Mode: ModeAdmin}}}); err == nil {
		t.Fatal("duplicate action unexpectedly accepted")
	}

	if _, err := store.SetPolicy(Policy{Rules: []Rule{{Action: "git.push", Mode: ModeConfirm}}}); err != nil {
		t.Fatal(err)
	}
	req, err := store.Create("git.push", "origin/main", "user-1", "alice", "")
	if err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	state, err := store.loadLocked()
	if err != nil {
		store.mu.Unlock()
		t.Fatal(err)
	}
	for i := range state.Requests {
		if state.Requests[i].ID == req.ID {
			state.Requests[i].ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
		}
	}
	if err := store.saveLocked(state); err != nil {
		store.mu.Unlock()
		t.Fatal(err)
	}
	store.mu.Unlock()

	if _, err := store.Consume(req.ID, req.Action, req.Resource, req.RequesterID); err == nil {
		t.Fatal("expired approval unexpectedly consumed")
	}
}
