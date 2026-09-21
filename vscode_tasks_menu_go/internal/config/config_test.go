package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRemoteRequiresAuth(t *testing.T) {
	cfg := Default()
	cfg.Bind = "0.0.0.0"
	if cfg.Validate() == nil {
		t.Fatal("expected remote auth validation error")
	}
	cfg.AuthEnabled = true
	cfg.Username = "u"
	cfg.Password = "p"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}


func TestLoadProtectsExistingConfigPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	workspace := t.TempDir()
	path := filepath.Join(workspace, "vscode_tasks_menu.ini")
	content := "[server]\nbind = 127.0.0.1\nport = 0\nopen_browser = true\n\n[auth]\nenabled = false\nusername = admin\npassword = change-me\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(workspace); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config mode=%#o want 0600", got)
	}
}


func TestEffectiveRemoteListenerRequiresAuthEvenWhenConfigBindIsLoopback(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default local config should validate: %v", err)
	}
	for _, address := range []string{"0.0.0.0:42882", "[::]:42882", "192.168.1.20:42882"} {
		if err := cfg.ValidateListenerAddress(address); err == nil {
			t.Fatalf("effective remote listener %q must require auth", address)
		}
	}
	if err := cfg.ValidateListenerAddress("127.0.0.1:42882"); err != nil {
		t.Fatalf("loopback listener unexpectedly rejected: %v", err)
	}

	cfg.AuthEnabled = true
	cfg.Username = "admin"
	cfg.Password = "strong-password"
	for _, address := range []string{"0.0.0.0:42882", "[::]:42882", "192.168.1.20:42882"} {
		if err := cfg.ValidateListenerAddress(address); err != nil {
			t.Fatalf("authenticated remote listener %q rejected: %v", address, err)
		}
	}
}


func TestAuthEnabledRejectsDefaultPasswordEvenOnLoopback(t *testing.T) {
	cfg := Default()
	cfg.AuthEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("auth enabled with change-me password must be rejected")
	}
	cfg.Password = "strong-password"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("loopback auth with custom password rejected: %v", err)
	}
}


func TestHTTPSIsDefaultProtocol(t *testing.T) {
	cfg := Default()
	if cfg.Protocol != ProtocolHTTPS || !cfg.TLS() {
		t.Fatalf("default protocol=%q TLS=%v want https/true", cfg.Protocol, cfg.TLS())
	}
}

func TestLegacyConfigWithoutProtocolDefaultsToHTTPS(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "vscode_tasks_menu.ini")
	content := "[server]\nbind = 127.0.0.1\nport = 42882\nopen_browser = false\n\n[auth]\nenabled = false\nusername = admin\npassword = change-me\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protocol != ProtocolHTTPS || !cfg.TLS() {
		t.Fatalf("legacy config protocol=%q TLS=%v want https/true", cfg.Protocol, cfg.TLS())
	}
}

func TestHTTPProtocolRejectsTLSCertificateFields(t *testing.T) {
	cfg := Default()
	cfg.Protocol = ProtocolHTTP
	cfg.TLSCert = "/tmp/server.crt"
	cfg.TLSKey = "/tmp/server.key"
	if err := cfg.Validate(); err == nil {
		t.Fatal("http protocol must reject tls_cert/tls_key")
	}
	cfg.TLSCert = ""
	cfg.TLSKey = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("plain http config rejected: %v", err)
	}
}
