//go:build linux || darwin

package agentchat

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// lockStoreDir takes an advisory exclusive lock on a dedicated lock file in
// the private Agent state directory. The lock is held across the complete
// read-compare-write cycle so two TaskDeck daemons sharing one state
// directory cannot both pass a revision compare. flock locks belong to the
// open file description, so separate opens inside one process also conflict.
//
// This is NOT a distributed filesystem protocol: NFS/SMB semantics are not
// guaranteed. Multi-daemon use on unsupported filesystems must be refused by
// deployment policy rather than assumed safe.
func lockStoreDir(dir string) (func(), error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("absolute agent state directory required for locking")
	}
	path := filepath.Join(dir, ".agent-store.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
