package dbmysql

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeRunnerFixture(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	path := filepath.Join(t.TempDir(), "mysql")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunClientUsesStdinAndPrivateOptionFile(t *testing.T) {
	argsLog := filepath.Join(t.TempDir(), "args.txt")
	stdinLog := filepath.Join(t.TempDir(), "stdin.txt")
	t.Setenv("TASKDECK_TEST_ARGS", argsLog)
	t.Setenv("TASKDECK_TEST_STDIN", stdinLog)

	clientPath := writeRunnerFixture(t,
		"printf '%s\\n' \"$@\" > \"$TASKDECK_TEST_ARGS\"\n"+
			"cat > \"$TASKDECK_TEST_STDIN\"\n"+
			"printf '%s\\n' '<?xml version=\"1.0\"?>'\n"+
			"printf '%s\\n' '<resultset statement=\"SELECT 1\"></resultset>'\n"+
			"exit 0",
	)
	config := Config{
		Host: "127.0.0.1", Port: 3306, Username: "app", Database: "main",
		Secret: "top-secret", Charset: "utf8mb4", ConnectTimeoutSeconds: 10,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := runClient(ctx, Client{Path: clientPath}, config, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output.stdout), "<resultset") {
		t.Fatalf("stdout=%q", output.stdout)
	}

	argsData, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	args := string(argsData)
	if strings.Contains(args, config.Secret) {
		t.Fatalf("password leaked into argv: %s", args)
	}
	var credentialPath string
	for _, line := range strings.Split(args, "\n") {
		if strings.HasPrefix(line, "--defaults-extra-file=") {
			credentialPath = strings.TrimPrefix(line, "--defaults-extra-file=")
			break
		}
	}
	if credentialPath == "" {
		t.Fatalf("credential option missing from argv: %s", args)
	}
	if _, err := os.Stat(credentialPath); !os.IsNotExist(err) {
		t.Fatalf("credential file still exists after command: %v", err)
	}

	stdinData, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdinData) != "SELECT 1\n" {
		t.Fatalf("stdin=%q", stdinData)
	}
}

func TestRunClientRedactsPasswordFromFailure(t *testing.T) {
	clientPath := writeRunnerFixture(t,
		"cat >/dev/null\n"+
			"printf 'access denied for password top-secret\\n' >&2\n"+
			"exit 1",
	)
	config := Config{
		Host: "127.0.0.1", Port: 3306, Secret: "top-secret",
		Charset: "utf8mb4", ConnectTimeoutSeconds: 10,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := runClient(ctx, Client{Path: clientPath}, config, "SELECT 1")
	if err == nil {
		t.Fatal("expected command failure")
	}
	if strings.Contains(err.Error(), config.Secret) {
		t.Fatalf("password leaked in error: %v", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("redaction marker missing: %v", err)
	}
}

func TestRunClientHonorsContextCancellation(t *testing.T) {
	clientPath := writeRunnerFixture(t,
		"cat >/dev/null\n"+
			"sleep 30",
	)
	config := Config{
		Host: "127.0.0.1", Port: 3306, Charset: "utf8mb4", ConnectTimeoutSeconds: 10,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err := runClient(ctx, Client{Path: clientPath}, config, "SELECT SLEEP(30)")
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("error=%v", err)
	}
}

func TestReadBoundedRejectsOversizedOutput(t *testing.T) {
	if _, err := readBounded(strings.NewReader("abcdef"), 5); err != errClientOutputTooLarge {
		t.Fatalf("error=%v want errClientOutputTooLarge", err)
	}
}
