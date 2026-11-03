//go:build !linux && !darwin
// +build !linux,!darwin

package selfupdate

import (
	"context"
	"time"
)

// Portable fallback where syscall.Flock is not available. Unix hosts use
// crash-safe kernel locks, while other ports retain the atomic mkdir guard.
func acquireUpdateLock(ctx context.Context, lockPath string) (func(), error) {
	return acquireDirectoryLock(ctx, lockPath, 35*time.Minute)
}
