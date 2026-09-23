package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestProtocolStateTracksAndClearsActivePrompt(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true, CommandsEnabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":4,"prompt_id":"p1","prompt_kind":"queue_selection"}`))
	if len(s.protocol.Prompt) == 0 {
		t.Fatal("prompt event was not retained")
	}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"run_finished","seq":5,"exit_code":0}`))
	if len(s.protocol.Prompt) != 0 {
		t.Fatal("run_finished must clear stale prompt state")
	}
}

func TestProtocolCommandRejectsStalePromptAndClearsAcceptedPrompt(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip(err)
	}
	manager := NewManager(1 << 20)
	meta, err := manager.Start(tasks.Execution{
		TaskID: -1,
		Label: "prompt response test",
		Command: "/bin/sh",
		Args: []string{"-c", "IFS= read -r line <&4; sleep 0.2"},
		Cwd: t.TempDir(),
		Env: os.Environ(),
		ProtocolEvents: true,
		ProtocolCommands: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	s, ok := manager.Get(meta.ID)
	if !ok {
		t.Fatal("session missing")
	}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":4,"prompt_id":"active","prompt_kind":"queue_selection"}`))

	stale := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"prompt_response","payload":{"prompt_id":"stale","action":"cancel"}}`)
	if err := manager.ProtocolCommand(meta.ID, stale); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("stale prompt response should fail, err=%v", err)
	}
	valid := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"prompt_response","payload":{"prompt_id":"active","action":"cancel"}}`)
	if err := manager.ProtocolCommand(meta.ID, valid); err != nil {
		t.Fatal(err)
	}
	state, err := manager.ProtocolState(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Prompt) != 0 {
		t.Fatal("accepted prompt response must clear active prompt")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, _ := manager.Metadata(meta.ID)
		if current.Status != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}
