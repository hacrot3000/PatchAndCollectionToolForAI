package dbadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func testAdapterManifest() Manifest {
	return Manifest{
		ID:              "test-adapter",
		Name:            "Test Adapter",
		Kind:            "test",
		ProtocolVersion: ProtocolVersion,
		Command:         os.Args[0],
		Args:            []string{"-test.run=TestDBAdapterHelperProcess"},
		Capabilities: CapabilitySet{
			Connect: true,
			Ping:    true,
			Execute: true,
			Cancel:  true,
		},
	}
}

func helperProcessEnv(mode string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env,
		"TASKDECK_DB_TEST_HELPER=1",
		"TASKDECK_DB_TEST_MODE="+mode,
	)
	return env
}

func TestAdapterProcessRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process, err := StartProcess(ctx, testAdapterManifest(), ProcessOptions{Env: helperProcessEnv("echo")})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()

	response, err := process.Request(ctx, OpPing, map[string]string{"value": "hello"}, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if response.Operation != OpPing || response.RequestID == "" || response.SessionID != "session-1" {
		t.Fatalf("response=%+v", response)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true {
		t.Fatalf("payload=%v", payload)
	}
}

func TestAdapterProcessNormalizesProtocolError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process, err := StartProcess(ctx, testAdapterManifest(), ProcessOptions{Env: helperProcessEnv("error")})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()

	_, err = process.Request(ctx, OpExecute, map[string]string{"query": "SELECT 1"}, "")
	if err == nil || !strings.Contains(err.Error(), "QUERY_FAILED: fixture failure") {
		t.Fatalf("error=%v", err)
	}
}

func TestAdapterProcessRejectsInvalidProtocolOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process, err := StartProcess(ctx, testAdapterManifest(), ProcessOptions{Env: helperProcessEnv("invalid")})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()

	_, err = process.Request(ctx, OpPing, nil, "")
	if err == nil || !strings.Contains(err.Error(), "invalid protocol message") {
		t.Fatalf("error=%v", err)
	}
}

func TestAdapterProcessRequestCancellationStopsProcess(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer parentCancel()
	process, err := StartProcess(parent, testAdapterManifest(), ProcessOptions{Env: helperProcessEnv("hang")})
	if err != nil {
		t.Fatal(err)
	}

	requestCtx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err = process.Request(requestCtx, OpPing, nil, "")
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("error=%v", err)
	}

	select {
	case <-process.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("adapter process did not stop after request cancellation")
	}
}

func TestBoundedDiagnosticTruncates(t *testing.T) {
	diagnostic := boundedDiagnostic{remaining: 5}
	if n, err := diagnostic.Write([]byte("abcdefgh")); err != nil || n != 8 {
		t.Fatalf("Write n=%d err=%v", n, err)
	}
	if got := diagnostic.String(); got != "abcde\n[diagnostic output truncated]" {
		t.Fatalf("diagnostic=%q", got)
	}
}

func TestDBAdapterHelperProcess(t *testing.T) {
	if os.Getenv("TASKDECK_DB_TEST_HELPER") != "1" {
		return
	}
	mode := os.Getenv("TASKDECK_DB_TEST_MODE")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64<<10), MaxMessageBytes)
	if !scanner.Scan() {
		os.Exit(3)
	}
	var request Envelope
	if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}

	switch mode {
	case "echo":
		response, err := NewResponse(request, map[string]interface{}{"ok": true})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(5)
		}
		if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
			os.Exit(6)
		}
		os.Exit(0)
	case "error":
		response, err := NewErrorResponse(request, "QUERY_FAILED", "fixture failure")
		if err != nil {
			os.Exit(7)
		}
		if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
			os.Exit(8)
		}
		os.Exit(0)
	case "invalid":
		fmt.Fprintln(os.Stdout, "{}")
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	case "hang":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	default:
		os.Exit(9)
	}
}
