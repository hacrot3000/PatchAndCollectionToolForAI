package server

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func TestCoordinationAPIExposesPeerAskAndAnswerKinds(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	ask := invokeAI(t, s.agentCoordinationAPI, http.MethodPost, "/api/agent/coordination", map[string]any{
		"kind": "peer_message", "message_kind": "ask", "conversation_id": "peer-a",
		"to_conversation_id": "peer-b", "group_id": "group-a", "body": "What is the API signature?",
	})
	if ask.Code != http.StatusOK {
		t.Fatalf("publish peer ask: HTTP %d: %s", ask.Code, ask.Body)
	}
	var askBody struct {
		Message agentrun.PeerMessage `json:"message"`
	}
	if err := json.Unmarshal(ask.Body.Bytes(), &askBody); err != nil {
		t.Fatal(err)
	}
	if askBody.Message.Kind != "ask" || askBody.Message.CorrelationID == "" {
		t.Fatalf("peer ask lost its kind or correlation ID: %+v", askBody.Message)
	}
	answer := invokeAI(t, s.agentCoordinationAPI, http.MethodPost, "/api/agent/coordination", map[string]any{
		"kind": "peer_message", "message_kind": "answer", "conversation_id": "peer-b",
		"to_conversation_id": "peer-a", "group_id": "group-a", "body": "authenticate(token: String): Result",
		"correlation_id": askBody.Message.CorrelationID,
	})
	if answer.Code != http.StatusOK {
		t.Fatalf("answer peer ask: HTTP %d: %s", answer.Code, answer.Body)
	}
	response := invokeAI(t, s.agentCoordinationAPI, http.MethodGet, "/api/agent/coordination?conversation_id=peer-a&group_id=group-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("read peer messages: HTTP %d: %s", response.Code, response.Body)
	}
	var result struct {
		Messages []agentrun.PeerMessage `json:"messages"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 1 || result.Messages[0].Status != "answered" || result.Messages[0].Answer != "authenticate(token: String): Result" {
		t.Fatalf("answer did not resolve the original peer ask: %+v", result.Messages)
	}
}
