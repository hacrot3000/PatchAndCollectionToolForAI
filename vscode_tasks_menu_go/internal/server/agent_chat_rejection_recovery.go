package server

import "bletonfc/vscode_tasks_menu/internal/agentchat"

// Recover a persisted, never-delivered machine rejection after a session
// status mismatch. The last provider tool batch, not an old card in the UI,
// is authoritative. Never infer rejection from a missing native receipt.
func agentRecoverableRejection(x agentchat.Session, requestedID string) (string, bool) {
	if x.Archived || x.Owner != "local" {
		return "", false
	}
	switch x.Status {
	case "failed", "completed", "tool_unknown", "tool_queuing", "requesting_ai", "waiting_tool":
	default:
		return "", false
	}
	// A completed provider journal is required for a requesting_ai mismatch.
	// Older sessions without a provider journal are eligible only when no
	// request is recorded as in flight. Explicit Stop is never overridden.
	if x.ProviderRun != nil {
		if x.ProviderRun.Phase != "completed" {
			return "", false
		}
	} else if x.Status == "requesting_ai" {
		return "", false
	}
	if len(x.History) == 0 || x.History[len(x.History)-1].Type != "function_call" {
		return "", false
	}
	tail := map[string]agentchat.HistoryItem{}
	for i := len(x.History)-1; i >= 0 && x.History[i].Type == "function_call"; i-- {
		item := x.History[i]
		if item.CallID == "" || item.OriginRunID != "" {
			return "", false
		}
		if _, exists := tail[item.CallID]; exists { return "", false }
		tail[item.CallID] = item
	}
	// A reused call ID or already appended output is not a new pending call.
	for _, item := range x.History[:len(x.History)-len(tail)] {
		if _, exists := tail[item.CallID]; exists { return "", false }
	}
	first, turn := "", ""
	seen := map[string]bool{}
	for _, call := range x.ToolCalls {
		item, current := tail[call.CallID]
		if !current {
			switch call.Status {
			case "pending", "queuing", "queued", "dispatching", "running", "unknown":
				return "", false
			}
			if !call.FeedbackSent || (call.FeedbackStatus != "" && call.FeedbackStatus != "acknowledged") {
				return "", false
			}
			continue
		}
		if seen[call.CallID] || call.AgentRunID != "" || call.TextFallback ||
			!agentRepairableToolDenial(call) || !agentPatchHasNoNativeAttempt(call) ||
			call.Name != item.Name || call.ArgumentsSHA256 == "" ||
			agentPatchPayloadSHA256(item.Arguments) != call.ArgumentsSHA256 {
			return "", false
		}
		seen[call.CallID] = true
		if first == "" {
			first, turn = call.CallID, call.ModelTurnID
		} else if turn == "" || turn != call.ModelTurnID {
			return "", false
		}
	}
	if first == "" || len(seen) != len(tail) || (requestedID != "" && !seen[requestedID]) {
		return "", false
	}
	// Grouping code delivers all denials with a common ModelTurnID. Refuse
	// any contradictory historical record that could join this group.
	if turn != "" {
		for _, call := range x.ToolCalls {
			if call.ModelTurnID == turn && !seen[call.CallID] { return "", false }
		}
	}
	return first, true
}

// Caller holds the same exclusive conversation lease as approval and egress.
// This only repairs local status; existing consent and egress rules still
// control whether the recorded rejection is sent to the provider.
func agentRestoreRejectedSession(x *agentchat.Session, requestedID string) bool {
	callID, ok := agentRecoverableRejection(*x, requestedID)
	if !ok { return false }
	x.Status = "tool_rejected"
	x.AppendEvent("tool_rejection_recovered", x.Status, callID, "pending_rejection_after_state_mismatch")
	return true
}
