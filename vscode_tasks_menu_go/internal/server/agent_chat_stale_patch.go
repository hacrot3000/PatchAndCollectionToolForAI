package server

import (
	"context"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

const agentLegacyStalePatchResult = "Workspace changed since review; no patch was queued. Request a new proposal."

func agentPatchHasNoNativeAttempt(call agentchat.ToolCallRecord) bool {
	return call.QueuePath == "" && call.PackageSHA256 == "" && call.PackageSize == 0 &&
		call.PatchSessionID == "" && call.NativeStartedAt == nil && call.RunID == "" &&
		call.PromptID == "" && call.SelectedIndex == 0 && len(call.Attempts) == 0 &&
		!call.FeedbackSent && call.FeedbackStatus == ""
}

func agentRecordStalePatchDenial(x *agentchat.Session, idx int) {
	call := &x.ToolCalls[idx]
	call.Status = "rejected"
	call.Result = agentRejectedPatchFeedback("PATCH_APPROVAL_STALE",
		"Workspace changed since this proposal was reviewed. No Patch was queued or executed. Inspect the current workspace and produce a NEW proposal with a new call ID; do not reuse the stale approval. Continue the original task.", true)
	call.AutoRunOnce = false
	call.SendAfterRetry = false
	x.Status = "tool_rejected"
	x.AppendEvent("tool_rejected", x.Status, call.CallID, "pre_queue_approval_stale")
}

// The legacy text is server-generated only before the first QUEUING marker.
// A complete event journal and empty native identifiers are additional
// independent fences. A trimmed, incomplete or contradictory journal is
// not sufficient proof; do not infer non-execution from empty IDs alone.
func agentLegacyStalePatchCanRecover(x agentchat.Session, idx int) bool {
	if x.Owner != "local" || x.Archived || x.Status != "tool_unknown" ||
		idx < 0 || idx >= len(x.ToolCalls) || x.EventSeq == 0 ||
		uint64(len(x.Events)) != x.EventSeq {
		return false
	}
	call := x.ToolCalls[idx]
	if call.AgentRunID != "" || call.Name != "taskdeck_submit_patch" ||
		call.Status != "unknown" || call.Result != agentLegacyStalePatchResult ||
		!agentPatchHasNoNativeAttempt(call) {
		return false
	}
	if x.ProviderRun != nil && (x.ProviderRun.Phase == "prepared" ||
		x.ProviderRun.Phase == "sending" || x.ProviderRun.Phase == "unknown") {
		return false
	}
	for i, event := range x.Events {
		if event.Seq != uint64(i+1) { return false }
		if event.CallID == call.CallID {
			switch event.Kind {
			case "tool_queuing", "tool_queued", "tool_running", "tool_final", "tool_recovered", "feedback_prepared", "feedback_acknowledged", "feedback_unknown":
				return false
			}
		}
	}
	for i, other := range x.ToolCalls {
		if i == idx { continue }
		switch other.Status {
		case "pending", "queuing", "queued", "dispatching", "running", "unknown":
			return false
		}
	}
	source, ok := findAgentToolCall(x.History, call.CallID)
	if !ok || call.ArgumentsSHA256 == "" || agentPatchPayloadSHA256(source.Arguments) != call.ArgumentsSHA256 {
		return false
	}
	for _, item := range x.History {
		if item.Type == "function_call_output" && item.CallID == call.CallID { return false }
	}
	return true
}

func (s *Server) recoverLegacyStalePatch(ctx context.Context, store *agentchat.Store, id string) {
	if s.Config.SharedServerEnabled || ctx.Err() != nil { return }
	release, ok := lockAgentTurn(s.agentOwnerWorkspace(), "local", id)
	if !ok { return }
	defer release()
	x, err := store.Get(id, "local")
	if err != nil || x.Workspace != s.agentOwnerWorkspace() ||
		s.agentConversationRequiresRunContinuation("local", id) { return }
	for i := range x.ToolCalls {
		if !agentLegacyStalePatchCanRecover(x, i) { continue }
		agentRecordStalePatchDenial(&x, i)
		// This is only a local journal transition. The existing scheduler
		// separately checks consent before delivering the denial to AI.
		_, _ = store.Save(x, x.Revision)
		return
	}
}
