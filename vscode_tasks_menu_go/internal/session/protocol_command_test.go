package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestValidateProtocolCommandIsBoundedAndVersioned(t *testing.T) {
	valid := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"future_prompt_response","payload":{"value":"yes"}}`)
	line, err := validateProtocolCommand(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(line), "future_prompt_response") {
		t.Fatalf("canonical command lost payload: %s", line)
	}
	for _, bad := range [][]byte{
		nil,
		[]byte("{bad"),
		[]byte("{\"protocol\":\"taskdeck.patch\",\"version\":2,\"type\":\"command\",\"seq\":1,\"command\":\"x\"}"),
		[]byte("{\"protocol\":\"taskdeck.patch\",\"version\":1,\"type\":\"command\",\"seq\":0,\"command\":\"x\"}"),
		[]byte("{\"protocol\":\"taskdeck.patch\",\"version\":1,\"type\":\"command\",\"seq\":1,\"command\":\"x\"}\n"),
	} {
		if _, err := validateProtocolCommand(bad); err == nil {
			t.Fatalf("invalid command accepted: %q", bad)
		}
	}
}

func TestSessionCommandPipeUsesFD4WithoutReplacingPTY(t *testing.T) {
	if testing.Short() {
		t.Skip("requires local shell and PTY")
	}
	shell := "/bin/sh"
	if _, err := os.Stat(shell); err != nil {
		t.Skip(err)
	}
	out := filepath.Join(t.TempDir(), "command.jsonl")
	manager := NewManager(1 << 20)
	meta, err := manager.Start(tasks.Execution{
		TaskID: -1,
		Label: "Patch protocol command test",
		Command: shell,
		Args: []string{"-c", "IFS= read -r line <&4; printf '%s' \"$line\" > \"$OUT\""},
		Cwd: t.TempDir(),
		Env: append(os.Environ(), "OUT="+out),
		Preview: "protocol command test",
		ProtocolEvents: true,
		ProtocolCommands: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"future_prompt_response","payload":{"value":"yes"}}`)
	if err := manager.ProtocolCommand(meta.ID, command); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, ok := manager.Metadata(meta.ID)
		if ok && current.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "future_prompt_response") {
		t.Fatalf("child did not receive command on FD4: %q", data)
	}
}

func TestProtocolCommandRequiresEventChannel(t *testing.T) {
	manager := NewManager(1 << 20)
	_, err := manager.Start(tasks.Execution{
		TaskID: -1,
		Label: "invalid command-only protocol",
		Command: "/bin/sh",
		Args: []string{"-c", "exit 0"},
		Cwd: t.TempDir(),
		Env: os.Environ(),
		ProtocolCommands: true,
	})
	if err == nil || !strings.Contains(err.Error(), "requires protocol events") {
		t.Fatalf("command-only protocol should fail closed, err=%v", err)
	}
}
