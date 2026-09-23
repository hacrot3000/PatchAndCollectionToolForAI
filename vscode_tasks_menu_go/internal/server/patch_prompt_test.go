package server

import (
	"encoding/json"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/session"
)

func queueSelectionState() session.ProtocolState {
	return session.ProtocolState{
		Available: true,
		Enabled: true,
		CommandsEnabled: true,
		Prompt: json.RawMessage(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","seq":7,"prompt_id":"abc123","prompt_kind":"queue_selection","actions":["select","cancel"],"items":[{"index":1,"name":"a.zip","kind":"PATCH"},{"index":2,"name":"b.zip","kind":"COLLECT"}]}`),
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
