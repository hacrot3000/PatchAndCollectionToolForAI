package server

import (
	"context"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func TestOwnerAgentRunProviderResolverPinsWorkspaceProviderModelAndVersion(t *testing.T) {
	s, _ := newAgentConversationFixture(t, "stateless")
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providers.Get("local", "custom-r2d")
	if err != nil {
		t.Fatal(err)
	}
	model := provider.Models[0]
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{
		RunID: "run-1", ConversationID: "conversation-1", OwnerWorkspaceID: ownerWorkspaceID,
		ExecutionWorkspaceID: ownerWorkspaceID, ProviderID: provider.ID, ModelID: model.ID,
		ProviderConfigVersion: agentchat.RunProviderConfigVersion(provider, model),
		CredentialFingerprint: agentchat.RunCredentialFingerprint([]byte("TEST-KEY")), ContextSnapshotID: "snapshot-1",
		AttemptID: "attempt-1", IdempotencyKey: "request-1",
	}
	resolved, err := (ownerAgentRunProviderResolver{server: s, owner: "local"}).Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Config.ID != spec.ProviderID || resolved.ConfigVersion != spec.ProviderConfigVersion || string(resolved.Credential) != "TEST-KEY" {
		t.Fatalf("resolver returned an incorrect owner-bound provider: %+v", resolved.Config)
	}
	if resolved.StreamingVerified {
		t.Fatal("streaming reported verified without capability evidence")
	}
}

func TestOwnerAgentRunProviderResolverRejectsWorkspaceAndConfigDrift(t *testing.T) {
	s, _ := newAgentConversationFixture(t, "stateless")
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providers.Get("local", "custom-r2d")
	if err != nil {
		t.Fatal(err)
	}
	model := provider.Models[0]
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	resolver := ownerAgentRunProviderResolver{server: s, owner: "local"}
	spec := agentchat.RunSpec{
		RunID: "run-1", ConversationID: "conversation-1", OwnerWorkspaceID: ownerWorkspaceID,
		ExecutionWorkspaceID: ownerWorkspaceID, ProviderID: provider.ID, ModelID: model.ID,
		ProviderConfigVersion: agentchat.RunProviderConfigVersion(provider, model),
		CredentialFingerprint: agentchat.RunCredentialFingerprint([]byte("TEST-KEY")), ContextSnapshotID: "snapshot-1",
		AttemptID: "attempt-1", IdempotencyKey: "request-1",
	}
	wrongWorkspace := spec
	wrongWorkspace.OwnerWorkspaceID = "not-the-owner"
	if _, err := resolver.Resolve(context.Background(), wrongWorkspace); err == nil {
		t.Fatal("mismatched owner workspace was accepted")
	}
	changedConfig := spec
	changedConfig.ProviderConfigVersion = "old-version"
	if _, err := resolver.Resolve(context.Background(), changedConfig); err == nil {
		t.Fatal("provider config drift was silently accepted")
	}
}
