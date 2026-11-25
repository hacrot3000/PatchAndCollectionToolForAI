package server

import (
	"context"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func TestOwnerRunRetryResolvesPinnedConfigAndRejectsRotatedCredential(t *testing.T) {
	s, _ := newAgentConversationFixture(t, "stateless")
	store, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.Get("local", "custom-r2d")
	if err != nil {
		t.Fatal(err)
	}
	model := old.Models[0]
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{
		RunID: "run-retry", ConversationID: "conversation-1", OwnerWorkspaceID: ownerWorkspaceID,
		ExecutionWorkspaceID: ownerWorkspaceID, ProviderID: old.ID, ModelID: model.ID,
		ProviderConfigVersion: agentchat.RunProviderConfigVersion(old, model),
		CredentialFingerprint: agentchat.RunCredentialFingerprint([]byte("TEST-KEY")),
		ContextSnapshotID:     "snapshot-1", AttemptID: "attempt-1", IdempotencyKey: "idem-1",
	}
	updated := old
	updated.BaseURL = "https://new-endpoint.example/v1"
	if err := store.Put("local", updated); err != nil {
		t.Fatal(err)
	}
	resolver := ownerAgentRunProviderResolver{server: s, owner: "local"}
	resolved, err := resolver.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatalf("retry did not resolve the pinned public config: %v", err)
	}
	if resolved.Config.BaseURL != old.BaseURL || resolved.ConfigVersion != spec.ProviderConfigVersion {
		t.Fatalf("retry config drifted to the new endpoint: %+v", resolved.Config)
	}
	if err := s.ConnectionSecrets.Put(agentProviderSecretID(s.agentOwnerWorkspace(), "local", old.ID), []byte("ROTATED-TEST-KEY")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background(), spec); err != agentchat.ErrProviderCredentialChanged {
		t.Fatalf("retry silently used rotated credential: %v", err)
	}
}
