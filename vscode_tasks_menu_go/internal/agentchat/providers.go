package agentchat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ModelConfig struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	ToolCalling            bool     `json:"tool_calling"`
	Vision                 bool     `json:"vision"`
	MaxInputTokens         int      `json:"max_input_tokens,omitempty"`
	MaxOutputTokens        int      `json:"max_output_tokens,omitempty"`
	ReasoningEfforts       []string `json:"reasoning_efforts,omitempty"`
	DefaultReasoningEffort string   `json:"default_reasoning_effort,omitempty"`
}
type ProviderConfig struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	APIType string `json:"api_type"`
	BaseURL string `json:"base_url"`
	// TrustedSharedIPs are exact CGNAT addresses explicitly trusted for this provider.
	TrustedSharedIPs []string `json:"trusted_shared_ips,omitempty"`
	// Stream opts into the SSE adapter. It is only usable when a streaming
	// capability probe verified this exact endpoint/model configuration.
	Stream bool          `json:"stream,omitempty"`
	Models []ModelConfig `json:"models"`
}

var providerIDPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)
var modelIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/~-]{0,127}$`)
var sharedProviderNetwork = &net.IPNet{IP: net.IPv4(100, 64, 0, 0).To4(), Mask: net.CIDRMask(10, 32)}
var blockedIPNetworks = func() []*net.IPNet {
	cidrs := []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "2001:db8::/32", "2001::/32"}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, sub, _ := net.ParseCIDR(cidr)
		out = append(out, sub)
	}
	return out
}()

func publicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, sub := range blockedIPNetworks {
		if sub.Contains(ip) {
			return false
		}
	}
	return true
}
func providerIPAllowed(ip net.IP, p ProviderConfig) bool {
	if publicIP(ip) {
		return true
	}
	if ip == nil || !sharedProviderNetwork.Contains(ip) {
		return false
	}
	for _, trusted := range p.TrustedSharedIPs {
		if ip.Equal(net.ParseIP(trusted)) {
			return true
		}
	}
	return false
}

type blockedProviderIPError struct{ IP net.IP }

func (e *blockedProviderIPError) Error() string { return "provider DNS includes private/reserved IP" }

func validateProviderDNS(ips []net.IPAddr, p ProviderConfig) error {
	if len(ips) == 0 {
		return errors.New("provider DNS returned no addresses")
	}
	for _, entry := range ips {
		if !providerIPAllowed(entry.IP, p) {
			return &blockedProviderIPError{IP: append(net.IP(nil), entry.IP...)}
		}
	}
	return nil
}

// Only shared-network IPv4 addresses can be offered for explicit trust.
func blockedSharedIP(err error) string {
	var blocked *blockedProviderIPError
	if errors.As(err, &blocked) && blocked.IP.To4() != nil && sharedProviderNetwork.Contains(blocked.IP) {
		return blocked.IP.String()
	}
	return ""
}
func ValidateProvider(p ProviderConfig) error {
	if !providerIDPattern.MatchString(p.ID) || len(p.Name) == 0 || len(p.Name) > 128 {
		return errors.New("invalid provider ID/name")
	}
	if p.APIType != "responses" && p.APIType != "chat_completions" {
		return errors.New("unsupported Agent Chat API type")
	}
	if len(p.TrustedSharedIPs) > 8 {
		return errors.New("at most 8 trusted shared IPs are permitted")
	}
	seenIPs := map[string]bool{}
	for _, value := range p.TrustedSharedIPs {
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() == nil || !sharedProviderNetwork.Contains(ip) || value != ip.String() || seenIPs[value] {
			return errors.New("trusted_shared_ips must contain unique, canonical IPv4 addresses in 100.64.0.0/10")
		}
		seenIPs[value] = true
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawFragment != "" {
		return errors.New("provider endpoint must be a public HTTPS base URL without credentials/query/fragment")
	}
	if strings.Contains(u.Hostname(), "%") || strings.ContainsAny(u.Hostname(), " \t\r\n") || strings.HasSuffix(u.Hostname(), ".local") || strings.EqualFold(u.Hostname(), "localhost") || strings.HasSuffix(u.Hostname(), ".localhost") {
		return errors.New("local or unsafe provider hostname refused")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !publicIP(ip) {
		return errors.New("provider IP must be public")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return errors.New("invalid HTTPS port")
		}
	}
	if strings.Contains(u.EscapedPath(), "%2f") || strings.Contains(u.EscapedPath(), "%2F") || strings.Contains(u.EscapedPath(), "%5c") || strings.Contains(u.EscapedPath(), "%5C") {
		return errors.New("encoded path separators refused")
	}
	if len(p.Models) == 0 || len(p.Models) > 24 {
		return errors.New("provider must declare 1-24 models")
	}
	seen := map[string]bool{}
	for _, m := range p.Models {
		if !modelIDPattern.MatchString(m.ID) || len(m.Name) > 128 {
			return errors.New("invalid model ID/name")
		}
		if seen[m.ID] {
			return errors.New("duplicate model ID")
		}
		seen[m.ID] = true
		if m.MaxInputTokens < 0 || m.MaxInputTokens > 4000000 || m.MaxOutputTokens < 0 || m.MaxOutputTokens > 200000 {
			return errors.New("invalid model token budget")
		}
		seenEffort := map[string]bool{}
		for _, effort := range m.ReasoningEfforts {
			if !allowedEffort(effort) || seenEffort[effort] {
				return errors.New("invalid/duplicate reasoning effort")
			}
			seenEffort[effort] = true
		}
		if m.DefaultReasoningEffort != "" && !seenEffort[m.DefaultReasoningEffort] {
			return errors.New("default reasoning effort is not advertised")
		}
	}
	return nil
}
func allowedEffort(s string) bool {
	switch s {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	}
	return false
}

func hasValidToolName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i, r := range name {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9' && i > 0)
		if !ok {
			return false
		}
	}
	return true
}
func (p ProviderConfig) FindModel(id string) (ModelConfig, bool) {
	for _, m := range p.Models {
		if m.ID == id {
			return m, true
		}
	}
	return ModelConfig{}, false
}
func (p ProviderConfig) APIURL(apiType string) (string, error) {
	if err := ValidateProvider(p); err != nil {
		return "", err
	}
	if apiType != p.APIType {
		return "", errors.New("requested API type does not match provider configuration")
	}
	endpoint := ""
	switch apiType {
	case "responses":
		endpoint = "responses"
	case "chat_completions":
		endpoint = "chat/completions"
	default:
		return "", errors.New("unsupported Agent Chat API type")
	}
	u, _ := url.Parse(p.BaseURL)
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/"+endpoint) {
		path += "/" + endpoint
	}
	u.Path = path
	u.RawPath = ""
	return u.String(), nil
}

func (p ProviderConfig) ResponsesURL() (string, error) {
	if p.APIType != "responses" {
		return "", errors.New("provider is not configured for Responses API")
	}
	return p.APIURL("responses")
}

// ProviderStore saves public model metadata only. API keys belong in the
// existing encrypted secretstore; provider config JSON MUST NEVER contain one.
type ProviderStore struct{ dir string }

var providerStoreMu sync.Mutex

func NewProviderStore(dir string) (*ProviderStore, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("absolute provider directory required")
	}
	return &ProviderStore{dir: filepath.Clean(dir)}, nil
}
func (s *ProviderStore) file(owner string) (string, error) {
	if owner == "" || len(owner) > 256 {
		return "", errors.New("invalid owner")
	}
	hash := sha256.Sum256([]byte(owner))
	return filepath.Join(s.dir, "providers-"+hex.EncodeToString(hash[:16])+".json"), nil
}
func (s *ProviderStore) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	fi, err := os.Lstat(s.dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe provider config directory")
	}
	return nil
}

type providerFile struct {
	Version   int              `json:"version"`
	Providers []ProviderConfig `json:"providers"`
}

func (s *ProviderStore) read(owner string) (providerFile, error) {
	path, err := s.file(owner)
	if err != nil {
		return providerFile{}, err
	}
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return providerFile{Version: 1, Providers: []ProviderConfig{}}, nil
	}
	if err != nil {
		return providerFile{}, err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 || fi.Size() > 128<<10 {
		return providerFile{}, errors.New("unsafe provider config file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return providerFile{}, err
	}
	var p providerFile
	if err = json.Unmarshal(raw, &p); err != nil {
		return providerFile{}, err
	}
	if p.Version != 1 || len(p.Providers) > 24 {
		return providerFile{}, errors.New("invalid provider config version/count")
	}
	names := map[string]bool{}
	for _, item := range p.Providers {
		if err = ValidateProvider(item); err != nil {
			return providerFile{}, err
		}
		if names[item.ID] {
			return providerFile{}, errors.New("duplicate provider")
		}
		names[item.ID] = true
	}
	return p, nil
}
func (s *ProviderStore) List(owner string) ([]ProviderConfig, error) {
	providerStoreMu.Lock()
	defer providerStoreMu.Unlock()
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	p, err := s.read(owner)
	if err != nil {
		return nil, err
	}
	return p.Providers, nil
}
func (s *ProviderStore) Get(owner, id string) (ProviderConfig, error) {
	ps, err := s.List(owner)
	if err != nil {
		return ProviderConfig{}, err
	}
	for _, p := range ps {
		if p.ID == id {
			return p, nil
		}
	}
	return ProviderConfig{}, ErrNotFound
}
func (s *ProviderStore) Put(owner string, p ProviderConfig) error {
	if err := ValidateProvider(p); err != nil {
		return err
	}
	providerStoreMu.Lock()
	defer providerStoreMu.Unlock()
	if err := s.ensureDir(); err != nil {
		return err
	}
	cfg, err := s.read(owner)
	if err != nil {
		return err
	}
	replaced := false
	for i := range cfg.Providers {
		if cfg.Providers[i].ID == p.ID {
			cfg.Providers[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		if len(cfg.Providers) >= 24 {
			return errors.New("provider count exceeded")
		}
		cfg.Providers = append(cfg.Providers, p)
	}
	sort.Slice(cfg.Providers, func(i, j int) bool { return cfg.Providers[i].ID < cfg.Providers[j].ID })
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if len(raw) > 128<<10 {
		return errors.New("provider config size exceeded")
	}
	path, _ := s.file(owner)
	temp, err := os.CreateTemp(s.dir, ".provider-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// Delete removes a provider's public metadata for the owner. It NEVER claims
// to revoke a key at the external vendor; local credential removal is a
// separate, explicit action. Existing sessions keep their recorded provider
// and will fail closed at the next turn instead of silently switching.
func (s *ProviderStore) Delete(owner, id string) error {
	if owner == "" || len(owner) > 256 || id == "" {
		return errors.New("invalid provider identity")
	}
	providerStoreMu.Lock()
	defer providerStoreMu.Unlock()
	if err := s.ensureDir(); err != nil {
		return err
	}
	cfg, err := s.read(owner)
	if err != nil {
		return err
	}
	found := false
	kept := make([]ProviderConfig, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		if p.ID == id {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if !found {
		return ErrNotFound
	}
	cfg.Providers = kept
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	path, err := s.file(owner)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".provider-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// Keep a small bounded pool keyed by the FULL provider network trust policy.
// Requests for the same endpoint can reuse TLS/HTTP2 connections, but a trust
// or endpoint change can never reuse an older policy's connection.
const providerHTTPTimeout = 180 * time.Second
const providerHeaderTimeout = 150 * time.Second
const providerClientCacheLimit = 32

type providerClientCacheEntry struct {
 key string
 client *http.Client
}
var providerClientCacheMu sync.Mutex
var providerClientCache []providerClientCacheEntry

func providerClientCacheKey(p ProviderConfig) string {
 raw, _ := json.Marshal(struct {
  ID string
  APIType string
  BaseURL string
  TrustedSharedIPs []string
 }{p.ID, p.APIType, p.BaseURL, p.TrustedSharedIPs})
 sum := sha256.Sum256(raw)
 return hex.EncodeToString(sum[:])
}

// newProviderClient pins each new DNS resolution to validated public/trusted
// addresses. Redirects and proxies remain disabled. An idle connection can
// only be reused for an identical endpoint and identical trust policy.
func newProviderClient(p ProviderConfig) *http.Client {
 providerClientCacheMu.Lock()
 defer providerClientCacheMu.Unlock()
 key := providerClientCacheKey(p)
 for i, entry := range providerClientCache {
  if entry.key == key {
   // Maintain an LRU so stale per-provider transports are evicted.
   providerClientCache = append(providerClientCache[:i],providerClientCache[i+1:]...)
   providerClientCache = append(providerClientCache,entry)
   return entry.client
  }
 }
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	resolver := net.DefaultResolver
	transport := &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: providerHeaderTimeout,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if ip := net.ParseIP(host); ip != nil {
				if !providerIPAllowed(ip, p) {
					return nil, errors.New("private or reserved provider IP refused")
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
			}
			ips, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, errors.New("provider DNS lookup failed")
			}
			// Refuse the entire answer if any address is not allowed.
			if err := validateProviderDNS(ips, p); err != nil {
				return nil, err
			}
			var lastErr error
			for _, entry := range ips {
				var conn net.Conn
				conn, lastErr = dialer.DialContext(ctx, network, net.JoinHostPort(entry.IP.String(), port))
				if lastErr == nil {
					return conn, nil
				}
			}
			return nil, fmt.Errorf("provider connection failed: %w", lastErr)
		},
	}
	client := &http.Client{Transport: transport, Timeout: providerHTTPTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirect refused") }}
	providerClientCache = append(providerClientCache, providerClientCacheEntry{key: key, client: client})
	if len(providerClientCache) > providerClientCacheLimit {
		providerClientCache[0].client.CloseIdleConnections()
		providerClientCache = providerClientCache[1:]
	}
	return client
}

func newProviderRequest(ctx context.Context, endpoint, key string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	applyProviderClientRequestID(req)
	return req, nil
}

func validateProviderCall(p ProviderConfig, key string, body []byte) (string, error) {
	if key == "" || len(key) > 8192 {
		return "", errors.New("provider credential unavailable")
	}
	endpoint, err := p.ResponsesURL()
	if err != nil {
		return "", err
	}
	if len(body) == 0 || len(body) > maxRequestBytes {
		return "", errors.New("request payload exceeds limit")
	}
	if !json.Valid(body) {
		return "", errors.New("invalid provider request payload")
	}
	return endpoint, nil
}

// classifyProviderResponseParseError marks only recoverable provider failures
// retryable. The returned response is never allowed to execute native tools.
func classifyProviderResponseParseError(raw []byte, cause error) *ProviderError {
 pe := &ProviderError{Code:"JSON_PARSE_FAILED", Stage:"provider_parse", Message:SafeProviderText(cause.Error())}
 if !json.Valid(raw) {
  pe.Retryable,pe.Ambiguous=true,true
  return pe
 }
 var v struct {
  Status string `json:"status"`
  IncompleteDetails struct { Reason string `json:"reason"` } `json:"incomplete_details"`
  Error struct { Code string `json:"code"` } `json:"error"`
 }
 if json.Unmarshal(raw,&v)!=nil {return pe}
 switch v.Status {
 case "incomplete":
  reason:=SafeProviderText(v.IncompleteDetails.Reason)
  pe.Code,pe.ProviderErrorCode="RESPONSE_INCOMPLETE",reason
  pe.Message="Provider returned an incomplete response"
  if reason!=""{pe.Message+=" ("+reason+")"}
  switch reason {
  case "content_filter","context_length_exceeded","context_window_exceeded","invalid_request","insufficient_quota":
  default:pe.Retryable=true
  }
 case "failed":
  reason:=SafeProviderText(v.Error.Code)
  pe.Code,pe.ProviderErrorCode="RESPONSE_FAILED",reason
  pe.Message="Provider reported a failed response"
  if reason!=""{pe.Message+=" ("+reason+")"}
  switch reason {
  case "server_error","rate_limit_exceeded","temporarily_unavailable","timeout","overloaded":
   pe.Retryable=true
  }
 }
 return pe
}

func DoResponses(ctx context.Context, p ProviderConfig, key string, body []byte) (ResponsesResult, error) {
	endpoint, err := validateProviderCall(p, key, body)
	if err != nil {
		return ResponsesResult{}, classifyProviderPreflightError(err)
	}
	client := newProviderClient(p)
	req, err := newProviderRequest(ctx, endpoint, key, body)
	if err != nil {
		return ResponsesResult{}, err
	}
	RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.request", Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, RequestBody: body, RequestBytes: len(body)})
	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		code, message, ambiguous := classifyProviderTransportError(ctx, err)
		diagnostic := DiagnosticEvent{Level: "error", Stage: "provider_http", Event: "http.error", Code: code, SafeMessage: message, Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, RequestBytes: len(body), RequestSHA256: BodyDigest(body), Duration: time.Since(started)}
		RecordProviderDiagnostic(ctx, diagnostic)
		return ResponsesResult{}, &ProviderError{Code: code, Stage: "provider_http", Message: message, Retryable: code == "CONNECT_FAILED" || code == "CONNECT_TIMEOUT" || code == "RESPONSE_HEADER_TIMEOUT" || code == "REQUEST_TIMEOUT", Ambiguous: ambiguous, BlockedSharedIP: blockedSharedIP(err)}
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRequestBytes+1))
	truncated := len(raw) > maxRequestBytes
	if truncated {
		raw = raw[:maxRequestBytes]
	}
	diagnostic := DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.response", Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, HTTPStatus: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), RetryAfter: resp.Header.Get("Retry-After"), ProviderRequestID: providerRequestID(resp.Header), ResponseBody: raw, RequestBytes: len(body), ResponseBytes: len(raw), RequestSHA256: BodyDigest(body), Truncated: truncated, Duration: time.Since(started)}
	if resp.StatusCode != http.StatusOK {
		providerErr := parseProviderHTTPError(resp.StatusCode, resp.Header.Get("Retry-After"), raw)
		diagnostic.Level = "error"
		diagnostic.Code = providerErr.Code
		diagnostic.SafeMessage = providerErr.Message
		RecordProviderDiagnostic(ctx, diagnostic)
		return ResponsesResult{}, providerErr
	}
	if readErr != nil {
		diagnostic.Level = "error"
		diagnostic.Code = "RESPONSE_READ_FAILED"
		diagnostic.SafeMessage = "Could not read provider response body"
		RecordProviderDiagnostic(ctx, diagnostic)
		return ResponsesResult{}, &ProviderError{Code: diagnostic.Code, Stage: "provider_http", Message: diagnostic.SafeMessage, Retryable: true, Ambiguous: true}
	}
	if truncated {
		diagnostic.Level = "error"
		diagnostic.Code = "RESPONSE_TOO_LARGE"
		diagnostic.SafeMessage = "Provider response exceeded the configured size limit"
		RecordProviderDiagnostic(ctx, diagnostic)
		return ResponsesResult{}, &ProviderError{Code: diagnostic.Code, Stage: "provider_http", Message: diagnostic.SafeMessage}
	}
	result, parseErr := ParseResponsesResult(raw)
	if parseErr != nil {
		pe := classifyProviderResponseParseError(raw,parseErr)
		diagnostic.Level = "error"
		diagnostic.Stage = pe.Stage
		diagnostic.Event = "response.parse_error"
		diagnostic.Code = pe.Code
		diagnostic.SafeMessage = pe.Message
		RecordProviderDiagnostic(ctx, diagnostic)
		return ResponsesResult{}, pe
	}
	RecordProviderDiagnostic(ctx, diagnostic)
	return result, nil
}

func providerRequestID(header http.Header) string {
	for _, key := range []string{"X-Request-ID", "Request-ID", "OpenAI-Request-ID"} {
		if value := strings.TrimSpace(header.Get(key)); value != "" {
			return SafeProviderText(value)
		}
	}
	return ""
}

func classifyProviderPreflightError(err error) error {
	message := SafeProviderText(err.Error())
	code := "CONFIG_INVALID"
	if strings.Contains(strings.ToLower(message), "credential") {
		code = "MISSING_API_KEY"
	}
	if strings.Contains(strings.ToLower(message), "payload exceeds") || strings.Contains(strings.ToLower(message), "too large") {
		code = "REQUEST_TOO_LARGE"
	}
	return &ProviderError{Code: code, Stage: "request_validation", Message: message, Retryable: false, Ambiguous: false}
}

func classifyProviderTransportError(ctx context.Context, err error) (string, string, bool) {
	if errors.Is(ctx.Err(), context.Canceled) {
		return "REQUEST_CANCELLED", "Provider request was cancelled", true
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "REQUEST_TIMEOUT", "Provider request timed out", true
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "dns lookup") || strings.Contains(text, "no such host") {
		return "DNS_LOOKUP_FAILED", "Provider DNS lookup failed", false
	}
	if strings.Contains(text, "private") || strings.Contains(text, "reserved") {
		return "PRIVATE_IP_BLOCKED", "Provider endpoint resolved to a blocked address", false
	}
	if strings.Contains(text, "tls") || strings.Contains(text, "certificate") {
		return "TLS_HANDSHAKE_FAILED", "Provider TLS handshake failed", false
	}
	if strings.Contains(text, "timeout awaiting response headers") || strings.Contains(text, "timeout waiting for response headers") {
		return "RESPONSE_HEADER_TIMEOUT", "Provider did not return response headers before the configured deadline", true
	}
	if strings.Contains(text, "timeout") || strings.Contains(text, "deadline") {
		return "CONNECT_TIMEOUT", "Provider connection timed out", true
	}
	return "CONNECT_FAILED", "Provider connection failed", true
}

func parseProviderHTTPError(status int, retryAfter string, body []byte) *ProviderError {
	code := fmt.Sprintf("PROVIDER_HTTP_%d", status)
	providerType, providerCode, providerMessage := "", "", ""
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		var detail struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(envelope.Error, &detail) == nil {
			providerType, providerCode, providerMessage = detail.Type, detail.Code, detail.Message
		} else if len(envelope.Error) > 0 {
			providerMessage = string(envelope.Error)
		}
	}
	providerType = SafeProviderText(providerType)
	providerCode = SafeProviderText(providerCode)
	providerMessage = SafeProviderText(providerMessage)
	message := fmt.Sprintf("Provider rejected the request (HTTP %d)", status)
	if providerMessage != "" {
		message += ": " + providerMessage
	}
	return &ProviderError{Code: code, Stage: "provider_http", Message: message, ProviderErrorType: providerType, ProviderErrorCode: providerCode, ProviderMessage: providerMessage, HTTPStatus: status, RetryAfter: SafeProviderText(retryAfter), Retryable: status == 408 || status == 425 || status == 429 || status == 499 || status >= 500}
}
