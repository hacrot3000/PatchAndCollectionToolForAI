package server

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestProjectFileIndexCompactOffsetsAtScale(t *testing.T) {
	paths := make([]string, 0, 10000)
	for i := 0; i < 10000; i++ {
		paths = append(paths, "src/pkg"+strconv.Itoa(i%50)+"/file"+strconv.Itoa(i)+".go")
	}
	idx, err := newProjectFileIndex(paths, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.offsets) != 10000 {
		t.Fatalf("offset count=%d want=10000", len(idx.offsets))
	}
	if got := idx.pathAt(0); got == "" {
		t.Fatal("first indexed path is empty")
	}
	if got := idx.pathAt(len(idx.offsets) - 1); got == "" {
		t.Fatal("last indexed path is empty")
	}
	if len(idx.blob) >= 2<<20 {
		t.Fatalf("compact index unexpectedly large: %d bytes", len(idx.blob))
	}
}

func TestProjectIndexFallbackSkipsGitAndSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	paths, err := projectIndexPathsFallback(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "src/main.go" {
		t.Fatalf("fallback paths=%#v", paths)
	}
}

func TestProjectIndexCacheRoundTripAndCorruption(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	root := t.TempDir()
	idx, err := newProjectFileIndex([]string{"b.txt", "src/a.go"}, time.Unix(123, 456))
	if err != nil {
		t.Fatal(err)
	}
	if err := saveProjectIndexCache(root, idx); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadProjectIndexCache(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.offsets) != 2 || loaded.pathAt(0) != "b.txt" || loaded.pathAt(1) != "src/a.go" {
		t.Fatalf("loaded index mismatch: %#v %#v", loaded.offsets, loaded.blob)
	}
	if !loaded.builtAt.Equal(idx.builtAt) {
		t.Fatalf("builtAt=%v want=%v", loaded.builtAt, idx.builtAt)
	}
	cacheFile, err := projectIndexCacheFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadProjectIndexCache(root); err == nil {
		t.Fatal("corrupt cache was accepted")
	}
}


func TestProjectIndexPathsRGParsesNULAndNormalizesPaths(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(t.TempDir(), "fake-rg")
	body := "#!/bin/sh\nprintf 'src/main.go\\0.vscode/tasks.json\\0'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	paths, err := projectIndexPathsRG(context.Background(), script, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "src/main.go" || paths[1] != ".vscode/tasks.json" {
		t.Fatalf("paths=%#v", paths)
	}
}

func TestProjectIndexFallbackHonorsRootGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored", "hidden.go"), []byte("package hidden"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scratch.tmp"), []byte("tmp"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.go"), []byte("package keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := projectIndexPathsFallback(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != ".gitignore" || paths[1] != "keep.go" {
		t.Fatalf("paths=%#v", paths)
	}
}
