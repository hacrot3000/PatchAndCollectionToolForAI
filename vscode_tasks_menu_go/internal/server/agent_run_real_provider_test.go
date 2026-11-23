package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

// realProviderStub is a real HTTPS provider bound to a trusted shared-network
// address. TaskDeck's provider transport (IP guard, TLS, bearer auth,
// canonical request, canonical parse) runs unchanged against it.
type realProviderStub struct {
	mu       sync.Mutex
	requests []map[string]any
	headers  []http.Header
	server   *httptest.Server
	port     int
	delay    time.Duration
}

const realProviderStubHost = "review-provider.test"

// newRealProviderStub starts a real HTTPS provider on a trusted shared-network
// address. The hostname must resolve to that address; the caller is expected
// to provide the mapping (for example through a namespaced hosts file).
func newRealProviderStub(t *testing.T, delay time.Duration) *realProviderStub {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "TaskDeck review test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "review-provider"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("100.64.0.18")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := x509.ParseCertificate(serverDER)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(t.TempDir(), "review-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := os.WriteFile(caPath, caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	// Go's system certificate pool honours SSL_CERT_FILE on Linux.
	t.Setenv("SSL_CERT_FILE", caPath)

	stub := &realProviderStub{delay: delay}
	listener, err := net.Listen("tcp", "100.64.0.18:0")
	if err != nil {
		t.Skipf("no trusted shared-network address available for a real provider stub: %v", err)
	}
	stub.port = listener.Addr().(*net.TCPAddr).Port
	stub.server = &httptest.Server{
		Listener: listener,
		TLS: &tls.Config{
			Certificates: []tls.Certificate{{Certificate: [][]byte{serverDER}, PrivateKey: serverKey, Leaf: serverCert}},
		},
		Config: &http.Server{Handler: http.HandlerFunc(stub.handle)},
	}
	stub.server.StartTLS()
	t.Cleanup(stub.server.Close)
	return stub
}

func (s *realProviderStub) handle(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	s.requests = append(s.requests, body)
	s.headers = append(s.headers, r.Header.Clone())
	s.mu.Unlock()
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "resp-real-stub", "model": "stub-model", "status": "completed",
		"output": []any{map[string]any{"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": "real provider answer"}}}},
	})
}

func (s *realProviderStub) baseURL() string {
	return "https://" + realProviderStubHost + ":" + strconv.Itoa(s.port) + "/v1"
}

func (s *realProviderStub) snapshot() ([]map[string]any, []http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.requests...), append([]http.Header(nil), s.headers...)
}

func TestRealProviderTransportRunsAgentRunOverHTTPS(t *testing.T) {
	stub := newRealProviderStub(t, 0)
	s, session, provider := prepareRunAPIServer(t, false)
	provider.BaseURL = stub.baseURL()
	provider.APIType = "responses"
	provider.Models = []agentchat.ModelConfig{{ID: "stub-model", Name: "Stub"}}
	provider.TrustedSharedIPs = []string{"100.64.0.18"}
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.Put("local", provider); err != nil {
		t.Fatal(err)
	}
	if err := s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace, "local", provider.ID), []byte("STUB-KEY")); err != nil {
		t.Fatal(err)
	}
	s.AgentRunExecutor = nil
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	configVersion := agentchat.RunProviderConfigVersion(provider, provider.Models[0])
	snapshotResponse := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{
		"conversation_id": session.ID, "prompt": "ask the real provider",
		"provider_id": provider.ID, "model_id": provider.Models[0].ID, "run_id": "real-provider-run",
	})
	if snapshotResponse.Code != http.StatusCreated {
		t.Fatalf("snapshot: HTTP %d %s", snapshotResponse.Code, snapshotResponse.Body)
	}
	var snapshot struct {
		ID           string `json:"context_snapshot_id"`
		ParentTurnID string `json:"parent_turn_id"`
	}
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{RunID: "real-provider-run", ConversationID: session.ID,
		OwnerWorkspaceID: ownerWorkspaceID, ExecutionWorkspaceID: ownerWorkspaceID,
		ProviderID: provider.ID, ModelID: provider.Models[0].ID, ProviderConfigVersion: configVersion,
		CredentialFingerprint: agentchat.RunCredentialFingerprint([]byte("STUB-KEY")),
		ParentTurnID:          snapshot.ParentTurnID, RetryMode: agentchat.RetryOff,
		ContextSnapshotID: snapshot.ID, AttemptID: "real-provider-run-a1", IdempotencyKey: "real-provider-run"}
	response := invokeAI(t, s.agentRunAPI, http.MethodPost, "/api/agent/runs", map[string]any{"spec": spec, "confirm_external": true})
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit run: HTTP %d %s", response.Code, response.Body)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.Completed)
	record, _ := manager.Get(spec.RunID)
	if len(record.Events) == 0 || !strings.Contains(string(record.Events[len(record.Events)-1].Payload), "real provider answer") {
		t.Fatalf("real provider answer did not reach the run journal: %+v", record.Events)
	}
	requests, headers := stub.snapshot()
	if len(requests) != 1 {
		t.Fatalf("real provider received %d requests, want 1", len(requests))
	}
	if headers[0].Get("Authorization") != "Bearer STUB-KEY" {
		t.Fatalf("real provider request did not use the configured credential: %q", headers[0].Get("Authorization"))
	}
	if requests[0]["model"] != "stub-model" {
		t.Fatalf("real provider request used the wrong model: %v", requests[0]["model"])
	}
	if input, _ := requests[0]["input"].([]any); len(input) == 0 {
		t.Fatal("real provider request carried no canonical input")
	}
}

