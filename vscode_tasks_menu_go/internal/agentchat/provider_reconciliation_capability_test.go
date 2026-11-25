package agentchat

import (
	"strings"
	"testing"
)

func m177Spec() RunSpec {
	return RunSpec{RunID: "run-1", ConversationID: "c", OwnerWorkspaceID: "w", ExecutionWorkspaceID: "e", ProviderID: "p1", ModelID: "m1", ProviderConfigVersion: strings.Repeat("a", 64), ContextSnapshotID: "s", AttemptID: "a1", IdempotencyKey: "i"}
}
func m177Provider() ProviderConfig {
	return ProviderConfig{ID: "p1", Name: "P", APIType: "responses", BaseURL: "https://api.example.com/v1", Models: []ModelConfig{{ID: "m1", Name: "M", ToolCalling: true}}}
}
func m177Doc() ProviderReconciliationReview { return ProviderReconciliationReview{Capability: ReconciliationDocumented} }
func m177Binding() ProviderResponseBinding {
	s := m177Spec()
	return ProviderResponseBinding{RunID: s.RunID, AttemptID: s.AttemptID, PayloadSHA256: strings.Repeat("b", 64), ProviderID: s.ProviderID, ConfigVersion: s.ProviderConfigVersion, ResponseSHA256: strings.Repeat("c", 64), Status: "completed"}
}

func TestReconciliationCapabilityDefaultsUnsupported(t *testing.T) {
	p := m177Provider(); s := m177Spec(); s.ProviderConfigVersion = RunProviderConfigVersion(p, p.Models[0])
	r := ProviderReconciliationCapabilityFor(s, p, p.Models[0])
	if r.Capability != ReconciliationUnsupported || r.ManualSupportRoute == "" { t.Fatalf("must be unsupported with manual route: %+v", r) }
	if DocumentedProviderReconciliation(p) != nil { t.Fatal("registry must be empty") }
}

func TestReconciliationCapabilityBindings(t *testing.T) {
	p := m177Provider(); m := p.Models[0]; base := m177Spec(); base.ProviderConfigVersion = RunProviderConfigVersion(p, m)
	cases := []struct{ name string; f func(*RunSpec); want string }{
		{"provider", func(s *RunSpec){ s.ProviderID = "x" }, "provider_binding_mismatch"},
		{"model", func(s *RunSpec){ s.ModelID = "x" }, "model_binding_mismatch"},
		{"config", func(s *RunSpec){ s.ProviderConfigVersion = strings.Repeat("d", 64) }, "config_version_revoked"},
		{"fingerprint", func(s *RunSpec){ s.CredentialFingerprint = "xyz" }, "invalid_credential_fingerprint"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T){
			s := base; tc.f(&s); r := ProviderReconciliationCapabilityFor(s, p, m)
			if r.Capability != ReconciliationUnsupported || r.Reason != tc.want { t.Fatalf("want unsupported/%s: %+v", tc.want, r) }
		})
	}
}

func TestValidReconciliationContract(t *testing.T) {
	good := ProviderReconciliationContract{Protocol: "responses", Host: "api.example.com", LookupKind: "get", Verified: true}
	if !validReconciliationContract(&good) { t.Fatal("valid contract rejected") }
	bad := []*ProviderReconciliationContract{nil,
		{Protocol: "responses", Host: "api.example.com", LookupKind: "get"},
		{Protocol: "custom", Host: "api.example.com", LookupKind: "get", Verified: true},
		{Protocol: "responses", Host: "API.Example.com", LookupKind: "get", Verified: true},
		{Protocol: "responses", Host: "api.example.com/a", LookupKind: "get", Verified: true},
		{Protocol: "responses", Host: "api.example.com", LookupKind: "", Verified: true},
	}
	for _, c := range bad { if validReconciliationContract(c) { t.Fatalf("invalid contract accepted: %+v", c) } }
}

func TestClassifyReconciliation(t *testing.T) {
	s := m177Spec(); payload := strings.Repeat("b", 64)
	cases := []struct{ name string; cap ProviderReconciliationReview; f func(*ProviderResponseBinding); want ProviderReconciliationOutcome; digest string }{
		{"unsupported", ProviderReconciliationReview{Capability: ReconciliationUnsupported}, func(b *ProviderResponseBinding){}, ReconcileUnknown, ""},
		{"ambiguous", m177Doc(), func(b *ProviderResponseBinding){ b.Ambiguous = true }, ReconcileUnknown, ""},
		{"run", m177Doc(), func(b *ProviderResponseBinding){ b.RunID = "x" }, ReconcileQuarantine, ""},
		{"attempt", m177Doc(), func(b *ProviderResponseBinding){ b.AttemptID = "x" }, ReconcileQuarantine, ""},
		{"payload", m177Doc(), func(b *ProviderResponseBinding){ b.PayloadSHA256 = strings.Repeat("e", 64) }, ReconcileQuarantine, ""},
		{"provider", m177Doc(), func(b *ProviderResponseBinding){ b.ProviderID = "x" }, ReconcileQuarantine, ""},
		{"config", m177Doc(), func(b *ProviderResponseBinding){ b.ConfigVersion = strings.Repeat("f", 64) }, ReconcileQuarantine, ""},
		{"progress", m177Doc(), func(b *ProviderResponseBinding){ b.Status = "in_progress" }, ReconcileUnknown, ""},
		{"notfound", m177Doc(), func(b *ProviderResponseBinding){ b.Status = "not_found" }, ReconcileUnknown, ""},
		{"expired", m177Doc(), func(b *ProviderResponseBinding){ b.Status = "expired" }, ReconcileUnknown, ""},
		{"rejected", m177Doc(), func(b *ProviderResponseBinding){ b.Status = "rejected" }, ReconcileReject, ""},
		{"baddigest", m177Doc(), func(b *ProviderResponseBinding){ b.ResponseSHA256 = "short" }, ReconcileQuarantine, ""},
		{"unknown", m177Doc(), func(b *ProviderResponseBinding){ b.Status = "mystery" }, ReconcileUnknown, ""},
		{"adopt", m177Doc(), func(b *ProviderResponseBinding){}, ReconcileAdopt, strings.Repeat("c", 64)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T){
			b := m177Binding(); tc.f(&b)
			got := ClassifyProviderReconciliation(tc.cap, s, payload, b)
			if got.Outcome != tc.want || got.ResponseSHA256 != tc.digest || got.AutomaticReplayAllowed { t.Fatalf("got %+v", got) }
		})
	}
}

func TestReconciliationCredentialBinding(t *testing.T) {
	s := m177Spec(); s.CredentialFingerprint = strings.Repeat("9", 64); b := m177Binding(); b.CredentialFingerprint = strings.Repeat("8", 64)
	if ClassifyProviderReconciliation(m177Doc(), s, strings.Repeat("b", 64), b).Outcome != ReconcileQuarantine { t.Fatal("stale credential must quarantine") }
	b.CredentialFingerprint = s.CredentialFingerprint
	if ClassifyProviderReconciliation(m177Doc(), s, strings.Repeat("b", 64), b).Outcome != ReconcileAdopt { t.Fatal("matching credential should adopt") }
}
