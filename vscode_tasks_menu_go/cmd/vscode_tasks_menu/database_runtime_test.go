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

	if _, err := runtime.Registry.Get("redis-go"); err != nil {
		t.Fatalf("built-in Redis adapter missing: %v", err)
	}
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

	adapters := runtime.Registry.List()
	if len(adapters) != 1 || adapters[0].ID != "redis-go" {
		t.Fatalf("expected only built-in Redis without MySQL client, got=%+v", adapters)
	}
	if runtime.Sessions == nil || runtime.Tunnels == nil || runtime.Secrets == nil {
		t.Fatalf("database runtime dependencies are incomplete: %+v", runtime)
	}
}

func TestDatabaseRuntimeAdvertisesMongoWhenMongoshExists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	bin := t.TempDir()
	mongosh := filepath.Join(bin, "mongosh")
	if err := os.WriteFile(mongosh, []byte("#!/bin/sh\necho '2.5.1-fixture'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	dbRuntime, err := newDatabaseRuntime(t.TempDir(), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	defer dbRuntime.Close()

	manifest, err := dbRuntime.Registry.Get("mongo-mongosh")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Kind != "mongo" {
		t.Fatalf("manifest=%+v", manifest)
	}
	if _, err := dbRuntime.Registry.Get("redis-go"); err != nil {
		t.Fatalf("built-in Redis adapter missing: %v", err)
	}
}

func TestDatabaseRuntimeAdvertisesSQLiteWhenPython3Exists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	bin := t.TempDir()
	python3 := filepath.Join(bin, "python3")
	if err := os.WriteFile(python3, []byte("#!/bin/sh\necho 'Python 3.12.1'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	dbRuntime, err := newDatabaseRuntime(t.TempDir(), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	defer dbRuntime.Close()

	manifest, err := dbRuntime.Registry.Get("sqlite-python")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Kind != "sqlite" {
		t.Fatalf("manifest=%+v", manifest)
	}
	if _, err := dbRuntime.Registry.Get("redis-go"); err != nil {
		t.Fatalf("built-in Redis adapter missing: %v", err)
	}
}
