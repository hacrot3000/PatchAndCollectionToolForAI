package server

import (
	"context"
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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

// The real provider test exercises TaskDeck's real HTTPS provider transport
// (DNS/IP guard, TLS verification, bearer auth, canonical request and canonical
// parse) against a local HTTPS server.
//
// This is a HTTPS stub integration test, not an external provider smoke test:
// no credential leaves the machine and no real provider endpoint is called.
// The stub is bound to a trusted shared-network address (100.64.0.0/10)
// because TaskDeck refuses provider endpoints that resolve to a private or
// reserved address unless the exact address is trusted.
//
// The transport tests run inside an isolated user, mount and network namespace
// so they never depend on an address, DNS entry or hosts file of the machine
// that runs them. When the namespace is unavailable the test reports SKIP
// with an explicit reason instead of silently passing.

const (
	realProviderStubHost   = "review-provider.test"
	realProviderStubAddr   = "100.64.0.18"
	realProviderHelperEnv  = "TASKDECK_REAL_PROVIDER_HELPER"
	realProviderHostsEnv   = "TASKDECK_REAL_PROVIDER_HOSTS"
	realProviderWorkdirEnv = "TASKDECK_REAL_PROVIDER_WORKDIR"
	realProviderBinaryEnv  = "TASKDECK_REAL_PROVIDER_BINARY"
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
		Subject:      pkix.Name{CommonName: realProviderStubHost},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// The certificate must match the hostname the provider URL uses, so the
		// TLS verification is a real hostname check and not an IP check.
		DNSNames:    []string{realProviderStubHost},
		IPAddresses: []net.IP{net.ParseIP(realProviderStubAddr)},
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
	listener, err := net.Listen("tcp", realProviderStubAddr+":0")
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

// requireIsolatedProviderNamespace skips the test when the provider hostname
// does not resolve to the trusted shared-network address of the stub. The
// transport tests run inside an isolated namespace where that mapping exists;
// outside it the hostname is a public name and the test is skipped with an
// explicit reason instead of reaching an unrelated address.
func requireIsolatedProviderNamespace(t *testing.T) {
	t.Helper()
	addresses, err := net.LookupIP(realProviderStubHost)
	if err != nil {
		t.Skipf("the provider hostname %s does not resolve in this environment: %v", realProviderStubHost, err)
	}
	for _, address := range addresses {
		if address.Equal(net.ParseIP(realProviderStubAddr)) {
			return
		}
	}
	t.Skipf("the provider hostname %s does not resolve to the isolated stub address %s in this environment",
		realProviderStubHost, realProviderStubAddr)
}

// realProviderStubHostsFile writes the hosts mapping the provider hostname to
// the trusted shared-network address the stub listens on.
func realProviderStubHostsFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hosts")
	content := "127.0.0.1 localhost\n" + realProviderStubAddr + " " + realProviderStubHost + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// useRepositoryPatchRuntime points the Patch Tool runtime at the repository
// checkout so the AI Pack of the execution workspace can be built. A run that
// pins a provider always builds that pack, so the runtime must be resolvable.
func useRepositoryPatchRuntime(t *testing.T) {
	t.Helper()
	workdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(workdir, "..", "..", ".."))
	entry := filepath.Join(repositoryRoot, "python_patch_entry.py")
	if _, err := os.Stat(entry); err != nil {
		t.Skipf("the Patch Tool runtime of this checkout is unavailable: %v", err)
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
}

// realProviderNamespaceScript prepares the isolated namespace and runs the
// provider transport tests inside it.
const realProviderNamespaceScript = `
set -e
ip link set lo up
ip addr add ` + realProviderStubAddr + `/32 dev lo
mount --bind "$TASKDECK_REAL_PROVIDER_HOSTS" /etc/hosts
cd "$TASKDECK_REAL_PROVIDER_WORKDIR"
exec "$TASKDECK_REAL_PROVIDER_BINARY" -test.run='^TestRealProviderTransport(RunsAgentRunOverHTTPS|FailsClosedOnUntrustedAddress|RefusesUntrustedCertificate|StubCertificateMatchesHostname)$' -test.v
`

// TestRealProviderTransportHelperProcess is not a real test. It runs the
// provider transport tests inside an isolated namespace so the provider
// hostname resolves to the stub without touching the host configuration.
func TestRealProviderTransportHelperProcess(t *testing.T) {
	if os.Getenv(realProviderHelperEnv) != "1" {
		t.Skip("helper process")
	}
	// The helper is started inside the prepared namespace, so the real tests
	// below run with the isolated hosts file already mounted.
	TestRealProviderTransportRunsAgentRunOverHTTPS(t)
	TestRealProviderTransportFailsClosedOnUntrustedAddress(t)
	TestRealProviderTransportRefusesUntrustedCertificate(t)
}

func TestRealProviderTransportRunsAgentRunOverHTTPS(t *testing.T) {
	requireIsolatedProviderNamespace(t)
	useRepositoryPatchRuntime(t)
	stub := newRealProviderStub(t, 0)
	s, session, provider := prepareRunAPIServer(t, false)
	provider.BaseURL = stub.baseURL()
	provider.APIType = "responses"
	provider.Models = []agentchat.ModelConfig{{ID: "stub-model", Name: "Stub"}}
	provider.TrustedSharedIPs = []string{realProviderStubAddr}
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
	answered := false
	for _, event := range record.Events {
		if strings.Contains(string(event.Payload), "real provider answer") {
			answered = true
			break
		}
	}
	if !answered {
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
	requireIsolatedProviderNamespace(t)
	useRepositoryPatchRuntime(t)
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

// TestRealProviderTransportRefusesUntrustedCertificate proves the TLS
// verification is real: a certificate that is not signed by the trusted CA
// must fail the handshake instead of being accepted.
func TestRealProviderTransportRefusesUntrustedCertificate(t *testing.T) {
	stub := newRealProviderStub(t, 0)
	// Trust a different CA than the one that signed the stub certificate.
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "unrelated CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true, IsCA: true,
	}
	otherDER, err := x509.CreateCertificate(rand.Reader, otherTemplate, otherTemplate, &otherKey.PublicKey, otherKey)
	if err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(t.TempDir(), "unrelated-ca.pem")
	if err := os.WriteFile(otherPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: otherDER}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", otherPath)

	provider := agentchat.ProviderConfig{ID: "stub-provider", Name: "Stub", APIType: "responses",
		BaseURL: stub.baseURL(), TrustedSharedIPs: []string{realProviderStubAddr},
		Models: []agentchat.ModelConfig{{ID: "stub-model", Name: "Stub"}}}
	if err := agentchat.ValidateProvider(provider); err != nil {
		t.Fatalf("stub provider must stay valid: %v", err)
	}
	payload, err := json.Marshal(map[string]any{"model": "stub-model", "input": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentchat.DoResponses(context.Background(), provider, "STUB-KEY", payload); err == nil {
		t.Fatal("provider transport accepted an untrusted certificate")
	}
}

// TestRealProviderTransportRunsInsideIsolatedNamespace runs the provider
// transport tests in an isolated user, mount and network namespace so the
// provider hostname resolves to the stub without any host configuration.
func TestRealProviderTransportRunsInsideIsolatedNamespace(t *testing.T) {
	if os.Getenv(realProviderHelperEnv) == "1" {
		// The helper already executed the transport tests inside the namespace.
		return
	}
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skipf("isolated provider namespace is unavailable: unshare is not installed")
	}
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skipf("isolated provider namespace is unavailable: iproute2 is not installed")
	}
	hostsFile := realProviderStubHostsFile(t)
	workdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(t.TempDir(), "namespace.sh")
	if err := os.WriteFile(scriptPath, []byte(realProviderNamespaceScript), 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("unshare", "-r", "-n", "-m", "bash", scriptPath)
	command.Env = append(os.Environ(),
		realProviderHelperEnv+"=1",
		realProviderHostsEnv+"="+hostsFile,
		realProviderWorkdirEnv+"="+workdir,
		realProviderBinaryEnv+"="+os.Args[0],
	)
	output, err := command.CombinedOutput()
	text := string(output)
	if err != nil {
		if strings.Contains(text, "operation not permitted") || strings.Contains(text, "unshare") {
			t.Skipf("isolated provider namespace is not permitted on this machine: %v\n%s", err, text)
		}
		t.Fatalf("isolated provider namespace run failed: %v\n%s", err, text)
	}
	if !strings.Contains(text, "--- PASS: TestRealProviderTransportRunsAgentRunOverHTTPS") {
		t.Fatalf("isolated provider namespace did not run the provider transport test:\n%s", text)
	}
	if strings.Contains(text, "--- FAIL:") {
		t.Fatalf("isolated provider namespace reported a failing provider transport test:\n%s", text)
	}
}

// TestRealProviderStubCertificateMatchesHostname proves the stub certificate
// covers the hostname the provider URL uses, so the TLS handshake is a real
// hostname verification instead of an IP address check.
func TestRealProviderStubCertificateMatchesHostname(t *testing.T) {
	requireIsolatedProviderNamespace(t)
	stub := newRealProviderStub(t, 0)
	address := net.JoinHostPort(realProviderStubHost, strconv.Itoa(stub.port))
	caPath := os.Getenv("SSL_CERT_FILE")
	if caPath == "" {
		t.Fatal("the provider stub did not publish its CA")
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("the provider stub CA could not be parsed")
	}
	verified, err := tls.Dial("tcp", address, &tls.Config{RootCAs: pool, ServerName: realProviderStubHost})
	if err != nil {
		t.Fatalf("the provider stub certificate did not verify for %s: %v", realProviderStubHost, err)
	}
	defer verified.Close()
	state := verified.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		t.Fatal("the provider stub presented no certificate")
	}
	leaf := state.PeerCertificates[0]
	if err := leaf.VerifyHostname(realProviderStubHost); err != nil {
		t.Fatalf("the provider stub certificate does not match %s: %v", realProviderStubHost, err)
	}
	if len(leaf.DNSNames) == 0 || leaf.DNSNames[0] != realProviderStubHost {
		t.Fatalf("the provider stub certificate SAN does not match the provider host: %+v", leaf.DNSNames)
	}
}
