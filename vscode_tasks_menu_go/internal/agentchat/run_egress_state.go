package agentchat

// RunEgressState is a bounded, fail-closed classification of one run's durable
// provider egress marker. It is diagnostic metadata only: it never authorizes a
// replay, never contacts a provider, never accepts native tool output and never
// claims a remote response was acknowledged. It is the unified state vocabulary
// required before run and legacy egress can share one state-machine policy.
type RunEgressState string

const (
	RunEgressNone      RunEgressState = "none"
	RunEgressPrepared  RunEgressState = "prepared"
	RunEgressSending   RunEgressState = "sending"
	RunEgressUnknown   RunEgressState = "unknown"
	RunEgressCompleted RunEgressState = "completed"
	RunEgressBlocked   RunEgressState = "blocked"
)

// RunEgressClassification pairs the state with the authoritative ambiguity
// review already owned by ProviderEgressReview. Reconciliation and replay flags
// are derived from that review, never from model text or a UI value.
type RunEgressClassification struct {
	State                  RunEgressState
	RequiresReconciliation bool
	AutomaticReplayAllowed bool
	Guidance               string
}

// ClassifyRunEgress maps a session's durable ProviderRun marker onto the unified
// run egress state machine. It delegates SENDING/UNKNOWN ambiguity to
// ProviderEgressReviewForSession so an unacknowledged request is never silently
// treated as terminal. It is pure: no files, no network, no persistence, no plan
// segments and no provider lookup. An unrecognized phase fail-closes to UNKNOWN.
func ClassifyRunEgress(x Session) RunEgressClassification {
	if x.ProviderRun == nil {
		return RunEgressClassification{State: RunEgressNone}
	}
	var state RunEgressState
	switch x.ProviderRun.Phase {
	case "prepared":
		state = RunEgressPrepared
	case "sending":
		state = RunEgressSending
	case "unknown":
		state = RunEgressUnknown
	case "completed", "acknowledged":
		state = RunEgressCompleted
	case "blocked":
		state = RunEgressBlocked
	default:
		state = RunEgressUnknown
	}
	c := RunEgressClassification{State: state}
	if review := ProviderEgressReviewForSession(x); review != nil {
		c.RequiresReconciliation = review.RequiresReconciliation
		c.Guidance = review.Guidance
	}
	// Fail closed for the whole machine: no state grants automatic replay,
	// including PREPARED recovery, which must still go through the exact-payload
	// gateway rather than a blanket replay permission.
	c.AutomaticReplayAllowed = false
	return c
}
