package server

import (
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

// agentAdmitPreparedPendingTurn commits an already-built request without sending it.
// Its caller must authorize egress, validate project identity and acquire the turn
// lease. Dispatch stays disabled until the entire handoff is verified.
func agentAdmitPreparedPendingTurn(store *agentchat.Store, owner string, x agentchat.Session, provider agentchat.ProviderConfig, model agentchat.ModelConfig, secret, payload []byte) (agentchat.Session, error) {
	if store == nil || owner == "" || x.Owner != owner || x.Archived || x.PendingUserTurn == nil || !agentSessionAcceptsUserMessage(x.Status) {
		return agentchat.Session{}, agentchat.ErrInvalidSession
	}
	pending := *x.PendingUserTurn
	if !agentPendingTurnBindingMatches(pending, provider, model, secret) {
		return agentchat.Session{}, agentchat.ErrInvalidSession
	}
	// Require the exact queued message in the candidate's canonical history;
	// payload construction and all other authorization remain the caller's job.
	if len(x.History) == 0 || x.History[len(x.History)-1] != (agentchat.HistoryItem{Type: "user", Content: pending.Message}) {
		return agentchat.Session{}, agentchat.ErrInvalidSession
	}
	x.PendingUserTurn = nil
	x.Status = "requesting_ai"
	x.AppendEvent("turn_started", "requesting_ai", "", "")
	if err := setAgentPreparedProviderRun(&x, "message", "", provider, model, payload); err != nil {
		return agentchat.Session{}, err
	}
	if err := agentPinActiveTurnBinding(&x, x.Mode, provider, model, pending.ReasoningEffort, secret, payload, pending.RequestID); err != nil {
		return agentchat.Session{}, err
	}
	if pending.RequestID != "" {
		x.LastRequestKey = pending.RequestID
		x.LastRequestSHA = pending.RequestDigest
		x.LastRequestCode = 0
		x.LastRequestResponse = ""
	}
	return store.AdmitPendingUserTurn(x, owner, x.Revision, payload)
}

// agentReleaseAndDispatchPending releases the active lease before dispatch may run.
func agentReleaseAndDispatchPending(release, dispatch func()) {
	release()
	go dispatch()
}

// Pending dispatch remains fail-closed until atomic journal handoff is implemented.
const agentPendingDispatchHandoffReady = false

// agentChatDispatchPendingUserTurn starts a queued Legacy turn after the active
// turn finishes. Dispatch is suspended until the atomic handoff is integrated.
func (s *Server) agentChatDispatchPendingUserTurn(owner, id string) {
	if !agentPendingDispatchHandoffReady {
		// Central fail-closed gate: the atomic owner-scoped pending-to-PREPARED
		// handoff is not verified. Never touch or clear a durable pending turn.
		return
	}
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
	// Fail closed: authorized message building has a prepared-only admission
	// branch, but automatic dispatch and recovery are not yet verified.
	// Never clear a durable pending turn here.
	release()
}
