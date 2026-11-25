package agentrun

import "testing"

func TestRunFenceDigestStableAndNamespaced(t *testing.T) {
	a := runFenceDigest("run-1")
	if len(a) != 32 || a == "" {
		t.Fatalf("unexpected digest: %q", a)
	}
	if a != runFenceDigest("run-1") {
		t.Fatal("digest is not stable")
	}
	if a == runFenceDigest("run-2") {
		t.Fatal("different runs must have different fence identities")
	}
	if runFenceDigest("") != "" {
		t.Fatal("empty run ID must have no fence identity")
	}
}

func TestRunFenceAdmissionFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		lease  bool
		admit  bool
		reason string
	}{
		{"queued free", Queued, false, true, ""},
		{"queued lease held", Queued, true, false, "lease_held"},
		{"running", Running, false, false, "active_state_requires_review"},
		{"running leased", Running, true, false, "active_state_requires_review"},
		{"waiting", WaitingTool, false, false, "active_state_requires_review"},
		{"interrupted", Interrupted, false, false, "uncertain_state_requires_review"},
		{"failed", Failed, false, false, "uncertain_state_requires_review"},
		{"completed", Completed, false, false, "terminal"},
		{"stopped", Stopped, false, false, "terminal"},
		{"unknown", Status("mystery"), false, false, "unknown_status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			admit, reason := RunFenceAdmission(tc.status, tc.lease)
			if admit != tc.admit || reason != tc.reason {
				t.Fatalf("got admit=%v reason=%q, want %v/%q", admit, reason, tc.admit, tc.reason)
			}
		})
	}
}
