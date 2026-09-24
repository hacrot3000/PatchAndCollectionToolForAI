package session

import "testing"

func TestProtocolHealthSnapshotRetainsTypedBoundedState(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"health_snapshot","seq":2,"status":"WARN","tool_version":"6.20.2","summary":{"pass":2,"warn":1,"fail":0,"total":3},"checks":[{"name":"version","status":"PASS","detail":"6.20.2"},{"name":"sha256sums","status":"PASS","entries":41,"failures":0,"missing_managed":0,"stale_managed":0},{"name":"python_cache_hygiene","status":"WARN","files":1,"dirs":1}],"warnings":["Python cache artifacts present"],"errors":[],"ignored_internal":{"x":1}}`))
	if s.protocol.Error != "" || s.protocol.HealthSnapshot == nil {
		t.Fatalf("valid Health snapshot rejected: %#v", s.protocol)
	}
	got := s.protocol.HealthSnapshot
	if got.Status != "WARN" || got.ToolVersion != "6.20.2" || got.Summary.Total != 3 ||
		len(got.Checks) != 3 || got.Checks[1].Entries == nil || *got.Checks[1].Entries != 41 ||
		len(got.Warnings) != 1 || len(got.Errors) != 0 {
		t.Fatalf("unexpected Health state: %#v", got)
	}
}

func TestProtocolHealthSnapshotRejectsInvalidState(t *testing.T) {
	cases := []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"health_snapshot","seq":1,"status":"OTHER","tool_version":"6.20.2","summary":{"pass":0,"warn":0,"fail":0,"total":0}}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"health_snapshot","seq":1,"status":"PASS","tool_version":"","summary":{"pass":0,"warn":0,"fail":0,"total":0}}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"health_snapshot","seq":1,"status":"PASS","tool_version":"6.20.2","summary":{"pass":0,"warn":0,"fail":0,"total":1},"checks":[]}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"health_snapshot","seq":1,"status":"PASS","tool_version":"6.20.2","summary":{"pass":1,"warn":0,"fail":0,"total":1},"checks":[{"name":"x","status":"PASS","entries":-1}]}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"health_snapshot","seq":1,"status":"WARN","tool_version":"6.20.2","summary":{"pass":0,"warn":1,"fail":0,"total":1},"checks":[{"name":"x","status":"WARN"}],"warnings":[""]}`,
	}
	for _, line := range cases {
		s := &managedSession{protocol: ProtocolState{Available: true, Enabled: true}}
		s.applyProtocolLine([]byte(line))
		if s.protocol.HealthSnapshot != nil || s.protocol.Error == "" {
			t.Fatalf("invalid Health snapshot accepted: %s state=%#v", line, s.protocol)
		}
	}
}

func TestProtocolHealthSnapshotCloneIsIndependent(t *testing.T) {
	entryCount := 7
	s := &managedSession{protocol: ProtocolState{
		HealthSnapshot: &ProtocolHealthSnapshotState{
			Status: "PASS",
			ToolVersion: "6.20.2",
			Summary: ProtocolHealthSummaryState{Pass:1, Total:1},
			Checks: []ProtocolHealthCheckState{{Name:"sha256sums",Status:"PASS",Entries:&entryCount}},
			Warnings: []string{"one"},
			Errors: []string{"two"},
		},
	}}
	clone := cloneProtocolState(s.protocol)
	clone.HealthSnapshot.Checks[0].Name = "changed"
	clone.HealthSnapshot.Warnings[0] = "changed"
	clone.HealthSnapshot.Errors[0] = "changed"
	if s.protocol.HealthSnapshot.Checks[0].Name != "sha256sums" ||
		s.protocol.HealthSnapshot.Warnings[0] != "one" ||
		s.protocol.HealthSnapshot.Errors[0] != "two" {
		t.Fatalf("Health clone mutated source: %#v", s.protocol.HealthSnapshot)
	}
}
