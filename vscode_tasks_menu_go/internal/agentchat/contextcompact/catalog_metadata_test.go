package contextcompact

import (
	"fmt"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// A complete native receipt must contribute bounded status/proof metadata to
// the next-step catalog without replaying source or mutating canonical history.
func TestCollectionCatalogCarriesBoundedStatusProofMetadata(t *testing.T) {
	content := `{"status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"partial_evidence_available":false,"run_id":"native-1","result_text":"result.txt","results":[{"files":[{"path":"src/a.go"}]}]}`
	call := agentchat.HistoryItem{Type: "function_call", CallID: "read-1", Name: "taskdeck_collect_workspace", Arguments: `{"actions":[{"type":"file","path":"src/a.go"}]}`}
	output := agentchat.HistoryItem{Type: "function_call_output", CallID: "read-1", Content: content}
	history := []agentchat.HistoryItem{{Type: "user", Content: "Fix the bug"}, call, output}
	// A lone small collection is retained as a full tool pair by Build, so
	// assert catalog-format metadata on the in-package summarizer directly.
	catalog := collectionSummary(call, output)
	for _, want := range []string{"src/a.go", "status=PASS", "native_status=PASS", "verified=true", "run=native-1", "result_hash="} {
		if !strings.Contains(catalog, want) {
			t.Fatalf("catalog missing %q: %s", want, catalog)
		}
	}
	visible, stats := Build(history, Policy{MaxInputTokens: 32768})
	if !strings.Contains(fmt.Sprint(visible), "src/a.go") {
		t.Fatal("full collection lost its path in provider-visible history")
	}
	if output.Content != content {
		t.Fatal("canonical receipt mutated")
	}
	if stats.BudgetExceeded {
		t.Fatalf("budget exceeded: %+v", stats)
	}
}

// Partial/unverified observations must be marked for invalidation/re-read in
// the bounded catalog, while the full partial payload never leaks through.
func TestPartialCollectionCatalogMarksInvalidationAndStaysBounded(t *testing.T) {
	history := []agentchat.HistoryItem{{Type: "user", Content: "Audit source"}}
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("read-%d", i)
		content := fmt.Sprintf(`{"status":"INCOMPLETE","native_status":"PASS","evidence_verified":true,"partial_evidence_available":true,"run_id":"native-%d","payload":%q}`, i, strings.Repeat("Z", 20000))
		history = append(history,
			agentchat.HistoryItem{Type: "function_call", CallID: id, Name: "taskdeck_collect_workspace", Arguments: fmt.Sprintf(`{"actions":[{"type":"file","path":"src/f%d.go"}]}`, i)},
			agentchat.HistoryItem{Type: "function_call_output", CallID: id, Content: content})
	}
	before := fmt.Sprint(history)
	visible, stats := Build(history, Policy{MaxInputTokens: 32768})
	joined := fmt.Sprint(visible)
	if !strings.Contains(joined, "coverage=partial; re-read required before reuse") {
		t.Fatalf("partial invalidation marker missing: %s", joined)
	}
	if strings.Contains(joined, strings.Repeat("Z", 100)) {
		t.Fatal("full partial payload leaked into catalog")
	}
	if stats.BudgetExceeded {
		t.Fatalf("budget exceeded: %+v", stats)
	}
	if len(visible) > MaxProviderHistoryItems {
		t.Fatalf("too many visible items: %d", len(visible))
	}
	if fmt.Sprint(history) != before {
		t.Fatal("canonical history mutated")
	}
}
