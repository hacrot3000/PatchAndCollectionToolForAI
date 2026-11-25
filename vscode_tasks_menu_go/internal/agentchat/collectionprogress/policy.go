// Package collectionprogress evaluates collection progress without executing tools,
// granting permission, or interpreting provider completion as task completion.
package collectionprogress

// Version identifies the policy semantics. Integrators must pin the policy to
// an objective generation and persist state with their authoritative journal.
const Version = 1

// Configuration bounds apply to advisory thresholds only, never the number
// of allowed native reads. They also keep threshold arithmetic bounded.
const MaxAdvisoryThreshold = 10000

type Intent string

const (
	Implementation Intent = "implementation"
	ReadOnly       Intent = "read_only"
	Mixed          Intent = "mixed"
	UnknownIntent  Intent = "unknown"
)

type Novelty string

const (
	VerifiedNew       Novelty = "verified_new"
	VerifiedDuplicate Novelty = "verified_duplicate"
	UnknownNovelty    Novelty = "unknown"
	ObservationError  Novelty = "error"
)

type DecisionKind string

const (
	Continue DecisionKind = "continue"
	Advise     DecisionKind = "advise"
	Replan     DecisionKind = "replan"
	Checkpoint DecisionKind = "checkpoint"
)

type Reason string

const (
	NoAdvisory       Reason = ""
	AssessReadiness  Reason = "assess_readiness"
	BoundNextScope   Reason = "bound_next_scope"
	RepeatedEvidence Reason = "repeated_verified_evidence"
)

type Policy struct {
	FirstReadAdvisory  uint64
	SecondReadAdvisory uint64
	DuplicateStreak    uint64
	DuplicateCooldown  uint64
}

func Defaults() Policy {
	return Policy{FirstReadAdvisory: 5, SecondReadAdvisory: 10, DuplicateStreak: 3, DuplicateCooldown: 5}
}

// Valid rejects invalid policy configurations. Thresholds are advisories, not
// read quotas. Zero values must be replaced with Defaults by the caller.
func (p Policy) Valid() bool {
	return p.FirstReadAdvisory > 0 && p.SecondReadAdvisory > p.FirstReadAdvisory &&
		p.SecondReadAdvisory <= MaxAdvisoryThreshold &&
		p.DuplicateStreak > 0 && p.DuplicateStreak <= MaxAdvisoryThreshold &&
		p.DuplicateCooldown > 0 && p.DuplicateCooldown <= MaxAdvisoryThreshold
}

// State contains only bounded metadata. The caller owns objective scoping,
// idempotency and durability; Apply must receive each logical observation once.
type State struct {
	ReadsSinceProgress      uint64
	DuplicateStreak         uint64
	FirstAdvised            bool
	SecondAdvised           bool
	DuplicateAdvised        bool
	LastDuplicateAdvisoryAt uint64
}

// Observation must be constructed from trusted receipt/acceptance metadata.
// New evidence and meaningful objective progress are independent properties.
// A proposal, wrapper PASS or provider completion alone is neither proof.
type Observation struct {
	Collected          bool
	Novelty            Novelty
	MeaningfulProgress bool
}

type Decision struct {
	Kind   DecisionKind
	Reason Reason
}

func increment(n uint64) uint64 {
	if n == ^uint64(0) {
		return n
	}
	return n + 1
}

// Apply is pure and advisory-only. It never denies a valid read or authorizes
// a PATCH. UNKNOWN and errors break a consecutive verified-duplicate streak.
func Apply(p Policy, intent Intent, state State, observation Observation) (State, Decision) {
	if !p.Valid() {
		p = Defaults()
	}
	if observation.MeaningfulProgress {
		state = State{}
	}
	if !observation.Collected {
		return state, Decision{Kind: Continue}
	}
	state.ReadsSinceProgress = increment(state.ReadsSinceProgress)
	switch observation.Novelty {
	case VerifiedDuplicate:
		state.DuplicateStreak = increment(state.DuplicateStreak)
	default:
		state.DuplicateStreak = 0
	}
	if state.DuplicateStreak >= p.DuplicateStreak && (!state.DuplicateAdvised || (state.ReadsSinceProgress >= state.LastDuplicateAdvisoryAt && state.ReadsSinceProgress-state.LastDuplicateAdvisoryAt >= p.DuplicateCooldown)) {
		state.DuplicateAdvised = true
		state.LastDuplicateAdvisoryAt = state.ReadsSinceProgress
		return state, Decision{Kind: Advise, Reason: RepeatedEvidence}
	}
	if intent == Implementation || intent == Mixed {
		if !state.SecondAdvised && state.ReadsSinceProgress >= p.SecondReadAdvisory {
			state.FirstAdvised, state.SecondAdvised = true, true
			return state, Decision{Kind: Advise, Reason: BoundNextScope}
		}
		if !state.FirstAdvised && state.ReadsSinceProgress >= p.FirstReadAdvisory {
			state.FirstAdvised = true
			return state, Decision{Kind: Advise, Reason: AssessReadiness}
		}
	}
	return state, Decision{Kind: Continue}
}
