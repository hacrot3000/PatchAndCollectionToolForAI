package collectionprogress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type EventKind string

const (
	CollectionRequested EventKind = "collection_requested"
	CollectionObserved  EventKind = "collection_observed"
	HTTPAttempt         EventKind = "http_attempt"
	PatchProposed       EventKind = "patch_proposed"
	PatchExecuted       EventKind = "patch_executed"
	PatchRejected       EventKind = "patch_rejected"
	PatchUncertain      EventKind = "patch_uncertain"
	VerifiedProgress    EventKind = "verified_progress"
)

// CounterEvent is derived only from a committed, scoped journal record.
// Sequence must be the stable, monotonic sequence of that record, not a new
// number assigned during retry or recovery. Identity is a SHA-256 of the
// logical record identity (call/receipt or attempt, as appropriate).
// NativeRead means an actual native read occurred; rejected preflight is
// an observed error but not a native read. Novelty is independent of progress.
type CounterEvent struct {
	Sequence   uint64    `json:"sequence"`
	Identity   string    `json:"identity"`
	Kind       EventKind `json:"kind"`
	Novelty    Novelty   `json:"novelty,omitempty"`
	NativeRead bool      `json:"native_read,omitempty"`
}

// CounterState is a bounded reducer state, not a second execution journal.
// It does not retain tool arguments, receipts, prompt or source bytes.
// The integrating store must persist it atomically with the event checkpoint.
type CounterState struct {
	Version                  int              `json:"version"`
	Objective                ObjectiveBinding `json:"objective"`
	LastSequence             uint64           `json:"last_sequence"`
	LastEventDigest          string           `json:"last_event_digest,omitempty"`
	Requested                uint64           `json:"requested"`
	Observed                 uint64           `json:"observed"`
	NativeReads              uint64           `json:"native_reads"`
	NewEvidence              uint64           `json:"verified_new"`
	DuplicateEvidence        uint64           `json:"verified_duplicate"`
	UnknownEvidence          uint64           `json:"unknown"`
	Errors                   uint64           `json:"errors"`
	HTTPAttempts             uint64           `json:"http_attempts"`
	PatchProposals           uint64           `json:"patch_proposals"`
	PatchExecutions          uint64           `json:"patch_executions"`
	PatchRejections          uint64           `json:"patch_rejections"`
	PatchUncertainOutcomes   uint64           `json:"patch_uncertain"`
	VerifiedProgressEvents   uint64           `json:"verified_progress_events"`
	ReadsSincePatchProposal  uint64           `json:"reads_since_patch_proposal"`
	ReadsSincePatchExecution uint64           `json:"reads_since_patch_execution"`
	Progress                 State            `json:"progress"`
}

func NewCounterState(objective ObjectiveBinding) (CounterState, error) {
	if !objective.Valid() {
		return CounterState{}, errors.New("valid objective binding required")
	}
	return CounterState{Version: Version, Objective: objective}, nil
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func validCounterEvent(e CounterEvent) bool {
	if e.Sequence == 0 || !validDigest(e.Identity) {
		return false
	}
	if e.Kind == CollectionObserved {
		switch e.Novelty {
		case VerifiedNew, VerifiedDuplicate:
			return e.NativeRead
		case UnknownNovelty, ObservationError:
			return true
		default:
			return false
		}
	}
	if e.Novelty != "" || e.NativeRead {
		return false
	}
	switch e.Kind {
	case CollectionRequested, HTTPAttempt, PatchProposed, PatchExecuted, PatchRejected, PatchUncertain, VerifiedProgress:
		return true
	default:
		return false
	}
}

// ApplyCounterEvent is a pure ordered reduction. Replaying the same durable
// sequence is a no-op; a conflicting event at the current sequence is
// refused. Older sequences cannot increment counts. This contract requires
// the journal adapter to preserve the original sequence for a logical event;
// it is not evidence that a caller allocating fresh sequences is idempotent.
// A PATCH proposal, execution, rejection or unknown/partial outcome alone
// never resets meaningful progress.
func ApplyCounterEvent(p Policy, current CounterState, e CounterEvent, patchProposalAvailable bool) (CounterState, Decision, bool, error) {
	decision := Decision{Kind: Continue}
	if current.Version != Version || !current.Objective.Valid() ||
		(current.LastSequence != 0 && !validDigest(current.LastEventDigest)) ||
		(current.LastSequence == 0 && current.LastEventDigest != "") ||
		!validCounterEvent(e) {
		return current, decision, false, errors.New("invalid collection counter state or event")
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return current, decision, false, err
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	if e.Sequence < current.LastSequence {
		return current, decision, false, nil
	}
	if e.Sequence == current.LastSequence {
		if digest != current.LastEventDigest {
			return current, decision, false, errors.New("conflicting collection counter sequence")
		}
		return current, decision, false, nil
	}
	next := current
	next.LastSequence, next.LastEventDigest = e.Sequence, digest
	switch e.Kind {
	case CollectionRequested:
		next.Requested = increment(next.Requested)
	case CollectionObserved:
		next.Observed = increment(next.Observed)
		switch e.Novelty {
		case VerifiedNew:
			next.NewEvidence = increment(next.NewEvidence)
		case VerifiedDuplicate:
			next.DuplicateEvidence = increment(next.DuplicateEvidence)
		case UnknownNovelty:
			next.UnknownEvidence = increment(next.UnknownEvidence)
		case ObservationError:
			next.Errors = increment(next.Errors)
		}
		if e.NativeRead {
			next.NativeReads = increment(next.NativeReads)
			next.ReadsSincePatchProposal = increment(next.ReadsSincePatchProposal)
			next.ReadsSincePatchExecution = increment(next.ReadsSincePatchExecution)
			next.Progress, decision = Apply(p, current.Objective.AdvisoryIntent(patchProposalAvailable), next.Progress, Observation{Collected: true, Novelty: e.Novelty})
		} else {
			// A rejected or uncertain non-read breaks a verified-duplicate
			// streak, but cannot advance the actual-read counters.
			next.Progress.DuplicateStreak = 0
		}
	case HTTPAttempt:
		next.HTTPAttempts = increment(next.HTTPAttempts)
	case PatchProposed:
		next.PatchProposals = increment(next.PatchProposals)
		next.ReadsSincePatchProposal = 0
	case PatchExecuted:
		next.PatchExecutions = increment(next.PatchExecutions)
		next.ReadsSincePatchExecution = 0
	case PatchRejected:
		next.PatchRejections = increment(next.PatchRejections)
	case PatchUncertain:
		next.PatchUncertainOutcomes = increment(next.PatchUncertainOutcomes)
	case VerifiedProgress:
		next.VerifiedProgressEvents = increment(next.VerifiedProgressEvents)
		next.Progress, decision = Apply(p, current.Objective.AdvisoryIntent(patchProposalAvailable), next.Progress, Observation{MeaningfulProgress: true})
	}
	return next, decision, true, nil
}
