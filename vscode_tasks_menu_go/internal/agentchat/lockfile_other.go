//go:build !linux && !darwin

package agentchat

// lockStoreDir degrades to in-process serialization on platforms without the
// flock implementation used by TaskDeck. Cross-process compare-and-swap is
// therefore NOT provided there; callers must not run two daemons against the
// same Agent state directory on these platforms.
func lockStoreDir(dir string) (func(), error) {
	return func() {}, nil
}
