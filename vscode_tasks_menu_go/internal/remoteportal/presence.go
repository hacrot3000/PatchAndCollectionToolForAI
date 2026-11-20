package remoteportal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// DefaultIdentityPath is stable across reboots and distinct by absolute
// workspace path. The key MUST NOT be kept in temporary runtime directories
// or in the repository; only device ID is sent to the Portal.
func DefaultIdentityPath(workspace string) (string, error) {
	if !filepath.IsAbs(workspace) {
		return "", errors.New("workspace must be absolute")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(workspace)))
	return filepath.Join(dir, "taskdeck", "remote-identities", hex.EncodeToString(sum[:])+".seed"), nil
}

// RunPresence never creates remote execution sessions. It sends signed
// heartbeats until its daemon context is canceled. Errors do not affect local
// TaskDeck operation. Caller decides what details are appropriate to log.
func RunPresence(ctx context.Context, agent *Agent, interval time.Duration, observe func(error)) {
	if agent == nil || interval < 10*time.Second {
		if observe != nil {
			observe(errors.New("invalid remote presence configuration"))
		}
		return
	}
	send := func() {
		callCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		err := agent.Heartbeat(callCtx)
		cancel()
		if err != nil && ctx.Err() == nil && observe != nil {
			observe(err)
		}
	}
	send()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}
