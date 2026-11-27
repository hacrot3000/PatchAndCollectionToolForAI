package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// CLP-04.02.03 (partial/invalid result): a native Collection seam that returns
// malformed/empty output must be converted by the REAL Unified run path into a
// bounded COLLECTION_RESULT_UNAVAILABLE diagnostic, with nativeRead=false, no
// success claim, no native replay, and the immutable provider/model/worktree
// binding preserved. This exercises the production path at
// agent_run_collection.go:139-163, not a pure helper.
func TestAgentRunCollectionPartialResultSafeRejectionPinnedBinding(t *testing.T) {
	for _, tc := range []struct{ name, observed string }{
		{"empty", ""},
		{"malformed_json", `{"status":"PASS","text":`},
		{"non_json_plain_text", "native returned a plain-text fragment not valid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _, spec := providerToolRunFixture(t, "run-partial-"+tc.name)
			spec, history := enableRunAutomationFixture(t, s, spec, true, false)
			var reads, requests atomic.Int32
			payloads := make(chan string, 2)
			s.AgentCollectionCall = func(workspace string, _ patchtool.ProjectIdentity, _ []byte) string {
				reads.Add(1)
				if workspace != s.Workspace {
					return `{"error":"unexpected execution workspace"}`
				}
				return tc.observed
			}
			s.AgentChatCall = func(_ context.Context, p agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
				if p.ID != spec.ProviderID { t.Errorf("provider drift: %s", p.ID) }
				var wire struct { Model string `json:"model"` }
				if json.Unmarshal(payload, &wire) != nil || wire.Model != spec.ModelID { t.Errorf("model drift") }
				count := requests.Add(1)
				if count <= 2 { payloads <- string(payload) }
				if count == 1 {
					return agentchat.ResponsesResult{ResponseID: "r1", Calls: []agentchat.FunctionCall{{
						CallID: "collect-partial", Name: "taskdeck_collect_workspace",
						Arguments: `{"actions":[{"type":"file","path":"main.go"}]}`,
					}}}, nil
				}
				return agentchat.ResponsesResult{ResponseID: "r2", Text: "Stopped: no verified observation; will not claim success."}, nil
			}
			manager, err := s.agentRunManager("local")
			if err != nil { t.Fatal(err) }
			defer manager.Close()
			if _, err = manager.Submit(spec, history); err != nil { t.Fatal(err) }
			waitForAgentRunStatus(t, manager, spec.RunID, agentrun.Completed)
			record, _ := manager.Get(spec.RunID)

			// Exactly one native attempt: an invalid result is not re-executed.
			if reads.Load() != 1 || requests.Load() != 2 {
				t.Fatalf("native replay or missing continuation: reads=%d requests=%d", reads.Load(), requests.Load())
			}
			first, second := <-payloads, <-payloads
			if !strings.Contains(first, "taskdeck_collect_workspace") { t.Fatal("collection call missing from first payload") }
			if !strings.Contains(second, "COLLECTION_RESULT_UNAVAILABLE") || !strings.Contains(second, "collect-partial") {
				t.Fatalf("partial result not surfaced as bounded diagnostic in continuation payload: %s", second)
			}
			if strings.Contains(second, `"status":"PASS"`) && strings.Contains(second, "fixture-collection-marker") {
				t.Fatal("partial/invalid result claimed a verified success")
			}
			// Immutable binding retained across the diagnostic observation.
			if record.Spec.ProviderID != spec.ProviderID || record.Spec.ModelID != spec.ModelID ||
				record.Spec.ExecutionWorkspaceID != spec.ExecutionWorkspaceID {
				t.Fatalf("immutable run binding changed: %+v", record.Spec)
			}
			last := record.History[len(record.History)-1]
			if last.CallID != "collect-partial" || last.OriginRunID != spec.RunID ||
				last.ProviderID != spec.ProviderID || last.ModelID != spec.ModelID {
				t.Fatalf("wrong diagnostic lineage: %+v", last)
			}
			if !strings.Contains(last.Content, "COLLECTION_RESULT_UNAVAILABLE") { t.Fatal("canonical diagnostic not committed to run history") }
		})
	}
}
