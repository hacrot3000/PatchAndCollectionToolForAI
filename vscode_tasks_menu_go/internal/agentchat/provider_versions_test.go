package agentchat

import (
	"path/filepath"
	"testing"
)

func TestProviderStoreRetainsPublicConfigVersionsForPinnedRuns(t *testing.T) {
	store, err := NewProviderStore(filepath.Join(t.TempDir(), "providers"))
	if err != nil {
		t.Fatal(err)
	}
	old := ProviderConfig{ID: "gpt", Name: "GPT", APIType: "responses", BaseURL: "https://old.example/v1", Models: []ModelConfig{{ID: "model-a", Name: "A", MaxInputTokens: 10000}}}
	if err := store.Put("owner", old); err != nil {
		t.Fatal(err)
	}
	oldVersion := RunProviderConfigVersion(old, old.Models[0])
	updated := old
	updated.BaseURL = "https://new.example/v1"
	updated.Models = []ModelConfig{{ID: "model-a", Name: "A", MaxInputTokens: 20000}}
	if err := store.Put("owner", updated); err != nil {
		t.Fatal(err)
	}
	current, currentModel, err := store.GetVersion("owner", "gpt", "model-a", RunProviderConfigVersion(updated, updated.Models[0]))
	if err != nil || current.BaseURL != updated.BaseURL || currentModel.MaxInputTokens != 20000 {
		t.Fatalf("current config version did not resolve: %+v %+v %v", current, currentModel, err)
	}
	pinned, pinnedModel, err := store.GetVersion("owner", "gpt", "model-a", oldVersion)
	if err != nil || pinned.BaseURL != old.BaseURL || pinnedModel.MaxInputTokens != 10000 {
		t.Fatalf("old pinned config was not retained: %+v %+v %v", pinned, pinnedModel, err)
	}
	if _, _, err := store.GetVersion("other-owner", "gpt", "model-a", oldVersion); err == nil {
		t.Fatal("provider configuration history crossed owner scope")
	}
}
