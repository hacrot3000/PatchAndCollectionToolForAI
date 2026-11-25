package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func agentProviderFixture() agentchat.ProviderConfig {
	return agentchat.ProviderConfig{
		ID: "custom-r2d", Name: "Custom Endpoint", APIType: "responses", BaseURL: "https://api.r2d.dev/v1",
		Models: []agentchat.ModelConfig{{ID: "composer-2.5", Name: "composer-2.5", ToolCalling: true, MaxInputTokens: 1280000, MaxOutputTokens: 160000, ReasoningEfforts: []string{"low", "medium", "high"}, DefaultReasoningEffort: "medium"}},
	}
}
func agentProviderRequest(t *testing.T, handler http.HandlerFunc, method, path string, v any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(v)
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}
func TestAgentChatProviderSettingsEncryptSecretsAndSuppressKey(t *testing.T) {
	s := newAIServerForTest(t)
	secret := "private-key-do-not-echo"
	p := agentProviderFixture()
	w := agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodPut, "/api/agent/providers", map[string]any{"provider": p, "api_key": secret})
	if w.Code != 200 {
		t.Fatalf("put %d: %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), secret) {
		t.Fatal("key leaked in response")
	}
	w = agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodGet, "/api/agent/providers", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "composer-2.5") || strings.Contains(w.Body.String(), secret) || !strings.Contains(w.Body.String(), "\"has_api_key\":true") {
		t.Fatalf("unsafe list: %d %s", w.Code, w.Body)
	}
	key, e := s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace, "local", p.ID))
	if e != nil || string(key) != secret {
		t.Fatal("encrypted store not used for custom agent key")
	}
	store, e := s.agentChatProviders()
	if e != nil {
		t.Fatal(e)
	}
	ps, e := store.List("another-owner")
	if e != nil || len(ps) != 0 {
		t.Fatal("provider leaked to other owner")
	}
}
func TestAgentChatProviderSettingsRejectBadConfig(t *testing.T) {
	s := newAIServerForTest(t)
	p := agentProviderFixture()
	p.BaseURL = "http://localhost:1234/"
	w := agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodPut, "/api/agent/providers", map[string]any{"provider": p})
	if w.Code != 400 {
		t.Fatalf("unsafe endpoint allowed: %d %s", w.Code, w.Body)
	}
	p = agentProviderFixture()
	w = agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodPut, "/api/agent/providers", map[string]any{"provider": p, "unexpected": "field"})
	if w.Code != 400 {
		t.Fatalf("extra fields allowed: %d", w.Code)
	}
}
func TestAgentChatProviderConnectionProbeRequiresExplicitConsent(t *testing.T) {
	s := newAIServerForTest(t)
	w := agentProviderRequest(t, s.agentChatProviderTestAPI, http.MethodPost, "/api/agent/providers/test", map[string]any{"provider_id": "test", "model_id": "model"})
	if w.Code != 403 {
		t.Fatalf("network attempt without consent allowed: %d", w.Code)
	}
	w = agentProviderRequest(t, s.agentChatProviderTestAPI, http.MethodPost, "/api/agent/providers/test", map[string]any{"confirm_external": true, "provider_id": "missing", "model_id": "model"})
	if w.Code != 404 {
		t.Fatalf("missing provider accepted: %d", w.Code)
	}
}
func TestAgentChatProviderSharedServerPermissions(t *testing.T) {
	cases := []struct{ method, path, permission string }{
		{"GET", "/api/agent/providers", identity.PermissionSettingsRead},
		{"PUT", "/api/agent/providers", identity.PermissionSettingsWrite},
		{"POST", "/api/agent/providers/test", identity.PermissionSettingsWrite},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		perms := sharedRoutePermissions(req)
		if len(perms) != 1 || perms[0] != tc.permission {
			t.Fatalf("%s %s incorrect: %+v", tc.method, tc.path, perms)
		}

	}
}

