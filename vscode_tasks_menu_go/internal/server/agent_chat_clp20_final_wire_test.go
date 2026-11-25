package server

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// The historical observations approximate the incident's 942 KB pressure.
// These are synthetic bytes, not the production incident transcript.
func TestAgentCLP20FinalWirePreservesMandatoryReceipt(t *testing.T) {
	goal := "Original owner goal: fix the reported regression"
	x := agentchat.Session{History: []agentchat.HistoryItem{{Type: "user", Content: goal}}}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("old-%d", i)
		x.History = append(x.History,
			agentchat.HistoryItem{Type: "function_call", CallID: id, Name: "taskdeck_collect_workspace", Arguments: "{}"},
			agentchat.HistoryItem{Type: "function_call_output", CallID: id, Content: strings.Repeat("old evidence ", 24154)})
	}
	call := agentchat.HistoryItem{Type: "function_call", CallID: "current-read", Name: "taskdeck_collect_workspace", Arguments: "{}"}
	x.History = append(x.History, call)
	original := append([]agentchat.HistoryItem(nil), x.History...)
	receipt := agentchat.HistoryItem{Type: "function_call_output", CallID: call.CallID, Content: strings.Repeat("fresh evidence ", 12000)}
	for _, api := range []string{"responses", "chat_completions"} {
		t.Run(api, func(t *testing.T) {
			cfg := agentchat.RequestConfig{Model: "m", Mode: agentchat.Stateless, Instructions: "Current trusted policy", Next: []agentchat.HistoryItem{receipt}, MaxOutputTokens: 8192, EnableCollectionTool: true}
			model := agentchat.ModelConfig{MaxInputTokens: 65536}
			body, err := agentBuildContextRequest(x, model, agentchat.ProviderConfig{APIType: api}, cfg)
			if err != nil { t.Fatal(err) }
			if !strings.Contains(string(body), goal) { t.Fatal("original owner goal missing") }
			var canonical struct { Input []map[string]any `json:"input"` }
			if err := json.Unmarshal(body, &canonical); err != nil { t.Fatal(err) }
			calls, outputs := 0, 0
			for _, item := range canonical.Input {
				if item["call_id"] != call.CallID { continue }
				switch item["type"] {
				case "function_call": calls++
				case "function_call_output":
					outputs++
					if item["output"] != receipt.Content { t.Fatal("current receipt changed") }
				}
			}
			if calls != 1 || outputs != 1 { t.Fatalf("current pair duplicated or lost: %d/%d", calls, outputs) }
			m, err := agentchat.MeasureProviderPayload(api, body)
			if err != nil || m.OutputBudget != 8192 || m.WireBytes <= 0 { t.Fatalf("bad final measurement: %+v %v", m, err) }
			if api == "chat_completions" {
				wire, err := agentchat.BuildChatCompletionsRequest(body)
				if err != nil { t.Fatal(err) }
				var target struct { Messages []map[string]any `json:"messages"` }
				if err := json.Unmarshal(wire, &target); err != nil { t.Fatal(err) }
				n := 0
				for _, msg := range target.Messages {
					if msg["tool_call_id"] == call.CallID {
						n++
						if msg["content"] != receipt.Content { t.Fatal("wire receipt changed") }
					}
				}
				if n != 1 || m.WireBytes != len(wire) { t.Fatal("wire receipt count or measurement mismatch") }
			}
			// A mandatory item above the technical limit must fail locally,
			// not be silently shortened during the minimal-history retry.
			cfg.Next[0].Content = strings.Repeat("x", (512<<10)+1)
			bad, err := agentBuildContextRequest(x, model, agentchat.ProviderConfig{APIType: api}, cfg)
			if err == nil || len(bad) != 0 || !strings.Contains(err.Error(), "mandatory context") { t.Fatalf("mandatory overflow not reported: %v", err) }
			if !reflect.DeepEqual(original, x.History) { t.Fatal("canonical history mutated") }
		})
	}
}

func TestAgentCLP20HistoricalPackDoesNotGrantAuthority(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		policy := agentChatInstructions("project", enabled)
		for _, phrase := range []string{"currently advertised native schema", "obsolete grep actions", "historical automation settings as permission", "original user goal"} {
			if !strings.Contains(policy, phrase) { t.Fatalf("missing compatibility policy %q", phrase) }
		}
	}
}
