package server

import (
	"encoding/json"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func TestAgentContextSnapshotReturnsSafePinnedRunBinding(t *testing.T) {
	s, sessionID := newAgentConversationFixture(t, "stateless")
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	providerStore, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := providerStore.Get("local", "custom-r2d")
	if err != nil {
		t.Fatal(err)
	}
	model := provider.Models[0]
	response := invokeAI(t, s.agentContextSnapshotAPI, "POST", "/api/agent/context-snapshots", map[string]any{
		"conversation_id": sessionID, "prompt": "Run this with the selected provider.", "run_id": "run-snapshot",
		"provider_id": provider.ID, "model_id": model.ID,
	})
	if response.Code != 201 {
		t.Fatalf("snapshot request: %d %s", response.Code, response.Body)
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["owner_workspace_id"] != ownerWorkspaceID || result["execution_workspace_id"] != ownerWorkspaceID || result["parent_turn_id"] == "" ||
		result["provider_id"] != provider.ID || result["model_id"] != model.ID ||
		result["provider_config_version"] != agentchat.RunProviderConfigVersion(provider, model) ||
		result["credential_fingerprint"] != agentchat.RunCredentialFingerprint([]byte("TEST-KEY")) {
		t.Fatalf("snapshot did not return the send-time run binding: %#v", result)
	}
	if _, exists := result["api_key"]; exists {
		t.Fatal("snapshot response exposed a provider credential")
	}
	binding, ok := result["run_binding"].(map[string]any)
	if !ok || binding["run_id"] != "run-snapshot" || binding["provider_id"] != provider.ID || binding["model_id"] != model.ID ||
		binding["credential_fingerprint"] != result["credential_fingerprint"] {
		t.Fatalf("snapshot did not persist the complete run identity: %#v", result["run_binding"])
	}
	snapshots, err := s.agentContextSnapshotStore("local")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshots.Get(result["context_snapshot_id"].(string), ownerWorkspaceID, sessionID, result["project_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Policy == nil || snapshot.Policy.RetryMode != agentchat.RetryOff ||
		!strings.Contains(snapshot.Policy.SystemInstructions, "text-only") || strings.Contains(snapshot.Policy.SystemInstructions, "TEST-KEY") {
		t.Fatalf("snapshot did not pin safe prompt/retry policy: %+v", snapshot.Policy)
	}
}

func TestAgentContextSnapshotPinsExplicitReasoningEffort(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 providers,err:=s.agentChatProviders();if err!=nil{t.Fatal(err)}
 provider,err:=providers.Get("local","custom-r2d");if err!=nil{t.Fatal(err)}
 provider.Models[0].ReasoningEfforts=[]string{"low","high"}
 provider.Models[0].DefaultReasoningEffort="low"
 if err=providers.Put("local",provider);err!=nil{t.Fatal(err)}
 body:=map[string]any{"conversation_id":id,"prompt":"Next request","run_id":"effort-test","provider_id":provider.ID,"model_id":provider.Models[0].ID,"reasoning_effort":"high"}
 response:=invokeAI(t,s.agentContextSnapshotAPI,"POST","/api/agent/context-snapshots",body)
 if response.Code!=201{t.Fatalf("snapshot: %d %s",response.Code,response.Body)}
 var result map[string]any
 if err=json.Unmarshal(response.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if result["reasoning_effort"]!="high"{t.Fatalf("wrong effort: %#v",result)}
 snapshots,err:=s.agentContextSnapshotStore("local");if err!=nil{t.Fatal(err)}
 snapshot,err:=snapshots.Get(result["context_snapshot_id"].(string),result["owner_workspace_id"].(string),id,result["project_id"].(string))
 if err!=nil||snapshot.Policy==nil||snapshot.Policy.ReasoningEffort!="high"{t.Fatalf("effort not pinned: %+v %v",snapshot.Policy,err)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 session,err:=store.Get(id,"local");if err!=nil||session.ReasoningEffort!=""{t.Fatal("original conversation settings changed")}
 body["reasoning_effort"]="max"
 invalid:=invokeAI(t,s.agentContextSnapshotAPI,"POST","/api/agent/context-snapshots",body)
 if invalid.Code!=400{t.Fatalf("unsupported effort accepted: %d %s",invalid.Code,invalid.Body)}
}
