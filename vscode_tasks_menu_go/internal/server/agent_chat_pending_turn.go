package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// agentChatQueuePendingUserTurn stores one bounded next-turn message while the
// conversation remains busy. The caller must already hold the session turn lock.
func (s *Server) agentChatQueuePendingUserTurn(store *agentchat.Store, owner string, x *agentchat.Session, message, requestID, requestDigest string, p agentchat.ProviderConfig, model agentchat.ModelConfig, effort string, credential []byte) error {
	if x == nil || x.PendingUserTurn != nil || strings.TrimSpace(message) == "" {
		return agentchat.ErrInvalidSession
	}
	pending := agentchat.PendingUserTurn{
		Message:               message,
		RequestID:             strings.TrimSpace(requestID),
		RequestDigest:         requestDigest,
		ProviderID:            p.ID,
		ModelID:               model.ID,
		ReasoningEffort:       strings.TrimSpace(effort),
		ProviderConfigVersion: agentchat.RunProviderConfigVersion(p, model),
		CredentialFingerprint: agentchat.RunCredentialFingerprint(credential),
		QueuedAt:              time.Now().UTC(),
	}
	if !agentchat.ValidPendingUserTurn(&pending) {
		return agentchat.ErrInvalidSession
	}
	x.PendingUserTurn = &pending
	x.AppendEvent("turn_queued", x.Status, "", "")
	saved, err := store.Save(*x, x.Revision)
	if err == nil {
		*x = saved
	}
	return err
}

// agentPendingTurnBindingMatches refuses to reinterpret a queued turn under a
// different provider/model configuration or credential.
func agentPendingTurnBindingMatches(pending agentchat.PendingUserTurn, provider agentchat.ProviderConfig, model agentchat.ModelConfig, secret []byte) bool {
	return len(secret) > 0 && pending.ProviderID == provider.ID && pending.ModelID == model.ID &&
		pending.ProviderConfigVersion == agentchat.RunProviderConfigVersion(provider, model) &&
		pending.CredentialFingerprint == agentchat.RunCredentialFingerprint(secret)
}

// agentReleaseAndDispatchPending releases the active lease before dispatch may run.
func agentReleaseAndDispatchPending(release, dispatch func()) {
	release()
	go dispatch()
}

// Pending dispatch remains fail-closed until atomic journal handoff is implemented.
const agentPendingDispatchHandoffReady = false

// agentChatDispatchPendingUserTurn starts a queued Legacy turn after the active
// turn finishes. It reuses the public message admission path internally.
func (s *Server) agentChatDispatchPendingUserTurn(owner, id string) {
	if s == nil || strings.TrimSpace(id) == "" {
		return
	}
	release, ok := lockAgentTurn(s.Workspace, owner, id)
	if !ok {
		return
	}
	store, err := s.agentChatStore()
	if err != nil {
		release()
		return
	}
	x, err := store.Get(id, owner)
	if err != nil || x.PendingUserTurn == nil || !agentSessionAcceptsUserMessage(x.Status) {
		release()
		return
	}
	// Dispatch is suspended until pending removal and PREPARED admission can
	// commit atomically. Clearing it before admission can permanently lose it.
	// Keep the existing implementation below for the forthcoming safe handoff.
	if !agentPendingDispatchHandoffReady {
		release()
		return
	}
	pending := *x.PendingUserTurn
	// Do not discard or dispatch a queued request under changed credentials or
	// provider configuration. Keep it durable for explicit owner review.
	provider, model, _, resolveErr := s.agentResolveTurnModel(owner, &x, pending.ProviderID, pending.ModelID, pending.ReasoningEffort)
	if resolveErr != nil || s.ConnectionSecrets == nil {
		release()
		return
	}
	secret, secretErr := s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace, owner, provider.ID))
	if secretErr != nil || !agentPendingTurnBindingMatches(pending, provider, model, secret) {
		release()
		return
	}
	x.PendingUserTurn = nil
	_ = agentApplyNextTurnSelection(&x, pending.ProviderID, pending.ModelID, pending.ReasoningEffort)
	x, err = store.Save(x, x.Revision)
	if err != nil {
		release()
		return
	}
	release()
	body, err := json.Marshal(map[string]any{
		"message": pending.Message, "confirm_external": true,
		"expected_revision": x.Revision, "request_id": pending.RequestID,
		"provider_id": pending.ProviderID, "model_id": pending.ModelID,
		"reasoning_effort": pending.ReasoningEffort,
	})
	if err != nil {
		return
	}
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions/"+id+"/messages", bytes.NewReader(body)).WithContext(context.Background())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.agentChatMessageAPI(w, req, id)
}
