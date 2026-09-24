package server

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/session"
)

func queueSelectionState() session.ProtocolState {
	return session.ProtocolState{
		Available: true,
		Enabled: true,
		CommandsEnabled: true,
		Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":7,"prompt_id":"abc123","prompt_kind":"queue_selection","actions":["select","cancel"],"item_actions":["inspect","preview","validate"],"queue_actions":["delete"],"items":[{"index":1,"name":"a.zip","kind":"PATCH"},{"index":2,"name":"b.zip","kind":"COLLECT"}]}`),
	}
}

func historyActionState() session.ProtocolState {
	return session.ProtocolState{
		Available: true,
		Enabled: true,
		CommandsEnabled: true,
		Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":11,"prompt_id":"history123","prompt_kind":"history_action","actions":["detail"],"runs":[{"run_id":"run-1"},{"run_id":"run-2"}],"constraints":{"read_only":true}}`),
	}
}

func resumeActionState() session.ProtocolState {
	return session.ProtocolState{
		Available: true,
		Enabled: true,
		CommandsEnabled: true,
		Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":9,"prompt_id":"resume123","prompt_kind":"resume_action","actions":["all","failed","remaining","collect_failed","delete_failed","history","normal"],"failed_items":[{"index":1,"name":"a.zip","can_retry":true,"can_collect":true,"can_delete":true},{"index":2,"name":"b.zip","can_retry":false,"can_collect":true,"can_delete":false}],"constraints":{"selection_actions":["failed","collect_failed","delete_failed"]}}`),
	}
}

