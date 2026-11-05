package broker

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

// macOS runtime directories may be purged without stopping a detached
// session broker. Reconnecting must recover its exact PID and existing PTYs.
func TestMissingBrokerStateRecoversLiveSocketAndPreservesSessions(t *testing.T) {
	ws := testWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- Run(ctx, ws, log.New(io.Discard, "", 0)) }()
	original := waitForClient(t, ws, errCh)
	defer original.Close()
	brokerPID := original.info.PID
	meta, err := original.Start(tasks.Execution{
		TaskID: 90, Label: "survive missing broker metadata",
		Command: "/bin/sh", Args: []string{"-c", "exec sleep 30"},
		Cwd: ws, Env: os.Environ(), Preview: "sleep 30",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = original.Stop(meta.ID) }()

	if err := os.Remove(StatePath(ws)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(StatePath(ws)); !os.IsNotExist(err) {
		t.Fatalf("broker metadata should be missing, got: %v", err)
	}
	recovered, err := EnsureClient(ws, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("failed to recover live session broker: %v", err)
	}
	defer recovered.Close()
	if recovered.info.PID != brokerPID || recovered.info.StartedAt != original.info.StartedAt {
		t.Fatalf("recovery replaced broker: before=%#v after=%#v", original.info, recovered.info)
	}
	if recovered.info.SocketPath != SocketPath(ws) {
		t.Fatalf("recovered wrong socket: %q", recovered.info.SocketPath)
	}
	disk, err := LoadInfo(ws)
	if err != nil {
		t.Fatalf("broker metadata not recreated: %v", err)
	}
	if disk.PID != brokerPID || disk.SocketPath != recovered.info.SocketPath {
		t.Fatalf("incorrect restored broker identity: %#v", disk)
	}
	if mode, err := os.Stat(StatePath(ws)); err != nil || mode.Mode().Perm() != 0o600 {
		t.Fatalf("restored broker metadata should be private: stat=%v mode=%v", err, mode)
	}
	preserved, ok := recovered.Metadata(meta.ID)
	if !ok || preserved.ID != meta.ID || preserved.Status != "running" {
		t.Fatalf("session was not preserved: %#v ok=%v", preserved, ok)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("broker shutdown error: %v", err)
		}
	case <-time.After(brokerTestShutdownTimeout):
		t.Fatal("broker did not stop")
	}
}

func TestLockedBrokerWithoutLiveSocketDoesNotSpawnReplacement(t *testing.T) {
	ws := testWorkspace(t)
	lock, err := acquireLock(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	client, err := EnsureClient(ws, log.New(io.Discard, "", 0))
	if client != nil {
		client.Close()
		t.Fatal("unexpected client for unavailable socket")
	}
	if err == nil || !strings.Contains(err.Error(), "broker is still running") ||
		!strings.Contains(err.Error(), "session-broker.log") ||
		!strings.Contains(err.Error(), "do not kill broker") {
		t.Fatalf("expected protective diagnostic rather than duplicate broker: %v", err)
	}
	if _, err := os.Stat(StatePath(ws)); !os.IsNotExist(err) {
		t.Fatalf("failed recovery must not invent broker identity: %v", err)
	}
}

func TestMissingBrokerStateRejectsUntrustedSocketIdentity(t *testing.T) {
	ws := testWorkspace(t)
	lock, err := acquireLock(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	socket := SocketPath(ws)
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(socket)
	defer listener.Close()
	if err := os.Chmod(socket, 0o600); err != nil {
		t.Fatal(err)
	}
	fake := NewInfo(ws)
	fake.Workspace = ws + "/other"
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"protocol_version":1,"pid":4321,"workspace":"`+fake.Workspace+`","socket_path":"`+socket+`"}`)
	})
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := Probe(ctx, ws); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("untrusted socket identity was accepted: %v", err)
	}
	if _, err := os.Stat(StatePath(ws)); !os.IsNotExist(err) {
		t.Fatalf("untrusted socket identity wrote metadata: %v", err)
	}
}
