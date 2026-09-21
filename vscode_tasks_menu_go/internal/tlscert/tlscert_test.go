package tlscert

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func TestResolveCreatesAndReusesPrivateSelfSignedPair(t *testing.T) {
	t.Setenv("VSCODE_TASKS_MENU_CONFIG_DIR", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Bind = "127.0.0.1"

	first, err := Resolve(workspace, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Auto || !first.Created || first.CertPath == "" || first.KeyPath == "" {
		t.Fatalf("unexpected first result: %#v", first)
	}
	for _, path := range []string{first.CertPath, first.KeyPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode=%#o want 0600", path, got)
		}
	}
	dirInfo, err := os.Stat(filepath.Dir(first.KeyPath))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("TLS dir mode=%#o want 0700", got)
	}

	certBefore, err := os.ReadFile(first.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Resolve(workspace, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Auto || second.Created {
		t.Fatalf("expected existing auto certificate reuse: %#v", second)
	}
	certAfter, err := os.ReadFile(second.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(certBefore) != sha256.Sum256(certAfter) {
		t.Fatal("self-signed certificate unexpectedly changed on reuse")
	}

	pair, err := tls.LoadX509KeyPair(first.CertPath, first.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := cert.VerifyHostname("localhost"); err != nil {
		t.Fatalf("certificate does not cover localhost: %v", err)
	}
	if err := cert.VerifyHostname("127.0.0.1"); err != nil {
		t.Fatalf("certificate does not cover loopback IP: %v", err)
	}
}

func TestResolveRegeneratesForNewAdvertiseHost(t *testing.T) {
	t.Setenv("VSCODE_TASKS_MENU_CONFIG_DIR", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	first, err := Resolve(workspace, cfg)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(first.CertPath)
	if err != nil {
		t.Fatal(err)
	}

	cfg.AdvertiseHost = "devbox.example.test"
	second, err := Resolve(workspace, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Created {
		t.Fatal("new advertise_host should regenerate certificate SANs")
	}
	after, err := os.ReadFile(second.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) == sha256.Sum256(after) {
		t.Fatal("certificate bytes unchanged after SAN regeneration")
	}
	pair, err := tls.LoadX509KeyPair(second.CertPath, second.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := cert.VerifyHostname("devbox.example.test"); err != nil {
		t.Fatalf("new certificate missing advertise host SAN: %v", err)
	}
}

func TestDesiredSANsWildcardIncludesAdvertisedAndLoopbackHosts(t *testing.T) {
	cfg := config.Default()
	cfg.Bind = "0.0.0.0"
	cfg.AdvertiseHost = "192.0.2.25"
	dns, ips := desiredSANs(cfg)
	foundLocalhost := false
	for _, name := range dns {
		if name == "localhost" {
			foundLocalhost = true
		}
	}
	if !foundLocalhost {
		t.Fatal("localhost DNS SAN missing")
	}
	for _, want := range []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("192.0.2.25")} {
		found := false
		for _, got := range ips {
			if got.Equal(want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("IP SAN missing %s", want)
		}
	}
}

func TestResolveValidatesCustomTLSKeyPair(t *testing.T) {
	t.Setenv("VSCODE_TASKS_MENU_CONFIG_DIR", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	auto, err := Resolve(workspace, cfg)
	if err != nil {
		t.Fatal(err)
	}

	custom := cfg
	custom.TLSCert = auto.CertPath
	custom.TLSKey = auto.KeyPath
	result, err := Resolve(workspace, custom)
	if err != nil {
		t.Fatal(err)
	}
	if result.Auto {
		t.Fatal("configured certificate must not be reported as auto")
	}

	custom.TLSKey = filepath.Join(t.TempDir(), "missing.key")
	if _, err := Resolve(workspace, custom); err == nil {
		t.Fatal("invalid configured TLS key pair must be rejected")
	}
}

func TestResolveHTTPDoesNotCreateTLSMaterial(t *testing.T) {
	base := t.TempDir()
	t.Setenv("VSCODE_TASKS_MENU_CONFIG_DIR", base)
	cfg := config.Default()
	cfg.Protocol = config.ProtocolHTTP
	result, err := Resolve(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.CertPath != "" || result.KeyPath != "" || result.Auto {
		t.Fatalf("HTTP unexpectedly resolved TLS material: %#v", result)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("HTTP unexpectedly created TLS directory: %v", entries)
	}
}

func TestAutoDirIsStablePerWorkspace(t *testing.T) {
	base := t.TempDir()
	t.Setenv("VSCODE_TASKS_MENU_CONFIG_DIR", base)
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := AutoDir(workspace)
	if err != nil {
		t.Fatal(err)
	}
	b, err := AutoDir(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if a != b || filepath.Dir(filepath.Dir(a)) != base {
		t.Fatalf("unstable auto TLS dir a=%q b=%q base=%q", a, b, base)
	}
	if _, err := hex.DecodeString(filepath.Base(a)); err != nil {
		t.Fatalf("auto TLS workspace key is not hex: %v", err)
	}
}
