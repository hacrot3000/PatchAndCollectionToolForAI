package session

import (
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
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"item_finished","seq":1,"index":1,"total":1,"name":"collect.zip","kind":"COLLECT","status":"FAIL","rc":2,"diagnosis_kind":"collect_failed","failure_reason":"expected source missing","output_tail":"line one\nline two"}`))

	state := cloneProtocolState(s.protocol)
	if len(state.Items) != 1 {
		t.Fatalf("items=%#v error=%q", state.Items, state.Error)
	}
	item := state.Items[0]
	if item.DiagnosisKind != "collect_failed" || item.FailureReason != "expected source missing" || item.OutputTail != "line one\nline two" {
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
