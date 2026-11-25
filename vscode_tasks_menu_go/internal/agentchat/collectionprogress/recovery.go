package collectionprogress

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
)

// Recovery is a separate, pure post-observation decision. The adapter must
// persist it before any continuation, use the existing approval/egress
// admission checks, and never fabricate an output or execute a tool here.
const MaxRecoveryAttempts = 1

const (
	StalledVerifiedEvidence Reason = "stalled_verified_evidence"
	RecoveryBudgetExhausted Reason = "recovery_budget_exhausted"
	RecoveryNotAuthorized   Reason = "recovery_not_authorized"
	RecoveryAlreadyRecorded Reason = "recovery_already_recorded"
	RecoveryStopped         Reason = "user_stopped"
	RecoveryUnknownEgress   Reason = "provider_egress_unresolved"
	RecoveryPendingCall     Reason = "canonical_call_output_pending"
	RecoveryInvalidState    Reason = "invalid_recovery_state"
)

type NextAction string

const (
	NoNextAction       NextAction = ""
	NarrowMissingScope NextAction = "narrow_missing_scope"
	ReviewCheckpoint   NextAction = "review_checkpoint"
	ReviewEgress       NextAction = "review_provider_egress"
	ReconcileCall      NextAction = "reconcile_pending_call"
	AwaitOwner         NextAction = "await_owner"
)

// RecoveryState is scoped to one objective generation. A fresh observation,
// new provider/model, new call ID or progress does not refill its allowance.
// It is not a receipt or a new journal.
type RecoveryState struct {
	Attempts         uint64 `json:"attempts"`
	LastSequence     uint64 `json:"last_sequence,omitempty"`
	LastAttemptReads uint64 `json:"last_attempt_reads,omitempty"`
}

// RecoveryGate consists of server-derived facts. The model cannot supply
// these flags. CanonicalOutputCommitted means the exact call/output was saved;
// SafeEgress means the existing provider journal permits a NEW continuation.
type RecoveryGate struct {
	Stopped                  bool
	SafeEgress               bool
	CanonicalOutputCommitted bool
	OwnerAllowsContinuation  bool
}

type RecoveryDecision struct {
	Kind       DecisionKind
	Reason     Reason
	NextAction NextAction
}

// AssessRecovery only reacts to a newly committed verified-duplicate native
// observation after an earlier advisory and cooldown. Unknown/error evidence
// cannot justify a stall. No hard round limit is imposed on legitimate reads.
func AssessRecovery(p Policy, counters CounterState, event CounterEvent, old RecoveryState, gate RecoveryGate) (RecoveryState, RecoveryDecision) {
	proceed := RecoveryDecision{Kind: Continue}
	hold := func(reason Reason, next NextAction) (RecoveryState, RecoveryDecision) {
		return old, RecoveryDecision{Kind: Checkpoint, Reason: reason, NextAction: next}
	}
	if gate.Stopped {
		return hold(RecoveryStopped, AwaitOwner)
	}
	if !gate.SafeEgress {
		return hold(RecoveryUnknownEgress, ReviewEgress)
	}
	if !gate.CanonicalOutputCommitted {
		return hold(RecoveryPendingCall, ReconcileCall)
	}
	if counters.Version != Version || !counters.Objective.Valid() ||
		old.Attempts > MaxRecoveryAttempts || old.LastSequence > counters.LastSequence ||
		old.LastAttemptReads > counters.NativeReads || (old.Attempts == 0 && (old.LastSequence != 0 || old.LastAttemptReads != 0)) ||
		(old.Attempts != 0 && (old.LastSequence == 0 || old.LastAttemptReads == 0)) {
		return hold(RecoveryInvalidState, ReviewCheckpoint)
	}
	if !validCounterEvent(event) || event.Sequence != counters.LastSequence ||
		event.Kind != CollectionObserved || event.Novelty != VerifiedDuplicate || !event.NativeRead {
		return old, proceed
	}
	raw,err:=json.Marshal(event)
	if err!=nil{return hold(RecoveryInvalidState,ReviewCheckpoint)}
	sum:=sha256.Sum256(raw)
	if hex.EncodeToString(sum[:])!=counters.LastEventDigest{return hold(RecoveryInvalidState,ReviewCheckpoint)}
	if !p.Valid() {
		p = Defaults()
	}
	if counters.Progress.DuplicateStreak < p.DuplicateStreak+p.DuplicateCooldown || !counters.Progress.DuplicateAdvised {
		return old, proceed
	}
	if old.Attempts != 0 && event.Sequence == old.LastSequence {
		// A durable decision already exists for this checkpoint. Recovery
		// must inspect that journal, not reserve another continuation.
		return hold(RecoveryAlreadyRecorded, ReviewCheckpoint)
	}
	if old.Attempts != 0 && counters.NativeReads-old.LastAttemptReads < p.DuplicateCooldown {
		return old, proceed
	}
	if old.Attempts >= MaxRecoveryAttempts {
		return hold(RecoveryBudgetExhausted, ReviewCheckpoint)
	}
	if !gate.OwnerAllowsContinuation {
		return hold(RecoveryNotAuthorized, AwaitOwner)
	}
	next := old
	next.Attempts++
	next.LastSequence = event.Sequence
	next.LastAttemptReads = counters.NativeReads
	return next, RecoveryDecision{Kind: Replan, Reason: StalledVerifiedEvidence, NextAction: NarrowMissingScope}
}
