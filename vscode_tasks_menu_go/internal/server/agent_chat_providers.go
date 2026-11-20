package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/secretstore"
)

const agentProviderConfigLimit = 128 << 10

func (s *Server) agentChatProviders() (*agentchat.ProviderStore, error) {
	privateFile, err := aiPrivatePath(s.Workspace)
	if err != nil {
		return nil, err
	}
	return agentchat.NewProviderStore(filepath.Join(filepath.Dir(privateFile), "agent-providers-"+aiWorkspaceHash(s.Workspace)))
}
func agentProviderSecretID(workspace, owner, providerID string) string {
	h := sha256.Sum256([]byte(owner))
	return "ai/agent/" + aiWorkspaceHash(workspace) + "/" + hex.EncodeToString(h[:12]) + "/" + providerID
}

// agentChatCapabilities returns the private probe-evidence store for this
// installation. It is separate from provider metadata and never holds keys.
func (s *Server) agentChatCapabilities() (*agentchat.CapabilityStore, error) {
	privateFile, err := aiPrivatePath(s.Workspace)
	if err != nil {
		return nil, err
	}
	return agentchat.NewCapabilityStore(filepath.Join(filepath.Dir(privateFile), "agent-providers-"+aiWorkspaceHash(s.Workspace)))
}

// agentModelCapabilityFingerprint is exposed so request building and probes
// agree on exactly which configuration a verified result applies to.
func agentModelCapabilityFingerprint(p agentchat.ProviderConfig, m agentchat.ModelConfig) string {
	return agentchat.CapabilityFingerprint(p, m)
}

// agentStreamCapabilityVerified reports whether the SSE adapter was verified
// for this exact provider/model configuration by a user-triggered probe.
func (s *Server) agentStreamCapabilityVerified(owner string, p agentchat.ProviderConfig) bool {
	store, err := s.agentChatCapabilities()
	if err != nil {
		return false
	}
	for _, m := range p.Models {
		rec, ok, err := store.Get(owner, p.ID, m.ID, agentchat.CapabilityFingerprint(p, m))
		if err != nil || !ok {
			continue
		}
		if rec.Capabilities[agentchat.ProbeStreaming] == agentchat.CapabilityVerified {
			return true
		}
	}
	return false
}

// agentConversationCapabilityVerified reports whether the endpoint/model pair
// has probe evidence for the Conversations API. Without it, Conversation mode
// must not send a conversation ID.
func (s *Server) agentConversationCapabilityVerified(owner string, p agentchat.ProviderConfig, m agentchat.ModelConfig) bool {
	store, err := s.agentChatCapabilities()
	if err != nil {
		return false
	}
	rec, ok, err := store.Get(owner, p.ID, m.ID, agentchat.CapabilityFingerprint(p, m))
	if err != nil || !ok {
		return false
	}
	return rec.Capabilities[agentchat.ProbeConversation] == agentchat.CapabilityVerified
}

// agentPatchToolStatus describes why the current provider/model can or cannot
// advertise the patch tool; the model's declaration alone is not evidence.
func (s *Server) agentPatchToolStatus(owner, providerID, modelID string) string {
	store, err := s.agentChatProviders()
	if err != nil {
		return "unavailable"
	}
	p, err := store.Get(owner, providerID)
	if err != nil {
		return "unavailable"
	}
	m, ok := p.FindModel(modelID)
	if !ok {
		return "unavailable"
	}
	if !m.ToolCalling {
		return "model_disabled"
	}
	caps, err := s.agentChatCapabilities()
	if err != nil {
		return "unavailable"
	}
	rec, ok, err := caps.Get(owner, p.ID, m.ID, agentchat.CapabilityFingerprint(p, m))
	if err != nil {
		return "unavailable"
	}
	if !ok || rec.Capabilities[agentchat.ProbeToolCalling] != agentchat.CapabilityVerified {
		return "unverified"
	}
	return "enabled"
}

// agentToolCapabilityVerified reports whether a probe (not a user declaration)
// demonstrated tool calling for this exact endpoint/model configuration.
// Anything other than explicit "verified" keeps the execution tool disabled.
func (s *Server) agentToolCapabilityVerified(owner string, p agentchat.ProviderConfig, m agentchat.ModelConfig) bool {
	store, err := s.agentChatCapabilities()
	if err != nil {
		return false
	}
	rec, ok, err := store.Get(owner, p.ID, m.ID, agentchat.CapabilityFingerprint(p, m))
	if err != nil || !ok {
		return false
	}
	return rec.Capabilities[agentchat.ProbeToolCalling] == agentchat.CapabilityVerified
}

