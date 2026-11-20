package agentchat

import (
	"testing"
)

func TestCompactHistoryProtectsToolExchangesAndTail(t *testing.T) {
	h := make([]HistoryItem, 0, 40)
	for i := 0; i < 30; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		h = append(h, HistoryItem{Type: role, Content: "turn"})
	}
	h = append(h,
		HistoryItem{Type: "assistant", Content: "proposing"},
		HistoryItem{Type: "function_call", CallID: "c1", Name: "taskdeck_submit_patch", Arguments: `{"name":"x","patch_python":"y"}`},
		HistoryItem{Type: "function_call_output", CallID: "c1", Content: `{"status":"PASS"}`},
	)
	res := CompactHistory(h, CompactionConfig{KeepTail: 4, MaxDropped: 100})
	if res.Dropped == 0 || res.DroppedSHA == "" {
		t.Fatalf("expected a bounded drop: %+v", res.Dropped)
	}
	for _, item := range res.History {
		if item.Type == "function_call" || item.Type == "function_call_output" {
			if item.Type == "function_call" && item.CallID == "c1" {
				return
			}
		}
	}
	t.Fatal("tool exchange was dropped by compaction")
}

func TestCompactHistoryRefusesWhenOnlyProtectedItemsRemain(t *testing.T) {
	h := []HistoryItem{
		{Type: "assistant", Content: "propose"},
		{Type: "function_call", CallID: "c1", Name: "taskdeck_submit_patch", Arguments: `{"name":"x","patch_python":"y"}`},
		{Type: "function_call_output", CallID: "c1", Content: `{"status":"PASS"}`},
		{Type: "assistant", Content: "summary"},
	}
	res := CompactHistory(h, CompactionConfig{KeepTail: 1, MaxDropped: 10})
	if res.Dropped != 0 || len(res.History) != len(h) {
		t.Fatalf("protected history was compacted: %+v", res)
	}
}

func TestCompactHistoryRespectsMaxDropped(t *testing.T) {
	h := make([]HistoryItem, 0, 40)
	for i := 0; i < 30; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		h = append(h, HistoryItem{Type: role, Content: "turn"})
	}
	res := CompactHistory(h, CompactionConfig{KeepTail: 2, MaxDropped: 3})
	if res.Dropped != 0 {
		t.Fatalf("over-budget compaction was applied: %+v", res.Dropped)
	}
}
