package agentchat

import (
	"errors"
	"strings"
	"time"
)

// TurnBindingVersion identifies the persisted per-turn provider admission record.
const TurnBindingVersion = 1

// TurnBinding is the immutable provider/model/effort identity for one admitted
// Legacy Agent turn. Composer selection may change while a turn runs, but egress
// for the active turn must match this record until the turn terminalizes.
type TurnBinding struct {
	Version               int         `json:"version"`
	TurnID                string      `json:"turn_id"`
	ProviderID            string      `json:"provider_id"`
	ModelID               string      `json:"model_id"`
	ReasoningEffort       string      `json:"reasoning_effort,omitempty"`
	ProviderConfigVersion string      `json:"provider_config_version"`
	CredentialFingerprint string      `json:"credential_fingerprint,omitempty"`
	ContextMode           ContextMode `json:"context_mode"`
	AttemptID             string      `json:"attempt_id,omitempty"`
	PayloadDigest         string      `json:"payload_digest"`
	RequestID             string      `json:"request_id,omitempty"`
	AdmittedAt            time.Time   `json:"admitted_at"`
}

// ValidTurnBinding rejects incomplete or ambiguous admission records.
func ValidTurnBinding(b *TurnBinding) bool {
	if b == nil {
		return true
	}
	if b.Version != TurnBindingVersion || b.TurnID == "" || len(b.TurnID) > 128 ||
		strings.ContainsAny(b.TurnID, "\x00\r\n") || b.ProviderID == "" || b.ModelID == "" ||
		len(b.ProviderID) > 128 || len(b.ModelID) > 128 || len(b.ReasoningEffort) > 32 ||
		b.ProviderConfigVersion == "" || len(b.ProviderConfigVersion) > 128 ||
		b.PayloadDigest == "" || len(b.PayloadDigest) != 64 || !validSHA256(b.PayloadDigest) ||
		b.AdmittedAt.IsZero() {
		return false
	}
	if b.CredentialFingerprint != "" && (len(b.CredentialFingerprint) != 64 || !validSHA256(strings.ToLower(b.CredentialFingerprint))) {
		return false
	}
	if b.AttemptID != "" && (len(b.AttemptID) > 128 || strings.ContainsAny(b.AttemptID, "\x00\r\n")) {
		return false
	}
	if b.RequestID != "" && (len(b.RequestID) > 128 || strings.ContainsAny(b.RequestID, "\x00\r\n")) {
		return false
	}
	if b.ContextMode != Stateless && b.ContextMode != PreviousResponse && b.ContextMode != Conversation {
		return false
	}
	return true
}

// BuildTurnBinding captures the send-time provider admission metadata.
func BuildTurnBinding(turnID, attemptID, requestID string, mode ContextMode, provider ProviderConfig, model ModelConfig, effort string, credential []byte, payload []byte) (TurnBinding, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" || mode == "" || len(payload) == 0 {
		return TurnBinding{}, errors.New("incomplete turn binding")
	}
	binding := TurnBinding{
		Version:               TurnBindingVersion,
		TurnID:                turnID,
		ProviderID:            provider.ID,
		ModelID:               model.ID,
		ReasoningEffort:       strings.TrimSpace(effort),
		ProviderConfigVersion: RunProviderConfigVersion(provider, model),
		CredentialFingerprint: RunCredentialFingerprint(credential),
		ContextMode:           mode,
		AttemptID:             strings.TrimSpace(attemptID),
		PayloadDigest:         ProviderPayloadDigest(payload),
		RequestID:             strings.TrimSpace(requestID),
		AdmittedAt:            time.Now().UTC(),
	}
	if !ValidTurnBinding(&binding) {
		return TurnBinding{}, errors.New("invalid turn binding")
	}
	return binding, nil
}
