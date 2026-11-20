package projectfiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMovesLegacyRootFileIntoVSCode(t *testing.T) {
	workspace := t.TempDir()
	name := "vscode_tasks_menu.ini"
	legacy := filepath.Join(workspace, name)
	if err := os.WriteFile(legacy, []byte("legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(workspace, name)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(workspace, ".vscode", name)
	if got != want {
		t.Fatalf("path=%q want %q", got, want)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file still exists or stat failed: %v", err)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "legacy\n" {
		t.Fatalf("migrated content=%q", data)
	}
}

func TestMigrationNeverOverwritesExistingVSCodeFile(t *testing.T) {
	workspace := t.TempDir()
	name := "vscode_tasks_menu.ini"
	if err := os.MkdirAll(filepath.Join(workspace, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(workspace, name)
	current := filepath.Join(workspace, ".vscode", name)
	if err := os.WriteFile(legacy, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyFiles(workspace); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(current); err != nil || string(data) != "new\n" {
		t.Fatalf("current=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(legacy); err != nil || string(data) != "old\n" {
		t.Fatalf("legacy should remain when target exists: %q err=%v", data, err)
	}
}

func TestMigrationMovesAllTaskDeckRootFiles(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{
		"vscode_tasks_menu.ini",
		"vscode_tasks_menu.state.json",
		"vscode_tasks_menu.presets.json",
		"vscode_tasks_menu.terminals.json",
		"vscode_tasks_menu.terminals.mobile.json",
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := MigrateLegacyFiles(workspace); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"vscode_tasks_menu.ini",
		"vscode_tasks_menu.state.json",
		"vscode_tasks_menu.presets.json",
		"vscode_tasks_menu.terminals.json",
		"vscode_tasks_menu.terminals.mobile.json",
	} {
		if _, err := os.Stat(filepath.Join(workspace, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy %s still exists: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(workspace, ".vscode", name)); err != nil {
			t.Fatalf("migrated %s missing: %v", name, err)
		}
	}
}
