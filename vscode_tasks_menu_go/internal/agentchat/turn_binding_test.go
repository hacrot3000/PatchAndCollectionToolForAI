package agentchat

import (
	"testing"
	"time"
)

func TestBuildTurnBindingPinsProviderIdentity(t *testing.T) {
	provider := ProviderConfig{ID: "custom-r2d", Name: "r2d", BaseURL: "https://example.test", APIType: "responses"}
	model := ModelConfig{ID: "composer-2.5", Name: "composer-2.5", MaxOutputTokens: 160000, ReasoningEfforts: []string{"low", "high"}, DefaultReasoningEffort: "low"}
	payload := []byte(`{"model":"composer-2.5"}`)
	binding, err := BuildTurnBinding("turn-1", "attempt-1", "req-1", Stateless, provider, model, "high", []byte("secret"), payload)
	if err != nil {
		t.Fatal(err)
	}
	if binding.ProviderID != provider.ID || binding.ModelID != model.ID || binding.ReasoningEffort != "high" || binding.TurnID != "turn-1" {
		t.Fatalf("unexpected binding: %+v", binding)
	}
	if binding.ProviderConfigVersion != RunProviderConfigVersion(provider, model) {
		t.Fatal("config version not pinned")
	}
	if binding.PayloadDigest != ProviderPayloadDigest(payload) {
		t.Fatal("payload digest mismatch")
	}
}

func TestValidTurnBindingRejectsStaleVersion(t *testing.T) {
	now := time.Now().UTC()
	bad := &TurnBinding{Version: 0, TurnID: "t", ProviderID: "p", ModelID: "m", ProviderConfigVersion: "v", PayloadDigest: ProviderPayloadDigest([]byte("x")), ContextMode: Stateless, AdmittedAt: now}
	if ValidTurnBinding(bad) {
		t.Fatal("expected invalid version to fail")
	}
}
