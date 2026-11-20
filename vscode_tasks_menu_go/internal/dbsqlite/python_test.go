package dbsqlite

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestFindPythonPrefersPython3(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	bin := t.TempDir()
	python3 := filepath.Join(bin, "python3")
	python := filepath.Join(bin, "python")
	if err := os.WriteFile(python3, []byte("#!/bin/sh\necho 'Python 3.12.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(python, []byte("#!/bin/sh\necho 'Python 2.7.18'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	found, err := FindPython()
	if err != nil {
		t.Fatal(err)
	}
	if found.Path != python3 || found.Version != "Python 3.12.1" {
		t.Fatalf("python=%+v", found)
	}
}


func TestFindPythonPrefersVersionedRuntimeOverLegacyPython3(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	bin := t.TempDir()
	python310 := filepath.Join(bin, "python3.10")
	legacyPython3 := filepath.Join(bin, "python3")
	if err := os.WriteFile(python310, []byte("#!/bin/sh\necho 'Python 3.10.19'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPython3, []byte("#!/bin/sh\necho 'Python 3.6.8'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("TASKDECK_PYTHON", "")

	found, err := FindPython()
	if err != nil {
		t.Fatal(err)
	}
	if found.Path != python310 || found.Version != "Python 3.10.19" {
		t.Fatalf("python=%+v want python3.10", found)
	}
}

func TestFindPythonRejectsPython2Only(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	bin := t.TempDir()
	python := filepath.Join(bin, "python")
	if err := os.WriteFile(python, []byte("#!/bin/sh\necho 'Python 2.7.18'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	if _, err := FindPython(); err == nil || !strings.Contains(err.Error(), "not Python 3") {
		t.Fatalf("error=%v", err)
	}
}

func TestSQLiteBuiltinManifest(t *testing.T) {
	manifest, err := BuiltinManifest("/opt/taskdeck")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ID != AdapterID || manifest.Kind != AdapterKind || manifest.ProtocolVersion != dbadapter.ProtocolVersion {
		t.Fatalf("manifest=%+v", manifest)
	}
	if manifest.Command != "/opt/taskdeck" || strings.Join(manifest.Args, " ") != "--db-adapter sqlite" {
		t.Fatalf("process boundary=%q %#v", manifest.Command, manifest.Args)
	}
	if !manifest.Capabilities.Execute || !manifest.Capabilities.ImportSQL || !manifest.Capabilities.Cancel || !manifest.Capabilities.Transactions {
		t.Fatalf("capabilities=%+v", manifest.Capabilities)
	}
}
