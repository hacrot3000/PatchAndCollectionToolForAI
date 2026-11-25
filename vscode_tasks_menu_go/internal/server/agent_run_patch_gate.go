package server

import (
	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

// runAutoPatchDispatchAllowed is a pure, fail-closed precondition gate for
// run-bound automatic Patch dispatch in a run-managed conversation. It reads no
// files, persists nothing and contacts no provider. It is intentionally
// narrower than the legacy agentAutoPatchCandidate: an approved call is
// eligible only when both the pinned automation snapshot and the live session
// consent are intact (or an explicit one-shot AutoRunOnce approval exists),
// there is no ambiguous or in-flight provider/native work, and the exact call
// resolves to this run under a namespaced call identity.
//
// This gate never bypasses the existing revision-fenced native approval
// gateway: exact package SHA, argument digest, run/attempt binding, workspace
// HEAD/branch/status and the native lease remain that gateway's responsibility.
// It also never silently enables Auto Patch - consent must already be explicit.
// The returned code is a stable reason suitable for structured same-run
// feedback; it is never provider text and never a claim a native operation ran.
func runAutoPatchDispatchAllowed(x agentchat.Session, policy agentrun.AutomationSnapshot, callID, agentRunID string, sharedServer bool) (bool, string) {
	if sharedServer {
		return false, "shared_server"
	}
	if callID == "" {
		return false, "missing_call_id"
	}
	if x.Owner != "local" || x.Archived {
		return false, "owner_scope"
	}
	if x.Status != "waiting_tool" {
		return false, "session_not_waiting_tool"
	}
	if x.ProviderRun != nil {
		switch x.ProviderRun.Phase {
		case "prepared", "sending", "unknown":
			return false, "provider_work_in_flight"
		}
	}
	for _, call := range x.ToolCalls {
		switch call.Status {
		case "queuing", "queued", "dispatching", "running", "unknown":
			return false, "native_work_in_flight"
		}
	}
	call, err := canonicalRunToolCall(x.ToolCalls, callID, agentRunID)
	if err != nil {
		return false, "call_identity_unresolved"
	}
	if call.Name != "taskdeck_submit_patch" {
		return false, "call_not_patch"
	}
	if call.Status != "pending" || call.FeedbackSent || call.ManualRetryOnly {
		return false, "call_not_eligible"
	}
	// A one-shot, explicitly user-approved reapply is independent of the
	// persistent Auto Patch grant but still requires a unique live call.
	if call.AutoRunOnce {
		return true, "one_shot_authorized"
	}
	if !policy.AutoPatch || !x.AutoPatch {
		return false, "automatic_patch_not_authorized"
	}
	return true, "pinned_and_live_consent"
}
