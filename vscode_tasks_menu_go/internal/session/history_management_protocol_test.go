package session

import (
	"os"
	"strings"
	"testing"
)

func TestProtocolHistoryManagementResultIsValidatedAndRetained(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"history_management_result","seq":2,"prompt_id":"p1","management_id":"m1","action":"export","run_id":"run-1","status":"PASS","rc":0,"history_changed":false,"message":"Exported run-1","artifact":{"label":"History export","path":"artifacts/patch_tool/exports/PTV_RUN_run-1.zip","upload_required":false}}`))
	if s.protocol.Error != "" || s.protocol.HistoryManagement == nil {
		t.Fatalf("valid History management rejected: %#v", s.protocol)
	}
	got := s.protocol.HistoryManagement
	if got.ManagementID != "m1" || got.Action != "export" || got.Status != "PASS" || got.Artifact == nil {
		t.Fatalf("unexpected History management state: %#v", got)
	}
	if got.Artifact.Path != "artifacts/patch_tool/exports/PTV_RUN_run-1.zip" {
		t.Fatalf("unexpected History export artifact: %#v", got.Artifact)
	}
}

func TestProtocolHistoryManagementResultRejectsUnsafeOrInvalidPayload(t *testing.T) {
	for _, line := range []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"history_management_result","seq":1,"prompt_id":"","management_id":"m","action":"pin","run_id":"r","status":"PASS","rc":0}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_management_result","seq":1,"prompt_id":"p","management_id":"m","action":"move","run_id":"r","status":"PASS","rc":0}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_management_result","seq":1,"prompt_id":"p","management_id":"m","action":"export","run_id":"r","status":"PASS","rc":0,"artifact":{"label":"x","path":"../outside.zip"}}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_management_result","seq":1,"prompt_id":"p","management_id":"m","action":"pin","run_id":"r","status":"PASS","rc":0,"artifact":{"label":"x","path":"artifacts/x.zip"}}`,
	} {
		s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
		s.applyProtocolLine([]byte(line))
		if s.protocol.HistoryManagement != nil || s.protocol.Error == "" {
			t.Fatalf("invalid History management accepted: %s state=%#v", line, s.protocol)
		}
	}
}

func TestProtocolCommandHistoryManageUsesFinalActivePromptGateAndKeepsPrompt(t *testing.T) {
	readFD, writeFD, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	defer readFD.Close()
	defer writeFD.Close()
	manager := NewManager(1 << 20)
	prompt := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"active","prompt_kind":"history_action","actions":["detail","pin"],"runs":[{"run_id":"run-1","actions":["detail","pin"]}]}`)
	s := &managedSession{
		meta: Metadata{ID:"history-manage-test", Status:"running"},
		protocol: ProtocolState{Available:true, Enabled:true, CommandsEnabled:true, Prompt:append([]byte(nil), prompt...)},
		protocolCommand: writeFD,
	}
	manager.sessions["history-manage-test"] = s

	stale := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"history_manage","payload":{"prompt_id":"stale","management_id":"m1","action":"pin","run_id":"run-1"}}`)
	if err := manager.ProtocolCommand("history-manage-test", stale); err == nil || !strings.Contains(err.Error(), "active prompt") {
		t.Fatalf("stale history_manage should fail closed, err=%v", err)
	}
	current := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"history_manage","payload":{"prompt_id":"active","management_id":"m2","action":"pin","run_id":"run-1"}}`)
	if err := manager.ProtocolCommand("history-manage-test", current); err != nil { t.Fatal(err) }
	buf := make([]byte, 4096)
	n, err := readFD.Read(buf)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(buf[:n]), `"command":"history_manage"`) {
		t.Fatalf("history_manage was not written to FD4: %q", buf[:n])
	}
	state, err := manager.ProtocolState("history-manage-test")
	if err != nil { t.Fatal(err) }
	if len(state.Prompt) == 0 {
		t.Fatal("history_manage must keep current prompt until Python refreshes it")
	}
}
