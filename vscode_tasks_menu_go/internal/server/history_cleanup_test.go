package server

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/session"
)

func historyCleanupState() session.ProtocolState {
	return session.ProtocolState{
		Available:true, Enabled:true, CommandsEnabled:true,
		Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"p1","prompt_kind":"history_action","actions":["detail","cleanup"],"runs":[{"run_id":"run-1","actions":["detail"]}],"constraints":{"destructive_actions":["cleanup"],"cleanup":{"eligible":3,"idle_eligible":1,"overflow_eligible":2,"pinned":1,"meaningful":32,"limit":30,"policy":"remove_unpinned_idle_then_oldest_unpinned_over_limit"}}}`),
	}
}

func TestBuildPatchHistoryCleanupCommandIsPromptBoundConfirmedAndListFree(t *testing.T) {
	command, cleanupID, err := buildPatchHistoryCleanupCommand(historyCleanupState(), patchHistoryCleanupRequest{PromptID:"p1", Confirmed:true})
	if err != nil { t.Fatal(err) }
	if cleanupID == "" { t.Fatal("cleanup id missing") }
	var wire map[string]any
	if err := json.Unmarshal(command, &wire); err != nil { t.Fatal(err) }
	if wire["command"] != "history_cleanup" { t.Fatalf("unexpected command: %#v", wire) }
	payload := wire["payload"].(map[string]any)
	if payload["prompt_id"] != "p1" || payload["confirmed"] != true || payload["cleanup_id"] != cleanupID {
		t.Fatalf("unexpected cleanup payload: %#v", payload)
	}
	for _, forbidden := range []string{"run_id","run_ids","candidates","paths"} {
		if _, ok := payload[forbidden]; ok { t.Fatalf("cleanup payload must not expose %s", forbidden) }
	}

	for _, req := range []patchHistoryCleanupRequest{
		{PromptID:"stale", Confirmed:true},
		{PromptID:"p1", Confirmed:false},
	} {
		if _, _, err := buildPatchHistoryCleanupCommand(historyCleanupState(), req); err == nil {
			t.Fatalf("invalid cleanup request accepted: %#v", req)
		}
	}
	state := historyCleanupState()
	state.Prompt = json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"p1","prompt_kind":"history_action","actions":["detail"],"constraints":{"destructive_actions":[],"cleanup":{"eligible":0,"policy":"remove_unpinned_idle_then_oldest_unpinned_over_limit"}}}`)
	if _, _, err := buildPatchHistoryCleanupCommand(state, patchHistoryCleanupRequest{PromptID:"p1",Confirmed:true}); err == nil {
		t.Fatal("unadvertised cleanup was accepted")
	}
}

func TestPublicHistoryCleanupEndpointIsNarrow(t *testing.T) {
	data, err := os.ReadFile("server.go")
	if err != nil { t.Fatal(err) }
	src := string(data)
	for _, want := range []string{
		"case \"history-cleanup\":",
		"patchHistoryCleanupRequest",
		"buildPatchHistoryCleanupCommand(state, req)",
		"http.MaxBytesReader(w, r.Body, 8<<10)",
		"map[string]any{\"accepted\": true, \"cleanup_id\": cleanupID}",
	} {
		if !strings.Contains(src, want) { t.Fatalf("History cleanup endpoint missing %q", want) }
	}
}
