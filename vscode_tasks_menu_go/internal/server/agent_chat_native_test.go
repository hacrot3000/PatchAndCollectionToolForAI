package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/session"
)

func TestAgentNativePatchSelectionRequiresExactNameAndActivePrompt(t *testing.T) {
	prompt := queueSelectionState()
	// Test that no implicit selection of unrelated queued packages is allowed.
	var parsed map[string]any
	if err := json.Unmarshal(prompt.Prompt, &parsed); err != nil {
		t.Fatal(err)
	}
	rows := parsed["items"].([]any)
	rows[0].(map[string]any)["name"] = "unrelated.zip"
	rows[1].(map[string]any)["name"] = "PATCH_agent_approved.zip"
	rows[1].(map[string]any)["kind"] = "PATCH"
	prompt.Prompt, _ = json.Marshal(parsed)
	selected, promptID, index, err := agentExactPatchSelection(prompt, "PATCH_agent_approved.zip")
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Indexes) != 1 || selected.Indexes[0] != 2 || selected.Action != "select" {
		t.Fatalf("incorrect native selection %+v", selected)
	}
	if promptID == "" || index != 2 {
		t.Fatalf("missing prompt/index binding %q %d", promptID, index)
	}
	if _, _, _, err := agentExactPatchSelection(prompt, "other.zip"); err == nil {
		t.Fatal("selected missing package")
	}
	rows[0].(map[string]any)["name"] = "PATCH_agent_approved.zip"
	prompt.Prompt, _ = json.Marshal(parsed)
	if _, _, _, err := agentExactPatchSelection(prompt, "PATCH_agent_approved.zip"); err == nil {
		t.Fatal("duplicate package name selected")
	}
	rows[0].(map[string]any)["name"] = "unrelated.zip"
	rows[1].(map[string]any)["selectable"] = false
	prompt.Prompt, _ = json.Marshal(parsed)
	if _, _, _, err := agentExactPatchSelection(prompt, "PATCH_agent_approved.zip"); err == nil {
		t.Fatal("disabled selection accepted")
	}
	prompt.CommandsEnabled = false
	if _, _, _, err := agentExactPatchSelection(prompt, "PATCH_agent_approved.zip"); err == nil {
		t.Fatal("disabled protocol command channel accepted")
	}
}
func TestAgentNativeResultsOnlyAcceptMatchingFinalProtocolItem(t *testing.T) {
	state := queueSelectionState()
	state.Items = []session.ProtocolItemState{
		{Name: "unrelated.zip", Kind: "PATCH", Status: "PASS", RunID: "run-other"},
		{Name: "approved.zip", Kind: "PATCH", Status: "RUNNING", Index: 2, RunID: "run-1"},
	}
	expect := agentPatchExpectation{Name: "approved.zip", PromptID: "agent-prompt-1", ItemIndex: 2}
	if _, _, _, ok := agentPatchResultFromProtocol(state, expect); ok {
		t.Fatal("running patch reported success")
	}
	state.Items[1].Status = "FAIL"
	rc := 5
	state.Items[1].RC = &rc
	state.Items[1].FailureReason = "test failed"
	state.Items[1].OutputTail = "some output"
	state.Items[1].Commands = []session.ProtocolCommandEvidenceState{{Index: 1, Stage: "apply", Status: "FAIL", Argv: []string{"secret-token"}, LogPath: "/private/secret/file"}}
	status, result, runID, ok := agentPatchResultFromProtocol(state, expect)
	if !ok || status != "error" {
		t.Fatalf("invalid FAIL result %q %s", status, result)
	}
	if runID != "run-1" {
		t.Fatalf("run ID not bound to result: %q", runID)
	}
	if strings.Contains(result, "secret-token") || strings.Contains(result, "/private/secret/file") {
		t.Fatalf("sensitive command metadata leaked: %s", result)
	}
	if !strings.Contains(result, `"rc":5`) {
		t.Fatalf("missing observed RC: %s", result)
	}
	state.Items[1].Status = "PASS"
	status, _, _, ok = agentPatchResultFromProtocol(state, expect)
	if !ok || status != "done" {
		t.Fatal("PASS not reported as done")
	}
	// A different approved index must never inherit this PASS.
	if _, _, _, ok := agentPatchResultFromProtocol(state, agentPatchExpectation{Name: "approved.zip", ItemIndex: 1}); ok {
		t.Fatal("final result attributed to a different approved index")
	}
	// A missing run ID is not evidence of completion.
	state.Items[1].RunID = ""
	if _, _, _, ok := agentPatchResultFromProtocol(state, expect); ok {
		t.Fatal("run-less item accepted as final evidence")
	}
}
func TestAgentToolContinuationExactCallIDAndOnceOnly(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	session.ToolCalls[0].Status = "done"
	session.ToolCalls[0].Result = `{"status":"PASS","rc":0,"name":"approved.zip"}`
	session, err = store.Save(session, session.Revision)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, body []byte) (agentchat.ResponsesResult, error) {
		calls++
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if req["previous_response_id"] != nil {
			t.Fatal("stateless test unexpectedly chained")
		}
		items := req["input"].([]any)
		output := items[len(items)-1].(map[string]any)
		if output["call_id"] != "call_agent_001" || output["type"] != "function_call_output" {
			t.Fatalf("wrong model call ID or role: %+v", output)
		}
		if !strings.Contains(output["output"].(string), `"PASS"`) {
			t.Fatal("missing verified native status")
		}
		tools := req["tools"].([]any)
		if len(tools) != 2 || tools[0].(map[string]any)["name"] != "taskdeck_collect_workspace" ||
			tools[1].(map[string]any)["name"] != "taskdeck_submit_patch" {
			t.Fatal("Collection/Patch tool contract not refreshed")
		}
		return agentchat.ResponsesResult{ResponseID: "resp_after_tool", Text: "Patch verified as PASS"}, nil
	}
	path := "/api/agent/sessions/" + id + "/tools/call_agent_001/continue"
	noConsent := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, path, map[string]any{"expected_revision": session.Revision})
	if noConsent.Code != 403 {
		t.Fatalf("external consent bypass %d", noConsent.Code)
	}
	ok := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, path, map[string]any{"expected_revision": session.Revision, "confirm_external": true})
	if ok.Code != 200 {
		t.Fatalf("tool continuation failed %d %s", ok.Code, ok.Body)
	}
	latest, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if !latest.ToolCalls[0].FeedbackSent || latest.Status != "completed" {
		t.Fatalf("feedback not persisted %+v", latest)
	}
	if latest.History[len(latest.History)-2].Type != "function_call_output" {
		t.Fatalf("missing tool result history: %+v", latest.History)
	}
	again := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, path, map[string]any{"expected_revision": latest.Revision, "confirm_external": true})
	if again.Code != 409 || calls != 1 {
		t.Fatalf("tool output was replayed: code=%d calls=%d", again.Code, calls)
	}
}
func TestAgentToolContinuationRequiresFinalNativeEvidence(t *testing.T) {
	s, id, _ := agentPendingCallFixture(t, true)
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	result := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/tools/call_agent_001/continue",
		map[string]any{"expected_revision": x.Revision, "confirm_external": true})
	if result.Code != 409 {
		t.Fatalf("unverified patch result sent to model: %d", result.Code)
	}
}
func TestAgentToolContinuationSharedPermissions(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/agent/sessions/s/tools/c/continue", nil)
	perms := sharedRoutePermissions(r)
	if len(perms) != 3 || perms[0] != identity.PermissionSettingsRead || perms[1] != identity.PermissionPatchView || perms[2] != identity.PermissionFilesRead {
		t.Fatalf("model result egress missing permissions: %+v", perms)
	}
}
