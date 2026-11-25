package agentrun

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// ErrRunFenceBusy means another live manager on this host already owns the
// run's cross-process lease. ErrRunFenceUnavailable means the platform cannot
// provide the required OS lease. Both fail closed without touching the run.
var (
	ErrRunFenceBusy        = errors.New("run is owned by another active manager")
	ErrRunFenceUnavailable = errors.New("same-host run fence unavailable")
)

// runFenceDigest derives a fixed, filename-safe identity from an exact run ID.
// The raw run ID is never embedded in a lock filename.
func runFenceDigest(runID string) string {
	if runID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(runID))
	return hex.EncodeToString(sum[:16])
}

// RunFenceAdmission decides whether a newly started manager may claim a run
// recorded in the durable journal. It is pure and platform independent; the OS
// lease is acquired separately by AcquireRunFence. A genuinely live run is
// never admitted, and uncertain states require explicit review rather than an
// automatic re-execution. This is the policy half of the M17-08 fence; wiring
// it into Manager.load/ReadRecords is a separate reviewed change.
func RunFenceAdmission(status Status, leaseHeld bool) (bool, string) {
	switch status {
	case Queued:
		if leaseHeld {
			return false, "lease_held"
		}
		return true, ""
	case Running, WaitingTool:
		return false, "active_state_requires_review"
	case Interrupted, Failed:
		return false, "uncertain_state_requires_review"
	case Completed, Stopped:
		return false, "terminal"
	default:
		return false, "unknown_status"
	}
}
