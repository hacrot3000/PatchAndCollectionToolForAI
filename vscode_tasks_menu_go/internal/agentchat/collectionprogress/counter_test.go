package collectionprogress

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func counterFixture(t *testing.T) CounterState {
	t.Helper()
	b, err := BindObjective(ObjectiveScope{Owner: "local", ProjectID: "p", OwnerWorkspace: "w", ExecutionWorkspace: "w", ConversationID: "c", Goal: "Fix the bug", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewCounterState(b)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func counterEvent(sequence uint64, kind EventKind, novelty Novelty, native bool) CounterEvent {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d/%s", sequence, kind)))
	return CounterEvent{Sequence: sequence, Identity: hex.EncodeToString(sum[:]), Kind: kind, Novelty: novelty, NativeRead: native}
}

func TestCounterEventsSeparateNativeReadsFromHTTPAndPatchProgress(t *testing.T) {
	s := counterFixture(t)
	sequence := uint64(0)
	apply := func(kind EventKind, novelty Novelty, native bool) Decision {
		t.Helper()
		sequence++
		e := counterEvent(sequence, kind, novelty, native)
		next, d, applied, err := ApplyCounterEvent(Defaults(), s, e, true)
		if err != nil || !applied {
			t.Fatalf("event %s: %v", kind, err)
		}
		replayed, _, applied, err := ApplyCounterEvent(Defaults(), next, e, true)
		if err != nil || applied || replayed != next {
			t.Fatal("same durable event double counted")
		}
		s = next
		return d
	}
	for i := 0; i < 5; i++ {
		apply(CollectionRequested, "", false)
		d := apply(CollectionObserved, VerifiedNew, true)
		if i == 4 && d.Reason != AssessReadiness {
			t.Fatalf("missing threshold advice: %+v", d)
		}
	}
	apply(HTTPAttempt, "", false)
	apply(HTTPAttempt, "", false)
	apply(PatchProposed, "", false)
	apply(PatchRejected, "", false)
	apply(PatchExecuted, "", false) // no-op or failed execution is not progress
	apply(PatchUncertain, "", false)
	apply(CollectionRequested, "", false)
	apply(CollectionObserved, ObservationError, false) // schema rejection
	if s.Requested != 6 || s.Observed != 6 || s.NativeReads != 5 || s.NewEvidence != 5 || s.Errors != 1 || s.HTTPAttempts != 2 ||
		s.PatchProposals != 1 || s.PatchExecutions != 1 || s.PatchRejections != 1 || s.PatchUncertainOutcomes != 1 ||
		s.Progress.ReadsSinceProgress != 5 || s.ReadsSincePatchProposal != 0 || s.ReadsSincePatchExecution != 0 {
		t.Fatalf("wrong independent counters: %+v", s)
	}
	apply(CollectionRequested, "", false)
	apply(CollectionObserved, UnknownNovelty, true)
	if s.UnknownEvidence != 1 || s.Progress.ReadsSinceProgress != 6 || s.ReadsSincePatchExecution != 1 {
		t.Fatalf("unknown read counters wrong: %+v", s)
	}
	apply(VerifiedProgress, "", false)
	if s.Progress != (State{}) || s.VerifiedProgressEvents != 1 || s.NativeReads != 6 || s.ReadsSincePatchExecution != 1 {
		t.Fatalf("verified progress discarded other counters: %+v", s)
	}
}

func TestCounterCursorRejectsConflictAndDoesNotCountOldReplay(t *testing.T) {
	s := counterFixture(t)
	first := counterEvent(3, CollectionObserved, VerifiedDuplicate, true)
	next, _, applied, err := ApplyCounterEvent(Defaults(), s, first, true)
	if err != nil || !applied {
		t.Fatal(err)
	}
	conflict := first
	conflict.Novelty = VerifiedNew
	if unchanged, _, applied, err := ApplyCounterEvent(Defaults(), next, conflict, true); err == nil || applied || unchanged != next {
		t.Fatal("conflicting durable sequence accepted")
	}
	second := counterEvent(5, HTTPAttempt, "", false)
	next, _, applied, err = ApplyCounterEvent(Defaults(), next, second, true)
	if err != nil || !applied {
		t.Fatal(err)
	}
	if unchanged, _, applied, err := ApplyCounterEvent(Defaults(), next, first, true); err != nil || applied || unchanged != next {
		t.Fatal("older journal event double counted")
	}
	if _, _, applied, err := ApplyCounterEvent(Defaults(), next, counterEvent(6, CollectionObserved, VerifiedDuplicate, false), true); err == nil || applied {
		t.Fatal("unexecuted read treated as verified duplicate")
	}
}