type agentProviderView struct {
	agentchat.ProviderConfig
	HasAPIKey bool `json:"has_api_key"`
}

func (s *Server) agentProviderView(owner string, p agentchat.ProviderConfig) agentProviderView {
	view := agentProviderView{ProviderConfig: p}
	if s.ConnectionSecrets != nil {
		key, err := s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace, owner, p.ID))
		view.HasAPIKey = err == nil && len(key) > 0
	}
	return view
}

func (s *Server) agentChatProvidersAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, err := s.agentChatProviders()
	if err != nil {
		http.Error(w, "agent provider store unavailable", http.StatusInternalServerError)
		return
	}
	owner := aiPlanOwner(r)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodDelete {
		s.agentChatProviderDeleteAPI(w, r, store, owner)
		return
	}
	if r.Method == http.MethodGet {
		providers, err := store.List(owner)
		if err != nil {
			http.Error(w, "cannot load agent providers", 500)
			return
		}
		views := make([]agentProviderView, 0, len(providers))
		for _, p := range providers {
			views = append(views, s.agentProviderView(owner, p))
		}
		writeJSON(w, 200, map[string]any{"providers": views})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, agentProviderConfigLimit+4096)
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json") {
		http.Error(w, "JSON Content-Type required", http.StatusUnsupportedMediaType)
		return
	}
	var req struct {
		Provider agentchat.ProviderConfig `json:"provider"`
		APIKey   string                   `json:"api_key,omitempty"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err = dec.Decode(&req); err != nil {
		http.Error(w, "invalid provider configuration", 400)
		return
	}
	var trailing any
	if err = dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one JSON object required", 400)
		return
	}
	if len(req.APIKey) > 8192 || strings.TrimSpace(req.APIKey) != req.APIKey {
		http.Error(w, "invalid API key length/whitespace", 400)
		return
	}
	if err = agentchat.ValidateProvider(req.Provider); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(req.Provider.TrustedSharedIPs) > 0 && !s.requireSharedActionPermission(w, r, identity.PermissionProjectAdmin, "agent.provider.trusted_shared_ips", req.Provider.ID) {
		return
	}
	if req.APIKey != "" {
		if s.ConnectionSecrets == nil {
			http.Error(w, "encrypted secret store unavailable", 503)
			return
		}
		if err = s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace, owner, req.Provider.ID), []byte(req.APIKey)); err != nil {
			http.Error(w, "unable to store encrypted provider key", 500)
			return
		}
	}
	if err = store.Put(owner, req.Provider); err != nil {
		http.Error(w, "unable to persist provider configuration", 500)
		return
	}
	writeJSON(w, 200, map[string]any{"provider": s.agentProviderView(owner, req.Provider)})
}

// Test Connection is user-triggered only. This intentionally tests text
// Responses compatibility, not future tool calling/streaming/stateful support.
func (s *Server) agentChatProviderTestAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		ProviderID      string `json:"provider_id"`
		ModelID         string `json:"model_id"`
		ConfirmExternal bool   `json:"confirm_external"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil {
		http.Error(w, "invalid provider test request", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one JSON object required", 400)
		return
	}
	if !req.ConfirmExternal {
		http.Error(w, "external provider consent required", 403)
		return
	}
	store, err := s.agentChatProviders()
	if err != nil {
		http.Error(w, "provider store unavailable", 500)
		return
	}
	owner := aiPlanOwner(r)
	p, err := store.Get(owner, req.ProviderID)
	if err != nil {
		http.Error(w, "provider not configured", 404)
		return
	}
	if _, ok := p.FindModel(req.ModelID); !ok {
		http.Error(w, "model not configured", 400)
		return
	}
	if s.ConnectionSecrets == nil {
		http.Error(w, "secret store unavailable", 503)
		return
	}
	secret, err := s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace, owner, p.ID))
	if err != nil || len(secret) == 0 {
		http.Error(w, "provider API key not configured", 503)
		return
	}
	release, allowed := s.reserveAIRequest(aiWorkspaceHash(s.Workspace)+":"+owner, 20)
	if !allowed {
		http.Error(w, "provider request quota exceeded", 429)
		return
	}
	defer release()
	body, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
		Model: req.ModelID, Instructions: "You are an API compatibility probe. Reply briefly. Do not use tools.",
		Mode: agentchat.Stateless, Next: []agentchat.HistoryItem{{Type: "user", Content: "Reply with the word OK."}},
		MaxOutputTokens: 64,
	})
	if err != nil {
		http.Error(w, "provider probe construction failed", 500)
		return
	}
	res, err, traceID := s.agentChatObservedCall(r.Context(), owner, "connection_test", "", p, string(secret), body)
	if err != nil {
		writeAgentStructuredError(w, 502, err, traceID, "PROVIDER_REQUEST_FAILED", "provider_http", "Provider connection test failed; inspect the trace")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"ok": true, "trace_id": traceID, "provider_id": p.ID, "model_id": req.ModelID, "response_id": res.ResponseID,
		"input_tokens": res.InputTokens, "output_tokens": res.OutputTokens,
		"text_supported":      strings.TrimSpace(res.Text) != "",
		"tool_calling_tested": false, "streaming_tested": false, "stateful_tested": false,
	})
}

