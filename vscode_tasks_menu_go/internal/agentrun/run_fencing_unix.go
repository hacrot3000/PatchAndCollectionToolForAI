//go:build linux || darwin

package agentrun

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// validateRunFenceFile rejects a swapped, hard-linked, symlinked or wrongly
// permissioned lock inode. Check before AND after the nonblocking flock: the
// path can change between inspection and acquiring the OS lease.
func validateRunFenceFile(f *os.File, path string) bool {
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return false
	}
	current, err := os.Lstat(path)
	return err == nil && current.Mode().IsRegular() && current.Mode().Perm()&0077 == 0 && os.SameFile(info, current)
}

// AcquireRunFence claims a durable, cross-process lease for one run ID on this
// host. It is nonblocking: a live lease held by another manager returns
// ErrRunFenceBusy. The lock inode is permanent (never unlinked on release) so
// inode replacement cannot create split-brain. Process death drops the flock
// but NEVER acknowledges provider egress; durable run records remain
// authoritative and uncertain states still require explicit review.
//
// Supported target: local Linux/macOS private storage only. NFS/SMB and other
// shared or network filesystems are explicitly unsupported because flock
// semantics there are not reliable; callers must fail closed on those mounts.
func AcquireRunFence(dir, runID string) (func(), error) {
	if dir == "" || !filepath.IsAbs(dir) || runID == "" || len(runID) > 256 {
		return nil, ErrRunFenceUnavailable
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrRunFenceUnavailable
	}
	digest := runFenceDigest(runID)
	if digest == "" {
		return nil, ErrRunFenceUnavailable
	}
	path := filepath.Join(dir, ".run-fence-"+digest+".lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, ErrRunFenceUnavailable
	}
	fail := func() (func(), error) { _ = f.Close(); return nil, ErrRunFenceUnavailable }
	if !validateRunFenceFile(f, path) {
		return fail()
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrRunFenceBusy
		}
		return nil, ErrRunFenceUnavailable
	}
	if !validateRunFenceFile(f, path) {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return fail()
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			_ = f.Close()
		})
	}, nil
}
