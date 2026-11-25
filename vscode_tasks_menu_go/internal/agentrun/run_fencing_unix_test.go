//go:build linux || darwin

package agentrun

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireRunFenceExcludesSecondOwner(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireRunFence(dir, "run-fence-test-1")
	if err != nil {
		t.Fatalf("first fence acquire failed: %v", err)
	}
	defer first()
	second, err := AcquireRunFence(dir, "run-fence-test-1")
	if !errors.Is(err, ErrRunFenceBusy) || second != nil {
		t.Fatalf("second owner must be refused with busy: gotRelease=%v err=%v", second != nil, err)
	}
	first()
	first() // release is idempotent
	again, err := AcquireRunFence(dir, "run-fence-test-1")
	if err != nil {
		t.Fatalf("fence must be reacquirable after release: %v", err)
	}
	again()
	other, err := AcquireRunFence(dir, "run-fence-test-2")
	if err != nil {
		t.Fatalf("a different run must get its own fence: %v", err)
	}
	other()
	// The permanent lock inode must remain a regular 0600 file.
	digest := runFenceDigest("run-fence-test-1")
	info, err := os.Lstat(filepath.Join(dir, ".run-fence-"+digest+".lock"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("lock inode was removed or made unsafe: info=%v err=%v", info, err)
	}
}

func TestAcquireRunFenceRejectsUnsafeDirectory(t *testing.T) {
	if _, err := AcquireRunFence("relative-dir", "run-1"); !errors.Is(err, ErrRunFenceUnavailable) {
		t.Fatalf("relative directory must fail closed: %v", err)
	}
	if _, err := AcquireRunFence(t.TempDir(), ""); !errors.Is(err, ErrRunFenceUnavailable) {
		t.Fatalf("empty run ID must fail closed: %v", err)
	}
}
