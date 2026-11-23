//go:build !linux && !darwin

package agentrun

// lockCoordinationJournal degrades to in-process serialization on platforms
// without the flock implementation used by TaskDeck. Cross-process exclusion is
// therefore NOT provided there; callers must not run two daemons against the
// same Agent state directory on these platforms.
func lockCoordinationJournal(path string) (func(), error) {
	return func() {}, nil
}
