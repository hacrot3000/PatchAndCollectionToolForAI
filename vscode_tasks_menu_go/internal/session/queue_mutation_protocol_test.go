package session

import (
	"os"
	"strings"
	"testing"
)

func TestProtocolQueueMutationResultStateIsValidatedAndRetained(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"queue_mutation_result","seq":2,"prompt_id":"p1","mutation_id":"m1","action":"delete","index":2,"item_name":"b.zip","item_kind":"COLLECT","status":"PASS","message":"Deleted b.zip","remaining":1}`))
	if s.protocol.Error != "" || s.protocol.QueueMutation == nil {
		t.Fatalf("valid queue mutation rejected: %#v", s.protocol)
	}
	got := s.protocol.QueueMutation
	if got.MutationID != "m1" || got.Status != "PASS" || got.Index != 2 || got.Remaining != 1 {
		t.Fatalf("unexpected queue mutation state: %#v", got)
	}
}

func TestProtocolQueueMutationResultRejectsInvalidPayload(t *testing.T) {
	for _, line := range []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"queue_mutation_result","seq":1,"prompt_id":"","mutation_id":"m","action":"delete","index":1,"item_name":"a.zip","item_kind":"PATCH","status":"PASS","remaining":0}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"queue_mutation_result","seq":1,"prompt_id":"p","mutation_id":"m","action":"move","index":1,"item_name":"a.zip","item_kind":"PATCH","status":"PASS","remaining":0}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"queue_mutation_result","seq":1,"prompt_id":"p","mutation_id":"m","action":"delete","index":0,"item_name":"a.zip","item_kind":"PATCH","status":"PASS","remaining":0}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"queue_mutation_result","seq":1,"prompt_id":"p","mutation_id":"m","action":"delete","index":1,"item_name":"a.zip","item_kind":"PATCH","status":"UNKNOWN","remaining":0}`,
	} {
		s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
		s.applyProtocolLine([]byte(line))
		if s.protocol.QueueMutation != nil || s.protocol.Error == "" {
			t.Fatalf("invalid queue mutation accepted: %s state=%#v", line, s.protocol)
		}
	}
}

func TestProtocolCommandQueueDeleteUsesFinalActivePromptGateAndKeepsPrompt(t *testing.T) {
	readFD, writeFD, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	defer readFD.Close()
	defer writeFD.Close()

	manager := NewManager(1 << 20)
	prompt := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":7,"prompt_id":"active","prompt_kind":"queue_selection","queue_actions":["delete"],"items":[{"index":1,"name":"a.zip","kind":"PATCH"}]}`)
	s := &managedSession{
		meta: Metadata{ID:"queue-delete-test", Status:"running"},
		protocol: ProtocolState{Available:true, Enabled:true, CommandsEnabled:true, Prompt:append([]byte(nil), prompt...)},
		protocolCommand: writeFD,
	}
	manager.sessions["queue-delete-test"] = s

	stale := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"queue_delete","payload":{"prompt_id":"stale","mutation_id":"m1","index":1}}`)
	if err := manager.ProtocolCommand("queue-delete-test", stale); err == nil || !strings.Contains(err.Error(), "active prompt") {
		t.Fatalf("stale queue_delete should fail closed, err=%v", err)
	}
	current := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"queue_delete","payload":{"prompt_id":"active","mutation_id":"m2","index":1}}`)
	if err := manager.ProtocolCommand("queue-delete-test", current); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	n, err := readFD.Read(buf)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(buf[:n]), `"command":"queue_delete"`) {
		t.Fatalf("queue_delete was not written to FD4: %q", buf[:n])
	}
	state, err := manager.ProtocolState("queue-delete-test")
	if err != nil { t.Fatal(err) }
	if len(state.Prompt) == 0 {
		t.Fatal("queue_delete must keep the current prompt until Python emits the refreshed prompt")
	}
}
