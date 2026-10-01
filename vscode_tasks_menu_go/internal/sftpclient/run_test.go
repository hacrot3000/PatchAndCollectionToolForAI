package sftpclient

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunFeedsCommandsAndKeepsArgsSeparate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-sftp")
	body := "#!/bin/sh\nprintf 'args:%s\\n' \"$*\"\nprintf 'stdin:'\ncat\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := Run(ctx, Command{
		Executable: script,
		Args: []string{"-q", "-P", "22", "deploy@example.com"},
	}, "pwd\n", os.Environ(), dir, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, "args:-q -P 22 deploy@example.com") || !strings.Contains(result.Stdout, "stdin:\npwd") {
		t.Fatalf("stdout=%q", result.Stdout)
	}
}

func TestRunRejectsInteractiveFailureReportedOnStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-sftp")
	body := "#!/bin/sh\ncat >/dev/null\necho \"remote open: No such file or directory\" >&2\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Command{Executable: script}, "get \"missing\" \"local\"\n", nil, dir, 64<<10)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "no such file") {
		t.Fatalf("expected reported command failure, got %v", err)
	}
}

func TestRunBoundsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-sftp")
	body := "#!/bin/sh\ncat >/dev/null\nprintf '1234567890'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Command{Executable: script}, "pwd\n", nil, dir, 4)
	if err == nil || !strings.Contains(err.Error(), "output exceeds") {
		t.Fatalf("expected output limit error, got %v", err)
	}
}
