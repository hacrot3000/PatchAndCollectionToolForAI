package agentchat

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderTrustedSharedIPsAreExactAndRestricted(t *testing.T) {
	p := sampleProvider()
	shared := net.ParseIP("100.64.0.19")
	if providerIPAllowed(shared, p) {
		t.Fatal("shared address allowed without explicit trust")
	}

	p.TrustedSharedIPs = []string{"100.64.0.19"}
	if err := ValidateProvider(p); err != nil {
		t.Fatal(err)
	}
	if !providerIPAllowed(shared, p) || providerIPAllowed(net.ParseIP("100.64.0.20"), p) || providerIPAllowed(net.ParseIP("127.0.0.1"), p) || providerIPAllowed(net.ParseIP("192.168.1.1"), p) {
		t.Fatal("allowlist did not enforce exact shared address")
	}
	if err := validateProviderDNS([]net.IPAddr{{IP: shared}, {IP: net.ParseIP("8.8.8.8")}}, p); err != nil {
		t.Fatalf("trusted and public DNS answers refused: %v", err)
	}
	if err := validateProviderDNS([]net.IPAddr{{IP: shared}, {IP: net.ParseIP("100.64.0.20")}}, p); err == nil {
		t.Fatal("mixed DNS answers containing an untrusted address were accepted")
	}
	for _, invalid := range [][]string{{"127.0.0.1"}, {"192.168.1.1"}, {"100.64.0.0/10"}, {"::ffff:100.64.0.19"}, {"100.64.0.19", "100.64.0.19"}} {
		p.TrustedSharedIPs = invalid
		if ValidateProvider(p) == nil {
			t.Errorf("accepted invalid trusted addresses: %v", invalid)
		}
	}
}

func TestBlockedProviderDNSReportsOnlyTrustableSharedIP(t *testing.T) {
	p := sampleProvider()
	for _, tc := range []struct{ address, want string }{{"100.64.0.19", "100.64.0.19"}, {"192.168.1.1", ""}, {"127.0.0.1", ""}} {
		err := validateProviderDNS([]net.IPAddr{{IP: net.ParseIP(tc.address)}}, p)
		if err == nil {
			t.Fatalf("expected %s to be blocked", tc.address)
		}
		if got := blockedSharedIP(fmt.Errorf("transport: %w", err)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.address, got, tc.want)
		}
	}
}

func TestProviderTrustChangeInvalidatesCapability(t *testing.T) {
	p := sampleProvider()
	before := CapabilityFingerprint(p, p.Models[0])
	p.TrustedSharedIPs = []string{"100.64.0.19"}
	if CapabilityFingerprint(p, p.Models[0]) == before {
		t.Fatal("trust change must invalidate capability evidence")
	}
}