var _ = secretstore.ErrNotFound

// agentChatProviderDeleteAPI removes provider metadata and, only when the
// caller explicitly asks, the local encrypted credential. Deleting local
// state is not vendor-side key revocation, which TaskDeck cannot perform.
// A partial failure is reported as such so the operator can retry the exact
// remaining step instead of assuming a clean delete.
func (s *Server) agentChatProviderDeleteAPI(w http.ResponseWriter, r *http.Request, store *agentchat.ProviderStore, owner string) {
	var req struct {
		ProviderID string `json:"provider_id"`
		DeleteKey  bool   `json:"delete_key"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid provider delete request", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one JSON object required", 400)
		return
	}
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	if req.ProviderID == "" || len(req.ProviderID) > 64 {
		http.Error(w, "invalid provider_id", 400)
		return
	}
	if _, err := store.Get(owner, req.ProviderID); errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "provider not configured", 404)
		return
	} else if err != nil {
		http.Error(w, "provider store unavailable", 500)
		return
	}
	keyDeleted := false
	if req.DeleteKey {
		if s.ConnectionSecrets == nil {
			http.Error(w, "encrypted secret store unavailable", 503)
			return
		}
		err := s.ConnectionSecrets.Delete(agentProviderSecretID(s.Workspace, owner, req.ProviderID))
		if err != nil {
			http.Error(w, "cannot delete local provider credential", 500)
			return
		}
		keyDeleted = true
	}
	if err := store.Delete(owner, req.ProviderID); err != nil {
		writeJSON(w, 200, map[string]any{
			"provider_id": req.ProviderID, "metadata_deleted": false, "key_deleted": keyDeleted,
			"partial": true, "note": "Local credential state changed but provider metadata was not removed; retry the delete.",
		})
		return
	}
	note := "Provider metadata removed. This is not vendor-side key revocation."
	if !keyDeleted {
		note += " The locally stored API key was kept."
	}
	writeJSON(w, 200, map[string]any{
		"provider_id": req.ProviderID, "metadata_deleted": true, "key_deleted": keyDeleted,
		"vendor_revocation": false, "note": note,
	})
}

// agentChatProviderProbeAPI performs user-triggered capability probes and
// records server-side evidence bound to the endpoint/model fingerprint. A
// timeout, transport error or 429 is INCONCLUSIVE, never "unsupported", and
// no probe sends project context or dispatches a real tool.
func (s *Server) agentChatProviderProbeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		ProviderID      string   `json:"provider_id"`
		ModelID         string   `json:"model_id"`
		Capabilities    []string `json:"capabilities"`
		ConfirmExternal bool     `json:"confirm_external"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil {
		http.Error(w, "invalid capability probe request", 400)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one JSON object required", 400)
		return
	}
	if !req.ConfirmExternal {
		http.Error(w, "external provider consent required", 403)
		return
	}
	if len(req.Capabilities) == 0 {
		req.Capabilities = []string{agentchat.ProbeText, agentchat.ProbeToolCalling, agentchat.ProbePreviousResponse}
	}
	if len(req.Capabilities) > 5 {
		http.Error(w, "too many capability probes", 400)
		return
	}
	for _, c := range req.Capabilities {
		switch c {
		case agentchat.ProbeText, agentchat.ProbeStreaming, agentchat.ProbeToolCalling, agentchat.ProbePreviousResponse, agentchat.ProbeConversation:
		default:
			http.Error(w, "unsupported capability probe", 400)
			return
		}
	}
	store, err := s.agentChatProviders()
	if err != nil {
		http.Error(w, "provider store unavailable", 500)
		return
	}
	owner := aiPlanOwner(r)
	p, err := store.Get(owner, req.ProviderID)
	if err != nil {
		http.Error(w, "provider not configured", 404)
		return
	}
	model, ok := p.FindModel(req.ModelID)
	if !ok {
		http.Error(w, "model not configured", 400)
		return
	}
	if s.ConnectionSecrets == nil {
		http.Error(w, "secret store unavailable", 503)
		return
	}
	secret, err := s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace, owner, p.ID))
	if err != nil || len(secret) == 0 {
		http.Error(w, "provider API key not configured", 503)
		return
	}
	release, allowed := s.reserveAIRequest(aiWorkspaceHash(s.Workspace)+":"+owner, 20)
	if !allowed {
		http.Error(w, "provider request quota exceeded", 429)
		return
	}
	defer release()
	rec := agentchat.CapabilityRecord{
		ProviderID: p.ID, ModelID: model.ID,
		Fingerprint: agentchat.CapabilityFingerprint(p, model),
		Version:     agentchat.CapabilityProbeVersion, Capabilities: map[string]string{},
		Detail: map[string]string{}, ProbedAt: time.Now().UTC(),
	}
	for _, capability := range req.Capabilities {
		status, detail := s.agentProbeCapability(r, owner, p, model, string(secret), capability)
		rec.Capabilities[capability] = status
		if detail != "" {
			rec.Detail[capability] = detail
		}
	}
	caps, err := s.agentChatCapabilities()
	if err != nil {
		http.Error(w, "capability store unavailable", 500)
		return
	}
	if err = caps.Put(owner, rec); err != nil {
		http.Error(w, "cannot persist capability evidence", 500)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"provider_id": p.ID, "model_id": model.ID, "fingerprint": rec.Fingerprint,
		"capabilities": rec.Capabilities, "detail": rec.Detail,
		"probed_at": rec.ProbedAt, "probe_version": rec.Version,
	})
}

