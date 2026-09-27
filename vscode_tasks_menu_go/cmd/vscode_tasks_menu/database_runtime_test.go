package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDatabaseRuntimeAdvertisesMysqlWhenClientExists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	bin := t.TempDir()
	mysql := filepath.Join(bin, "mysql")
	if err := os.WriteFile(mysql, []byte("#!/bin/sh\necho 'mysql fixture 8.0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	runtime, err := newDatabaseRuntime(t.TempDir(), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	manifest, err := runtime.Registry.Get("mysql-cli")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Kind != "mysql" {
		t.Fatalf("manifest=%+v", manifest)
	}
	if runtime.Sessions == nil || runtime.Tunnels == nil || runtime.Secrets == nil {
		t.Fatalf("database runtime dependencies are incomplete: %+v", runtime)
	}
}

func TestDatabaseRuntimeStartsWithoutMysqlClient(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	runtime, err := newDatabaseRuntime(t.TempDir(), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	if got := runtime.Registry.List(); len(got) != 0 {
		t.Fatalf("unexpected adapters=%+v", got)
	}
	if runtime.Sessions == nil || runtime.Tunnels == nil || runtime.Secrets == nil {
		t.Fatalf("database runtime dependencies are incomplete: %+v", runtime)
	}
}
