package sshtunnel

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/sshclient"
)

func reserveTestMetadata(t *testing.T) (*portReservation, Metadata) {
	t.Helper()
	reservation, err := reserveLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	meta := Metadata{
		ID:           "tunnel-test",
		SSHProfileID: "ssh-test",
		LocalHost:    "127.0.0.1",
		LocalPort:    reservation.Port(),
		RemoteHost:   "db.internal",
		RemotePort:   3306,
		StartedAt:    time.Now().Format(time.RFC3339),
	}
	return reservation, meta
}

func helperCommand(mode string, endpoint string) (sshclient.Command, []string) {
	env := append([]string(nil), os.Environ()...)
	env = append(env,
		"TASKDECK_SSHTUNNEL_HELPER=1",
		"TASKDECK_SSHTUNNEL_MODE="+mode,
		"TASKDECK_SSHTUNNEL_ENDPOINT="+endpoint,
	)
	return sshclient.Command{
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestSSHTunnelHelperProcess"},
		Destination: "fixture",
	}, env
}

func TestStartTunnelProcessWaitsForReadyAndCloses(t *testing.T) {
	reservation, meta := reserveTestMetadata(t)
	command, env := helperCommand("ready", reservation.Endpoint())

	lifetime, lifetimeCancel := context.WithCancel(context.Background())
	defer lifetimeCancel()
	startup, startupCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer startupCancel()

	tunnel, err := startTunnelProcess(lifetime, startup, command, env, meta, reservation)
	if err != nil {
		t.Fatal(err)
	}
	if tunnel.LocalEndpoint() != net.JoinHostPort("127.0.0.1", fmt.Sprint(meta.LocalPort)) {
		t.Fatalf("endpoint=%q", tunnel.LocalEndpoint())
	}
	if err := tunnel.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-tunnel.Done():
	case <-time.After(time.Second):
		t.Fatal("tunnel did not close")
	}
}

func TestStartTunnelProcessReportsEarlyExitDiagnostics(t *testing.T) {
	reservation, meta := reserveTestMetadata(t)
	command, env := helperCommand("fail", reservation.Endpoint())
	startup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := startTunnelProcess(context.Background(), startup, command, env, meta, reservation)
	if err == nil || !strings.Contains(err.Error(), "fixture tunnel failure") {
		t.Fatalf("error=%v", err)
	}
}

func TestStartTunnelProcessStartupTimeoutKillsProcess(t *testing.T) {
	reservation, meta := reserveTestMetadata(t)
	command, env := helperCommand("hang", reservation.Endpoint())
	startup, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := startTunnelProcess(context.Background(), startup, command, env, meta, reservation)
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("error=%v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatalf("startup cancellation was too slow: %v", time.Since(started))
	}
}

func TestBoundedDiagnosticTruncates(t *testing.T) {
	diag := boundedDiagnostic{remaining: 5}
	if n, err := diag.Write([]byte("abcdefgh")); err != nil || n != 8 {
		t.Fatalf("Write n=%d err=%v", n, err)
	}
	if got := diag.String(); got != "abcde\n[diagnostic output truncated]" {
		t.Fatalf("diagnostic=%q", got)
	}
}

func TestSSHTunnelHelperProcess(t *testing.T) {
	if os.Getenv("TASKDECK_SSHTUNNEL_HELPER") != "1" {
		return
	}
	switch os.Getenv("TASKDECK_SSHTUNNEL_MODE") {
	case "ready":
		listener, err := net.Listen("tcp4", os.Getenv("TASKDECK_SSHTUNNEL_ENDPOINT"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "fixture listen failed:", err)
			os.Exit(10)
		}
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				os.Exit(0)
			}
			_ = conn.Close()
		}
	case "fail":
		fmt.Fprintln(os.Stderr, "fixture tunnel failure")
		os.Exit(12)
	case "hang":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	default:
		os.Exit(13)
	}
}
