package server

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Synthetic legacy checkpoint, not a copy of production history or proof that
// any of the incident's reads were redundant. Historical PATCH outcomes are
// pre-seeded; no PATCH is executed by this fixture.
func TestAgentCLP20LegacyCollectionFailurePreservesAcknowledgedPatches(t *testing.T) {
	for _, failure := range []string{"timeout", "context_rejected", "stop"} {
		t.Run(failure, func(t *testing.T) {
			s, id := newAgentConversationFixture(t, "stateless")
			store, err := s.agentChatStore()
			if err != nil {
				t.Fatal(err)
			}
			x, err := store.Get(id, "local")
			if err != nil {
				t.Fatal(err)
			}
			x.Status = "completed"
			x.RetryMode = agentchat.RetryOff
			x.AutoFeedback = true
			x.AutoFeedbackPersistent = true
			x.History = []agentchat.HistoryItem{{Type: "user", Content: "Synthetic original objective: inspect then propose bounded changes."}}
			appendPair := func(name, callID, arguments, receipt string) {
				x.History = append(x.History,
					agentchat.HistoryItem{Type: "function_call", Name: name, CallID: callID, Arguments: arguments},
					agentchat.HistoryItem{Type: "function_call_output", CallID: callID, Content: receipt})
			}
			readArgs := func(n int) string {
				return fmt.Sprintf(`{"actions":[{"type":"read","path":"fixture.go","start_line":%d,"max_lines":1}]}`, n)
			}
			readReceipt := func(n int) string {
				return fmt.Sprintf(`{"source":"native_patch_tool_collect","status":"PASS","evidence_verified":true,"results":[{"type":"read","files":[{"path":"fixture.go","content":"synthetic-read-%d"}]}]}`, n)
			}
			read := 0
			for batch, count := range []int{12, 7} {
				for i := 0; i < count; i++ {
					read++
					appendPair("taskdeck_collect_workspace", fmt.Sprintf("old-read-%d", read), readArgs(read), readReceipt(read))
				}
				callID := fmt.Sprintf("old-patch-%d", batch+1)
				runID := fmt.Sprintf("synthetic-patch-run-%d", batch+1)
				result := fmt.Sprintf(`{"status":"PASS","run_id":%q}`, runID)
				appendPair("taskdeck_submit_patch", callID, `{"name":"synthetic historical patch","patch_python":"print('fixture')"}`, result)
				x.ToolCalls = append(x.ToolCalls, agentchat.ToolCallRecord{
					CallID: callID, Name: "taskdeck_submit_patch", Status: "done", RunID: runID,
					Result: result, FeedbackSent: true, FeedbackStatus: "acknowledged",
				})
			}
			x, err = store.Save(x, x.Revision)
			if err != nil {
				t.Fatal(err)
			}
			historical := append([]agentchat.HistoryItem(nil), x.History...)
			acknowledged := append([]agentchat.ToolCallRecord(nil), x.ToolCalls...)

			// Actual legacy request loop; injected collector and provider
			// return deterministic synthetic observations/errors immediately.
			const invalid = `{"actions":[{"type":"grep","path":"fixture.go"}]}`
			requests := []string{readArgs(20), readArgs(21), readArgs(22), invalid, readArgs(23), readArgs(24), readArgs(25), readArgs(26), readArgs(27)}
			nativeReads, schemaRejections, providerCalls := 0, 0, 0
			observed := make(map[string]string)
			s.AgentCollectionCall = func(_ string, _ patchtool.ProjectIdentity, raw []byte) string {
				if strings.Contains(string(raw), `"grep"`) {
					schemaRejections++
					return `{"error":{"code":"COLLECT_SCHEMA_INVALID","message":"synthetic native schema rejection for grep","retry_allowed":true}}`
				}
				nativeReads++
				return readReceipt(19 + nativeReads)
			}
			s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
				providerCalls++
				if providerCalls > 1 {
					var wire struct {
						Input []map[string]any `json:"input"`
					}
					if err := json.Unmarshal(payload, &wire); err != nil {
						t.Fatal(err)
					}
					prior := fmt.Sprintf("clp20-collect-%d", providerCalls-1)
					for _, item := range wire.Input {
						if item["type"] == "function_call_output" && item["call_id"] == prior {
							if value, ok := item["output"].(string); ok {
								observed[prior] = value
							}
						}
					}
					if observed[prior] == "" {
						t.Fatalf("missing exact current result for %s", prior)
					}
				}
				if providerCalls <= len(requests) {
					return agentchat.ResponsesResult{ResponseID: fmt.Sprintf("clp20-response-%d", providerCalls), Calls: []agentchat.FunctionCall{{
						CallID: fmt.Sprintf("clp20-collect-%d", providerCalls), Name: "taskdeck_collect_workspace", Arguments: requests[providerCalls-1],
					}}}, nil
				}
				if providerCalls != len(requests)+1 {
					t.Fatalf("unexpected egress after terminal failure: %d", providerCalls)
				}
				switch failure {
				case "context_rejected":
					return agentchat.ResponsesResult{}, &agentchat.ProviderError{Code: "PROVIDER_HTTP_400", HTTPStatus: 400, ProviderErrorCode: "context_length_exceeded"}
				case "stop":
					if !cancelAgentRequest(s.Workspace, "local", id) {
						t.Fatal("Stop could not cancel the exact active request")
					}
					return agentchat.ResponsesResult{}, context.Canceled
				default:
					return agentchat.ResponsesResult{}, &agentchat.ProviderError{Code: "CONNECT_TIMEOUT", Stage: "provider_http", Retryable: true, Ambiguous: true}
				}
			}
			w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
				map[string]any{"message": "Continue original synthetic objective", "confirm_external": true, "expected_revision": x.Revision})
			if w.Code < 400 || providerCalls != 10 || nativeReads != 8 || schemaRejections != 1 {
				t.Fatalf("unexpected incident path: HTTP=%d provider=%d reads=%d schema=%d body=%s", w.Code, providerCalls, nativeReads, schemaRejections, w.Body)
			}
			saved, err := store.Get(id, "local")
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := "failed"
			if failure == "stop" {
				wantStatus = "cancelled"
			}
			if saved.Status != wantStatus || saved.ProviderRun == nil || saved.ProviderRun.Kind != "collection" ||
				saved.ProviderRun.Phase != "unknown" || saved.ProviderRun.CallID != "clp20-collect-9" {
				t.Fatalf("wrong terminal collection domain: %s %+v", saved.Status, saved.ProviderRun)
			}
			if !reflect.DeepEqual(saved.ToolCalls, acknowledged) {
				t.Fatalf("historical PATCH acknowledgements changed: %+v", saved.ToolCalls)
			}
			if len(saved.History) < len(historical) || !reflect.DeepEqual(saved.History[:len(historical)], historical) {
				t.Fatal("canonical historical checkpoint changed")
			}
			for callID, receipt := range observed {
				count := 0
				for _, item := range saved.History {
					if item.Type == "function_call_output" && item.CallID == callID {
						count++
						if item.Content != receipt {
							t.Fatalf("receipt changed for %s", callID)
						}
					}
				}
				if count != 1 {
					t.Fatalf("receipt count for %s: %d", callID, count)
				}
			}
			if _, eligible := agentAutoFeedbackCandidate(saved); eligible {
				t.Fatal("acknowledged PATCH was made eligible for resend")
			}
		})
	}
}
