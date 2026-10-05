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


func TestManagedSessionProtocolRetainsRunFinishedExitCode(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"run_finished","seq":9,"status":"failed","exit_code":2}`))
	state := cloneProtocolState(s.protocol)
	var last struct {
		Type     string `json:"type"`
		Status   string `json:"status"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal(state.LastEvent, &last); err != nil {
		t.Fatal(err)
	}
	if last.Type != "run_finished" || last.Status != "failed" || last.ExitCode != 2 {
		t.Fatalf("unexpected run_finished event: %#v", last)
	}
}


func TestManagedSessionProtocolRetainsRunErrorsUntilNextRun(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"run_started","seq":1}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"error","seq":2,"phase":"spawn","message":"cannot start Patch child"}`))
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"run_finished","seq":3,"status":"failed","exit_code":2}`))

	state := cloneProtocolState(s.protocol)
	if len(state.RunErrors) != 1 || state.RunErrors[0].Phase != "spawn" || state.RunErrors[0].Message != "cannot start Patch child" {
		t.Fatalf("run error evidence was not retained: %#v", state.RunErrors)
	}
	var last map[string]any
	if err := json.Unmarshal(state.LastEvent, &last); err != nil {
		t.Fatal(err)
	}
	if last["type"] != "run_finished" {
		t.Fatalf("run_finished must remain the latest event: %#v", last)
	}

	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"run_started","seq":4}`))
	if len(s.protocol.RunErrors) != 0 {
		t.Fatalf("new run must clear old run errors: %#v", s.protocol.RunErrors)
	}
}

func TestManagedSessionProtocolRejectsInvalidRunError(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"error","seq":1,"phase":"spawn","message":""}`))
	if len(s.protocol.RunErrors) != 0 || s.protocol.Error == "" {
		t.Fatalf("invalid run error event was accepted: %#v", s.protocol)
	}
}