func TestRealProviderTransportFailsClosedOnUntrustedAddress(t *testing.T) {
	stub := newRealProviderStub(t, 0)
	s, session, provider := prepareRunAPIServer(t, false)
	provider.BaseURL = stub.baseURL()
	provider.APIType = "responses"
	provider.Models = []agentchat.ModelConfig{{ID: "stub-model", Name: "Stub"}}
	provider.TrustedSharedIPs = nil
	providers, err := s.agentChatProviders()
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.Put("local", provider); err != nil {
		t.Fatal(err)
	}
	if err := s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace, "local", provider.ID), []byte("STUB-KEY")); err != nil {
		t.Fatal(err)
	}
	s.AgentRunExecutor = nil
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	configVersion := agentchat.RunProviderConfigVersion(provider, provider.Models[0])
	snapshotResponse := invokeAI(t, s.agentContextSnapshotAPI, http.MethodPost, "/api/agent/context-snapshots", map[string]any{
		"conversation_id": session.ID, "prompt": "ask the untrusted provider",
		"provider_id": provider.ID, "model_id": provider.Models[0].ID, "run_id": "untrusted-provider-run",
	})
	if snapshotResponse.Code != http.StatusCreated {
		t.Fatalf("snapshot: HTTP %d %s", snapshotResponse.Code, snapshotResponse.Body)
	}
	var snapshot struct {
		ID           string `json:"context_snapshot_id"`
		ParentTurnID string `json:"parent_turn_id"`
	}
	if err := json.Unmarshal(snapshotResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	spec := agentchat.RunSpec{RunID: "untrusted-provider-run", ConversationID: session.ID,
		OwnerWorkspaceID: ownerWorkspaceID, ExecutionWorkspaceID: ownerWorkspaceID,
		ProviderID: provider.ID, ModelID: provider.Models[0].ID, ProviderConfigVersion: configVersion,
		CredentialFingerprint: agentchat.RunCredentialFingerprint([]byte("STUB-KEY")),
		ParentTurnID:          snapshot.ParentTurnID, RetryMode: agentchat.RetryOff,
		ContextSnapshotID: snapshot.ID, AttemptID: "untrusted-provider-run-a1", IdempotencyKey: "untrusted-provider-run"}
	response := invokeAI(t, s.agentRunAPI, http.MethodPost, "/api/agent/runs", map[string]any{"spec": spec, "confirm_external": true})
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit run: HTTP %d %s", response.Code, response.Body)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.Failed)
	requests, _ := stub.snapshot()
	if len(requests) != 0 {
		t.Fatalf("untrusted provider received %d requests", len(requests))
	}
}
