//go:build linux || darwin

package agentrun

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// lockCoordinationJournal takes an advisory exclusive lock on a dedicated lock
// file next to the coordination journal. The lock is held across the complete
// read-modify-write cycle so a migration cannot overwrite a journal while an
// API request updates it, and so two TaskDeck daemons sharing one state
// directory cannot migrate the same owner twice.
//
// flock locks belong to the open file description, so separate opens inside
// one process conflict as well.
//
// This is NOT a distributed filesystem protocol: NFS/SMB semantics are not
// guaranteed. Multi-daemon use on unsupported filesystems must be refused by
// deployment policy rather than assumed safe.
func lockCoordinationJournal(path string) (func(), error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("absolute coordination journal path required for locking")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dir, ".coordination-journal.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
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
