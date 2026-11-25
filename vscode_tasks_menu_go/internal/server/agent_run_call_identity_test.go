package server

import (
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestCanonicalRunToolCallResolvesUniqueNamespacedIdentity(t *testing.T) {
	calls := []agentchat.ToolCallRecord{
		{CallID: "c1", AgentRunID: "old-run", Status: "done"},
		{CallID: "c1", AgentRunID: "current-run", Status: "pending"},
		{CallID: "c2", AgentRunID: "current-run", Status: "done"},
	}
	got, err := canonicalRunToolCall(calls, "c1", "current-run")
	if err != nil {
		t.Fatalf("exact run binding was not resolved: %v", err)
	}
	if got.AgentRunID != "current-run" || got.Status != "pending" {
		t.Fatalf("wrong call resolved: %+v", got)
	}
}

func TestCanonicalRunToolCallFailsClosed(t *testing.T) {
	calls := []agentchat.ToolCallRecord{
		{CallID: "dup", AgentRunID: "run-a", Status: "done"},
		{CallID: "dup", AgentRunID: "run-b", Status: "done"},
		{CallID: "once", AgentRunID: "run-a", Status: "done"},
		{CallID: "legacy", Status: "done"},
	}
	tests := []struct {
		name   string
		callID string
		runID  string
	}{
		{name: "empty call id", runID: "run-a"},
		{name: "not found", callID: "missing", runID: "run-a"},
		{name: "wrong run is not borrowed", callID: "once", runID: "run-b"},
		{name: "legacy call not bound to a specific run", callID: "legacy", runID: "run-a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := canonicalRunToolCall(calls, tc.callID, tc.runID); err == nil {
				t.Fatal("ambiguous or absent call identity was accepted")
			}
		})
	}
}

func TestCanonicalRunToolCallLegacyCompatibility(t *testing.T) {
	calls := []agentchat.ToolCallRecord{{CallID: "legacy", Status: "done"}}
	got, err := canonicalRunToolCall(calls, "legacy", "")
	if err != nil || got.Status != "done" {
		t.Fatalf("legacy call was not accepted without a run scope: %+v %v", got, err)
	}
}

func TestCanonicalRunToolCallAmbiguousLegacyFailsClosed(t *testing.T) {
	calls := []agentchat.ToolCallRecord{
		{CallID: "legacy", Status: "done"},
		{CallID: "legacy", Status: "error"},
	}
	if _, err := canonicalRunToolCall(calls, "legacy", ""); err == nil {
		t.Fatal("duplicate legacy call identity was accepted")
	}
}
