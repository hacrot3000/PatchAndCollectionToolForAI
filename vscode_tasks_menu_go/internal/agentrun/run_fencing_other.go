//go:build !linux && !darwin

package agentrun

// AcquireRunFence fails closed on platforms without the verified OS lease.
// Queued runs stay untouched for an operator to review; no worker may claim a
// run through an unavailable fence.
func AcquireRunFence(dir, runID string) (func(), error) {
	return nil, ErrRunFenceUnavailable
}
