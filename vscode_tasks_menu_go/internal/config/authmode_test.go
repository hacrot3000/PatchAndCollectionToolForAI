package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAuthenticationModePreservesUnrelatedConfig(t *testing.T) {
	workspace := t.TempDir()
	vscode := filepath.Join(workspace, ".vscode")
	if err := os.MkdirAll(vscode, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vscode, "vscode_tasks_menu.ini")
	original := "[server]\nprotocol = http\nbind = 127.0.0.1\nport = 0\nopen_browser = true\n\n[auth]\nenabled = true\nusername = old-admin\npassword = old-password-123\n\n[shared_server]\nenabled = false\n\n[self_update]\nbranch = release/test\nrun_full_validation_tests = true\n\n[ui]\npage_title = Keep Me\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Default()
	cfg.Protocol = ProtocolHTTPS
	cfg.Port = 8443
	cfg.AuthEnabled = false
	cfg.Username = "old-admin"
	cfg.Password = "change-me"
	cfg.SharedServerEnabled = true
	cfg.SharedProjectID = "project-a"
	cfg.SharedIdentityDB = "/tmp/taskdeck-identity-test.db"
	if err := WriteAuthenticationMode(workspace, cfg); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"protocol = https",
		"bind = 127.0.0.1",
		"port = 8443",
		"enabled = false\nusername = old-admin\npassword = change-me",
		"enabled = true\nproject_id = project-a\nidentity_db = /tmp/taskdeck-identity-test.db",
		"branch = release/test",
		"run_full_validation_tests = true",
		"page_title = Keep Me",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated auth config missing %q:\n%s", want, text)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode=%o want 600", info.Mode().Perm())
	}
}

func TestWriteAuthenticationModeSwitchesBackToSingleAuth(t *testing.T) {
	workspace := t.TempDir()
	vscode := filepath.Join(workspace, ".vscode")
	if err := os.MkdirAll(vscode, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(vscode, "vscode_tasks_menu.ini")
	initial := "[server]\nprotocol = https\nbind = 127.0.0.1\nport = 8443\n\n[auth]\nenabled = false\nusername = admin\npassword = change-me\n\n[shared_server]\nenabled = true\nproject_id = project-a\nidentity_db = /tmp/taskdeck-identity-test.db\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Default()
	cfg.Protocol = ProtocolHTTPS
	cfg.Port = 8443
	cfg.AuthEnabled = true
	cfg.Username = "local-admin"
	cfg.Password = "new-local-password"
	cfg.SharedServerEnabled = false
	cfg.SharedProjectID = "project-a"
	cfg.SharedIdentityDB = "/tmp/taskdeck-identity-test.db"
	if err := WriteAuthenticationMode(workspace, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SharedServerEnabled || !loaded.AuthEnabled || loaded.Username != "local-admin" || loaded.Password != "new-local-password" {
		t.Fatalf("unexpected single-auth config after migration: %+v", loaded)
	}
	if loaded.SharedProjectID != "project-a" || loaded.SharedIdentityDB != "/tmp/taskdeck-identity-test.db" {
		t.Fatalf("shared settings should be retained for later migration: %+v", loaded)
	}
}

func TestWriteAuthenticationModeRejectsINIInjection(t *testing.T) {
	workspace := t.TempDir()
	cfg := Default()
	cfg.AuthEnabled = true
	cfg.Username = "admin"
	cfg.Password = "bad\n[shared_server]"
	if err := WriteAuthenticationMode(workspace, cfg); err == nil {
		t.Fatal("expected newline-bearing auth password to be rejected")
	}
}
