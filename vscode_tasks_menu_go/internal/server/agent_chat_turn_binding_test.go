package server

import (
	"testing"
	"time"

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

// This exercises the dispatch ordering helper, not the complete queued-turn handoff.
func TestAgentReleaseAndDispatchPendingOrdersLeaseRelease(t *testing.T) {
 released := make(chan struct{})
 result := make(chan bool, 1)
 agentReleaseAndDispatchPending(func() { close(released) }, func() {
  select { case <-released: result <- true; default: result <- false }
 })
 select {
 case ok := <-result:
  if !ok { t.Fatal("dispatch ran before lease release") }
 case <-time.After(2 * time.Second):
  t.Fatal("pending dispatch did not run")
 }
}

func TestAgentSessionAcceptsUserMessageStatuses(t *testing.T) {
	if !agentSessionAcceptsUserMessage("completed") || agentSessionAcceptsUserMessage("requesting_ai") {
		t.Fatal("status classification must separate idle vs busy")
	}
}

func TestAgentPendingTurnBindingMatches(t *testing.T) {
	p := agentchat.ProviderConfig{ID: "p", Name: "p", BaseURL: "https://example.test", APIType: "responses"}
	m := agentchat.ModelConfig{ID: "m", Name: "m", MaxOutputTokens: 1000}
	secret := []byte("key")
	pending := agentchat.PendingUserTurn{ProviderID: p.ID, ModelID: m.ID, ProviderConfigVersion: agentchat.RunProviderConfigVersion(p, m), CredentialFingerprint: agentchat.RunCredentialFingerprint(secret)}
	if !agentPendingTurnBindingMatches(pending, p, m, secret) { t.Fatal("matching binding rejected") }
	if agentPendingTurnBindingMatches(pending, p, m, []byte("other")) { t.Fatal("changed credential accepted") }
	changed := m; changed.MaxOutputTokens++
	if agentPendingTurnBindingMatches(pending, p, changed, secret) { t.Fatal("changed configuration accepted") }
	if agentPendingTurnBindingMatches(pending, p, m, nil) { t.Fatal("missing credential accepted") }
}
