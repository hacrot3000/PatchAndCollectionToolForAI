package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestProtocolStateRetainsLatestValidatedActionResult(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true, CommandsEnabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":1,"prompt_id":"p1","prompt_kind":"queue_selection"}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"action_result","seq":2,"prompt_id":"p1","action_id":"a1","action":"preview","index":1,"item_name":"demo.zip","item_kind":"PATCH","status":"PASS","rc":0,"timed_out":false,"elapsed_seconds":1.5,"output":"ok\n","output_truncated":false}`))
	if s.protocol.Error != "" || s.protocol.ActionResult == nil {
		t.Fatalf("valid action result rejected: %#v", s.protocol)
	}
	got := s.protocol.ActionResult
	if got.ActionID != "a1" || got.Action != "preview" || got.Status != "PASS" || got.RC != 0 || got.Output != "ok\n" {
		t.Fatalf("unexpected action result: %#v", got)
	}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":3,"prompt_id":"p2","prompt_kind":"queue_selection"}`))
	if s.protocol.ActionResult != nil {
		t.Fatal("new prompt must clear stale native action result")
	}
}

func TestProtocolStateRejectsInvalidActionResult(t *testing.T) {
	for _, line := range []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"action_result","seq":1,"prompt_id":"","action_id":"a","action":"preview","index":1,"item_name":"x","item_kind":"PATCH","status":"PASS","rc":0,"elapsed_seconds":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"action_result","seq":1,"prompt_id":"p","action_id":"a","action":"execute","index":1,"item_name":"x","item_kind":"PATCH","status":"PASS","rc":0,"elapsed_seconds":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"action_result","seq":1,"prompt_id":"p","action_id":"a","action":"preview","index":0,"item_name":"x","item_kind":"PATCH","status":"PASS","rc":0,"elapsed_seconds":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"action_result","seq":1,"prompt_id":"p","action_id":"a","action":"preview","index":1,"item_name":"x","item_kind":"PATCH","status":"UNKNOWN","rc":0,"elapsed_seconds":1}`,
	} {
		s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
		s.applyProtocolLine([]byte(line))
		if s.protocol.ActionResult != nil || s.protocol.Error == "" {
			t.Fatalf("invalid action result accepted: %s state=%#v", line, s.protocol)
		}
	}
}

func TestProtocolCommandRejectsStaleItemActionWithoutClearingPrompt(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil { t.Skip(err) }
	manager := NewManager(1 << 20)
	meta, err := manager.Start(tasks.Execution{TaskID: -1, Label: "item action prompt binding", Command: "/bin/sh", Args: []string{"-c", "IFS= read -r line <&4; sleep 0.2"}, Cwd: t.TempDir(), Env: os.Environ(), ProtocolEvents: true, ProtocolCommands: true})
	if err != nil { t.Fatal(err) }
	s, ok := manager.Get(meta.ID)
	if !ok { t.Fatal("session missing") }
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":4,"prompt_id":"active","prompt_kind":"queue_selection"}`))
	stale := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"item_action","payload":{"prompt_id":"stale","action_id":"a1","action":"preview","index":1}}`)
	if err := manager.ProtocolCommand(meta.ID, stale); err == nil || !strings.Contains(err.Error(), "does not match") { t.Fatalf("stale item action should fail, err=%v", err) }
	valid := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"item_action","payload":{"prompt_id":"active","action_id":"a2","action":"preview","index":1}}`)
	if err := manager.ProtocolCommand(meta.ID, valid); err != nil { t.Fatal(err) }
	state, err := manager.ProtocolState(meta.ID)
	if err != nil { t.Fatal(err) }
	if len(state.Prompt) == 0 { t.Fatal("item_action must keep the active queue prompt") }
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) { current, _ := manager.Metadata(meta.ID); if current.Status != "running" { break }; time.Sleep(10 * time.Millisecond) }
}