func TestBuildPatchPromptResponseCommandIsNarrowAndPromptBound(t *testing.T) {
	data, err := buildPatchPromptResponseCommand(queueSelectionState(), patchPromptResponseRequest{
		PromptID: "abc123",
		Action: "select",
		Indexes: []int{1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	var command map[string]any
	if err := json.Unmarshal(data, &command); err != nil {
		t.Fatal(err)
	}
	if command["command"] != "prompt_response" || command["type"] != "command" {
		t.Fatalf("unexpected command envelope: %#v", command)
	}
	payload := command["payload"].(map[string]any)
	if payload["prompt_id"] != "abc123" || payload["action"] != "select" {
		t.Fatalf("unexpected prompt payload: %#v", payload)
	}
	// Go intentionally does not duplicate the Python COLLECT-exclusive rule:
	// indexes 1+2 are transport-valid and Python remains authoritative.
}

func TestBuildPatchPromptResponseCommandRejectsStaleOrOutOfRangeInput(t *testing.T) {
	state := queueSelectionState()
	tests := []patchPromptResponseRequest{
		{PromptID: "stale", Action: "select", Indexes: []int{1}},
		{PromptID: "abc123", Action: "select", Indexes: nil},
		{PromptID: "abc123", Action: "select", Indexes: []int{3}},
		{PromptID: "abc123", Action: "cancel", Indexes: []int{1}},
		{PromptID: "abc123", Action: "other"},
	}
	for _, req := range tests {
		if _, err := buildPatchPromptResponseCommand(state, req); err == nil {
			t.Fatalf("invalid prompt response accepted: %#v", req)
		}
	}
}

func TestPublicPromptResponseEndpointDoesNotExposeRawCommand(t *testing.T) {
	data, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		`case "prompt-response":`,
		"patchPromptResponseRequest",
		"buildPatchPromptResponseCommand(state, req)",
		"session.ProtocolCommandWriter",
		"http.MaxBytesReader(w, r.Body, 64<<10)",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("public prompt response contract missing %q", want)
		}
	}
	if strings.Contains(src, `case "command":`) {
		t.Fatal("TaskDeck public API must not expose the raw protocol command endpoint")
	}
}

func TestBuildPatchItemActionCommandIsNarrowPromptBoundAndPatchOnly(t *testing.T) {
	data, actionID, err := buildPatchItemActionCommand(queueSelectionState(), patchItemActionRequest{PromptID: "abc123", Action: "preview", Index: 1})
	if err != nil { t.Fatal(err) }
	if actionID == "" { t.Fatal("missing action id") }
	var command map[string]any
	if err := json.Unmarshal(data, &command); err != nil { t.Fatal(err) }
	if command["command"] != "item_action" || command["type"] != "command" { t.Fatalf("unexpected command envelope: %#v", command) }
	payload := command["payload"].(map[string]any)
	if payload["prompt_id"] != "abc123" || payload["action"] != "preview" || int(payload["index"].(float64)) != 1 || payload["action_id"] != actionID {
		t.Fatalf("unexpected item_action payload: %#v", payload)
	}
}

func TestBuildPatchItemActionCommandRejectsStaleUnsupportedAndCollect(t *testing.T) {
	state := queueSelectionState()
	tests := []patchItemActionRequest{
		{PromptID: "stale", Action: "preview", Index: 1},
		{PromptID: "abc123", Action: "execute", Index: 1},
		{PromptID: "abc123", Action: "inspect", Index: 0},
		{PromptID: "abc123", Action: "validate", Index: 3},
		{PromptID: "abc123", Action: "inspect", Index: 2},
	}
	for _, req := range tests {
		if _, _, err := buildPatchItemActionCommand(state, req); err == nil { t.Fatalf("invalid item action accepted: %#v", req) }
	}
}

func TestPublicItemActionEndpointDoesNotExposeRawCommand(t *testing.T) {
	data, err := os.ReadFile("server.go")
	if err != nil { t.Fatal(err) }
	src := string(data)
	for _, want := range []string{`case "item-action":`, "patchItemActionRequest", "buildPatchItemActionCommand(state, req)", `"action_id": actionID`, "session.ProtocolCommandWriter", "http.MaxBytesReader(w, r.Body, 16<<10)"} {
		if !strings.Contains(src, want) { t.Fatalf("public item action contract missing %q", want) }
	}
	if strings.Contains(src, `case "command":`) { t.Fatal("TaskDeck public API must not expose the raw protocol command endpoint") }
}


func TestBuildPatchQueueDeleteCommandIsNarrowPromptBoundAndAdvertised(t *testing.T) {
	data, mutationID, err := buildPatchQueueDeleteCommand(queueSelectionState(), patchQueueDeleteRequest{PromptID:"abc123", Index:2})
	if err != nil { t.Fatal(err) }
	if mutationID == "" { t.Fatal("missing mutation id") }
	var command map[string]any
	if err := json.Unmarshal(data, &command); err != nil { t.Fatal(err) }
	if command["command"] != "queue_delete" || command["type"] != "command" {
		t.Fatalf("unexpected Queue delete command: %#v", command)
	}
	payload := command["payload"].(map[string]any)
	if payload["prompt_id"] != "abc123" || payload["mutation_id"] != mutationID || int(payload["index"].(float64)) != 2 {
		t.Fatalf("unexpected Queue delete payload: %#v", payload)
	}
}

func TestBuildPatchQueueDeleteCommandRejectsStaleUnavailableAndUnadvertised(t *testing.T) {
	state := queueSelectionState()
	for _, req := range []patchQueueDeleteRequest{
		{PromptID:"stale", Index:1},
		{PromptID:"abc123", Index:0},
		{PromptID:"abc123", Index:3},
	} {
		if _, _, err := buildPatchQueueDeleteCommand(state, req); err == nil {
			t.Fatalf("invalid Queue delete accepted: %#v", req)
		}
	}
	state.Prompt = json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"abc123","prompt_kind":"queue_selection","items":[{"index":1,"name":"a.zip","kind":"PATCH"}]}`)
	if _, _, err := buildPatchQueueDeleteCommand(state, patchQueueDeleteRequest{PromptID:"abc123", Index:1}); err == nil {
		t.Fatal("Queue delete accepted without advertised queue_actions capability")
	}
}

func TestPublicQueueDeleteEndpointDoesNotExposeRawCommand(t *testing.T) {
	data, err := os.ReadFile("server.go")
	if err != nil { t.Fatal(err) }
	src := string(data)
	for _, want := range []string{
		`case "queue-delete":`,
		"patchQueueDeleteRequest",
		"buildPatchQueueDeleteCommand(state, req)",
		`"mutation_id": mutationID`,
		"session.ProtocolCommandWriter",
		"http.MaxBytesReader(w, r.Body, 16<<10)",
	} {
		if !strings.Contains(src, want) { t.Fatalf("public Queue delete contract missing %q", want) }
	}
	if strings.Contains(src, `case "command":`) { t.Fatal("TaskDeck public API must not expose the raw protocol command endpoint") }
}


func TestBuildPatchResumeActionCommandIsNarrowPromptBoundAndCapabilityChecked(t *testing.T) {
	data, err := buildPatchResumeActionCommand(resumeActionState(), patchResumeActionRequest{
		PromptID: "resume123",
		Action: "failed",
		FailedIndexes: []int{1},
	})
	if err != nil { t.Fatal(err) }
	var command map[string]any
	if err := json.Unmarshal(data, &command); err != nil { t.Fatal(err) }
	if command["command"] != "resume_action" || command["type"] != "command" {
		t.Fatalf("unexpected Resume command envelope: %#v", command)
	}
	payload := command["payload"].(map[string]any)
	if payload["prompt_id"] != "resume123" || payload["action"] != "failed" {
		t.Fatalf("unexpected Resume payload: %#v", payload)
	}
	indexes := payload["failed_indexes"].([]any)
	if len(indexes) != 1 || int(indexes[0].(float64)) != 1 {
		t.Fatalf("unexpected failed indexes: %#v", indexes)
	}
}

func TestBuildPatchResumeActionCommandRejectsStaleUnsupportedAndUnavailable(t *testing.T) {
	state := resumeActionState()
	tests := []patchResumeActionRequest{
		{PromptID: "stale", Action: "all"},
		{PromptID: "resume123", Action: "execute"},
		{PromptID: "resume123", Action: "failed"},
		{PromptID: "resume123", Action: "failed", FailedIndexes: []int{2}},
		{PromptID: "resume123", Action: "delete_failed", FailedIndexes: []int{2}},
		{PromptID: "resume123", Action: "all", FailedIndexes: []int{1}},
		{PromptID: "resume123", Action: "collect_failed", FailedIndexes: []int{3}},
	}
	for _, req := range tests {
		if _, err := buildPatchResumeActionCommand(state, req); err == nil {
			t.Fatalf("invalid Resume action accepted: %#v", req)
		}
	}
	if _, err := buildPatchResumeActionCommand(state, patchResumeActionRequest{PromptID:"resume123", Action:"collect_failed", FailedIndexes:[]int{2}}); err != nil {
		t.Fatalf("advertised collect_failed capability rejected: %v", err)
	}
}

func TestPublicResumeActionEndpointDoesNotExposeRawCommand(t *testing.T) {
	data, err := os.ReadFile("server.go")
	if err != nil { t.Fatal(err) }
	src := string(data)
	for _, want := range []string{
		`case "resume-action":`,
		"patchResumeActionRequest",
		"buildPatchResumeActionCommand(state, req)",
		"session.ProtocolCommandWriter",
		"http.MaxBytesReader(w, r.Body, 16<<10)",
		`"TASKDECK_PATCH_NATIVE_RESUME": "1"`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("public Resume action contract missing %q", want)
		}
	}
	if strings.Contains(src, `case "command":`) {
		t.Fatal("TaskDeck public API must not expose the raw protocol command endpoint")
	}
}


func TestBuildPatchHistoryDetailCommandIsNarrowPromptBoundAndAdvertised(t *testing.T) {
	data, err := buildPatchHistoryDetailCommand(historyActionState(), patchHistoryDetailRequest{PromptID:"history123", RunID:"run-2"})
	if err != nil { t.Fatal(err) }
	var command map[string]any
	if err := json.Unmarshal(data, &command); err != nil { t.Fatal(err) }
	if command["command"] != "history_detail" || command["type"] != "command" {
		t.Fatalf("unexpected History command envelope: %#v", command)
	}
	payload := command["payload"].(map[string]any)
	if payload["prompt_id"] != "history123" || payload["run_id"] != "run-2" {
		t.Fatalf("unexpected History detail payload: %#v", payload)
	}
}

func TestBuildPatchHistoryDetailCommandRejectsStaleOrUnadvertisedRun(t *testing.T) {
	state := historyActionState()
	for _, req := range []patchHistoryDetailRequest{
		{PromptID:"stale", RunID:"run-1"},
		{PromptID:"history123", RunID:""},
		{PromptID:"history123", RunID:"not-advertised"},
		{PromptID:"history123", RunID:strings.Repeat("x", 129)},
	} {
		if _, err := buildPatchHistoryDetailCommand(state, req); err == nil {
			t.Fatalf("invalid History detail accepted: %#v", req)
		}
	}
	wrong := state
	wrong.Prompt = json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":1,"prompt_id":"history123","prompt_kind":"queue_selection","actions":["detail"],"runs":[{"run_id":"run-1"}]}`)
	if _, err := buildPatchHistoryDetailCommand(wrong, patchHistoryDetailRequest{PromptID:"history123",RunID:"run-1"}); err == nil {
		t.Fatal("History detail accepted against non-History prompt")
	}
}

func TestPublicHistoryDetailEndpointDoesNotExposeRawCommand(t *testing.T) {
	data, err := os.ReadFile("server.go")
	if err != nil { t.Fatal(err) }
	src := string(data)
	for _, want := range []string{
		`case "history-detail":`,
		"patchHistoryDetailRequest",
		"buildPatchHistoryDetailCommand(state, req)",
		"session.ProtocolCommandWriter",
		"http.MaxBytesReader(w, r.Body, 16<<10)",
		`"TASKDECK_PATCH_NATIVE_HISTORY": "1"`,
	} {
		if !strings.Contains(src, want) { t.Fatalf("public History detail contract missing %q", want) }
	}
	if strings.Contains(src, `case "command":`) {
		t.Fatal("TaskDeck public API must not expose the raw protocol command endpoint")
	}
}
