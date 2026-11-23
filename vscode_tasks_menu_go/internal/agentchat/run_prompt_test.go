package agentchat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalHistoryBridgesVisibleTextWithoutProviderSessionState(t *testing.T) {
	history := []HistoryItem{
		{Type: "user", Content: "Inspect the parser"},
		{Type: "assistant", Content: "I found the parser entry point.", OriginRunID: "run-101", ProviderID: "composer", ModelID: "composer-2.5"},
	}
	body, err := BuildResponsesRequest(RequestConfig{
		Model: "gpt-6.1-sol", Instructions: "Follow current task instructions.",
		Mode: Stateless, History: history,
		Next: []HistoryItem{{Type: "user", Content: "Continue with the finding."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(payload["input"])
	visible := string(encoded)
	if !strings.Contains(visible, "I found the parser entry point.") || !strings.Contains(visible, "Continue with the finding.") {
		t.Fatalf("visible canonical history was not replayed: %s", visible)
	}
	for _, opaque := range []string{"previous_response_id", "conversation_id", "origin_run_id", "provider_id", "composer-2.5"} {
		if strings.Contains(string(body), opaque) {
			t.Fatalf("provider-specific state leaked into portable prompt (%q): %s", opaque, body)
		}
	}
}
