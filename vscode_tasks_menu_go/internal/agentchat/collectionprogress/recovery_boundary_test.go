package collectionprogress

import "testing"

func recoveryApplyEvent(t *testing.T, s CounterState, e CounterEvent) CounterState {
	t.Helper()
	next, _, applied, err := ApplyCounterEvent(Defaults(), s, e, true)
	if err != nil || !applied {
		t.Fatalf("event not applied: %v", err)
	}
	return next
}

// recoveryStallState feeds verified duplicate observations until the
// advisory/cooldown threshold is met, so a single recovery may be reserved.
func recoveryStallState(t *testing.T) (CounterState, CounterEvent) {
	t.Helper()
	s := counterFixture(t)
	var e CounterEvent
	for i := uint64(1); i <= 8; i++ {
		e = counterEvent(i, CollectionObserved, VerifiedDuplicate, true)
		s = recoveryApplyEvent(t, s, e)
	}
	return s, e
}

func fullRecoveryGate() RecoveryGate {
	return RecoveryGate{SafeEgress: true, CanonicalOutputCommitted: true, OwnerAllowsContinuation: true}
}

// Once one replan is reserved for a generation, re-evaluating the same
// committed event must never reserve another continuation (no storm), and
// following duplicate reads can only Continue or checkpoint at exhaustion.
func TestRecoveryNoContinuationStormAtReservedStall(t *testing.T) {
	s, e := recoveryStallState(t)
	gate := fullRecoveryGate()
	recovery, d := AssessRecovery(Defaults(), s, e, RecoveryState{}, gate)
	if d.Kind != Replan || d.Reason != StalledVerifiedEvidence || d.NextAction != NarrowMissingScope ||
		recovery.Attempts != 1 || recovery.LastSequence != e.Sequence {
		t.Fatalf("first replan not reserved exactly once: %+v %+v", recovery, d)
	}
	for i := 0; i < 5; i++ {
		again, d2 := AssessRecovery(Defaults(), s, e, recovery, gate)
		if d2.Kind != Checkpoint || d2.Reason != RecoveryAlreadyRecorded || again != recovery {
			t.Fatalf("replayed stall reserved a second continuation: %+v %+v", again, d2)
		}
	}
	replayed, _, applied, err := ApplyCounterEvent(Defaults(), s, e, true)
	if err != nil || applied || replayed != s {
		t.Fatalf("duplicate committed observation re-counted: %+v %v", replayed, err)
	}
	for i := uint64(9); i <= 13; i++ {
		next := counterEvent(i, CollectionObserved, VerifiedDuplicate, true)
		s = recoveryApplyEvent(t, s, next)
		var d3 RecoveryDecision
		recovery, d3 = AssessRecovery(Defaults(), s, next, recovery, gate)
		if d3.Kind == Replan || recovery.Attempts != 1 {
			t.Fatalf("second replan stormed at read %d: %+v %+v", i, recovery, d3)
		}
	}
}

// Genuine new evidence resumes immediately and preserves the already-used
// recovery allowance; it never refills or resets the generation budget.
func TestRecoveryNoveltyResumesWithBudgetPreserved(t *testing.T) {
	s, e := recoveryStallState(t)
	gate := fullRecoveryGate()
	recovery, d := AssessRecovery(Defaults(), s, e, RecoveryState{}, gate)
	if d.Kind != Replan || recovery.Attempts != 1 {
		t.Fatalf("setup replan failed: %+v %+v", recovery, d)
	}
	novel := counterEvent(9, CollectionObserved, VerifiedNew, true)
	s = recoveryApplyEvent(t, s, novel)
	after, d2 := AssessRecovery(Defaults(), s, novel, recovery, gate)
	if d2.Kind != Continue || after != recovery {
		t.Fatalf("novel evidence blocked or budget reset: %+v %+v", after, d2)
	}
}

// Stop, unresolved egress, an uncommitted pending call and missing live
// consent each checkpoint without reserving a recovery or sending egress.
func TestRecoveryPendingPairGatesBlockEgress(t *testing.T) {
	s, e := recoveryStallState(t)
	for _, tc := range []struct {
		gate RecoveryGate
		kind DecisionKind
		reason Reason
		next NextAction
	}{
		{RecoveryGate{Stopped: true}, Checkpoint, RecoveryStopped, AwaitOwner},
		{RecoveryGate{SafeEgress: false, CanonicalOutputCommitted: true, OwnerAllowsContinuation: true}, Checkpoint, RecoveryUnknownEgress, ReviewEgress},
		{RecoveryGate{SafeEgress: true, CanonicalOutputCommitted: false, OwnerAllowsContinuation: true}, Checkpoint, RecoveryPendingCall, ReconcileCall},
		{RecoveryGate{SafeEgress: true, CanonicalOutputCommitted: true, OwnerAllowsContinuation: false}, Checkpoint, RecoveryNotAuthorized, AwaitOwner},
	} {
		old := RecoveryState{}
		got, d := AssessRecovery(Defaults(), s, e, old, tc.gate)
		if got != old || d.Kind != tc.kind || d.Reason != tc.reason || d.NextAction != tc.next {
			t.Fatalf("unsafe gate admitted recovery: %+v %+v", got, d)
		}
	}
}

// Corrupt or future recovery state fails closed to a checkpoint instead of
// reserving extra continuations.
func TestRecoveryInvalidStateFailClosed(t *testing.T) {
	s, e := recoveryStallState(t)
	gate := fullRecoveryGate()
	for _, old := range []RecoveryState{
		{Attempts: MaxRecoveryAttempts + 1, LastSequence: e.Sequence, LastAttemptReads: s.NativeReads},
		{Attempts: 1, LastSequence: e.Sequence + 1, LastAttemptReads: s.NativeReads},
		{Attempts: 0, LastSequence: e.Sequence, LastAttemptReads: 0},
	} {
		got, d := AssessRecovery(Defaults(), s, e, old, gate)
		if got != old || d.Kind != Checkpoint || d.Reason != RecoveryInvalidState || d.NextAction != ReviewCheckpoint {
			t.Fatalf("invalid recovery state not fenced: %+v %+v", got, d)
		}
	}
}
