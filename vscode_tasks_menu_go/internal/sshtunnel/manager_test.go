package sshtunnel

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

type memorySecrets struct {
	mu      sync.Mutex
	records map[string][]byte
}

func (s *memorySecrets) Put(id string, secret []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string][]byte)
	}
	s.records[id] = append([]byte(nil), secret...)
	return nil
}

func (s *memorySecrets) Get(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.records[id]
	if !ok {
		return nil, secretstore.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *memorySecrets) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}

func tunnelFixtureExecutable(t *testing.T, mode string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh")
	script := "#!/bin/sh\nexec \"$TASKDECK_TUNNEL_HELPER\" -test.run=TestSSHTunnelHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASKDECK_TUNNEL_HELPER", os.Args[0])
	t.Setenv("TASKDECK_TUNNEL_HELPER_MODE", mode)
	return path
}

func tunnelTestProfile() sshprofile.Profile {
	return sshprofile.Profile{
		ID:                       "ssh-prod",
		Name:                     "Production",
		Host:                     "jump.example.com",
		Username:                 "deploy",
		AuthMethod:               sshprofile.AuthAgent,
		ConnectTimeoutSeconds:    1,
		ServerAliveIntervalSeconds: 15,
		ServerAliveCountMax:      3,
	}
}

func TestManagerOpensLoopbackTunnelAndCleansItUp(t *testing.T) {
	executable := tunnelFixtureExecutable(t, "listen")
	manager, err := NewManager(nil, Options{
		RuntimeDir:    t.TempDir(),
		SSHExecutable: executable,
		MaxTunnels:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	meta, err := manager.Open(ctx, tunnelTestProfile(), "db.internal", 3306)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LocalHost != "127.0.0.1" || meta.LocalPort < 1 || meta.RemoteHost != "db.internal" || meta.RemotePort != 3306 {
		t.Fatalf("metadata=%+v", meta)
	}
	conn, err := net.DialTimeout("tcp4", net.JoinHostPort(meta.LocalHost, strconv.Itoa(meta.LocalPort)), 250*time.Millisecond)
	if err != nil {
		t.Fatalf("dial ready tunnel: %v", err)
	}
	_ = conn.Close()
	if len(manager.List()) != 1 {
		t.Fatalf("tunnels=%+v", manager.List())
	}
	diagnostics, err := manager.Diagnostics(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagnostics, "fixture ready") {
		t.Fatalf("diagnostics=%q", diagnostics)
	}

	if err := manager.CloseTunnel(meta.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(meta.ID); !errors.Is(err, ErrTunnelNotFound) {
		t.Fatalf("Get after close error=%v", err)
	}
}

func TestManagerCancelsTunnelStartup(t *testing.T) {
	executable := tunnelFixtureExecutable(t, "hang")
	manager, err := NewManager(nil, Options{RuntimeDir: t.TempDir(), SSHExecutable: executable})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = manager.Open(ctx, tunnelTestProfile(), "db.internal", 3306)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v want deadline exceeded", err)
	}
	if len(manager.List()) != 0 {
		t.Fatalf("cancelled startup leaked tunnels: %+v", manager.List())
	}
}

func TestManagerReportsEarlySSHFailure(t *testing.T) {
	executable := tunnelFixtureExecutable(t, "exit")
	manager, err := NewManager(nil, Options{RuntimeDir: t.TempDir(), SSHExecutable: executable})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = manager.Open(ctx, tunnelTestProfile(), "db.internal", 3306)
	if err == nil || !strings.Contains(err.Error(), "fixture SSH failure") {
		t.Fatalf("error=%v", err)
	}
	if len(manager.List()) != 0 {
		t.Fatalf("failed startup leaked tunnels: %+v", manager.List())
	}
}

func TestManagerConfiguresAskpassForStoredSecret(t *testing.T) {
	executable := tunnelFixtureExecutable(t, "check-askpass")
	secrets := &memorySecrets{records: map[string][]byte{"ssh/prod/auth/password": []byte("secret")}}
	manager, err := NewManager(secrets, Options{
		RuntimeDir:    t.TempDir(),
		AskpassHelper: os.Args[0],
		SSHExecutable: executable,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	profile := tunnelTestProfile()
	profile.AuthMethod = sshprofile.AuthPassword
	profile.SecretRef = "ssh/prod/auth/password"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	meta, err := manager.Open(ctx, profile, "db.internal", 3306)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseTunnel(meta.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSSHTunnelHelperProcess(t *testing.T) {
	if os.Getenv("TASKDECK_TUNNEL_HELPER_MODE") == "" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	forward := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-L" {
			forward = args[i+1]
			break
		}
	}
	if forward == "" {
		os.Exit(10)
	}
	parts := strings.SplitN(forward, ":", 3)
	if len(parts) < 3 {
		os.Exit(11)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 1 {
		os.Exit(12)
	}

	switch os.Getenv("TASKDECK_TUNNEL_HELPER_MODE") {
	case "exit":
		_, _ = os.Stderr.WriteString("fixture SSH failure\n")
		os.Exit(255)
	case "hang":
		select {}
	case "check-askpass":
		for _, key := range []string{
			"SSH_ASKPASS",
			"SSH_ASKPASS_REQUIRE",
			"TASKDECK_SSH_ASKPASS",
			"TASKDECK_SSH_ASKPASS_SOCKET",
			"TASKDECK_SSH_ASKPASS_TOKEN",
		} {
			if os.Getenv(key) == "" {
				os.Exit(20)
			}
		}
		if os.Getenv("SSH_ASKPASS_REQUIRE") != "force" || os.Getenv("TASKDECK_SSH_ASKPASS") != "1" {
			os.Exit(21)
		}
		fallthrough
	case "listen":
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			os.Exit(13)
		}
		defer listener.Close()
		_, _ = os.Stderr.WriteString("fixture ready\n")
		for {
			conn, err := listener.Accept()
			if err != nil {
				os.Exit(0)
			}
			_ = conn.Close()
		}
	default:
		os.Exit(14)
	}
}


func TestTunnelMetadataIncludesLivePID(t *testing.T) {
	manager, err := NewManager(nil, Options{
		RuntimeDir: t.TempDir(),
		SSHExecutable: os.Args[0],
		StartupTimeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	t.Setenv("TASKDECK_TUNNEL_HELPER", os.Args[0])
	profile := sshprofile.Profile{
		ID: "fixture",
		Name: "fixture",
		Host: "127.0.0.1",
		Port: 22,
		User: "tester",
	}
	meta, err := manager.Open(context.Background(), profile, "127.0.0.1", 3306)
	if err != nil {
		t.Fatal(err)
	}
	if meta.PID <= 0 {
		t.Fatalf("live tunnel PID was not exposed: %#v", meta)
	}
	stored, err := manager.Get(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PID != meta.PID {
		t.Fatalf("stored tunnel PID=%d want=%d", stored.PID, meta.PID)
	}
}