// agentProbeCapability runs exactly one bounded, harmless probe. It never
// executes a tool and never includes project/AI Pack data.
func (s *Server) agentProbeCapability(r *http.Request, owner string, p agentchat.ProviderConfig, m agentchat.ModelConfig, key, capability string) (string, string) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	switch capability {
	case agentchat.ProbeText:
		body, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
			Model: m.ID, Instructions: "Capability probe. Reply briefly without tools.",
			Mode: agentchat.Stateless, Next: []agentchat.HistoryItem{{Type: "user", Content: "Reply with the word OK."}}, MaxOutputTokens: 32,
		})
		if err != nil {
			return agentchat.CapabilityInconclusive, "probe_build_failed"
		}
		res, err, _ := s.agentChatObservedCall(ctx, owner, "capability_probe_"+capability, "", p, key, body)
		if err != nil {
			return agentchat.CapabilityInconclusive, err.Error()
		}
		if strings.TrimSpace(res.Text) == "" {
			return agentchat.CapabilityUnsupported, "empty_text"
		}
		return agentchat.CapabilityVerified, ""
	case agentchat.ProbeToolCalling:
		body, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
			Model: m.ID, Instructions: "Capability probe. Do not call the probe function unless asked. Reply with the word OK.",
			Mode: agentchat.Stateless, Next: []agentchat.HistoryItem{{Type: "user", Content: "Reply with the word OK."}}, MaxOutputTokens: 32,
			ProbeToolName: "taskdeck_capability_probe",
		})
		if err != nil {
			return agentchat.CapabilityInconclusive, "probe_build_failed"
		}
		if _, err, _ := s.agentChatObservedCall(ctx, owner, "capability_probe_"+capability, "", p, key, body); err != nil {
			return agentchat.CapabilityInconclusive, err.Error()
		}
		// A successful structured Responses exchange with a function schema proves
		// the endpoint accepted tool definitions. Actual tool-call generation is
		// not required and is not inferred.
		return agentchat.CapabilityVerified, "schema_accepted"
	case agentchat.ProbePreviousResponse:
		if p.APIType == "chat_completions" {
			return agentchat.CapabilityUnsupported, "Chat Completions uses stateless canonical history"
		}
		body, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
			Model: m.ID, Instructions: "Capability probe. Reply briefly.", Mode: agentchat.PreviousResponse,
			Next: []agentchat.HistoryItem{{Type: "user", Content: "Remember the number 41. Reply OK."}}, MaxOutputTokens: 32,
		})
		if err != nil {
			return agentchat.CapabilityInconclusive, "probe_build_failed"
		}
		first, err, _ := s.agentChatObservedCall(ctx, owner, "capability_probe_"+capability, "", p, key, body)
		if err != nil {
			return agentchat.CapabilityInconclusive, err.Error()
		}
		if first.ResponseID == "" {
			return agentchat.CapabilityUnsupported, "missing_response_id"
		}
		follow, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
			Model: m.ID, Instructions: "Capability probe. Reply briefly.", Mode: agentchat.PreviousResponse,
			PreviousResponseID: first.ResponseID,
			Next:               []agentchat.HistoryItem{{Type: "user", Content: "What number did I ask you to remember? Add one. Reply with only the number."}}, MaxOutputTokens: 32,
		})
		if err != nil {
			return agentchat.CapabilityInconclusive, "probe_build_failed"
		}
		second, err, _ := s.agentChatObservedCall(ctx, owner, "capability_probe_"+capability, "", p, key, follow)
		if err != nil {
			return agentchat.CapabilityInconclusive, err.Error()
		}
		if strings.Contains(second.Text, "42") {
			return agentchat.CapabilityVerified, ""
		}
		return agentchat.CapabilityUnsupported, "chained_context_not_recalled"
	case agentchat.ProbeConversation:
		if p.APIType == "chat_completions" {
			return agentchat.CapabilityUnsupported, "Chat Completions uses stateless canonical history"
		}
		// Two turns: the first must return a conversation ID, the second must send
		// that ID back. A single response cannot prove reuse.
		body, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
			Model: m.ID, Instructions: "Capability probe. Reply briefly.", Mode: agentchat.Conversation,
			Next: []agentchat.HistoryItem{{Type: "user", Content: "Remember the number 41. Reply OK."}}, MaxOutputTokens: 32,
		})
		if err != nil {
			return agentchat.CapabilityInconclusive, "probe_build_failed"
		}
		first, err, _ := s.agentChatObservedCall(ctx, owner, "capability_probe_"+capability, "", p, key, body)
		if err != nil {
			return agentchat.CapabilityInconclusive, err.Error()
		}
		if first.ConversationID == "" {
			return agentchat.CapabilityUnsupported, "no_conversation_id"
		}
		second, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
			Model: m.ID, Instructions: "Capability probe. Reply briefly.", Mode: agentchat.Conversation,
			ConversationID: first.ConversationID,
			Next:           []agentchat.HistoryItem{{Type: "user", Content: "What number did I ask you to remember? Add one. Reply with only the number."}}, MaxOutputTokens: 32,
		})
		if err != nil {
			return agentchat.CapabilityInconclusive, "probe_build_failed"
		}
		follow, err, _ := s.agentChatObservedCall(ctx, owner, "capability_probe_"+capability, "", p, key, second)
		if err != nil {
			return agentchat.CapabilityInconclusive, err.Error()
		}
		if strings.Contains(follow.Text, "42") {
			return agentchat.CapabilityVerified, ""
		}
		return agentchat.CapabilityUnsupported, "conversation_context_not_reused"
	case agentchat.ProbeStreaming:
		if !p.Stream {
			return agentchat.CapabilityUnsupported, "provider_stream_disabled"
		}
		body, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
			Model: m.ID, Instructions: "Capability probe. Reply briefly.", Mode: agentchat.Stateless,
			Next: []agentchat.HistoryItem{{Type: "user", Content: "Reply OK."}}, MaxOutputTokens: 32,
		})
		if err != nil {
			return agentchat.CapabilityInconclusive, "probe_build_failed"
		}
		var res agentchat.ResponsesResult
		if p.APIType == "chat_completions" {
			res, err = agentchat.DoChatCompletionsStream(ctx, p, key, body)
		} else {
			res, err = agentchat.DoResponsesStream(ctx, p, key, body)
		}
		if err != nil {
			return agentchat.CapabilityInconclusive, err.Error()
		}
		if res.ResponseID == "" || strings.TrimSpace(res.Text) == "" {
			return agentchat.CapabilityUnsupported, "empty_stream_result"
		}
		return agentchat.CapabilityVerified, ""
	}
	return agentchat.CapabilityUnknown, ""
}
