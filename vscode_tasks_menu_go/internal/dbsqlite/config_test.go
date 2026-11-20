package dbsqlite

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestConfigFromConnectPayloadResolvesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.sqlite")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{
		File:     path,
		ReadOnly: true,
		Options: map[string]string{
			"busy_timeout_ms": "2500",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.File != path || !config.ReadOnly || config.BusyTimeoutMS != 2500 {
		t.Fatalf("config=%+v", config)
	}
}

func TestConfigFromConnectPayloadResolvesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture is POSIX-oriented")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.sqlite")
	link := filepath.Join(dir, "db.sqlite")
	if err := os.WriteFile(target, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	config, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{File: link})
	if err != nil {
		t.Fatal(err)
	}
	if config.File != target {
		t.Fatalf("resolved file=%q want=%q", config.File, target)
	}
}

func TestConfigRejectsNetworkAndInvalidFileInputs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.sqlite")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []dbadapter.ConnectPayload{
		{},
		{File: filepath.Join(dir, "missing.sqlite")},
		{File: dir},
		{File: path, Host: "127.0.0.1"},
		{File: path, Port: 1234},
		{File: path, Username: "user"},
		{File: path, Secret: "secret"},
		{File: path, Database: "main"},
		{File: path, Options: map[string]string{"busy_timeout_ms": "-1"}},
		{File: path, Options: map[string]string{"busy_timeout_ms": "999999"}},
		{File: path, Options: map[string]string{"unknown": "1"}},
	}
	for i, payload := range tests {
		if _, err := ConfigFromConnectPayload(payload); err == nil {
			t.Fatalf("invalid payload %d unexpectedly accepted: %+v", i, payload)
		}
	}
}

func TestConfigRejectsNULPath(t *testing.T) {
	_, err := ConfigFromConnectPayload(dbadapter.ConnectPayload{File: "bad\x00path"})
	if err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Fatalf("error=%v", err)
	}
}
