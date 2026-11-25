package server

import (
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func orderTestSession() agentchat.Session {
	return agentchat.Session{Owner: "local", Status: "completed", AutoContinueText: true}
}

func orderTestPolicy() agentrun.AutomationSnapshot {
	return agentrun.AutomationSnapshot{AutoContinueText: true}
}

func TestRunNextPromptOrderStageRequiresAckAndReceipt(t *testing.T) {
	x := orderTestSession()
	x.ToolCalls = []agentchat.ToolCallRecord{{CallID: "c1", Name: "taskdeck_submit_patch", Status: "done"}}
	if got := runNextPromptOrderStage(x, orderTestPolicy(), true); got != "feedback_not_acknowledged" {
		t.Fatalf("unacknowledged tool result did not block: %q", got)
	}
	x.ToolCalls[0].FeedbackSent = true
	x.ToolCalls[0].FeedbackStatus = "acknowledged"
	if got := runNextPromptOrderStage(x, orderTestPolicy(), false); got != "canonical_receipt_unavailable" {
		t.Fatalf("missing canonical receipt did not block: %q", got)
	}
	if got := runNextPromptOrderStage(x, orderTestPolicy(), true); got != "" {
		t.Fatalf("fully ordered turn still blocked: %q", got)
	}
}

func TestRunNextPromptOrderStageBlocksInFlightAndScope(t *testing.T) {
	x := orderTestSession()
	x.Archived = true
	if got := runNextPromptOrderStage(x, orderTestPolicy(), true); got != "owner_scope" {
		t.Fatalf("archived/non-local session was not blocked: %q", got)
	}
	x = orderTestSession()
	x.ProviderRun = &agentchat.ProviderRun{Phase: "sending"}
	if got := runNextPromptOrderStage(x, orderTestPolicy(), true); got != "provider_result_not_durable" {
		t.Fatalf("in-flight provider result was not blocked: %q", got)
	}
	x = orderTestSession()
	x.ToolCalls = []agentchat.ToolCallRecord{{CallID: "c1", Name: "taskdeck_submit_patch", Status: "running"}}
	if got := runNextPromptOrderStage(x, orderTestPolicy(), true); got != "unresolved_native_or_provider_work" {
		t.Fatalf("in-flight native work was not blocked: %q", got)
	}
}

func TestRunNextPromptOrderStageRequiresLiveConsent(t *testing.T) {
	x := orderTestSession()
	policy := orderTestPolicy()
	policy.AutoContinueText = false
	if got := runNextPromptOrderStage(x, policy, true); got != "conversation_continuation_not_authorized" {
		t.Fatalf("revoked pinned consent did not block: %q", got)
	}
	x.AutoContinueText = false
	if got := runNextPromptOrderStage(x, orderTestPolicy(), true); got != "conversation_continuation_not_authorized" {
		t.Fatalf("revoked live consent did not block: %q", got)
	}
}

func TestRunNextPromptOrderStageNoToolCallNeedsNoReceipt(t *testing.T) {
	x := orderTestSession()
	if got := runNextPromptOrderStage(x, orderTestPolicy(), false); got != "" {
		t.Fatalf("text-only turn incorrectly required a receipt: %q", got)
	}
}
