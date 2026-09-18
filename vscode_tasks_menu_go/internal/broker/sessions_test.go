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

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func waitForClient(t *testing.T, workspace string) *Client {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		client, err := NewClient(workspace)
		if err == nil {
			return client
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("broker client did not become available")
	return nil
}

func TestBrokerExecutionWirePreservesEnvironment(t *testing.T) {
	spec := tasks.Execution{
		TaskID: 7, Label: "wire", Detail: "detail", Command: "/bin/sh",
		Args: []string{"-c", "true"}, Cwd: "/tmp",
		Env: []string{"ONE=1", "TWO=two"}, Preview: "true",
	}
	got := executionFromWire(executionToWire(spec))
	if len(got.Env) != 2 || got.Env[0] != "ONE=1" || got.Env[1] != "TWO=two" {
		t.Fatalf("environment lost in broker wire conversion: %#v", got.Env)
	}
	if got.Command != spec.Command || got.Cwd != spec.Cwd || got.TaskID != spec.TaskID {
		t.Fatalf("execution changed in broker wire conversion: %#v", got)
	}
}

func TestBrokerClientOwnsAndControlsPTYSession(t *testing.T) {
	ws := testWorkspace(t)
	ctx, cancelBroker := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- Run(ctx, ws, log.New(io.Discard, "", 0)) }()
	client := waitForClient(t, ws)
	defer client.Close()

	spec := tasks.Execution{
		TaskID: 42,
		Label: "Broker integration",
		Command: "/bin/sh",
		Args: []string{"-c", "printf '%s\\n' \"$BROKER_TEST_VALUE\"; sleep 30"},
		Cwd: ws,
		Env: append(os.Environ(), "BROKER_TEST_VALUE=broker-env-ok"),
		Preview: "broker integration",
	}
	meta, err := client.Start(spec)
	if err != nil {
		cancelBroker()
		t.Fatal(err)
	}
	if meta.ID == "" || meta.Status != "running" || meta.TaskID != spec.TaskID {
		t.Fatalf("unexpected started metadata: %#v", meta)
	}

	items := client.List()
	found := false
	for _, item := range items {
		if item.ID == meta.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("started session %s missing from broker list", meta.ID)
	}

	cwd, err := client.CurrentCwd(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(cwd) != filepath.Clean(ws) {
		t.Fatalf("cwd = %q, want %q", cwd, ws)
	}
	if err := client.Resize(meta.ID, 40, 100); err != nil {
		t.Fatal(err)
	}

	backlog, stream, unsubscribe, err := client.Subscribe(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	output := string(backlog)
	deadline := time.After(3 * time.Second)
	for !strings.Contains(output, "broker-env-ok") {
		select {
		case chunk, ok := <-stream:
			if !ok {
				t.Fatalf("broker stream closed before expected output; got %q", output)
			}
			output += string(chunk)
		case <-deadline:
			t.Fatalf("timed out waiting for broker output; got %q", output)
		}
	}

	if err := client.Clear(meta.ID); err != nil {
		t.Fatal(err)
	}
	if err := client.Stop(meta.ID); err != nil {
		t.Fatal(err)
	}
	stopDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(stopDeadline) {
		got, ok := client.Metadata(meta.ID)
		if ok && got.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got, ok := client.Metadata(meta.ID); !ok || got.Status == "running" {
		t.Fatalf("session did not stop through broker: %#v ok=%v", got, ok)
	}

	unsubscribe()
	if err := client.Remove(meta.ID); err != nil {
		t.Fatal(err)
	}
	cancelBroker()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("broker shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("broker did not stop")
	}
}
