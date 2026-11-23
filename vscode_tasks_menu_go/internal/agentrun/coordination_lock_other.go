//go:build !linux && !darwin

package agentrun

// lockCoordinationJournal is a no-op on platforms without flock. In-process
// exclusion is provided by coordinationJournalMutex, which CoordinationHub
// holds before calling this function and which migration holds separately.
// Cross-process exclusion is NOT provided; running two daemons against the
// same state directory must be refused by deployment policy.
func lockCoordinationJournal(path string) (func(), error) {
	return func() {}, nil
}
