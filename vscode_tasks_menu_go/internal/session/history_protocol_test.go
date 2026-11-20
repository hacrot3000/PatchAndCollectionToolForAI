package session

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestProtocolStateRetainsHistorySnapshotAndLatestReport(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"history_snapshot","seq":1,"status":"available","runs":[{"run_id":"run-1"}],"total":1}`))
	if len(s.protocol.HistorySnapshot) == 0 || s.protocol.Error != "" {
		t.Fatalf("history snapshot not retained: %#v", s.protocol)
	}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":2,"prompt_id":"history123","prompt_kind":"history_action","actions":["detail"],"runs":[{"run_id":"run-1"}]}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"history_report","seq":3,"prompt_id":"history123","status":"available","run":{"run_id":"run-1"},"items":[]}`))
	if len(s.protocol.HistoryReport) == 0 {
		t.Fatal("history report not retained")
	}
	clone := cloneProtocolState(s.protocol)
	if len(clone.HistorySnapshot) == 0 || len(clone.HistoryReport) == 0 {
		t.Fatal("cloned protocol state lost History projection data")
	}
}

func TestHistoryDetailCommandIsPromptBoundAndDoesNotConsumePrompt(t *testing.T) {
	readFD, writeFD, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	defer readFD.Close()
	defer writeFD.Close()
	id := "history-session"
	manager := NewManager(1 << 20)
	manager.sessions[id] = &managedSession{
		meta: Metadata{ID:id, Status:"running"},
		protocol: ProtocolState{
			Available:true, Enabled:true, CommandsEnabled:true,
			Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":1,"prompt_id":"history123","prompt_kind":"history_action"}`),
		},
		protocolCommand: writeFD,
	}
	command := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"history_detail","payload":{"prompt_id":"history123","run_id":"run-1"}}`)
	if err := manager.ProtocolCommand(id, command); err != nil { t.Fatal(err) }
	if len(manager.sessions[id].protocol.Prompt) == 0 {
		t.Fatal("history_detail must keep the active History prompt for further read-only requests")
	}
	if err := manager.ProtocolCommand(id, command); err != nil {
		t.Fatalf("second History detail request should remain allowed: %v", err)
	}
	buf := make([]byte, 8192)
	n, err := readFD.Read(buf)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(buf[:n]), `"command":"history_detail"`) {
		t.Fatalf("FD4 did not receive History detail command: %q", buf[:n])
	}
}

func TestHistoryDetailCommandRejectsStalePromptAtFinalWriteGate(t *testing.T) {
	readFD, writeFD, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	defer readFD.Close()
	defer writeFD.Close()
	id := "history-stale"
	manager := NewManager(1 << 20)
	manager.sessions[id] = &managedSession{
		meta: Metadata{ID:id, Status:"running"},
		protocol: ProtocolState{
			Available:true, Enabled:true, CommandsEnabled:true,
			Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":1,"prompt_id":"current","prompt_kind":"history_action"}`),
		},
		protocolCommand: writeFD,
	}
	command := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"history_detail","payload":{"prompt_id":"stale","run_id":"run-1"}}`)
	if err := manager.ProtocolCommand(id, command); err == nil || !strings.Contains(err.Error(), "does not match the active prompt") {
		t.Fatalf("stale History detail accepted: %v", err)
	}
}