func sampleProvider() ProviderConfig {
	return ProviderConfig{ID: "custom-r2d", Name: "Custom Endpoint", APIType: "responses", BaseURL: "https://api.r2d.dev/v1",
		Models: []ModelConfig{{ID: "composer-2.5", Name: "Composer", ToolCalling: true, Vision: true, MaxInputTokens: 1280000, MaxOutputTokens: 160000, ReasoningEfforts: []string{"low", "medium", "high"}, DefaultReasoningEffort: "medium"}}}
}
func TestProviderValidationAndResponsePath(t *testing.T) {
	p := sampleProvider()
	if err := ValidateProvider(p); err != nil {
		t.Fatal(err)
	}
	u, err := p.ResponsesURL()
	if err != nil || u != "https://api.r2d.dev/v1/responses" {
		t.Fatalf("%s %v", u, err)
	}
	p.BaseURL = "https://api.r2d.dev/v1/responses"
	u, err = p.ResponsesURL()
	if err != nil || strings.Contains(u, "responses/responses") {
		t.Fatalf("duplicate endpoint: %q %v", u, err)
	}
}
func TestProviderRejectsUnsafeDestinations(t *testing.T) {
	for _, endpoint := range []string{
		"http://api.r2d.dev/v1", "https://localhost/v1", "https://foo.local/v1",
		"https://127.0.0.1/v1", "https://192.168.1.1/v1", "https://169.254.169.254/latest",
		"https://100.64.0.2/v1", "https://198.19.0.1/v1", "https://[::1]/v1",
		"https://user:pass@api.r2d.dev/v1", "https://api.r2d.dev/v1?token=x",
	} {
		p := sampleProvider()
		p.BaseURL = endpoint
		if err := ValidateProvider(p); err == nil {
			t.Errorf("accepted unsafe URL: %s", endpoint)
		}
	}
}
func TestProviderModelAndReasoningValidation(t *testing.T) {
	p := sampleProvider()
	p.Models[0].DefaultReasoningEffort = "max"
	if ValidateProvider(p) == nil {
		t.Fatal("accepted undeclared reasoning effort")
	}
	p = sampleProvider()
	p.Models = append(p.Models, p.Models[0])
	if ValidateProvider(p) == nil {
		t.Fatal("accepted duplicate model")
	}
	p = sampleProvider()
	p.APIType = "chat_completions"
	if err := ValidateProvider(p); err != nil {
		t.Fatalf("rejected Chat Completions adapter: %v", err)
	}
	p.APIType = "messages"
	if ValidateProvider(p) == nil {
		t.Fatal("accepted unsupported adapter")
	}
}
func TestProviderStorePersistsWithoutSecretOrOwnerLeak(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "providers")
	s, err := NewProviderStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := sampleProvider()
	if err = s.Put("alice", p); err != nil {
		t.Fatal(err)
	}
	ps, err := s.List("alice")
	if err != nil || len(ps) != 1 {
		t.Fatalf("missing provider: %v %+v", err, ps)
	}
	ps, err = s.List("bob")
	if err != nil || len(ps) != 0 {
		t.Fatalf("provider leaked to other owner: %v %+v", err, ps)
	}
	again, err := NewProviderStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := again.Get("alice", "custom-r2d")
	if err != nil || got.BaseURL != p.BaseURL {
		t.Fatalf("provider not persisted: %v %+v", err, got)
	}
	path, err := s.file("alice")
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0600 {
		t.Fatalf("private config file mode %o", stat.Mode().Perm())
	}
	if strings.Contains(path, "alice") {
		t.Fatal("owner should not appear in filename")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(raw)), "api_key") {
		t.Fatal("key field leaked into config")
	}
	p.Name = "Changed"
	if err = s.Put("alice", p); err != nil {
		t.Fatal(err)
	}
	ps, err = s.List("alice")
	if err != nil || len(ps) != 1 || ps[0].Name != "Changed" {
		t.Fatalf("upsert failed %v %+v", err, ps)
	}
}

func TestProviderResponseParseRetryClassification(t *testing.T) {
 cases:=[]struct{body,code,reason string;retry bool}{
  {`{"id":"one","status":"incomplete","incomplete_details":{"reason":"timeout"}}`,"RESPONSE_INCOMPLETE","timeout",true},
  {`{"id":"one","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}`,"RESPONSE_INCOMPLETE","max_output_tokens",true},
  {`{"id":"one","status":"incomplete","incomplete_details":{"reason":"content_filter"}}`,"RESPONSE_INCOMPLETE","content_filter",false},
  {`{"id":"one","status":"failed","error":{"code":"server_error"}}`,"RESPONSE_FAILED","server_error",true},
  {`{"id":"one","status":"failed","error":{"code":"invalid_request"}}`,"RESPONSE_FAILED","invalid_request",false},
  {`{"id":"one","status":"completed","output":[]}`,"JSON_PARSE_FAILED","",false},
  {`{"id":"one","status":"completed","output":`,"JSON_PARSE_FAILED","",true},
 }
 for _,tc:=range cases {
  pe:=classifyProviderResponseParseError([]byte(tc.body),fmt.Errorf("provider parse failed"))
  if pe.Code!=tc.code||pe.ProviderErrorCode!=tc.reason||pe.Retryable!=tc.retry{
   t.Errorf("body %s: %+v",tc.body,pe)
  }
 }
}
