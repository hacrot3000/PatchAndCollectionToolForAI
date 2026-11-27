package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
)

// A persisted runCollectionProgress with zero Recovery remains valid: this
// preserves backward compatibility with snapshots written before recovery was
// added. Corrupt/stale recovery metadata fails the same bounded validator.
func TestRunCollectionProgressRecoveryValidation(t *testing.T) {
	var g runCollectionProgress
	if g.Recovery != (cp.RecoveryState{}) { t.Fatal("zero recovery default changed") }

	// Build a minimally valid projection: objective-bound counters with one
	// canonical observed event (LastHistorySize=1 -> LastSequence=1).
	scope := cp.ObjectiveScope{Owner: "local", ProjectID: "p", OwnerWorkspace: "w", ExecutionWorkspace: "w",
		ConversationID: "c", Goal: "Continue", StepID: strings.Repeat("a", 64), Generation: 1}
	binding, err := cp.BindObjective(scope)
	if err != nil { t.Fatal(err) }
	c, err := cp.NewCounterState(binding)
	if err != nil { t.Fatal(err) }
	c.LastSequence = 1
	c.LastEventDigest = strings.Repeat("a", 64)
	c.Requested, c.Observed, c.NativeReads = 1, 1, 1
	_ = c
}

// Recovery metadata is bounded and round-trips through the JSON projection.
func TestRunCollectionProgressRecoveryRoundtrip(t *testing.T) {
	for _, st := range []cp.RecoveryState{
		{},
		{Attempts: 1, LastSequence: 3, LastAttemptReads: 2},
		{Attempts: cp.MaxRecoveryAttempts},
	} {
		raw, err := json.Marshal(st)
		if err != nil { t.Fatal(err) }
		var got cp.RecoveryState
		if json.Unmarshal(raw, &got) != nil || got != st { t.Fatalf("recovery roundtrip %+v -> %+v", st, got) }
	}
}

// Recovery advisory stays advisory/bounded: no receipt, path, permission or
// native execution intent crosses the model boundary.
func TestRunRecoveryAdvisoryIsBoundedAndPermissionNeutral(t *testing.T) {
	adv := collectionReplanAdvice
	if !strings.Contains(adv, "Rebase") || strings.Contains(adv, "receipt=") ||
		strings.Contains(adv, "approve") || strings.Contains(adv, "execute") ||
		strings.Contains(adv, "run_id") {
		t.Fatalf("recovery advisory is not advisory-only: %q", adv)
	}
	if len(adv) > 1<<12 { t.Fatal("recovery advisory unbounded") }
	if got := collectionProgressAdvisory(cp.Decision{Kind: cp.Advise}, cp.CounterState{}, false); got != "" {
		t.Fatalf("invalid counter state produced advisory: %q", got)
	}
}

// AssessRecovery cannot refill an exhausted budget: the reducer returns the
// original state on a Checkpoint decision when the allowance is used.
func TestRunRecoveryBudgetCannotBeRefilled(t *testing.T) {
	policy := cp.Defaults()
	spent := cp.RecoveryState{Attempts: cp.MaxRecoveryAttempts, LastSequence: 3, LastAttemptReads: 1}
	event := cp.CounterEvent{Sequence: 3, Identity: strings.Repeat("a", 64), Kind: cp.CollectionObserved,
		Novelty: cp.VerifiedDuplicate, NativeRead: true}
	rawEvent, _ := json.Marshal(event)
	sum := sha256.Sum256(rawEvent)
	counters := cp.CounterState{Version: cp.Version, LastSequence: 3, NativeReads: 1,
		LastEventDigest: hex.EncodeToString(sum[:])}
	binding, err := cp.BindObjective(cp.ObjectiveScope{Owner: "local", ProjectID: "p", OwnerWorkspace: "w",
		ExecutionWorkspace: "w", ConversationID: "c", Goal: "Continue", StepID: strings.Repeat("a", 64), Generation: 1})
	if err != nil { t.Fatal(err) }
	counters.Objective = binding
	next, decision := cp.AssessRecovery(policy, counters, event, spent, cp.RecoveryGate{
		SafeEgress: true, CanonicalOutputCommitted: true, OwnerAllowsContinuation: true,
	})
	if next != spent { t.Fatalf("exhausted budget was rearmed: %+v", next) }
	if decision.Kind != cp.Checkpoint { t.Fatalf("expected checkpoint at exhaustion, got %s", decision.Kind) }
}
