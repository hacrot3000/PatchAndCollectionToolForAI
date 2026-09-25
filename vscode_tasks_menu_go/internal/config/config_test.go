package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	_, loadedPath, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(loadedPath)
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


func TestProtocolIsCaseInsensitive(t *testing.T) {
	cfg := Default()
	cfg.Protocol = "HTTPS"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if !cfg.TLS() {
		t.Fatal("HTTPS protocol should enable TLS regardless of case")
	}
	cfg.Protocol = "HTTP"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.TLS() {
		t.Fatal("HTTP protocol should disable TLS regardless of case")
	}
}


func TestNewConfigFileWritesHTTPSProtocol(t *testing.T) {
	workspace := t.TempDir()
	cfg, path, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protocol != ProtocolHTTPS {
		t.Fatalf("new config protocol=%q want https", cfg.Protocol)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "protocol = https") {
		t.Fatalf("generated config missing HTTPS protocol:\n%s", data)
	}
}


func TestLoadMigratesLegacyConfigIntoVSCode(t *testing.T) {
	workspace := t.TempDir()
	legacy := filepath.Join(workspace, "vscode_tasks_menu.ini")
	content := "[server]\nprotocol = https\nbind = 127.0.0.1\nport = 0\nopen_browser = false\n\n[auth]\nenabled = false\nusername = admin\npassword = change-me\n"
	if err := os.WriteFile(legacy, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, path, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	if path != want {
		t.Fatalf("config path=%q want %q", path, want)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy config still exists: %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("migrated config missing: %v", err)
	}
}

func TestSharedServerDefaultsDisabledAndWritesINI(t *testing.T) {
	workspace := t.TempDir()
	cfg, path, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SharedServerEnabled || cfg.SharedProjectID != "" || cfg.SharedIdentityDB != "" {
		t.Fatalf("unexpected shared-server defaults: %+v", cfg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"[shared_server]",
		"enabled = false",
		"# project_id = my-project",
		"# identity_db = /var/lib/taskdeck/identity.db",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated config missing %q:\n%s", want, text)
		}
	}
}

func TestLoadSharedServerSettings(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "vscode_tasks_menu.ini")
	content := "[server]\nprotocol = https\nbind = 127.0.0.1\nport = 0\nopen_browser = false\n\n[auth]\nenabled = false\nusername = admin\npassword = change-me\n\n[shared_server]\nenabled = true\nproject_id = m3-client\nidentity_db = /var/lib/taskdeck/identity.db\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SharedServerEnabled || cfg.SharedProjectID != "m3-client" || cfg.SharedIdentityDB != "/var/lib/taskdeck/identity.db" {
		t.Fatalf("shared-server config not parsed: %+v", cfg)
	}
}

func TestSharedServerRequiresStableProjectID(t *testing.T) {
	cfg := Default()
	cfg.SharedServerEnabled = true
	for _, projectID := range []string{"", "has space", "/tmp/project", ".starts-with-dot", strings.Repeat("a", 129)} {
		cfg.SharedProjectID = projectID
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid shared project_id %q accepted", projectID)
		}
	}
	for _, projectID := range []string{"m3-client", "bletonfc", "project_01", "datbike.ota"} {
		cfg.SharedProjectID = projectID
		if err := cfg.Validate(); err != nil {
			t.Fatalf("valid shared project_id %q rejected: %v", projectID, err)
		}
	}
}

func TestSharedServerIdentityDBMustBeAbsoluteWhenConfigured(t *testing.T) {
	cfg := Default()
	cfg.SharedServerEnabled = true
	cfg.SharedProjectID = "m3-client"
	cfg.SharedIdentityDB = "state/identity.db"
	if err := cfg.Validate(); err == nil {
		t.Fatal("relative shared identity_db must be rejected")
	}
	cfg.SharedIdentityDB = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty identity_db should allow platform default: %v", err)
	}
	cfg.SharedIdentityDB = filepath.Join(t.TempDir(), "identity.db")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("absolute identity_db rejected: %v", err)
	}
}

func TestSharedServerConfigFoundationDoesNotWeakenRemoteAuthGuard(t *testing.T) {
	cfg := Default()
	cfg.Bind = "0.0.0.0"
	cfg.SharedServerEnabled = true
	cfg.SharedProjectID = "m3-client"
	if err := cfg.Validate(); err == nil {
		t.Fatal("shared config alone must not expose remote listener before shared auth middleware exists")
	}
	cfg.AuthEnabled = true
	cfg.Username = "admin"
	cfg.Password = "temporary-legacy-guard"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("remote shared config with current legacy guard rejected: %v", err)
	}
}

