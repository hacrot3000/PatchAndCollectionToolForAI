package session

import "testing"

func TestProtocolProgressStateKeepsLatestValidatedEvent(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"progress","seq":2,"scope":"collect","run_id":"run-1","index":1,"total":1,"item_name":"request.zip","item_kind":"COLLECT","phase":"search","status":"RUNNING","elapsed_seconds":1.25,"output_lines":7,"detail":"searching files"}`))
	if s.protocol.Error != "" || s.protocol.Progress == nil {
		t.Fatalf("valid progress rejected: %#v", s.protocol)
	}
	if s.protocol.Progress.Phase != "search" || s.protocol.Progress.Status != "RUNNING" || s.protocol.Progress.OutputLines != 7 {
		t.Fatalf("unexpected progress: %#v", s.protocol.Progress)
	}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"progress","seq":3,"scope":"collect","run_id":"run-1","index":1,"total":1,"item_name":"request.zip","item_kind":"COLLECT","phase":"zip","status":"PASS","elapsed_seconds":2.5,"output_lines":12,"detail":"done"}`))
	if s.protocol.Progress == nil || s.protocol.Progress.Phase != "zip" || s.protocol.Progress.Status != "PASS" {
		t.Fatalf("latest progress not retained: %#v", s.protocol.Progress)
	}
}

func TestProtocolProgressStateRejectsInvalidPayload(t *testing.T) {
	for _, line := range []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"progress","seq":1,"scope":"","phase":"search","status":"RUNNING","elapsed_seconds":1,"output_lines":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"progress","seq":1,"scope":"collect","index":2,"total":1,"phase":"search","status":"RUNNING","elapsed_seconds":1,"output_lines":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"progress","seq":1,"scope":"collect","phase":"search","status":"RUNNING","elapsed_seconds":-1,"output_lines":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"progress","seq":1,"scope":"collect","phase":"search","status":"RUNNING","elapsed_seconds":1,"output_lines":-1}`,
	} {
		s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
		s.applyProtocolLine([]byte(line))
		if s.protocol.Progress != nil || s.protocol.Error == "" {
			t.Fatalf("invalid progress accepted: %s state=%#v", line, s.protocol)
		}
	}
}
