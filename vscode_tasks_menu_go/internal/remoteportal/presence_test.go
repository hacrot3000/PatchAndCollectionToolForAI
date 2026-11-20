package remoteportal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIdentityPathIsStableAndDistinct(t *testing.T) {
	one, err := DefaultIdentityPath(filepath.Join(t.TempDir(), "a"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := DefaultIdentityPath(filepath.Dir(one))
	if err != nil || one == again {
		t.Fatal("identity path was not derived from workspace", err)
	}
	p2, err := DefaultIdentityPath(filepath.Join(t.TempDir(), "b"))
	if err != nil || p2 == one {
		t.Fatal("two different workspaces share remote identity", err)
	}
	if !filepath.IsAbs(one) {
		t.Fatal("identity path must be absolute")
	}
	if _, err := DefaultIdentityPath("relative"); err == nil {
		t.Fatal("relative workspace accepted")
	}
	_ = os.ErrNotExist
}

func TestPresenceCancelAndNilAgent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		RunPresence(ctx, nil, time.Second, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2*time.Second):
		t.Fatal("nil agent should return immediately")
	}
}
