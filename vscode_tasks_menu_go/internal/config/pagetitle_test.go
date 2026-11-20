package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetPageTitlePreservesExistingConfig(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".vscode", "vscode_tasks_menu.ini")
	original := `[server]
bind = 127.0.0.1
port = 0

[auth]
enabled = true
username = admin
password = secret-value
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetPageTitle(workspace, "BleToNfc OTA Console"); err != nil {
		t.Fatal(err)
	}
	title, err := ReadPageTitle(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if title != "BleToNfc OTA Console" {
		t.Fatalf("title=%q", title)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"[server]", "bind = 127.0.0.1", "[auth]", "password = secret-value", "[ui]", "page_title = BleToNfc OTA Console"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated config missing %q:\n%s", want, text)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%#o want 0600", info.Mode().Perm())
	}
}

func TestSetPageTitleUpdatesExistingUISectionAndClearsToDefault(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "vscode_tasks_menu.ini")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(path, []byte("[ui]\npage_title = Old title\n\n[server]\nbind = 127.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetPageTitle(workspace, "New title"); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadPageTitle(workspace); err != nil || got != "New title" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if err := SetPageTitle(workspace, "   "); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadPageTitle(workspace); err != nil || got != "" {
		t.Fatalf("cleared title=%q err=%v", got, err)
	}
}

func TestPageTitleRejectsUnsafeOrOversizedValues(t *testing.T) {
	workspace := t.TempDir()
	for _, value := range []string{"line one\nline two", strings.Repeat("x", maxPageTitleRunes+1)} {
		if err := SetPageTitle(workspace, value); err == nil {
			t.Fatalf("SetPageTitle(%q) unexpectedly succeeded", value)
		}
	}
}