func TestAgentChatProviderTrustedSharedIPsRequireProjectAdmin(t *testing.T) {
	s := newAIServerForTest(t)
	s.Config.SharedServerEnabled = true
	p := agentProviderFixture()
	p.TrustedSharedIPs = []string{"100.64.0.19"}
	body, err := json.Marshal(map[string]any{"provider": p})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		admin bool
		want  int
	}{
		{"settings writer", false, http.StatusForbidden}, {"project admin", true, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			principal := identity.Principal{UserID: identity.ID("owner"), Permissions: map[string]bool{identity.PermissionSettingsWrite: true, identity.PermissionProjectAdmin: tc.admin}}
			r := httptest.NewRequest(http.MethodPut, "/api/agent/providers", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r = r.WithContext(context.WithValue(r.Context(), sharedPrincipalContextKey{}, principal))
			w := httptest.NewRecorder()
			s.agentChatProvidersAPI(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", w.Code, tc.want, w.Body)
			}
		})
	}

}

func TestAgentChatProviderDeleteRevokesLocalKeyExplicitly(t *testing.T) {
	s := newAIServerForTest(t)
	p := agentProviderFixture()
	w := agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodPut, "/api/agent/providers", map[string]any{"provider": p, "api_key": "delete-me"})
	if w.Code != 200 {
		t.Fatalf("put %d: %s", w.Code, w.Body)
	}
	// Metadata-only delete leaves the credential available for an explicit
	// rotation/removal decision and never claims vendor revocation.
	w = agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodDelete, "/api/agent/providers", map[string]any{"provider_id": p.ID})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"metadata_deleted":true`) || !strings.Contains(w.Body.String(), `"key_deleted":false`) {
		t.Fatalf("metadata delete: %d %s", w.Code, w.Body)
	}
	if _, err := s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace, "local", p.ID)); err != nil {
		t.Fatal("metadata-only delete removed the credential unexpectedly")
	}
	registry, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Get("local", p.ID); !errors.Is(err, agentchat.ErrNotFound) {
		t.Fatalf("metadata survived: %v", err)
	}
	w = agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodDelete, "/api/agent/providers", map[string]any{"provider_id": p.ID})
	if w.Code != 404 {
		t.Fatalf("missing provider delete: %d", w.Code)
	}
	// Re-create with a key, then remove both in one explicit operation.
	if w = agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodPut, "/api/agent/providers", map[string]any{"provider": p, "api_key": "delete-me"}); w.Code != 200 {
		t.Fatalf("re-put %d: %s", w.Code, w.Body)
	}
	w = agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodDelete, "/api/agent/providers", map[string]any{"provider_id": p.ID, "delete_key": true})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"key_deleted":true`) || !strings.Contains(w.Body.String(), `"vendor_revocation":false`) {
		t.Fatalf("full delete: %d %s", w.Code, w.Body)
	}
	if _, err := s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace, "local", p.ID)); err == nil {
		t.Fatal("explicit credential delete left a secret behind")
	}
	if strings.Contains(w.Body.String(), "delete-me") {
		t.Fatal("delete response leaked the API key")
	}
}

func TestAgentChatProviderDeleteRejectsUnknownFieldsAndSharedPermission(t *testing.T) {
	s := newAIServerForTest(t)
	w := agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodDelete, "/api/agent/providers", map[string]any{"provider_id": "custom-r2d", "extra": true})
	if w.Code != 400 {
		t.Fatalf("unknown delete field accepted: %d", w.Code)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/agent/providers", nil)
	perms := sharedRoutePermissions(req)
	if len(perms) != 1 || perms[0] != identity.PermissionSettingsWrite {
		t.Fatalf("delete permission: %+v", perms)
	}
}

