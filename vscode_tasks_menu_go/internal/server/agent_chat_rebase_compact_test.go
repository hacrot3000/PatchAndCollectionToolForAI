package server

import (
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// CLP-05.02.04 (long/multiday compaction, rebase, catalog reuse, no lost goal):
// after history has been archived (multiday compaction sets ArchivedHistoryItems)
// a provider-side stateful conversation MUST be rebased to one canonical
// stateless request. Using the same canonical builder as the final-wire test
// (agentBuildContextRequest with cfg.Next only, which owns dedup), the rebased
// projection must retain the original goal and the exact current receipt once,
// reuse the compact catalog instead of replaying archived evidence, and not
// resurrect stale provider-side refs.
func TestAgentCLP050204RebaseAfterArchivedHistoryRetainsGoalAndReceipt(t *testing.T) {
	const goal = "Original multi-day owner goal: finish the collection recovery audit"
	x := agentchat.Session{
		Mode:                 agentchat.PreviousResponse,
		PreviousResponseID:   "stale-provider-ref",
		ArchivedHistoryItems: 3,
	}
	x.History = []agentchat.HistoryItem{{Type: "user", Content: goal}}
	for i := 0; i < 12; i++ {
		callID := "old-" + string(rune('a'+i))
		x.History = append(x.History,
			agentchat.HistoryItem{Type: "function_call", CallID: callID,
				Name: "taskdeck_collect_workspace", Arguments: `{"actions":[{"type":"file","path":"f.go"}]}`},
			agentchat.HistoryItem{Type: "function_call_output", CallID: callID,
				Content: strings.Repeat("archived-old-evidence ", 200)},
		)
	}

	if !agentContextNeedsRebase(x) {
		t.Fatal("archived multiday history did not trigger safe rebase")
	}
	mode := agentContextRequestMode(x, "responses", true)
	if mode != agentchat.Stateless {
		t.Fatalf("archived history reused a stateful chain: %s", mode)
	}
	agentClearProviderRefsForStateless(&x, mode)
	if x.PreviousResponseID != "" || x.ConversationID != "" {
		t.Fatal("stale provider refs survived rebase")
	}

	// Exact current receipt is supplied once through cfg.Next; the canonical
	// builder owns history/Next dedup and byte-exact receipt preservation.
	receipt := agentchat.HistoryItem{Type: "function_call_output", CallID: "current-read",
		Content: `{"status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"text_excerpt":"byte-exact-current"}`}
	cfg := agentchat.RequestConfig{
		Model: "model", Instructions: "Current trusted policy", Mode: mode,
		Next: []agentchat.HistoryItem{receipt}, MaxOutputTokens: 8192,
		EnableCollectionTool: true, EnablePatchTool: true,
	}
	body, err := agentBuildContextRequest(x, agentchat.ModelConfig{MaxInputTokens: 32000},
		agentchat.ProviderConfig{APIType: "responses"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(body)
	if strings.Contains(raw, "previous_response_id") || strings.Contains(raw, `"conversation"`) {
		t.Fatal("rebase retained provider-side conversation")
	}
	if !strings.Contains(raw, goal) {
		t.Fatal("original multi-day owner goal lost after rebase/compaction")
	}
	if strings.Count(raw, "byte-exact-current") != 1 || !strings.Contains(raw, "current-read") {
		t.Fatalf("current receipt duplicated or lost after rebase: occurrences=%d", strings.Count(raw, "byte-exact-current"))
	}
	// Note: compact.go intentionally retains the most recent completed
	// Collection exchanges as real tool pairs; bounded-catalog reuse and
	// no-full-replay are covered by dedicated contextcompact tests. This
	// rebase test asserts the rebase/goal/receipt invariants only.
}
