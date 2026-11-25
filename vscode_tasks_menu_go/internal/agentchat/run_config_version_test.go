package agentchat

import "testing"

func TestRunProviderConfigVersionPinsBehavioralSettingsWithoutCredentials(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", BaseURL: "https://provider.example/v1", Stream: true}
	model := ModelConfig{ID: "gpt-6.1-sol", Name: "GPT", ToolCalling: true, Vision: true, MaxInputTokens: 100000, MaxOutputTokens: 4096}
	version := RunProviderConfigVersion(provider, model)
	if version == "" || len(version) != 64 {
		t.Fatalf("invalid config version: %q", version)
	}
	changed := model
	changed.Vision = false
	if RunProviderConfigVersion(provider, changed) == version {
		t.Fatal("model capability drift did not change run config version")
	}
	changedProvider := provider
	changedProvider.BaseURL = "https://other.example/v1"
	if RunProviderConfigVersion(changedProvider, model) == version {
		t.Fatal("provider endpoint drift did not change run config version")
	}
	renamedProvider := provider
	renamedProvider.Name = "renamed display"
	if RunProviderConfigVersion(renamedProvider, model) != version {
		t.Fatal("non-executable display metadata unexpectedly changed binding")
	}
}
