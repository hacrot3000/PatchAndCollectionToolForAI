package server

import (
	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

// runNextPromptOrderStage is a pure, fail-closed ordered precondition check for
// the M17-05 run-completion chain. It returns the first unmet stage as a stable
// machine code, or "" when the run may hand off to the next owner-authorized
// prompt. It reads no files, persists nothing, sends nothing and does not read
// plan segments. The caller supplies hasReceipt from the verified canonical
// receipt path; a turn with no native tool call needs no receipt.
//
// This is intentionally narrower than a full adoption: stage order is fixed and
// the first failure short-circuits, so a later stage can never be reported as
// ready while an earlier one is unmet. Wiring into the live completion
// scheduler (including fresh plan CAS and staged idempotency) remains a
// separate, reviewed change.
func runNextPromptOrderStage(x agentchat.Session, policy agentrun.AutomationSnapshot, hasReceipt bool) string {
	if x.Owner != "local" || x.Archived {
		return "owner_scope"
	}
	if agentchat.HasUnresolvedWork(x) {
		return "unresolved_native_or_provider_work"
	}
	if x.ProviderRun != nil {
		switch x.ProviderRun.Phase {
		case "prepared", "sending", "unknown":
			return "provider_result_not_durable"
		}
	}
	needsReceipt := false
	for _, call := range x.ToolCalls {
		if call.Name == "" {
			continue
		}
		needsReceipt = true
		switch call.Status {
		case "done", "error", "rejected":
			if !call.FeedbackSent || call.FeedbackStatus != "acknowledged" {
				return "feedback_not_acknowledged"
			}
		}
	}
	if needsReceipt && !hasReceipt {
		return "canonical_receipt_unavailable"
	}
	if x.TaskPlan != nil {
		if !policy.AutoContinuePlan || !x.TaskPlan.AutoContinue {
			return "plan_continuation_not_authorized"
		}
		return ""
	}
	if !policy.AutoContinueText || !x.AutoContinueText {
		return "conversation_continuation_not_authorized"
	}
	return ""
}
