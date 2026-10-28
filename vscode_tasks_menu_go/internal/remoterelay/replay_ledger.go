package remoterelay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ReplayLedger is a small, crash-resistant single-use ticket ledger for the
// Relay process. It stores ONLY random Portal JTI and expiry, never the token,
// device address, user credentials, terminal output or HTTP body.
//
// Each signed JTI is claimed with O_EXCL, so restarting Relay cannot make a
// previously consumed 30-second browser ticket valid again. Multiple replicas
// require a shared transactional store; this directory is for ONE Relay host.
type ReplayLedger struct {
	mu       sync.Mutex
	dir      string
	entries  int
	sinceGC  int
}

const maxLedgerRecords = 20000

func NewReplayLedger(dir string) (*ReplayLedger, error) {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, errors.New("ticket replay directory must be an absolute, normalized path")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create replay directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("replay directory must be a regular owner-only directory (0700)")
	}
	store := &ReplayLedger{dir: dir}
	if err := store.pruneLocked(time.Now().Unix()); err != nil {
		return nil, err
	}
	return store, nil
}

// Consume fails closed if the disk is unavailable or a signed JTI has already
// been consumed. The file and containing directory are synced before success.
func (l *ReplayLedger) Consume(jti string, expires int64, now time.Time) error {
	if l == nil || !jtiPattern.MatchString(jti) || expires <= now.Unix() ||
		expires > now.Unix()+70 {
		return errors.New("invalid replay claim")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sinceGC >= 128 || l.entries >= maxLedgerRecords {
		if err := l.pruneLocked(now.Unix()); err != nil {
			return err
		}
	}
	if l.entries >= maxLedgerRecords {
		return errors.New("ticket replay ledger capacity reached")
	}
	path := filepath.Join(l.dir, jti)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return errors.New("ticket already consumed")
	}
	if err != nil {
		return fmt.Errorf("replay ledger unavailable: %w", err)
	}
	value := []byte(strconv.FormatInt(expires, 10))
	_, writeErr := f.Write(value)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		d, openErr := os.Open(l.dir)
		if openErr != nil {
			writeErr = openErr
		} else {
			writeErr = d.Sync()
			_ = d.Close()
		}
	}
	if writeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("persist replay record: %w", writeErr)
	}
	l.entries++
	l.sinceGC++
	return nil
}

// pruneLocked is used only under the ledger mutex, except during construction.
func (l *ReplayLedger) pruneLocked(now int64) error {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return fmt.Errorf("read replay ledger: %w", err)
	}
	l.entries = 0
	for _, e := range entries {
		if e.IsDir() || !jtiPattern.MatchString(e.Name()) {
			continue
		}
		// A truncated, tampered or symlinked record MUST NOT be deleted:
		// it may represent an already-burned, still-valid ticket after a
		// power failure. Refuse to start instead of silently permitting replay.
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 ||
			info.Size() < 1 || info.Size() > 24 {
			return fmt.Errorf("unsafe/corrupt ticket replay record %q", e.Name())
		}
		path := filepath.Join(l.dir, e.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil || len(data) != int(info.Size()) {
			return fmt.Errorf("cannot read ticket replay record %q", e.Name())
		}
		exp, parseErr := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if parseErr != nil || exp <= 0 {
			return fmt.Errorf("invalid ticket replay record %q (manual recovery required)", e.Name())
		}
		if exp <= now {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("prune expired replay record: %w", err)
			}
			continue
		}
		l.entries++
	}
	l.sinceGC = 0
	return nil
}