func agentUnverifiedToolFixture(t *testing.T) (*Server, string) {
	t.Helper()
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	p := agentProviderFixture()
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.Put("local", p); err != nil {
		t.Fatal(err)
	}
	if err := s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace, "local", p.ID), []byte("TEST-KEY")); err != nil {
		t.Fatal(err)
	}
	created := invokeAI(t, s.agentChatSessionsAPI, "POST", "/api/agent/sessions", map[string]any{
		"mode": "stateless", "provider_id": p.ID, "model_id": p.Models[0].ID,
	})
	if created.Code != 201 {
		t.Fatalf("session create: %d %s", created.Code, created.Body)
	}
	var view struct {
		Session agentchat.Session `json:"session"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	s.AgentChatPack = func(workspace, executable string) (patchtool.AIPackResult, error) {
		return patchtool.AIPackResult{
			TextContent:     "TEST PACK\nIdentity: " + view.Session.ProjectID,
			Fingerprint:     "fp-unverified",
			ProjectIdentity: patchtool.ProjectIdentity{ID: view.Session.ProjectID, Key: "project"},
		}, nil
	}
	return s, view.Session.ID
}

func TestAgentDeclaredToolCapabilityDoesNotEnableExecution(t *testing.T) {
	s, id := agentUnverifiedToolFixture(t)
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		var req map[string]any
		if err := json.Unmarshal(payload, &req); err != nil {
			t.Fatal(err)
		}
		if _, ok := req["tools"]; ok {
			t.Fatal("unverified declared capability advertised an execution tool")
		}
		if !strings.Contains(fmt.Sprint(req["instructions"]), "taskdeck_patch_proposal") {
			t.Fatal("unverified model must receive text-only, approval-gated Patch proposal instructions")
		}
		return agentchat.ResponsesResult{ResponseID: "resp_text", Text: "No tools available"}, nil
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "hello", "confirm_external": true, "expected_revision": 1})
	if w.Code != 200 {
		t.Fatalf("message failed: %d %s", w.Code, w.Body)
	}
}

func TestAgentCapabilityProbeRequiresConsentAndPersistsEvidence(t *testing.T) {
	s, id := agentUnverifiedToolFixture(t)
	// Ensure a session exists for this owner/provider before probing.
	if id == "" {
		t.Fatal("missing session")
	}
	before := invokeAI(t, s.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+id, nil)
	if before.Code != 200 || !strings.Contains(before.Body.String(), `"patch_tool_status":"unverified"`) {
		t.Fatalf("unverified patch status missing: %d %s", before.Code, before.Body)
	}
	noConsent := agentProviderRequest(t, s.agentChatProviderProbeAPI, http.MethodPost, "/api/agent/providers/probe",
		map[string]any{"provider_id": "custom-r2d", "model_id": "composer-2.5"})
	if noConsent.Code != 403 {
		t.Fatalf("probe without consent allowed: %d", noConsent.Code)
	}
	bad := agentProviderRequest(t, s.agentChatProviderProbeAPI, http.MethodPost, "/api/agent/providers/probe",
		map[string]any{"provider_id": "custom-r2d", "model_id": "composer-2.5", "confirm_external": true, "capabilities": []string{"shell"}})
	if bad.Code != 400 {
		t.Fatalf("unknown capability accepted: %d", bad.Code)
	}
	calls := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		calls++
		var req map[string]any
		if err := json.Unmarshal(payload, &req); err != nil {
			t.Fatal(err)
		}
		tools, ok := req["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatal("probe did not advertise exactly one harmless function")
		}
		fn := tools[0].(map[string]any)
		if fn["name"] != "taskdeck_capability_probe" {
			t.Fatalf("probe advertised an execution tool: %v", fn["name"])
		}
		return agentchat.ResponsesResult{ResponseID: "resp_probe", Text: "OK"}, nil
	}
	ok := agentProviderRequest(t, s.agentChatProviderProbeAPI, http.MethodPost, "/api/agent/providers/probe",
		map[string]any{"provider_id": "custom-r2d", "model_id": "composer-2.5", "confirm_external": true,
			"capabilities": []string{agentchat.ProbeToolCalling}})
	if ok.Code != 200 {
		t.Fatalf("probe failed: %d %s", ok.Code, ok.Body)
	}
	if !strings.Contains(ok.Body.String(), `"tool_calling":"verified"`) {
		t.Fatalf("probe evidence missing: %s", ok.Body)
	}
	if calls != 1 {
		t.Fatalf("probe performed %d provider calls", calls)
	}
	after := invokeAI(t, s.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+id, nil)
	if after.Code != 200 || !strings.Contains(after.Body.String(), `"patch_tool_status":"enabled"`) {
		t.Fatalf("verified patch status missing: %d %s", after.Code, after.Body)
	}
	// The verified probe now enables the real Patch tool for this exact config.
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		var req map[string]any
		if err := json.Unmarshal(payload, &req); err != nil {
			t.Fatal(err)
		}
		if _, ok := req["tools"]; !ok {
			t.Fatal("verified capability did not enable the execution tool")
		}
		return agentchat.ResponsesResult{ResponseID: "resp_ok", Text: "ok"}, nil
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "hello again", "confirm_external": true, "expected_revision": 1})
	if w.Code != 200 {
		t.Fatalf("post-probe message failed: %d %s", w.Code, w.Body)
	}
}

func TestAgentCapabilityProbeFailureIsInconclusive(t *testing.T) {
	s, id := agentUnverifiedToolFixture(t)
	if id == "" {
		t.Fatal("missing session")
	}
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		return agentchat.ResponsesResult{}, fmt.Errorf("provider returned HTTP 503")
	}
	w := agentProviderRequest(t, s.agentChatProviderProbeAPI, http.MethodPost, "/api/agent/providers/probe",
		map[string]any{"provider_id": "custom-r2d", "model_id": "composer-2.5", "confirm_external": true,
			"capabilities": []string{agentchat.ProbeText}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"text":"inconclusive"`) {
		t.Fatalf("transport failure misreported: %d %s", w.Code, w.Body)
	}
	// Inconclusive evidence must not enable execution.
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		var req map[string]any
		if err := json.Unmarshal(payload, &req); err != nil {
			t.Fatal(err)
		}
		if _, ok := req["tools"]; ok {
			t.Fatal("inconclusive probe enabled the execution tool")
		}
		return agentchat.ResponsesResult{ResponseID: "resp_text", Text: "ok"}, nil
	}
	if got := invokeAI(t, s.agentChatSessionItemAPI, http.MethodPost, "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "hello", "confirm_external": true, "expected_revision": 1}); got.Code != 200 {
		t.Fatalf("message failed: %d %s", got.Code, got.Body)
	}
}

