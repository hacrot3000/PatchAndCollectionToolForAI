package collectionprogress

import "testing"

// Provider/model/attempt/call-ID signals are deliberately OUTSIDE
// ObjectiveScope. Re-binding the same scope (e.g. after a provider switch)
// must keep the exact key; a user Stop suspends without destroying the
// retained binding, and resuming the same objective stays Unchanged.
func TestObjectiveSurvivesProviderSwitchAndStopWithoutReset(t *testing.T) {
	scope := ObjectiveScope{
		Owner: "local", ProjectID: "project", OwnerWorkspace: "owner-root",
		ExecutionWorkspace: "worktree-a", ConversationID: "conversation",
		Goal: "Fix the bug", StepID: "step-1", Generation: 2,
	}
	original, err := BindObjective(scope)
	if err != nil || !original.Valid() {
		t.Fatalf("invalid baseline objective: %+v %v", original, err)
	}

	// Simulate a provider/model switch plus a fresh attempt/call ID: none of
	// these are ObjectiveScope fields, so the binding cannot change.
	rebound, err := BindObjective(scope)
	if err != nil || rebound != original {
		t.Fatalf("provider/model/attempt signal reset objective: %+v %v", rebound, err)
	}
	if got := CompareObjective(original, rebound, false); got != ObjectiveUnchanged {
		t.Fatalf("provider switch changed objective: %s", got)
	}

	// User Stop suspends; it must not reset or rebind the retained objective.
	if got := CompareObjective(original, rebound, true); got != ObjectiveSuspended {
		t.Fatalf("Stop did not suspend: %s", got)
	}
	if got := CompareObjective(original, rebound, false); got != ObjectiveUnchanged {
		t.Fatalf("Stop destroyed retained binding: %s", got)
	}
}

// A genuine step/worktree transition changes the objective; a subsequent
// provider switch at the same step and generation keeps that new objective.
func TestObjectiveStepAndWorktreeTransitionsArePinnedAcrossProviderSwitch(t *testing.T) {
	scope := ObjectiveScope{
		Owner: "local", ProjectID: "project", OwnerWorkspace: "owner-root",
		ExecutionWorkspace: "worktree-a", ConversationID: "conversation",
		Goal: "Fix the bug", StepID: "step-1", Generation: 1,
	}
	original, err := BindObjective(scope)
	if err != nil {
		t.Fatal(err)
	}

	stepped := scope
	stepped.StepID = "step-2"
	stepped.Generation++
	next, err := BindObjective(stepped)
	if err != nil {
		t.Fatal(err)
	}
	if got := CompareObjective(original, next, false); got != ObjectiveChanged {
		t.Fatalf("accepted step transition not isolated: %s", got)
	}

	worktree := stepped
	worktree.ExecutionWorkspace = "worktree-b"
	worktree.Generation++
	moved, err := BindObjective(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if got := CompareObjective(next, moved, false); got != ObjectiveChanged {
		t.Fatalf("worktree change not isolated: %s", got)
	}

	// Provider switch at the same scope keeps the newly accepted objective.
	same, err := BindObjective(worktree)
	if err != nil || same != moved {
		t.Fatalf("provider switch reset accepted objective: %+v %v", same, err)
	}
	if got := CompareObjective(moved, same, false); got != ObjectiveUnchanged {
		t.Fatalf("accepted objective lost on provider switch: %s", got)
	}
}
