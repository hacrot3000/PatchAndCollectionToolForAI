package session

import (
	"os"
	"strings"
	"testing"
)

func TestProtocolHistoryCleanupResultIsValidatedRetainedAndCloned(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":2,"prompt_id":"p1","cleanup_id":"c1","status":"PASS","rc":0,"history_changed":true,"removed":3,"pinned":2,"remaining":30,"policy":"remove_unpinned_idle_then_oldest_unpinned_over_limit","eligible_before":4,"message":"History cleanup removed 3 entries"}`))
	if s.protocol.Error != "" || s.protocol.HistoryCleanup == nil {
		t.Fatalf("valid History cleanup rejected: %#v", s.protocol)
	}
	got := s.protocol.HistoryCleanup
	if got.CleanupID!="c1" || got.Removed!=3 || got.Pinned!=2 || got.Remaining!=30 || !got.HistoryChanged {
		t.Fatalf("unexpected History cleanup state: %#v", got)
	}
	clone := cloneProtocolState(s.protocol)
	if clone.HistoryCleanup == nil || clone.HistoryCleanup.CleanupID != "c1" {
		t.Fatal("cloned protocol state lost History cleanup result")
	}
}

func TestProtocolHistoryCleanupResultRejectsInconsistentPayload(t *testing.T) {
	for _, line := range []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"","cleanup_id":"c","status":"PASS","rc":0,"removed":0,"pinned":0,"remaining":0,"policy":"remove_unpinned_idle_then_oldest_unpinned_over_limit","eligible_before":0}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"p","cleanup_id":"c","status":"PASS","rc":0,"history_changed":false,"removed":2,"pinned":0,"remaining":0,"policy":"remove_unpinned_idle_then_oldest_unpinned_over_limit","eligible_before":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"p","cleanup_id":"c","status":"PASS","rc":0,"history_changed":true,"removed":1,"pinned":0,"remaining":0,"policy":"other","eligible_before":1}`,
	} {
		s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
		s.applyProtocolLine([]byte(line))
		if s.protocol.HistoryCleanup != nil || s.protocol.Error == "" {
			t.Fatalf("invalid History cleanup accepted: %s state=%#v", line, s.protocol)
		}
	}
}

func TestProtocolCommandHistoryCleanupUsesFinalActivePromptGateAndKeepsPrompt(t *testing.T) {
	readFD, writeFD, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	defer readFD.Close()
	defer writeFD.Close()
	manager := NewManager(1 << 20)
	prompt := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"active","prompt_kind":"history_action","actions":["cleanup"],"constraints":{"destructive_actions":["cleanup"],"cleanup":{"eligible":2,"policy":"remove_unpinned_idle_then_oldest_unpinned_over_limit"}}}`)
	s := &managedSession{
		meta: Metadata{ID:"history-cleanup-test", Status:"running"},
		protocol: ProtocolState{Available:true, Enabled:true, CommandsEnabled:true, Prompt:append([]byte(nil), prompt...)},
		protocolCommand: writeFD,
	}
	manager.sessions["history-cleanup-test"] = s

	stale := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"history_cleanup","payload":{"prompt_id":"stale","cleanup_id":"c1","confirmed":true}}`)
	if err := manager.ProtocolCommand("history-cleanup-test", stale); err == nil || !strings.Contains(err.Error(), "active prompt") {
		t.Fatalf("stale history_cleanup should fail closed, err=%v", err)
	}
	current := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"history_cleanup","payload":{"prompt_id":"active","cleanup_id":"c2","confirmed":true}}`)
	if err := manager.ProtocolCommand("history-cleanup-test", current); err != nil { t.Fatal(err) }
	buf := make([]byte, 4096)
	n, err := readFD.Read(buf)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(buf[:n]), `"command":"history_cleanup"`) {
		t.Fatalf("history_cleanup was not written to FD4: %q", buf[:n])
	}
	state, err := manager.ProtocolState("history-cleanup-test")
	if err != nil { t.Fatal(err) }
	if len(state.Prompt) == 0 {
		t.Fatal("history_cleanup must keep current prompt until Python refreshes it")
	}
}