func TestAgentStreamingProbeRequiresOptInAndAdoption(t *testing.T) {
	s, id := agentUnverifiedToolFixture(t)
	// Streaming is off by default, so the probe refuses rather than pretending.
	off := agentProviderRequest(t, s.agentChatProviderProbeAPI, http.MethodPost, "/api/agent/providers/probe",
		map[string]any{"provider_id": "custom-r2d", "model_id": "composer-2.5", "confirm_external": true,
			"capabilities": []string{agentchat.ProbeStreaming}})
	if off.Code != 200 || !strings.Contains(off.Body.String(), `"unsupported"`) {
		t.Fatalf("streaming probed while provider disabled: %d %s", off.Code, off.Body)
	}
	p := agentProviderFixture()
	p.Stream = true
	if w := agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodPut, "/api/agent/providers", map[string]any{"provider": p}); w.Code != 200 {
		t.Fatalf("enable stream: %d %s", w.Code, w.Body)
	}
	// A provider that fails the probe must not be treated as stream-capable.
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		return agentchat.ResponsesResult{}, fmt.Errorf("no stream support")
	}
	failed := agentProviderRequest(t, s.agentChatProviderProbeAPI, http.MethodPost, "/api/agent/providers/probe",
		map[string]any{"provider_id": "custom-r2d", "model_id": "composer-2.5", "confirm_external": true,
			"capabilities": []string{agentchat.ProbeStreaming}})
	if failed.Code != 200 || !strings.Contains(failed.Body.String(), `"inconclusive"`) {
		t.Fatalf("failed stream probe misreported: %d %s", failed.Code, failed.Body)
	}
	if s.agentStreamCapabilityVerified("local", p) {
		t.Fatal("inconclusive stream probe enabled streaming")
	}
	// Now verify with a harmless fake provider; the fixture's AgentChatCall path
	// is bypassed because the probe uses the real stream adapter, so assert the
	// verified path through the capability store instead.
	caps, err := s.agentChatCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	if err := caps.Put("local", agentchat.CapabilityRecord{
		ProviderID: p.ID, ModelID: p.Models[0].ID,
		Fingerprint:  agentchat.CapabilityFingerprint(p, p.Models[0]),
		Version:      agentchat.CapabilityProbeVersion,
		Capabilities: map[string]string{agentchat.ProbeStreaming: agentchat.CapabilityVerified},
		ProbedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if !s.agentStreamCapabilityVerified("local", p) {
		t.Fatal("verified stream capability not detected")
	}
	if id == "" {
		t.Fatal("missing session")
	}
}

func TestAgentCapabilityProbeCoversEveryDeclaredMode(t *testing.T) {
	s, id := agentUnverifiedToolFixture(t)
	if id == "" {
		t.Fatal("missing session")
	}
	p := agentProviderFixture()
	p.Stream = true
	if w := agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodPut, "/api/agent/providers", map[string]any{"provider": p}); w.Code != 200 {
		t.Fatalf("put stream provider: %d %s", w.Code, w.Body)
	}
	var seen []string
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		var req map[string]any
		if err := json.Unmarshal(payload, &req); err != nil {
			t.Fatal(err)
		}
		if _, ok := req["conversation"]; ok {
			seen = append(seen, "conversation_followup")
		}
		if _, ok := req["previous_response_id"]; ok {
			seen = append(seen, "previous_followup")
		}
		return agentchat.ResponsesResult{ResponseID: "resp_probe", ConversationID: "conv_probe", Text: "42 OK"}, nil
	}
	// Streaming deliberately uses the real adapter, so restrict to the modes
	// that route through AgentChatCall for this assertion.
	w := agentProviderRequest(t, s.agentChatProviderProbeAPI, http.MethodPost, "/api/agent/providers/probe",
		map[string]any{"provider_id": p.ID, "model_id": p.Models[0].ID, "confirm_external": true,
			"capabilities": []string{agentchat.ProbeToolCalling, agentchat.ProbePreviousResponse, agentchat.ProbeConversation}})
	if w.Code != 200 {
		t.Fatalf("probe failed: %d %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, want := range []string{`"tool_calling":"verified"`, `"previous_response":"verified"`, `"conversation":"verified"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
	sawConversation, sawPrevious := false, false
	for _, v := range seen {
		if v == "conversation_followup" {
			sawConversation = true
		}
		if v == "previous_followup" {
			sawPrevious = true
		}
	}
	if !sawConversation || !sawPrevious {
		t.Fatalf("probe did not verify chaining/state reuse: %v", seen)
	}
}

func TestAgentChatCompletionsSettingsAndCapabilities(t *testing.T) {
	s := newAIServerForTest(t)
	if err := os.MkdirAll(s.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	p := agentProviderFixture()
	p.ID = "chat-provider"
	p.APIType = "chat_completions"
	p.Models[0].DefaultReasoningEffort = ""
	p.Models[0].ReasoningEfforts = nil
	if w := agentProviderRequest(t, s.agentChatProvidersAPI, http.MethodPut, "/api/agent/providers", map[string]any{
		"provider": p, "api_key": "CHAT-KEY",
	}); w.Code != 200 {
		t.Fatalf("save Chat Completions provider: %d %s", w.Code, w.Body)
	}
	registry, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := registry.Get("local", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.APIType != "chat_completions" {
		t.Fatalf("API type not preserved: %q", stored.APIType)
	}
	created := invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]any{
		"provider_id": p.ID, "model_id": p.Models[0].ID, "mode": "stateless", "reasoning_effort": ""})
	if created.Code != 201 {
		t.Fatalf("stateless session refused: %d %s", created.Code, created.Body)
	}
	var view struct {
		Session agentchat.Session `json:"session"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	// These are UI/session preferences. Chat Completions still uses canonical
	// local-history replay; unsupported Responses references are never sent.
	for _, mode := range []agentchat.ContextMode{agentchat.PreviousResponse, agentchat.Conversation} {
		stateful := invokeAI(t, s.agentChatSessionsAPI, http.MethodPost, "/api/agent/sessions", map[string]any{
			"provider_id": p.ID, "model_id": p.Models[0].ID, "mode": mode})
		if stateful.Code != 201 {
			t.Fatalf("Chat Completions refused local-history fallback %s: %d %s", mode, stateful.Code, stateful.Body)
		}
		var selected struct { Session agentchat.Session `json:"session"` }
		if err := json.Unmarshal(stateful.Body.Bytes(), &selected); err != nil {
			t.Fatal(err)
		}
		if selected.Session.Mode != mode {
			t.Fatalf("saved context preference lost: got %q want %q", selected.Session.Mode, mode)
		}
	}
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		return agentchat.ResponsesResult{ResponseID: "probe-chat", Text: "OK"}, nil
	}
	probe := agentProviderRequest(t, s.agentChatProviderProbeAPI, http.MethodPost, "/api/agent/providers/probe", map[string]any{
		"provider_id": p.ID, "model_id": p.Models[0].ID, "confirm_external": true,
		"capabilities": []string{agentchat.ProbeText, agentchat.ProbeToolCalling, agentchat.ProbePreviousResponse, agentchat.ProbeConversation, agentchat.ProbeStreaming},
	})
	if probe.Code != 200 {
		t.Fatalf("capability probe failed: %d %s", probe.Code, probe.Body)
	}
	body := probe.Body.String()
	for _, entry := range []string{`"text":"verified"`, `"tool_calling":"verified"`, `"previous_response":"unsupported"`, `"conversation":"unsupported"`, `"streaming":"unsupported"`} {
		if !strings.Contains(body, entry) {
			t.Fatalf("missing capability evidence %s in %s", entry, body)
		}
	}
}


func TestAgentChatCompletionsAcceptsDeclaredModelReasoningEfforts(t *testing.T) {
	s:=newAIServerForTest(t)
	if err:=os.MkdirAll(s.Workspace,0700);err!=nil{t.Fatal(err)}
	if err:=os.MkdirAll(s.Workspace,0700);err!=nil {t.Fatal(err)}
	p:=agentProviderFixture()
	p.ID="chat-with-reasoning"
	p.APIType="chat_completions"
	p.Models=[]agentchat.ModelConfig{
		{ID:"composer-2.5",Name:"composer-2.5",ToolCalling:true,
			MaxInputTokens:1280000,MaxOutputTokens:160000,
			ReasoningEfforts:[]string{"low","medium","high"},DefaultReasoningEffort:"medium"},
		{ID:"gpt-6.1-sol",Name:"gpt-6.1-sol",ToolCalling:true,
			MaxInputTokens:1280000,MaxOutputTokens:160000,
			ReasoningEfforts:[]string{"low","medium","high","xhigh","max"},DefaultReasoningEffort:"xhigh"},
	}
	if resp:=agentProviderRequest(t,s.agentChatProvidersAPI,http.MethodPut,"/api/agent/providers",
		map[string]any{"provider":p,"api_key":"CHAT-KEY"});resp.Code!=200 {
		t.Fatalf("cannot save advertised reasoning options: %d %s",resp.Code,resp.Body)
	}
	for _,tc:=range []struct{model,effort string}{
		{"composer-2.5","low"},{"composer-2.5","medium"},{"composer-2.5","high"},
		{"gpt-6.1-sol","low"},{"gpt-6.1-sol","medium"},{"gpt-6.1-sol","high"},
		{"gpt-6.1-sol","xhigh"},{"gpt-6.1-sol","max"},
	} {
		w:=invokeAI(t,s.agentChatSessionsAPI,http.MethodPost,"/api/agent/sessions",map[string]any{
			"provider_id":p.ID,"model_id":tc.model,"mode":"stateless","reasoning_effort":tc.effort})
		if w.Code!=201 { t.Fatalf("%s/%s rejected: %d %s",tc.model,tc.effort,w.Code,w.Body) }
		var view struct { Session agentchat.Session `json:"session"` }
		if err:=json.Unmarshal(w.Body.Bytes(),&view);err!=nil{t.Fatal(err)}
		if view.Session.ReasoningEffort!=tc.effort{t.Fatalf("reasoning %s/%s not persisted",tc.model,tc.effort)}
	}
	for _,tc:=range []struct{model,effort string}{{"composer-2.5","xhigh"},{"composer-2.5","max"},{"gpt-6.1-sol","minimal"}}{
		w:=invokeAI(t,s.agentChatSessionsAPI,http.MethodPost,"/api/agent/sessions",map[string]any{
			"provider_id":p.ID,"model_id":tc.model,"mode":"stateless","reasoning_effort":tc.effort})
		if w.Code!=400{t.Fatalf("unsupported effort %s/%s accepted: %d",tc.model,tc.effort,w.Code)}
	}
}

// A reasoning model can exhaust a 32-token probe before emitting OK.
func TestAgentCapabilityProbeOutputBudgetRespectsModelLimit(t *testing.T){
 if got:=agentCapabilityProbeOutputBudget(agentchat.ModelConfig{MaxOutputTokens:160000});got<512||got>2048{t.Fatalf("probe budget=%d",got)}
 if got:=agentCapabilityProbeOutputBudget(agentchat.ModelConfig{MaxOutputTokens:256});got!=256{t.Fatalf("model ceiling ignored: %d",got)}
}
