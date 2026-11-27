package server

import (
	"context"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
)

func clp20GuardSession(t *testing.T, goal string) agentchat.Session {
	t.Helper()
	x, err := agentchat.NewSession("local", "/workspace", "project", "provider", "model", agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	if !x.BindCollectionGuard(goal, "", false) {
		t.Fatal("bind guard")
	}
	return *x
}

func clp20GuardFeed(t *testing.T, x *agentchat.Session, callID string, kind cp.EventKind, novelty cp.Novelty, native bool) {
	t.Helper()
	g, ok := x.ReadCollectionGuard()
	if !ok {
		t.Fatal("guard missing")
	}
	reduceLegacyCollectionEvent(x, g, callID, kind, novelty, native, true)
}

// A read-only objective with genuine new evidence must never be mistaken for
// a coding stall: no duplicate streak, no stall candidate, no Patch grant.
func TestAgentCLP20ReadOnlyProgressIsNotCodingStall(t *testing.T) {
	x := clp20GuardSession(t, "Audit the source read-only")
	initial, _ := x.ReadCollectionGuard()
	if initial.Counters.Objective.Intent != cp.ReadOnly {
		t.Fatalf("read-only goal not classified: %+v", initial.Counters.Objective)
	}
	x.AutoContinueText = true
	for i := 0; i < 20; i++ {
		clp20GuardFeed(t, &x, "ro-read", cp.CollectionObserved, cp.VerifiedNew, true)
	}
	g, _ := x.ReadCollectionGuard()
	if g.Counters.NativeReads != 20 || g.Counters.NewEvidence != 20 ||
		g.Counters.DuplicateEvidence != 0 || g.Counters.Progress.DuplicateStreak != 0 {
		t.Fatalf("read-only progress miscounted: %+v", g.Counters)
	}
	if x.AutoPatch {
		t.Fatal("read-only goal granted Patch")
	}
	if legacyCollectionStallCandidate(x, "ro-read") {
		t.Fatal("verified new reads treated as coding stall")
	}
}

// Owner-attested and verified acceptance reset bounded progress state at the
// server gate, preserve independent read counters, and clear the stall signal
// without reserving extra recovery.
func TestAgentCLP20AcceptanceResetsStallWithoutRecovery(t *testing.T) {
	x := clp20GuardSession(t, "Fix the bug")
	x.AutoContinueText = true
	for i := 0; i < 8; i++ {
		clp20GuardFeed(t, &x, "stall", cp.CollectionObserved, cp.VerifiedDuplicate, true)
	}
	if !legacyCollectionStallCandidate(x, "stall") {
		t.Fatal("verified stall missed")
	}
	before, _ := x.ReadCollectionGuard()

	clp20GuardFeed(t, &x, "attest", cp.OwnerAttestedProgress, "", false)
	attested, _ := x.ReadCollectionGuard()
	if attested.Counters.Progress != (cp.State{}) || attested.Counters.AttestedProgressEvents != 1 ||
		attested.Counters.NativeReads != before.Counters.NativeReads {
		t.Fatalf("attested acceptance reset wrong: %+v", attested.Counters)
	}
	if legacyCollectionStallCandidate(x, "stall") {
		t.Fatal("stall survived attested acceptance")
	}

	clp20GuardFeed(t, &x, "verified", cp.VerifiedProgress, "", false)
	verified, _ := x.ReadCollectionGuard()
	if verified.Counters.Progress != (cp.State{}) || verified.Counters.VerifiedProgressEvents != 1 ||
		verified.Counters.NativeReads != before.Counters.NativeReads {
		t.Fatalf("verified acceptance reset wrong: %+v", verified.Counters)
	}
	if verified.Recovery.Attempts != 0 {
		t.Fatal("acceptance reserved recovery")
	}
}

// A pending continuation with no accepted plan step binds only a neutral
// fallback objective and never implies mutation authority.
func TestAgentCLP20PendingContinuationBindsNeutralFallbackWithoutPatch(t *testing.T) {
	x, err := agentchat.NewSession("local", "/workspace", "project", "provider", "model", agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	bindLegacyCollectionObjective(context.Background(), &x, "Continue")
	if x.AutoPatch {
		t.Fatal("continuation granted Patch")
	}
	if g, ok := x.ReadCollectionGuard(); ok {
		if g.Counters.Objective.Intent == cp.Implementation || g.Counters.Objective.Intent == cp.Mixed {
			t.Fatalf("fallback continuation implied mutation: %+v", g.Counters.Objective)
		}
	}
}
