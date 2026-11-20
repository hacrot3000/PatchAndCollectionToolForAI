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
		Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"p1","prompt_kind":"history_action","actions":["detail","cleanup"],"runs":[{"run_id":"run-1","actions":["detail"]}],"constraints":{"destructive_actions":["cleanup"],"cleanup":{"pinned":1,"age_policy":"remove_unpinned_older_than_days","min_days":1,"max_days":3650,"day_options":[7,14,30,60,90,180,365]}}}`),
	}
}

func TestBuildPatchHistoryCleanupPreviewIsPromptBoundAndListFree(t *testing.T) {
	command, cleanupID, err := buildPatchHistoryCleanupCommand(
		historyCleanupState(),
		patchHistoryCleanupRequest{PromptID:"p1", OlderThanDays:30, Confirmed:false},
	)
	if err != nil { t.Fatal(err) }
	if cleanupID == "" { t.Fatal("cleanup id missing") }

	var wire map[string]any
	if err := json.Unmarshal(command, &wire); err != nil { t.Fatal(err) }
	if wire["command"] != "history_cleanup" { t.Fatalf("unexpected command: %#v", wire) }
	payload := wire["payload"].(map[string]any)
	if payload["prompt_id"] != "p1" || payload["confirmed"] != false ||
		payload["older_than_days"] != float64(30) || payload["cleanup_id"] != cleanupID {
		t.Fatalf("unexpected cleanup preview payload: %#v", payload)
	}
	for _, forbidden := range []string{
		"run_id","run_ids","candidates","paths",
		"preview_cleanup_id","cutoff_at","candidate_digest",
	} {
		if _, ok := payload[forbidden]; ok { t.Fatalf("cleanup preview payload must not expose %s", forbidden) }
	}
}

func TestBuildPatchHistoryCleanupDeleteUsesCurrentPreviewBinding(t *testing.T) {
	state := historyCleanupState()
	state.HistoryCleanup = &session.ProtocolHistoryCleanupResultState{
		PromptID:"p1",
		CleanupID:"preview-1",
		Mode:"preview",
		Status:"PASS",
		RC:0,
		Policy:"remove_unpinned_older_than_days",
		EligibleBefore:2,
		OlderThanDays:30,
		CutoffAt:"2026-08-26T10:00:00+00:00",
		CandidateDigest:strings.Repeat("a",64),
	}
	command, cleanupID, err := buildPatchHistoryCleanupCommand(
		state,
		patchHistoryCleanupRequest{
			PromptID:"p1", OlderThanDays:30, Confirmed:true, PreviewCleanupID:"preview-1",
		},
	)
	if err != nil { t.Fatal(err) }
	if cleanupID == "" || cleanupID == "preview-1" { t.Fatalf("unexpected delete cleanup id %q", cleanupID) }

	var wire map[string]any
	if err := json.Unmarshal(command, &wire); err != nil { t.Fatal(err) }
	payload := wire["payload"].(map[string]any)
	if payload["prompt_id"] != "p1" || payload["confirmed"] != true ||
		payload["older_than_days"] != float64(30) ||
		payload["preview_cleanup_id"] != "preview-1" ||
		payload["cutoff_at"] != state.HistoryCleanup.CutoffAt ||
		payload["candidate_digest"] != state.HistoryCleanup.CandidateDigest {
		t.Fatalf("unexpected cleanup delete payload: %#v", payload)
	}
	for _, forbidden := range []string{"run_id","run_ids","candidates","paths"} {
		if _, ok := payload[forbidden]; ok { t.Fatalf("cleanup delete payload must not expose %s", forbidden) }
	}
}

func TestBuildPatchHistoryCleanupRejectsStaleUnsafeOrUnadvertisedRequests(t *testing.T) {
	for _, req := range []patchHistoryCleanupRequest{
		{PromptID:"stale", OlderThanDays:30, Confirmed:false},
		{PromptID:"p1", OlderThanDays:0, Confirmed:false},
		{PromptID:"p1", OlderThanDays:3651, Confirmed:false},
		{PromptID:"p1", OlderThanDays:30, Confirmed:true, PreviewCleanupID:"missing"},
	} {
		if _, _, err := buildPatchHistoryCleanupCommand(historyCleanupState(), req); err == nil {
			t.Fatalf("invalid cleanup request accepted: %#v", req)
		}
	}

	state := historyCleanupState()
	state.HistoryCleanup = &session.ProtocolHistoryCleanupResultState{
		PromptID:"p1", CleanupID:"preview-1", Mode:"preview",
		Policy:"remove_unpinned_older_than_days", OlderThanDays:30,
		CutoffAt:"2026-08-26T10:00:00+00:00", CandidateDigest:strings.Repeat("b",64),
	}
	if _, _, err := buildPatchHistoryCleanupCommand(
		state,
		patchHistoryCleanupRequest{PromptID:"p1",OlderThanDays:30,Confirmed:true,PreviewCleanupID:"other"},
	); err == nil {
		t.Fatal("mismatched cleanup preview id was accepted")
	}

	state = historyCleanupState()
	state.Prompt = json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"p1","prompt_kind":"history_action","actions":["detail"],"constraints":{"destructive_actions":[],"cleanup":{"age_policy":"remove_unpinned_older_than_days","min_days":1,"max_days":3650}}}`)
	if _, _, err := buildPatchHistoryCleanupCommand(
		state,
		patchHistoryCleanupRequest{PromptID:"p1",OlderThanDays:30,Confirmed:false},
	); err == nil {
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
		"OlderThanDays",
		"PreviewCleanupID",
	} {
		if !strings.Contains(src, want) { t.Fatalf("History cleanup endpoint missing %q", want) }
	}
}
