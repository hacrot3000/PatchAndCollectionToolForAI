package dbmysql

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestFindClientPrefersMysql(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	bin := t.TempDir()
	mysql := filepath.Join(bin, "mysql")
	mariadb := filepath.Join(bin, "mariadb")
	if err := os.WriteFile(mysql, []byte("#!/bin/sh\necho mysql-fixture\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mariadb, []byte("#!/bin/sh\necho mariadb-fixture\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	client, err := FindClient()
	if err != nil {
		t.Fatal(err)
	}
	if client.Path != mysql || client.Flavor != "mysql" || !strings.Contains(client.Version, "mysql-fixture") {
		t.Fatalf("client=%+v", client)
	}
}

func TestFindClientFallsBackToMariaDB(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	bin := t.TempDir()
	mariadb := filepath.Join(bin, "mariadb")
	if err := os.WriteFile(mariadb, []byte("#!/bin/sh\necho mariadb-fixture\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	client, err := FindClient()
	if err != nil {
		t.Fatal(err)
	}
	if client.Path != mariadb || client.Flavor != "mariadb" {
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
	if manifest.Command != "/opt/taskdeck" {
		t.Fatalf("command=%q", manifest.Command)
	}
	if strings.Join(manifest.Args, " ") != "--db-adapter mysql" {
		t.Fatalf("args=%#v", manifest.Args)
	}
	if !manifest.Capabilities.Connect || !manifest.Capabilities.Execute || !manifest.Capabilities.ImportSQL || manifest.Capabilities.Transactions {
		t.Fatalf("capabilities=%+v", manifest.Capabilities)
	}
}
