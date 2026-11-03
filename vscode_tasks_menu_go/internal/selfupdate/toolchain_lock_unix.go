//go:build linux || darwin
// +build linux darwin

package selfupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// An OS-backed advisory file lock survives unrelated project processes and
// is automatically released when the updater exits or crashes. Unlike stale
// mkdir locks it never requires deleting a lock another process may own.
func acquireUpdateLock(ctx context.Context, lockPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(lockPath,
		syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open TaskDeck update lock %q: %w", lockPath, err)
	}
	file := os.NewFile(uintptr(fd), lockPath)
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("unsafe TaskDeck update lock path %q", lockPath)
	}
	for {
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			var once sync.Once
			return func() { once.Do(func() {
				_ = syscall.Flock(fd, syscall.LOCK_UN)
				_ = file.Close()
			}) }, nil
		}
		if err != syscall.EAGAIN && err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			_ = file.Close()
			return nil, fmt.Errorf("acquire TaskDeck update lock %q: %w", lockPath, err)
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
