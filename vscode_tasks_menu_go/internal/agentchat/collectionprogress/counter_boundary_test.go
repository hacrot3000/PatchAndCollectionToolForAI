package collectionprogress

import (
	"strings"
	"testing"
)

// TestCounterCountersSaturateWithoutWrappingAtBoundaries verifies that the
// bounded reducer never wraps a saturated counter, and that replay/conflict
// handling remains exact at a numeric boundary. It uses only the exported
// reducer contract and the existing in-package fixtures.
func TestCounterCountersSaturateWithoutWrappingAtBoundaries(t *testing.T) {
	s := counterFixture(t)
	max := ^uint64(0)
	s.LastSequence = 3
	s.LastEventDigest = strings.Repeat("a", 64)
	s.Requested = max
	s.Observed = max
	s.NativeReads = max
	s.NewEvidence = max
	s.HTTPAttempts = max
	s.Progress.ReadsSinceProgress = max
	s.Progress.DuplicateStreak = 7

	e := counterEvent(4, CollectionObserved, VerifiedNew, true)
	next, _, applied, err := ApplyCounterEvent(Defaults(), s, e, true)
	if err != nil || !applied {
		t.Fatalf("boundary event not applied: %v", err)
	}
	if next.Requested != max || next.Observed != max || next.NewEvidence != max ||
		next.NativeReads != max || next.HTTPAttempts != max ||
		next.Progress.ReadsSinceProgress != max {
		t.Fatalf("counter wrapped at boundary: %+v", next)
	}

	replayed, _, applied, err := ApplyCounterEvent(Defaults(), next, e, true)
	if err != nil || applied || replayed != next {
		t.Fatalf("same durable boundary event double counted: %+v %v", replayed, err)
	}
	conflict := CounterEvent{
		Sequence:   4,
		Identity:   strings.Repeat("b", 64),
		Kind:       CollectionObserved,
		Novelty:    VerifiedNew,
		NativeRead: true,
	}
	if unchanged, _, applied, err := ApplyCounterEvent(Defaults(), next, conflict, true); err == nil || applied || unchanged != next {
		t.Fatalf("conflicting boundary event accepted: %+v %v", unchanged, err)
	}
	older := counterEvent(3, CollectionObserved, VerifiedDuplicate, true)
	if unchanged, _, applied, err := ApplyCounterEvent(Defaults(), next, older, true); err != nil || applied || unchanged != next {
		t.Fatalf("older boundary replay counted again: %+v %v", unchanged, err)
	}
}

// TestCounterMaxSequenceReplayAndProgressResetAtBoundary checks that a
// maximum-sequence durable event is idempotent on replay and that an owner
// attestation still resets bounded progress state at the numeric boundary.
func TestCounterMaxSequenceReplayAndProgressResetAtBoundary(t *testing.T) {
	max := ^uint64(0)

	s := counterFixture(t)
	s.LastSequence = max - 1
	s.LastEventDigest = strings.Repeat("a", 64)
	atMax := counterEvent(max, HTTPAttempt, "", false)
	first, _, applied, err := ApplyCounterEvent(Defaults(), s, atMax, true)
	if err != nil || !applied || first.LastSequence != max {
		t.Fatalf("max sequence event not applied: %+v %v", first, err)
	}
	replay, _, applied, err := ApplyCounterEvent(Defaults(), first, atMax, true)
	if err != nil || applied || replay != first {
		t.Fatalf("max sequence replay not idempotent: %+v %v", replay, err)
	}

	boundary := counterFixture(t)
	boundary.LastSequence = 3
	boundary.LastEventDigest = strings.Repeat("a", 64)
	boundary.Progress.ReadsSinceProgress = max
	boundary.Progress.DuplicateStreak = 7
	boundary.Progress.FirstAdvised = true
	boundary.Progress.SecondAdvised = true
	boundary.Progress.DuplicateAdvised = true
	attest := counterEvent(4, OwnerAttestedProgress, "", false)
	after, decision, applied, err := ApplyCounterEvent(Defaults(), boundary, attest, true)
	if err != nil || !applied {
		t.Fatalf("owner attestation at boundary not applied: %v", err)
	}
	if after.Progress != (State{}) || after.AttestedProgressEvents != 1 || after.NativeReads != 0 {
		t.Fatalf("boundary progress reset failed: %+v", after)
	}
	if decision.Kind != Continue {
		t.Fatalf("attestation must remain advisory-neutral: %+v", decision)
	}
}
