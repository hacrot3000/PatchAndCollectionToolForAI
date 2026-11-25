package agentchat

import "testing"

func egressStateTestSession(phase string) Session {
	if phase == "" {
		return Session{}
	}
	return Session{ProviderRun: &ProviderRun{ID: "run-1", Kind: "message", Phase: phase, PayloadSHA256: "abc"}}
}

func TestClassifyRunEgressStates(t *testing.T) {
	cases := []struct {
		phase string
		want  RunEgressState
	}{
		{"", RunEgressNone},
		{"prepared", RunEgressPrepared},
		{"sending", RunEgressSending},
		{"unknown", RunEgressUnknown},
		{"completed", RunEgressCompleted},
		{"acknowledged", RunEgressCompleted},
		{"blocked", RunEgressBlocked},
		{"unrecognized-phase", RunEgressUnknown},
	}
	for _, tc := range cases {
		got := ClassifyRunEgress(egressStateTestSession(tc.phase))
		if got.State != tc.want {
			t.Fatalf("phase %q: got state %s, want %s", tc.phase, got.State, tc.want)
		}
	}
}

func TestClassifyRunEgressFailClosed(t *testing.T) {
	for _, phase := range []string{"prepared", "sending", "unknown", "completed", "blocked"} {
		got := ClassifyRunEgress(egressStateTestSession(phase))
		if got.AutomaticReplayAllowed {
			t.Fatalf("phase %q granted automatic replay", phase)
		}
	}
	for _, phase := range []string{"sending", "unknown"} {
		got := ClassifyRunEgress(egressStateTestSession(phase))
		if !got.RequiresReconciliation {
			t.Fatalf("phase %q did not require reconciliation", phase)
		}
		if got.Guidance == "" {
			t.Fatalf("phase %q had no reconciliation guidance", phase)
		}
	}
	if got := ClassifyRunEgress(egressStateTestSession("prepared")); got.RequiresReconciliation {
		t.Fatal("provably-unsent PREPARED must not require SENDING/UNKNOWN reconciliation")
	}
	if got := ClassifyRunEgress(egressStateTestSession("")); got.RequiresReconciliation {
		t.Fatal("no durable egress marker must not require reconciliation")
	}
}
