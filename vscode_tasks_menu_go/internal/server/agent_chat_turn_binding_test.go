package server

import (
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentPinActiveTurnBindingPersistsAdmissionRecord(t *testing.T) {
	x := &agentchat.Session{ProviderRun: &agentchat.ProviderRun{ID: "turn-9", Phase: "prepared"}}
	provider := agentchat.ProviderConfig{ID: "p", Name: "p", BaseURL: "https://example.test", APIType: "responses"}
	model := agentchat.ModelConfig{ID: "m", Name: "m", MaxOutputTokens: 1000}
	payload := []byte(`{"model":"m"}`)
	if err := agentPinActiveTurnBinding(x, agentchat.Stateless, provider, model, "high", []byte("key"), payload, "req-1"); err != nil {
		t.Fatal(err)
	}
	if x.TurnBinding == nil || x.TurnBinding.TurnID != "turn-9" || x.TurnBinding.ProviderID != "p" || x.TurnBinding.ReasoningEffort != "high" {
		t.Fatalf("unexpected binding: %+v", x.TurnBinding)
	}
	agentClearActiveTurnBinding(x)
	if x.TurnBinding != nil {
		t.Fatal("turn binding must clear after turn completion")
	}
}

func TestAgentSessionAcceptsUserMessageStatuses(t *testing.T) {
	if !agentSessionAcceptsUserMessage("completed") || agentSessionAcceptsUserMessage("requesting_ai") {
		t.Fatal("status classification must separate idle vs busy")
	}
}
