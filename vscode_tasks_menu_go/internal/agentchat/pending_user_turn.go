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
