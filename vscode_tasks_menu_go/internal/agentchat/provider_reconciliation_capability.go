package agentchat

import (
	"net/url"
	"strings"
)

// Capability verdict for authoritative provider response lookup. This is a
// declaration only: never an acknowledgement, proof of acceptance, idempotency
// key or permission to replay. A client request ID, heartbeat, HTTP timeout or
// local IdempotencyKey is explicitly NOT authoritative. Default is Unsupported.
type ProviderReconciliationCapability string

const (
	ReconciliationUnsupported ProviderReconciliationCapability = "unsupported"
	ReconciliationDocumented  ProviderReconciliationCapability = "documented"
)

// Maintainer-authored, reviewed contract for a documented retrieval endpoint.
// Never inferred from provider config JSON, narration or live responses.
type ProviderReconciliationContract struct {
	Protocol   string // responses | chat_completions
	Host       string // exact lowercase API hostname
	LookupKind string // documented retrieval operation id
	Verified   bool   // explicit review marker
}

func validReconciliationContract(c *ProviderReconciliationContract) bool {
	if c == nil || !c.Verified { return false }
	if c.Protocol != "responses" && c.Protocol != "chat_completions" { return false }
	if c.LookupKind == "" || len(c.LookupKind) > 64 || strings.ContainsAny(c.LookupKind, "\r\n\x00") { return false }
	h := c.Host
	return h != "" && len(h) <= 253 && h == strings.ToLower(strings.TrimSpace(h)) &&
		!strings.ContainsAny(h, " \t\r\n/") && !strings.HasSuffix(h, ".local") &&
		!strings.EqualFold(h, "localhost") && !strings.HasSuffix(h, ".localhost")
}

// Single source of truth. Empty in this build: no configured protocol has a
// verified retrieval contract, so every lookup stays blocked with a manual
// route. Adding an entry requires a reviewed transport + exact-correlation
// tests; it is never driven by live provider output.
func DocumentedProviderReconciliation(provider ProviderConfig) *ProviderReconciliationContract { return nil }

const providerReconciliationManualRoute = "Authoritative provider reconciliation is unavailable in this build. Export redacted diagnostics and the handoff, then reconcile manually through the provider's own records. Never replay an ambiguous request."

type ProviderReconciliationReview struct {
	Capability         ProviderReconciliationCapability `json:"capability"`
	Reason             string                           `json:"reason"`
	ManualSupportRoute string                           `json:"manual_support_route,omitempty"`
}

// Fails closed. Supported only with a documented, verified contract for this
// exact protocol/host AND a still-valid immutable run binding. Pure: no I/O.
func ProviderReconciliationCapabilityFor(spec RunSpec, provider ProviderConfig, model ModelConfig) ProviderReconciliationReview {
	u := func(reason string) ProviderReconciliationReview {
		return ProviderReconciliationReview{Capability: ReconciliationUnsupported, Reason: reason, ManualSupportRoute: providerReconciliationManualRoute}
	}
	if provider.ID != spec.ProviderID { return u("provider_binding_mismatch") }
	if model.ID != spec.ModelID || provider.FindModel(spec.ModelID).ID == "" { return u("model_binding_mismatch") }
	if RunProviderConfigVersion(provider, model) != spec.ProviderConfigVersion { return u("config_version_revoked") }
	if spec.CredentialFingerprint != "" && (len(spec.CredentialFingerprint) != 64 || strings.Trim(spec.CredentialFingerprint, "0123456789abcdef") != "") { return u("invalid_credential_fingerprint") }
	c := DocumentedProviderReconciliation(provider)
	if !validReconciliationContract(c) { return u("no_documented_contract") }
	if c.Protocol != provider.APIType { return u("protocol_mismatch") }
	if !strings.EqualFold(c.Host, reconciliationHost(provider.BaseURL)) { return u("endpoint_host_mismatch") }
	return ProviderReconciliationReview{Capability: ReconciliationDocumented, Reason: "documented_verified_contract"}
}

func reconciliationHost(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" { return "" }
	return strings.ToLower(u.Hostname())
}

// Content-free binding from one authoritative lookup: identifiers/digests only.
type ProviderResponseBinding struct {
	RunID, AttemptID, PayloadSHA256, ProviderID, ConfigVersion string
	CredentialFingerprint, ResponseSHA256 string
	Status string // completed | in_progress | not_found | expired | rejected
	Ambiguous bool
}

type ProviderReconciliationOutcome string
const (
	ReconcileAdopt      ProviderReconciliationOutcome = "adopt"
	ReconcileUnknown    ProviderReconciliationOutcome = "unknown"
	ReconcileQuarantine ProviderReconciliationOutcome = "quarantine"
	ReconcileReject     ProviderReconciliationOutcome = "reject"
)

type ProviderReconciliationDecision struct {
	Outcome                ProviderReconciliationOutcome `json:"outcome"`
	ResponseSHA256         string                        `json:"response_sha256,omitempty"`
	Reason                 string                        `json:"reason"`
	AutomaticReplayAllowed bool                          `json:"automatic_replay_allowed"`
}

// Maps a looked-up binding to a bounded decision. No content/network/persistence.
// Never grants replay. Unsupported/ambiguous/incomplete/not-found/expired/unknown
// statuses stay UNKNOWN; identity mismatches quarantine; only an exactly
// correlated completed response with a valid digest may be adopted (digest only).
func ClassifyProviderReconciliation(cap ProviderReconciliationReview, spec RunSpec, expectedPayloadSHA string, got ProviderResponseBinding) ProviderReconciliationDecision {
	d := func(o ProviderReconciliationOutcome, reason string) ProviderReconciliationDecision {
		return ProviderReconciliationDecision{Outcome: o, Reason: reason, AutomaticReplayAllowed: false}
	}
	if cap.Capability != ReconciliationDocumented { return d(ReconcileUnknown, "authoritative_lookup_unsupported") }
	if got.Ambiguous { return d(ReconcileUnknown, "ambiguous_lookup") }
	if got.RunID != spec.RunID || got.AttemptID != spec.AttemptID { return d(ReconcileQuarantine, "run_or_attempt_mismatch") }
	if expectedPayloadSHA != "" && got.PayloadSHA256 != expectedPayloadSHA { return d(ReconcileQuarantine, "payload_digest_mismatch") }
	if got.ProviderID != spec.ProviderID || got.ConfigVersion != spec.ProviderConfigVersion { return d(ReconcileQuarantine, "provider_or_config_mismatch") }
	if spec.CredentialFingerprint != "" && got.CredentialFingerprint != spec.CredentialFingerprint { return d(ReconcileQuarantine, "credential_fingerprint_mismatch") }
	switch got.Status {
	case "in_progress": return d(ReconcileUnknown, "provider_response_in_progress")
	case "not_found", "expired": return d(ReconcileUnknown, "provider_response_uncertain")
	case "rejected": return d(ReconcileReject, "provider_authoritative_nonacceptance")
	case "completed":
		if len(got.ResponseSHA256) != 64 || strings.Trim(got.ResponseSHA256, "0123456789abcdef") != "" { return d(ReconcileQuarantine, "invalid_response_digest") }
		return ProviderReconciliationDecision{Outcome: ReconcileAdopt, ResponseSHA256: got.ResponseSHA256, Reason: "exact_authoritative_response", AutomaticReplayAllowed: false}
	default: return d(ReconcileUnknown, "unrecognized_response_status")
	}
}
