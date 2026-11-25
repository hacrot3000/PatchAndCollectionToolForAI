package server

import (
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func autoPatchGateSession() agentchat.Session {
	return agentchat.Session{
		Owner:  "local",
		Status: "waiting_tool",
		ToolCalls: []agentchat.ToolCallRecord{{
			CallID: "c1", AgentRunID: "run-1", Name: "taskdeck_submit_patch", Status: "pending",
		}},
	}
}

func autoPatchGatePolicy() agentrun.AutomationSnapshot {
	return agentrun.AutomationSnapshot{AutoPatch: true}
}

func TestRunAutoPatchDispatchRequiresPinnedAndLiveConsent(t *testing.T) {
	x := autoPatchGateSession()
	x.AutoPatch = true
	policy := autoPatchGatePolicy()
	if ok, code := runAutoPatchDispatchAllowed(x, policy, "c1", "run-1", false); !ok || code != "pinned_and_live_consent" {
		t.Fatalf("authorized run patch was blocked: %v %s", ok, code)
	}
	policy.AutoPatch = false
	if ok, code := runAutoPatchDispatchAllowed(x, policy, "c1", "run-1", false); ok || code != "automatic_patch_not_authorized" {
		t.Fatalf("revoked pinned consent still dispatched: %v %s", ok, code)
	}
	policy = autoPatchGatePolicy()
	x.AutoPatch = false
	if ok, code := runAutoPatchDispatchAllowed(x, policy, "c1", "run-1", false); ok || code != "automatic_patch_not_authorized" {
		t.Fatalf("revoked live consent still dispatched: %v %s", ok, code)
	}
}

func TestRunAutoPatchDispatchBlocksInFlightWork(t *testing.T) {
	for _, phase := range []string{"prepared", "sending", "unknown"} {
		x := autoPatchGateSession()
		x.AutoPatch = true
		x.ProviderRun = &agentchat.ProviderRun{Phase: phase}
		if ok, code := runAutoPatchDispatchAllowed(x, autoPatchGatePolicy(), "c1", "run-1", false); ok || code != "provider_work_in_flight" {
			t.Fatalf("provider phase %q allowed dispatch: %v %s", phase, ok, code)
		}
	}
	for _, status := range []string{"queuing", "queued", "dispatching", "running", "unknown"} {
		x := autoPatchGateSession()
		x.AutoPatch = true
		x.ToolCalls = append(x.ToolCalls, agentchat.ToolCallRecord{CallID: "other", AgentRunID: "run-1", Status: status})
		if ok, code := runAutoPatchDispatchAllowed(x, autoPatchGatePolicy(), "c1", "run-1", false); ok || code != "native_work_in_flight" {
			t.Fatalf("native status %q allowed dispatch: %v %s", status, ok, code)
		}
	}
}

func TestRunAutoPatchDispatchFailsClosedOnScope(t *testing.T) {
	base := autoPatchGateSession()
	base.AutoPatch = true
	tests := []struct {
		name      string
		mutate    func(*agentchat.Session)
		shared    bool
		wantCode  string
	}{
		{name: "shared server", shared: true, wantCode: "shared_server"},
		{name: "non-local owner", mutate: func(x *agentchat.Session) { x.Owner = "other" }, wantCode: "owner_scope"},
		{name: "archived", mutate: func(x *agentchat.Session) { x.Archived = true }, wantCode: "owner_scope"},
		{name: "wrong status", mutate: func(x *agentchat.Session) { x.Status = "completed" }, wantCode: "session_not_waiting_tool"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := base
			x.ToolCalls = append([]agentchat.ToolCallRecord(nil), base.ToolCalls...)
			if tc.mutate != nil {
				tc.mutate(&x)
			}
			if ok, code := runAutoPatchDispatchAllowed(x, autoPatchGatePolicy(), "c1", "run-1", tc.shared); ok || code != tc.wantCode {
				t.Fatalf("got %v %s, want blocked %s", ok, code, tc.wantCode)
			}
		})
	}
}

func TestRunAutoPatchDispatchResolvesExactCallOnly(t *testing.T) {
	x := autoPatchGateSession()
	x.AutoPatch = true
	if ok, code := runAutoPatchDispatchAllowed(x, autoPatchGatePolicy(), "", "run-1", false); ok || code != "missing_call_id" {
		t.Fatalf("empty call id dispatched: %v %s", ok, code)
	}
	if ok, code := runAutoPatchDispatchAllowed(x, autoPatchGatePolicy(), "missing", "run-1", false); ok || code != "call_identity_unresolved" {
		t.Fatalf("missing call dispatched: %v %s", ok, code)
	}
	if ok, code := runAutoPatchDispatchAllowed(x, autoPatchGatePolicy(), "c1", "run-2", false); ok || code != "call_identity_unresolved" {
		t.Fatalf("call from another run was borrowed: %v %s", ok, code)
	}
	x.ToolCalls = append(x.ToolCalls, agentchat.ToolCallRecord{CallID: "c1", AgentRunID: "run-1", Name: "taskdeck_submit_patch", Status: "pending"})
	if ok, code := runAutoPatchDispatchAllowed(x, autoPatchGatePolicy(), "c1", "run-1", false); ok || code != "call_identity_unresolved" {
		t.Fatalf("ambiguous call identity dispatched: %v %s", ok, code)
	}
	notPatch := autoPatchGateSession()
	notPatch.AutoPatch = true
	notPatch.ToolCalls[0].Name = "taskdeck_collect_workspace"
	if ok, code := runAutoPatchDispatchAllowed(notPatch, autoPatchGatePolicy(), "c1", "run-1", false); ok || code != "call_not_patch" {
		t.Fatalf("non-patch call dispatched: %v %s", ok, code)
	}
}

func TestRunAutoPatchDispatchOneShotOnlyWhenEligible(t *testing.T) {
	base := autoPatchGateSession()
	noConsent := agentrun.AutomationSnapshot{}
	if ok, code := runAutoPatchDispatchAllowed(base, noConsent, "c1", "run-1", false); ok {
		t.Fatalf("pending call dispatched without any consent: %s", code)
	}
	base.ToolCalls[0].AutoRunOnce = true
	if ok, code := runAutoPatchDispatchAllowed(base, noConsent, "c1", "run-1", false); !ok || code != "one_shot_authorized" {
		t.Fatalf("explicit one-shot approval was blocked: %v %s", ok, code)
	}
	base.ToolCalls[0].ManualRetryOnly = true
	if ok, code := runAutoPatchDispatchAllowed(base, noConsent, "c1", "run-1", false); ok || code != "call_not_eligible" {
		t.Fatalf("manual retry was auto-dispatched: %v %s", ok, code)
	}
	base.ToolCalls[0].ManualRetryOnly = false
	base.ToolCalls[0].FeedbackSent = true
	if ok, code := runAutoPatchDispatchAllowed(base, noConsent, "c1", "run-1", false); ok || code != "call_not_eligible" {
		t.Fatalf("already-reported call was re-dispatched: %v %s", ok, code)
	}
}
