package session

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestProtocolStateRetainsResumeSnapshot(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"resume_snapshot","seq":4,"status":"available","summary":{"failed":1},"items":[],"failed_items":[],"actions":[]}`))
	if s.protocol.Error != "" {
		t.Fatalf("resume snapshot rejected: %s", s.protocol.Error)
	}
	if len(s.protocol.ResumeSnapshot) == 0 {
		t.Fatal("resume snapshot not retained")
	}
	var snapshot map[string]any
	if err := json.Unmarshal(s.protocol.ResumeSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["type"] != "resume_snapshot" || snapshot["status"] != "available" {
		t.Fatalf("unexpected resume snapshot: %#v", snapshot)
	}
	clone := cloneProtocolState(s.protocol)
	if len(clone.ResumeSnapshot) == 0 {
		t.Fatal("cloned protocol state lost resume snapshot")
	}
}


func TestResumeActionCommandIsPromptBoundAndConsumedAtFinalWriteGate(t *testing.T) {
	readFD, writeFD, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	defer readFD.Close()
	defer writeFD.Close()

	id := "resume-session"
	manager := NewManager(1 << 20)
	manager.sessions[id] = &managedSession{
		meta: Metadata{ID:id, Status:"running"},
		protocol: ProtocolState{
			Available:true,
			Enabled:true,
			CommandsEnabled:true,
			Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":1,"prompt_id":"resume123","prompt_kind":"resume_action"}`),
		},
		protocolCommand: writeFD,
	}
	command := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"resume_action","payload":{"prompt_id":"resume123","action":"all"}}`)
	if err := manager.ProtocolCommand(id, command); err != nil { t.Fatal(err) }
	if len(manager.sessions[id].protocol.Prompt) != 0 {
		t.Fatal("resume_action must consume the active prompt after a successful FD4 write")
	}
	if err := manager.ProtocolCommand(id, command); err == nil || !strings.Contains(err.Error(), "no active prompt") {
		t.Fatalf("double Resume action must fail at final write gate, err=%v", err)
	}
	buf := make([]byte, 4096)
	n, err := readFD.Read(buf)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(buf[:n]), `"command":"resume_action"`) {
		t.Fatalf("FD4 did not receive Resume command: %q", buf[:n])
	}
}

func TestResumeActionCommandRejectsStalePromptAtFinalWriteGate(t *testing.T) {
	readFD, writeFD, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	defer readFD.Close()
	defer writeFD.Close()
	id := "resume-stale"
	manager := NewManager(1 << 20)
	manager.sessions[id] = &managedSession{
		meta: Metadata{ID:id, Status:"running"},
		protocol: ProtocolState{
			Available:true, Enabled:true, CommandsEnabled:true,
			Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":1,"prompt_id":"current","prompt_kind":"resume_action"}`),
		},
		protocolCommand: writeFD,
	}
	command := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"resume_action","payload":{"prompt_id":"stale","action":"all"}}`)
	if err := manager.ProtocolCommand(id, command); err == nil || !strings.Contains(err.Error(), "does not match the active prompt") {
		t.Fatalf("stale Resume action accepted: %v", err)
	}
}
