package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadGitSettingsDefaults(t *testing.T) {
	workspace := t.TempDir()
	got, err := ReadGitSettings(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ScanEnabled || got.ScanDepth != 4 || got.DefaultRepository != "." || got.AutoSelectFromTerminalCWD || len(got.Repositories) != 0 {
		t.Fatalf("defaults=%+v", got)
	}
}

func TestReadGitSettingsFromWorkspaceINI(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil { t.Fatal(err) }
	content := `[server]
bind = 127.0.0.1

[git]
scan_enabled = true
scan_depth = 6
default_repository = projects/m3-client
auto_select_from_terminal_cwd = true

[git.repositories]
M3 Client = projects/m3-client
M3 Server = ./projects/m3-server
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil { t.Fatal(err) }
	got, err := ReadGitSettings(workspace)
	if err != nil { t.Fatal(err) }
	if !got.ScanEnabled || got.ScanDepth != 6 || got.DefaultRepository != "projects/m3-client" || !got.AutoSelectFromTerminalCWD {
		t.Fatalf("settings=%+v", got)
	}
	if got.Repositories["M3 Client"] != "projects/m3-client" || got.Repositories["M3 Server"] != "projects/m3-server" {
		t.Fatalf("repositories=%v", got.Repositories)
	}
}

func TestReadGitSettingsRejectsTraversalAndExcessiveDepth(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(configPath, []byte("[git]\ndefault_repository = ../outside\n"), 0o600); err != nil { t.Fatal(err) }
	if _, err := ReadGitSettings(workspace); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if err := os.WriteFile(configPath, []byte("[git]\nscan_depth = 99\n"), 0o600); err != nil { t.Fatal(err) }
	if _, err := ReadGitSettings(workspace); err == nil {
		t.Fatal("expected excessive scan depth rejection")
	}
}
