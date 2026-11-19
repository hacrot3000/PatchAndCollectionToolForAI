package session

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProtocolItemLifecycleStateIsBoundedAndUpserted(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"run_started","seq":1}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"item_started","seq":2,"index":2,"total":3,"name":"b.zip","kind":"PATCH","started_at":"now"}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"item_started","seq":3,"index":1,"total":3,"name":"a.zip","kind":"PATCH","started_at":"now"}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"item_finished","seq":4,"index":1,"total":3,"name":"a.zip","kind":"PATCH","status":"PASS","rc":0,"started_at":"now","elapsed_seconds":1.25}`))

	state := cloneProtocolState(s.protocol)
	if len(state.Items) != 2 {
		t.Fatalf("items=%#v", state.Items)
	}
	if state.Items[0].Index != 1 || state.Items[0].Status != "PASS" || state.Items[0].RC == nil || *state.Items[0].RC != 0 {
		t.Fatalf("finished item not retained: %#v", state.Items[0])
	}
	if state.Items[1].Index != 2 || state.Items[1].Status != "RUNNING" {
		t.Fatalf("running item not retained: %#v", state.Items[1])
	}
}

func TestProtocolItemLifecycleRejectsUnboundedIndex(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"item_started","seq":1,"index":4097,"total":4097,"name":"x.zip","kind":"PATCH"}`))
	if len(s.protocol.Items) != 0 || s.protocol.Error == "" {
		t.Fatalf("unbounded item event accepted: %#v", s.protocol)
	}
}


func TestProtocolItemLifecycleCarriesBoundedFailureEvidence(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"item_finished","seq":1,"index":1,"total":1,"name":"collect.zip","kind":"COLLECT","status":"FAIL","rc":2,"diagnosis_kind":"collect_failed","failure_reason":"expected source missing","repair_hint":"Check request action[2].path","discovery_detail":"schema_error:request.actions[2].path","output_tail":"line one\nline two"}`))

	state := cloneProtocolState(s.protocol)
	if len(state.Items) != 1 {
		t.Fatalf("items=%#v error=%q", state.Items, state.Error)
	}
	item := state.Items[0]
	if item.DiagnosisKind != "collect_failed" || item.FailureReason != "expected source missing" ||
        item.RepairHint != "Check request action[2].path" ||
        item.DiscoveryDetail != "schema_error:request.actions[2].path" ||
        item.OutputTail != "line one\nline two" {
		t.Fatalf("failure evidence not retained: %#v", item)
	}
}

func TestProtocolItemLifecycleRejectsOversizedFailureEvidence(t *testing.T) {
	payload := `{"type":"item_finished","index":1,"total":1,"name":"x.zip","kind":"PATCH","status":"FAIL","rc":2,"output_tail":"` +
		strings.Repeat("x", 16385) + `"}`
	if _, _, err := protocolItemEvent([]byte(payload)); err == nil {
		t.Fatal("oversized item failure evidence accepted")
	}
}

func TestProtocolItemRejectsOversizedRepairHintAndDiscoveryDetail(t *testing.T) {
    for _, key := range []string{"repair_hint", "discovery_detail"} {
        payload := `{"type":"item_finished","index":1,"total":1,"name":"invalid.zip","kind":"COLLECT INVALID","status":"FAIL","rc":2,"` +
            key + `":"` + strings.Repeat("x", 4097) + `"}`
        if _, _, err := protocolItemEvent([]byte(payload)); err == nil {
            t.Fatalf("%s oversized evidence accepted", key)
        }
    }
}
func TestProtocolItemStartedDoesNotInheritFailureAdvice(t *testing.T) {
    payload := []byte(`{"type":"item_started","index":1,"total":1,"name":"collect.zip","kind":"COLLECT","repair_hint":"ignore me","discovery_detail":"old data"}`)
    item, _, err := protocolItemEvent(payload)
    if err != nil { t.Fatal(err) }
    if item.RepairHint != "" || item.DiscoveryDetail != "" {
        t.Fatalf("started item leaked old failure evidence: %+v", item)
    }
}


func TestProtocolItemPreservesCommandConsoleEvidence(t *testing.T) {
	payload := []byte(`{"type":"item_finished","index":1,"total":1,"name":"commands.zip","kind":"PATCH","status":"PASS","rc":0,"commands":[{"index":1,"name":"Compile project","stage":"POST PATCH","argv":["bash","scripts/build.sh"],"status":"PASS","rc":0,"log_path":"artifacts/patch_tool/runs/20261118a/command_items/001_commands_zip/command_logs/001_post_patch.log"}]}`)
	item, event, err := protocolItemEvent(payload)
	if err != nil {
		t.Fatal(err)
	}
	if event != "item_finished" || len(item.Commands) != 1 ||
		item.Commands[0].Name != "Compile project" || item.Commands[0].RC == nil ||
		*item.Commands[0].RC != 0 {
		t.Fatalf("command evidence lost: %#v", item)
	}
}

