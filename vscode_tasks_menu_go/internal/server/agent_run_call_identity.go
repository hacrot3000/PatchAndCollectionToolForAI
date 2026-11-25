package server

import (
	"errors"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Canonical native tool-call identity for run-managed conversations.
//
// A provider CallID is only unique within one provider turn. Across retries,
// sibling runs and legacy sessions the same CallID can reappear, so resolving
// a receipt by bare CallID is ambiguous. This resolver namespaced-matches a
// call within a caller-supplied owner session and fails closed when the
// identity is missing or ambiguous. It is a pure primitive: it reads nothing,
// writes nothing and never contacts a provider. Any projection into the Task
// Plan ledger must reuse this resolver rather than reimplementing matching.
//
// Legacy compatibility: a call recorded before AgentRunID existed has an
// empty AgentRunID. Such a call is accepted only when no run-namespaced call
// with the same CallID exists; it is never preferred over an exact binding.
func canonicalRunToolCall(calls []agentchat.ToolCallRecord, callID, agentRunID string) (agentchat.ToolCallRecord, error) {
	if callID == "" {
		return agentchat.ToolCallRecord{}, errors.New("tool call identity is missing")
	}
	var exact *agentchat.ToolCallRecord
	var legacy *agentchat.ToolCallRecord
	for i := range calls {
		call := &calls[i]
		if call.CallID != callID {
			continue
		}
		if agentRunID != "" && call.AgentRunID == agentRunID {
			if exact != nil {
				return agentchat.ToolCallRecord{}, errors.New("ambiguous run tool call identity")
			}
			exact = call
			continue
		}
		if call.AgentRunID == "" {
			if legacy != nil {
				return agentchat.ToolCallRecord{}, errors.New("ambiguous legacy tool call identity")
			}
			legacy = call
		}
	}
	if exact != nil {
		return *exact, nil
	}
	if legacy != nil && agentRunID == "" {
		return *legacy, nil
	}
	return agentchat.ToolCallRecord{}, errors.New("run tool call identity not found")
}
