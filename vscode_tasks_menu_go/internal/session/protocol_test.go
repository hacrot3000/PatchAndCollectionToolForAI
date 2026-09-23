package session

import (
	"encoding/json"
	"testing"
)

func TestManagedSessionProtocolStateKeepsLatestViews(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"hello","seq":1}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"queue_snapshot","seq":2,"status":"empty","items":[],"total":0}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"run_started","seq":3}`))

	state := cloneProtocolState(s.protocol)
	if !state.Available || !state.Enabled || state.EventCount != 3 || state.LastSeq != 3 {
		t.Fatalf("unexpected protocol state: %#v", state)
	}
	var queue map[string]any
	if err := json.Unmarshal(state.QueueSnapshot, &queue); err != nil {
		t.Fatal(err)
	}
	if queue["type"] != "queue_snapshot" || queue["status"] != "empty" {
		t.Fatalf("unexpected queue snapshot: %#v", queue)
	}
	var last map[string]any
	if err := json.Unmarshal(state.LastEvent, &last); err != nil {
		t.Fatal(err)
	}
	if last["type"] != "run_started" {
		t.Fatalf("unexpected last event: %#v", last)
	}
}

func TestManagedSessionProtocolRejectsInvalidEnvelope(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"other","version":1,"type":"queue_snapshot","seq":1}`))
	if s.protocol.EventCount != 0 || s.protocol.Error == "" {
		t.Fatalf("invalid envelope was accepted: %#v", s.protocol)
	}
}
