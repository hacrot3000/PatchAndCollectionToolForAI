package session

import (
	"os"
	"strings"
	"testing"
)

func TestProtocolHistorySupportResultIsValidatedAndRetained(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"history_support_result","seq":2,"prompt_id":"p1","support_id":"s1","run_id":"run-1","item_index":2,"item_name":"demo.zip","status":"PASS","message":"Created support ZIP for demo.zip","artifact":{"label":"Support ZIP","path":"artifacts/support/PTV_SUPPORT_demo.zip","upload_required":false}}`))
	if s.protocol.Error != "" || s.protocol.HistorySupport == nil {
		t.Fatalf("valid History support rejected: %#v", s.protocol)
	}
	got := s.protocol.HistorySupport
	if got.SupportID != "s1" || got.RunID != "run-1" || got.ItemIndex != 2 || got.Status != "PASS" || got.Artifact == nil {
		t.Fatalf("unexpected History support state: %#v", got)
	}
	if got.Artifact.Path != "artifacts/support/PTV_SUPPORT_demo.zip" {
		t.Fatalf("unexpected History support artifact: %#v", got.Artifact)
	}
	clone := cloneProtocolState(s.protocol)
	if clone.HistorySupport == nil || clone.HistorySupport.Artifact == nil || clone.HistorySupport.Artifact.Path != got.Artifact.Path {
		t.Fatal("cloned protocol state lost History support result")
	}
}

func TestProtocolHistorySupportResultRejectsUnsafeOrInconsistentPayload(t *testing.T) {
	for _, line := range []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"history_support_result","seq":1,"prompt_id":"","support_id":"s","run_id":"r","item_index":1,"item_name":"x","status":"FAIL"}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_support_result","seq":1,"prompt_id":"p","support_id":"s","run_id":"r","item_index":0,"item_name":"x","status":"FAIL"}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_support_result","seq":1,"prompt_id":"p","support_id":"s","run_id":"r","item_index":1,"item_name":"x","status":"PASS"}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_support_result","seq":1,"prompt_id":"p","support_id":"s","run_id":"r","item_index":1,"item_name":"x","status":"PASS","artifact":{"label":"Support ZIP","path":"../outside.zip"}}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_support_result","seq":1,"prompt_id":"p","support_id":"s","run_id":"r","item_index":1,"item_name":"x","status":"FAIL","artifact":{"label":"Support ZIP","path":"artifacts/support/x.zip"}}`,
	} {
		s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
		s.applyProtocolLine([]byte(line))
		if s.protocol.HistorySupport != nil || s.protocol.Error == "" {
			t.Fatalf("invalid History support accepted: %s state=%#v", line, s.protocol)
		}
	}
}

func TestProtocolCommandHistorySupportUsesFinalActivePromptGateAndKeepsPrompt(t *testing.T) {
	readFD, writeFD, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	defer readFD.Close()
	defer writeFD.Close()
	manager := NewManager(1 << 20)
	prompt := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"active","prompt_kind":"history_action","constraints":{"item_actions":["support"]}}`)
	s := &managedSession{
		meta: Metadata{ID:"history-support-test", Status:"running"},
		protocol: ProtocolState{Available:true, Enabled:true, CommandsEnabled:true, Prompt:append([]byte(nil), prompt...)},
		protocolCommand: writeFD,
	}
	manager.sessions["history-support-test"] = s

	stale := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"history_support","payload":{"prompt_id":"stale","support_id":"s1","run_id":"run-1","item_index":1}}`)
	if err := manager.ProtocolCommand("history-support-test", stale); err == nil || !strings.Contains(err.Error(), "active prompt") {
		t.Fatalf("stale history_support should fail closed, err=%v", err)
	}
	current := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"history_support","payload":{"prompt_id":"active","support_id":"s2","run_id":"run-1","item_index":1}}`)
	if err := manager.ProtocolCommand("history-support-test", current); err != nil { t.Fatal(err) }
	buf := make([]byte, 4096)
	n, err := readFD.Read(buf)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(buf[:n]), `"command":"history_support"`) {
		t.Fatalf("history_support was not written to FD4: %q", buf[:n])
	}
	state, err := manager.ProtocolState("history-support-test")
	if err != nil { t.Fatal(err) }
	if len(state.Prompt) == 0 {
		t.Fatal("history_support must not consume the active History prompt")
	}
	if err := manager.ProtocolCommand("history-support-test", current); err != nil {
		t.Fatalf("second History support request should remain allowed: %v", err)
	}
}
