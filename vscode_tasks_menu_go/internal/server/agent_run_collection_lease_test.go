package server

import "testing"

// CLP-04.02.04 (concurrent runs / worktree / lease / native execution count):
// resolveRunCollection acquires lockAgentTurn before any native read
// (agent_run_collection.go:92-96). This test exercises that admission fence:
// exactly ONE native-collection lease is admitted per session at a time, a
// second concurrent contender for the same session is refused (so the native
// Collection executes at most once concurrently), a distinct worktree/session
// is not starved, and releasing the lease re-enables acquisition (no permanent
// budget exhaustion across retries/restarts).
//
// Scope note: this is the deterministic in-process lease/admission boundary.
// It complements (does not replace) the sequential four-read native-execution
// count and cross-worktree progress-isolation assertions already in
// TestAgentRunCollectionContinuesExactProviderWithNativeObservation's matrix
// (agent_run_collection_test.go). Full multi-run-manager concurrency is an
// operator verification item.
func TestCLP040204NativeCollectionLeaseContentionAndWorktreeIsolation(t *testing.T) {
	s1, id1 := newAgentConversationFixture(t, "stateless")
	s2, id2 := newAgentConversationFixture(t, "stateless")
	if id1 == id2 || s1.Workspace == "" || s2.Workspace == "" {
		t.Fatal("fixtures did not produce distinct sessions/workspaces")
	}

	// First contender owns the exact session lease (native Collection may run).
	release1, ok := lockAgentTurn(s1.Workspace, "local", id1)
	if !ok || release1 == nil {
		t.Fatal("first native-collection lease not admitted")
	}

	// A second concurrent contender for the SAME session cannot also execute:
	// this is the exactly-once native execution admission fence.
	if release2, ok2 := lockAgentTurn(s1.Workspace, "local", id1); ok2 {
		if release2 != nil {
			release2()
		}
		t.Fatal("second concurrent native collection for the same session was admitted")
	}

	// A distinct worktree/session is independent and must not be starved by a
	// contended lease held elsewhere.
	release2, ok := lockAgentTurn(s2.Workspace, "local", id2)
	if !ok || release2 == nil {
		t.Fatal("distinct worktree/session was starved by an unrelated lease")
	}
	release2()

	// Release the first lease and confirm the session can be re-acquired: the
	// lease is not a one-shot budget and does not leak across a released turn.
	release1()
	again, ok := lockAgentTurn(s1.Workspace, "local", id1)
	if !ok || again == nil {
		t.Fatal("session lease could not be re-acquired after release")
	}
	again()
}
