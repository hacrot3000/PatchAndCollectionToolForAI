package agentchat

import (
	"strings"
	"time"
)

// PendingUserTurn records exactly one serialized user message waiting for the
// current Legacy turn to finish. It pins the provider/model used for admission.
type PendingUserTurn struct {
	Message                 string    `json:"message"`
	RequestID               string    `json:"request_id,omitempty"`
	RequestDigest           string    `json:"request_digest"`
	ProviderID              string    `json:"provider_id"`
	ModelID                 string    `json:"model_id"`
	ReasoningEffort         string    `json:"reasoning_effort,omitempty"`
	ProviderConfigVersion   string    `json:"provider_config_version"`
	CredentialFingerprint   string    `json:"credential_fingerprint,omitempty"`
	QueuedAt                time.Time `json:"queued_at"`
}

// pendingAdmissionBindingMatches checks the immutable identities needed for a
// pending-to-PREPARED transition. It does not authorize or commit a transition;
// the caller must check ownership, revision and current state under the lock.
func pendingAdmissionBindingMatches(p *PendingUserTurn, b *TurnBinding, r *ProviderRun) bool {
	return p != nil && ValidPendingUserTurn(p) && b != nil && ValidTurnBinding(b) &&
		r != nil && validProviderRun(r) && r.Kind == "message" && r.Phase == "prepared" &&
		b.TurnID == r.ID && b.AttemptID == r.ID && b.PayloadDigest == r.PayloadSHA256 &&
		b.RequestID == p.RequestID && b.ProviderID == p.ProviderID && b.ModelID == p.ModelID &&
		b.ReasoningEffort == p.ReasoningEffort && b.ProviderConfigVersion == p.ProviderConfigVersion &&
		b.CredentialFingerprint == p.CredentialFingerprint
}

// pendingAdmissionCandidateMatches is a prerequisite only, not authorization
// to dispatch. Call with the authoritative current session under the store
// lock; the caller must still check project identity and commit atomically.
func pendingAdmissionCandidateMatches(current, candidate Session, owner string, expectedRevision uint64, payload []byte) bool {
	return owner != "" && current.Owner == owner && candidate.Owner == owner &&
		current.ID == candidate.ID && current.Workspace == candidate.Workspace && current.ProjectID == candidate.ProjectID &&
		current.Revision == expectedRevision && candidate.Revision == expectedRevision &&
		current.PendingUserTurn != nil && candidate.PendingUserTurn == nil &&
		(current.ProviderRun == nil || current.ProviderRun.Phase == "completed") &&
		candidate.Status == "requesting_ai" &&
		pendingAdmissionBindingMatches(current.PendingUserTurn, candidate.TurnBinding, candidate.ProviderRun) &&
		candidate.Mode == current.Mode && candidate.TurnBinding.ContextMode == current.Mode &&
		len(payload) > 0 && len(payload) <= maxProviderRunPayload &&
		ProviderPayloadDigest(payload) == candidate.ProviderRun.PayloadSHA256
}

// ValidPendingUserTurn enforces bounded queued-turn records.
func ValidPendingUserTurn(p *PendingUserTurn) bool {
	if p == nil {
		return true
	}
	msg := strings.TrimSpace(p.Message)
	if msg == "" || len(msg) > maxItemBytes || p.RequestDigest == "" || len(p.RequestDigest) != 64 ||
		!validSHA256(p.RequestDigest) || p.ProviderID == "" || p.ModelID == "" ||
		len(p.ProviderID) > 128 || len(p.ModelID) > 128 || len(p.ReasoningEffort) > 32 ||
		p.ProviderConfigVersion == "" || len(p.ProviderConfigVersion) > 128 || p.QueuedAt.IsZero() {
		return false
	}
	if p.CredentialFingerprint != "" && (len(p.CredentialFingerprint) != 64 || !validSHA256(strings.ToLower(p.CredentialFingerprint))) {
		return false
	}
	if p.RequestID != "" && (len(p.RequestID) > 128 || strings.ContainsAny(p.RequestID, "\x00\r\n")) {
		return false
	}
	return true
}
