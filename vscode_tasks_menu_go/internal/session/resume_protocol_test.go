package session

import (
	"encoding/json"
	"testing"
)

func TestProtocolStateRetainsResumeSnapshot(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"resume_snapshot","seq":4,"status":"available","summary":{"failed":1},"items":[],"failed_items":[],"actions":[]}`))
	if s.protocol.Error != "" {
		t.Fatalf("resume snapshot rejected: %s", s.protocol.Error)
	}
	if len(s.protocol.ResumeSnapshot) == 0 {
		t.Fatal("resume snapshot not retained")
	}
	var snapshot map[string]any
	if err := json.Unmarshal(s.protocol.ResumeSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["type"] != "resume_snapshot" || snapshot["status"] != "available" {
		t.Fatalf("unexpected resume snapshot: %#v", snapshot)
	}
	clone := cloneProtocolState(s.protocol)
	if len(clone.ResumeSnapshot) == 0 {
		t.Fatal("cloned protocol state lost resume snapshot")
	}
}
