package dbmongo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestFindClientDiscoversMongosh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	bin := t.TempDir()
	path := filepath.Join(bin, "mongosh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho '2.5.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	client, err := FindClient()
	if err != nil {
		t.Fatal(err)
	}
	if client.Path != path || client.Version != "2.5.1" {
		t.Fatalf("client=%+v", client)
	}
}

func TestBuiltinManifestUsesTaskDeckProcessBoundary(t *testing.T) {
	manifest, err := BuiltinManifest("/opt/taskdeck")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ID != AdapterID || manifest.Kind != AdapterKind || manifest.ProtocolVersion != dbadapter.ProtocolVersion {
		t.Fatalf("manifest=%+v", manifest)
	}
	if manifest.Command != "/opt/taskdeck" || strings.Join(manifest.Args, " ") != "--db-adapter mongo" {
		t.Fatalf("process boundary=%q %#v", manifest.Command, manifest.Args)
	}
	if !manifest.Capabilities.Connect || !manifest.Capabilities.Execute || manifest.Capabilities.Transactions {
		t.Fatalf("capabilities=%+v", manifest.Capabilities)
	}
}