func TestProtocolItemRejectsUnsafeCommandConsolePaths(t *testing.T) {
	for _, path := range []string{
		"../secrets.log",
		"/etc/passwd",
		"artifacts/patch_tool/runs/run/command_items/../secrets.log",
		"artifacts/patch_tool/runs/run/items/output.log",
	} {
		payload := map[string]interface{}{
			"type":"item_finished", "index":1, "total":1,
			"name":"bad.zip", "kind":"PATCH", "status":"PASS",
			"commands":[]map[string]interface{}{{
				"index":1, "name":"command", "stage":"POST PATCH",
				"status":"PASS", "log_path":path,
			}},
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := protocolItemEvent(data); err == nil {
			t.Fatalf("unsafe command console log path accepted: %q", path)
		}
	}
}


func TestProtocolItemLiveCommandQueueRunningDoneAndSkipped(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	lines := []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"run_started","seq":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"item_started","seq":2,"index":1,"total":1,"name":"build.zip","kind":"PATCH"}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"command_state","seq":3,"item_index":1,"item_name":"build.zip","command":{"index":1,"name":"Compile","stage":"POST PATCH","argv":["bash","build.sh"],"status":"QUEUE"}}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"command_state","seq":4,"item_index":1,"item_name":"build.zip","command":{"index":2,"name":"Upload","stage":"POST PATCH","argv":["./upload.sh"],"status":"QUEUE"}}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"command_state","seq":5,"item_index":1,"item_name":"build.zip","command":{"index":1,"name":"Compile","stage":"POST PATCH","argv":["bash","build.sh"],"status":"RUNNING","started_at":"2026-11-19T10:00:00Z","log_path":"artifacts/patch_tool/runs/test/command_items/001_build/command_logs/001_post_patch.log"}}`,
	}
	for _, line := range lines {
		s.applyProtocolLine([]byte(line))
	}
	current := cloneProtocolState(s.protocol)
	if len(current.Items) != 1 || len(current.Items[0].Commands) != 2 ||
		current.Items[0].Commands[0].Status != "RUNNING" || current.Items[0].Commands[1].Status != "QUEUE" {
		t.Fatalf("live nested command lifecycle missing: %#v", current.Items)
	}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"command_state","seq":6,"item_index":1,"item_name":"build.zip","command":{"index":1,"name":"Compile","stage":"POST PATCH","status":"PASS","rc":0,"started_at":"2026-11-19T10:00:00Z","elapsed_seconds":62.5,"log_path":"artifacts/patch_tool/runs/test/command_items/001_build/command_logs/001_post_patch.log"}}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"item_finished","seq":7,"index":1,"total":1,"name":"build.zip","kind":"PATCH","status":"FAIL","rc":2}`))
	current = cloneProtocolState(s.protocol)
	if len(current.Items) != 1 || len(current.Items[0].Commands) != 2 ||
		current.Items[0].Commands[0].Status != "PASS" ||
		current.Items[0].Commands[0].ElapsedSeconds == nil ||
		*current.Items[0].Commands[0].ElapsedSeconds != 62.5 ||
		current.Items[0].Commands[1].Status != "SKIPPED" {
		t.Fatalf("completed command evidence was lost on parent finish: %#v", current.Items)
	}
}

func TestProtocolItemCommandStateRejectsInvalidEvent(t *testing.T) {
	for _, command := range []string{
		`{"index":0,"name":"Bad","status":"QUEUE"}`,
		`{"index":1,"name":"Bad","status":"EXECUTING"}`,
		`{"index":1,"name":"Bad","status":"RUNNING","log_path":"../../token"}`,
		`{"index":65,"name":"Bad","status":"QUEUE"}`,
		`{"index":1,"name":"Bad","status":"RUNNING","elapsed_seconds":-3}`,
	} {
		event := []byte(`{"type":"command_state","item_index":1,"item_name":"a.zip","command":` + command + "}")
		if _, _, _, err := protocolCommandStateEvent(event); err == nil {
			t.Fatalf("invalid command state was accepted: %s", event)
		}
	}
}


func TestProtocolItemPreservesRunIDForCommandJournalRecovery(t *testing.T) {
	item, event, err := protocolItemEvent([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"item_started","index":1,"total":2,"run_id":"1795061800896644_898883","name":"example.zip","kind":"PATCH"}`))
	if err != nil || event != "item_started" || item.RunID != "1795061800896644_898883" {
		t.Fatalf("run_id needed for live journal was dropped: %#v %q %v", item, event, err)
	}
	for _, invalid := range []string{"../escape", "/etc/passwd", "bad\\path", "bad.name"} {
		data := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"item_started","index":1,"total":1,"run_id":` + `"` + invalid + `"` + `,"name":"example.zip","kind":"PATCH"}`)
		if _, _, err := protocolItemEvent(data); err == nil {
			t.Fatalf("unsafe run ID accepted: %q", invalid)
		}
	}
}
