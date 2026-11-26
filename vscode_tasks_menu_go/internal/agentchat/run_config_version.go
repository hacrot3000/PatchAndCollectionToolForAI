package agentchat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type runProviderConfigVersionInput struct {
	ProviderID       string      `json:"provider_id"`
	APIType          string      `json:"api_type"`
	BaseURL          string      `json:"base_url"`
	Stream           bool        `json:"stream"`
	TrustedSharedIPs []string    `json:"trusted_shared_ips,omitempty"`
	Model            ModelConfig `json:"model"`
	CacheAffinity string `json:"cache_affinity,omitempty"`
	CacheSessionHeader string `json:"cache_session_header,omitempty"`
}

// RunProviderConfigVersion pins every public setting that can affect request
// routing, payload support, or model limits. Credentials are deliberately absent.
func RunCredentialFingerprint(credential []byte) string {
	prefix := []byte("taskdeck-provider-credential-v1\x00")
	input := make([]byte, 0, len(prefix)+len(credential))
	input = append(input, prefix...)
	input = append(input, credential...)
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])
}

func RunProviderConfigVersion(provider ProviderConfig, model ModelConfig) string {
	input := runProviderConfigVersionInput{
		ProviderID: provider.ID, APIType: provider.APIType, BaseURL: provider.BaseURL,
		Stream: provider.Stream, TrustedSharedIPs: append([]string(nil), provider.TrustedSharedIPs...),
		Model: model, CacheAffinity:provider.CacheAffinity, CacheSessionHeader:provider.CacheSessionHeader,
	}
	body, _ := json.Marshal(input)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
