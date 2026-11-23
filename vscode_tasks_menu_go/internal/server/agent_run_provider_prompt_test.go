package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestRunPromptUsesPinnedSnapshotAndExecutionWorkspacePack(t *testing.T) {
	s, sessionID := newAgentConversationFixture(t, "stateless")
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Get(sessionID, "local")
	if err != nil {
		t.Fatal(err)
	}
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	providerStore, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providerStore.Get("local", session.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := provider.FindModel(session.ModelID)
	if !ok {
		t.Fatal("session model is missing")
	}
	version := agentchat.RunProviderConfigVersion(provider, model)
	credentialFingerprint := agentchat.RunCredentialFingerprint([]byte("TEST-KEY"))
	parentTurnID := "turn-" + sessionID + "-revision"
	history := []agentchat.HistoryItem{{Type: "user", Content: "Inspect the pinned context", ParentTurnID: parentTurnID}}
	snapshots, err := s.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	snapshotPolicy := &agentrun.ContextSnapshotPolicy{
		SystemInstructions: "Pinned Agent Run system policy. This concurrent Agent Run is text-only: no tool, Collection or Native Patch side effects are connected to it.", RetryMode: agentchat.RetryOff,
		ReasoningEffort: "",
		RunBinding: &agentrun.RunBindingSnapshot{RunID: "run-prompt", ConversationID: sessionID,
			OwnerWorkspaceID: ownerWorkspaceID, ExecutionWorkspaceID: ownerWorkspaceID, ParentTurnID: parentTurnID,
			ProviderID: provider.ID, ModelID: model.ID, ProviderConfigVersion: version,
			CredentialFingerprint: credentialFingerprint, RetryMode: agentchat.RetryOff},
	}
	snapshot, err := snapshots.CreateWithPolicy(ownerWorkspaceID, sessionID, session.ProjectID, history, nil, snapshotPolicy)
	if err != nil {
		t.Fatal(err)
	}
	var packedWorkspace string
	s.AgentChatPack = func(workspace, _ string) (patchtool.AIPackResult, error) {
		packedWorkspace = workspace
		return patchtool.AIPackResult{TextContent: "PINNED EXECUTION WORKSPACE PACK", ProjectIdentity: patchtool.ProjectIdentity{ID: session.ProjectID, Key: "project"}}, nil
	}
	spec := agentchat.RunSpec{
		RunID: "run-prompt", ConversationID: sessionID, OwnerWorkspaceID: ownerWorkspaceID,
		ExecutionWorkspaceID: ownerWorkspaceID, ProviderID: provider.ID, ModelID: model.ID,
		ProviderConfigVersion: version, CredentialFingerprint: credentialFingerprint,
		ParentTurnID: parentTurnID, RetryMode: agentchat.RetryOff, ContextSnapshotID: snapshot.ID,
		AttemptID: "attempt-1", IdempotencyKey: "request-1",
	}
	options := serverAgentRunPromptOptions{server: s, owner: "local"}
	builder := agentchat.CanonicalProviderPayloadBuilder{Options: options}
	payload, err := builder.Build(spec, provider, model, history)
	if err != nil {
		t.Fatal(err)
	}
	if packedWorkspace != s.Workspace {
		t.Fatalf("AI Pack read %q, want pinned execution workspace %q", packedWorkspace, s.Workspace)
	}
	if err := options.VerifyHistory(spec, history); err != nil {
		t.Fatalf("pinned snapshot verification failed: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatal(err)
	}
	instructions := body["instructions"].(string)
	if !strings.Contains(instructions, "PINNED EXECUTION WORKSPACE PACK") {
		t.Fatalf("run prompt is missing the pinned execution workspace context: %s", instructions)
	}
	// The fixture's model has verified tool-calling evidence and the owner
	// workspace binding grants approval-controlled Patch, so the run may
	// advertise exactly the Native Patch tool and nothing else.
	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected exactly the Native Patch tool schema: %#v", body["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["name"] != "taskdeck_submit_patch" {
		t.Fatalf("unexpected advertised tool: %#v", tools[0])
	}
	encoded, _ := json.Marshal(payload)
	if strings.Contains(string(encoded), "TEST-KEY") {
		t.Fatal("provider credential leaked into the request body")
	}
}

func TestRunPromptAdvertisesPatchToolOnlyWithCapabilityAndScope(t *testing.T) {
	s, sessionID := newAgentConversationFixture(t, "stateless")
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Get(sessionID, "local")
	if err != nil {
		t.Fatal(err)
	}
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	providerStore, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providerStore.Get("local", session.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	model := provider.Models[0]
	version := agentchat.RunProviderConfigVersion(provider, model)
	credentialFingerprint := agentchat.RunCredentialFingerprint([]byte("TEST-KEY"))
	parentTurnID := "turn-" + sessionID + "-tools"
	history := []agentchat.HistoryItem{{Type: "user", Content: "Propose the reviewed patch", ParentTurnID: parentTurnID}}
	snapshots, err := s.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshots.CreateWithPolicy(ownerWorkspaceID, sessionID, session.ProjectID, history, nil, &agentrun.ContextSnapshotPolicy{
		SystemInstructions: "Pinned Agent Run system policy.", RetryMode: agentchat.RetryOff,
		RunBinding: &agentrun.RunBindingSnapshot{RunID: "run-tools", ConversationID: sessionID,
			OwnerWorkspaceID: ownerWorkspaceID, ExecutionWorkspaceID: ownerWorkspaceID, ParentTurnID: parentTurnID,
			ProviderID: provider.ID, ModelID: model.ID, ProviderConfigVersion: version,
			CredentialFingerprint: credentialFingerprint, RetryMode: agentchat.RetryOff},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{
		RunID: "run-tools", ConversationID: sessionID, OwnerWorkspaceID: ownerWorkspaceID,
		ExecutionWorkspaceID: ownerWorkspaceID, ProviderID: provider.ID, ModelID: model.ID,
		ProviderConfigVersion: version, CredentialFingerprint: credentialFingerprint,
		ParentTurnID: parentTurnID, RetryMode: agentchat.RetryOff, ContextSnapshotID: snapshot.ID,
		AttemptID: "attempt-1", IdempotencyKey: "request-tools",
	}
	options := serverAgentRunPromptOptions{server: s, owner: "local"}
	support, err := options.ToolSupport(spec, provider, model)
	if err != nil {
		t.Fatal(err)
	}
	if !support.PatchTool {
		t.Fatal("Patch tool was not advertised after verified tool-calling capability")
	}
	caps, err := s.agentChatCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	if err := caps.Put("local", agentchat.CapabilityRecord{
		ProviderID: provider.ID, ModelID: model.ID,
		Fingerprint:  agentchat.CapabilityFingerprint(provider, model),
		Version:      agentchat.CapabilityProbeVersion,
		Capabilities: map[string]string{agentchat.ProbeToolCalling: agentchat.CapabilityUnsupported},
		ProbedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	support, err = options.ToolSupport(spec, provider, model)
	if err != nil {
		t.Fatal(err)
	}
	if support.PatchTool {
		t.Fatal("Patch tool stayed advertised after tool-calling evidence was withdrawn")
	}
	builder := agentchat.CanonicalProviderPayloadBuilder{Options: options}
	payload, err := builder.Build(spec, provider, model, history)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["tools"]; ok {
		t.Fatalf("run advertised tools without verified tool-calling evidence: %#v", body["tools"])
	}
	if strings.Contains(string(payload), "TEST-KEY") {
		t.Fatal("provider credential leaked into the request body")
	}
}
