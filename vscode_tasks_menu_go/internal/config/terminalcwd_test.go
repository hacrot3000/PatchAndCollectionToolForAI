package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalCWDSettingsPersistInWorkspaceINI(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "vscode_tasks_menu.ini")
	original := "[server]\nbind = 127.0.0.1\n\n[auth]\npassword = keep-me\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	want := TerminalCWDSettings{SelectedCWD: "patch", CustomDirs: []string{"patch", "tools/scripts"}}
	if err := SetTerminalCWDSettings(workspace, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTerminalCWDSettings(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectedCWD != "patch" || len(got.CustomDirs) != 2 || got.CustomDirs[0] != "patch" || got.CustomDirs[1] != "tools/scripts" {
		t.Fatalf("settings=%+v", got)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, wantText := range []string{"password = keep-me", "[terminal]", "selected_cwd = patch", `custom_dirs = ["patch","tools/scripts"]`} {
		if !strings.Contains(text, wantText) {
			t.Fatalf("config missing %q:\n%s", wantText, text)
		}
	}
}

func TestTerminalCWDSettingsDefaultAndRejectTraversal(t *testing.T) {
	workspace := t.TempDir()
	got, err := ReadTerminalCWDSettings(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectedCWD != "." || len(got.CustomDirs) != 0 {
		t.Fatalf("default settings=%+v", got)
	}
	if _, err := NormalizeTerminalCWDSettings(TerminalCWDSettings{SelectedCWD: "../outside"}); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
