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
