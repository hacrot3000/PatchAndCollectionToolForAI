package broker

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testWorkspace(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	ws := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestBrokerInfoRoundTrip(t *testing.T) {
	ws := testWorkspace(t)
	info := NewInfo(ws)
	if err := SaveInfo(info); err != nil {
		t.Fatal(err)
	}
	got, err := LoadInfo(ws)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProtocolVersion != ProtocolVersion || got.PID != info.PID || got.Workspace != ws || got.SocketPath != SocketPath(ws) {
		t.Fatalf("unexpected broker info: %#v", got)
	}
	stat, err := os.Stat(StatePath(ws))
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("broker state mode = %o, want 600", stat.Mode().Perm())
	}
	RemoveInfoIfPID(ws, info.PID+1)
	if _, err := os.Stat(StatePath(ws)); err != nil {
		t.Fatalf("wrong pid removed broker state: %v", err)
	}
	RemoveInfoIfPID(ws, info.PID)
	if _, err := os.Stat(StatePath(ws)); !os.IsNotExist(err) {
		t.Fatalf("broker state still exists after pid-aware cleanup: %v", err)
	}
}

func TestBrokerRunProbeAndCleanup(t *testing.T) {
	ws := testWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, ws, log.New(io.Discard, "", 0))
	}()

	var info Info
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		info, err = Probe(probeCtx, ws)
		probeCancel()
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("broker never became probeable: %v", err)
	}
	if info.Workspace != ws || info.ProtocolVersion != ProtocolVersion || info.PID <= 0 {
		t.Fatalf("unexpected live broker info: %#v", info)
	}
	if stat, err := os.Stat(SocketPath(ws)); err != nil {
		t.Fatal(err)
	} else if stat.Mode().Perm() != 0o600 {
		t.Fatalf("broker socket mode = %o, want 600", stat.Mode().Perm())
	}

	secondCtx, secondCancel := context.WithCancel(context.Background())
	defer secondCancel()
	if err := Run(secondCtx, ws, log.New(io.Discard, "", 0)); err == nil {
		t.Fatal("second broker for same workspace unexpectedly started")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("broker shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("broker did not stop after context cancellation")
	}
	if _, err := os.Stat(SocketPath(ws)); !os.IsNotExist(err) {
		t.Fatalf("broker socket still exists after shutdown: %v", err)
	}
	if _, err := os.Stat(StatePath(ws)); !os.IsNotExist(err) {
		t.Fatalf("broker state still exists after shutdown: %v", err)
	}
}

func TestBrokerSocketPathFallsBackWhenRuntimePathIsTooLong(t *testing.T) {
	longBase := filepath.Join(t.TempDir(), strings.Repeat("very-long-runtime-segment-", 6))
	if err := os.MkdirAll(longBase, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", longBase)
	ws := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	path := SocketPath(ws)
	if len(path) > 96 {
		t.Fatalf("broker socket path too long: %d bytes: %s", len(path), path)
	}
	socketDir := filepath.Dir(path)
	if socketDir == os.TempDir() {
		t.Fatalf("fallback socket must not live directly in shared temp dir: %s", path)
	}
	if filepath.Dir(socketDir) != os.TempDir() {
		t.Fatalf("fallback socket should live in private child of temp dir: %s", path)
	}
	if !strings.HasPrefix(filepath.Base(socketDir), "vtm-broker-") {
		t.Fatalf("unexpected fallback socket directory: %s", socketDir)
	}
}

func TestEnsureClientReusesAndCanShutdownRunningBroker(t *testing.T) {
	ws := testWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- Run(ctx, ws, log.New(io.Discard, "", 0)) }()

	client := waitForClient(t, ws, errCh)
	client.Close()

	reused, err := EnsureClient(ws, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	info, err := LoadInfo(ws)
	if err != nil {
		t.Fatal(err)
	}
	if reused.info.PID != info.PID {
		t.Fatalf("EnsureClient did not reuse live broker: client pid=%d state pid=%d", reused.info.PID, info.PID)
	}
	if err := reused.ShutdownBroker(); err != nil {
		t.Fatal(err)
	}
	reused.Close()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("broker shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("broker did not exit after ShutdownBroker")
	}
}

func TestBrokerTitleCapabilityBackwardCompatibility(t *testing.T) {
	legacy := &Client{info: Info{ProtocolVersion: ProtocolVersion}}
	if legacy.SupportsSessionTitle() {
		t.Fatal("legacy broker without capabilities must not advertise session title support")
	}

	ws := testWorkspace(t)
	modern := &Client{info: NewInfo(ws)}
	if !modern.SupportsSessionTitle() {
		t.Fatal("new broker must advertise session title support")
	}
}


func TestBrokerFallbackSocketDirectoryIsPrivate(t *testing.T) {
	longBase := filepath.Join(t.TempDir(), strings.Repeat("very-long-runtime-segment-", 6))
	if err := os.MkdirAll(longBase, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", longBase)
	ws := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- Run(ctx, ws, log.New(io.Discard, "", 0)) }()

	var probeErr error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		_, probeErr = Probe(probeCtx, ws)
		probeCancel()
		if probeErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if probeErr != nil {
		cancel()
		t.Fatalf("broker never became probeable: %v", probeErr)
	}

	socketDir := filepath.Dir(SocketPath(ws))
	info, err := os.Stat(socketDir)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		cancel()
		t.Fatalf("fallback socket dir mode=%#o want 0700", got)
	}
	socketInfo, err := os.Stat(SocketPath(ws))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if got := socketInfo.Mode().Perm(); got != 0o600 {
		cancel()
		t.Fatalf("fallback socket mode=%#o want 0600", got)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("broker shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("broker did not stop")
	}
}
