package dbmongo

import (
	"context"
	"os"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeMongoshRunnerFixture(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	path := filepath.Join(t.TempDir(), "mongosh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunScriptKeepsSecretOutOfArgvAndCleansFile(t *testing.T) {
	argsLog := filepath.Join(t.TempDir(), "args.txt")
	scriptLog := filepath.Join(t.TempDir(), "script.txt")
	t.Setenv("TASKDECK_MONGO_ARGS", argsLog)
	t.Setenv("TASKDECK_MONGO_SCRIPT", scriptLog)

	clientPath := writeMongoshRunnerFixture(t,
		"printf '%s\\n' \"$@\" > \"$TASKDECK_MONGO_ARGS\"\n"+
			"script=''\n"+
			"prev=''\n"+
			"for arg in \"$@\"; do if [ \"$prev\" = \"--file\" ]; then script=\"$arg\"; break; fi; prev=\"$arg\"; done\n"+
			"cp \"$script\" \"$TASKDECK_MONGO_SCRIPT\"\n"+
			"printf '%s\\n' '"+resultMarker+"{\"ok\":true}'\n",
	)
	secret := "top-secret"
	script := "const uri=\"mongodb://user:top-secret@db/main\"; print(\""+resultMarker+"{}\");"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := runScript(ctx, Client{Path: clientPath}, script, secret)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output.stdout), resultMarker) {
		t.Fatalf("stdout=%q", output.stdout)
	}

	argsData, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	args := string(argsData)
	if strings.Contains(args, secret) {
		t.Fatalf("secret leaked into mongosh argv: %s", args)
	}
	for _, want := range []string{"--nodb", "--norc", "--quiet", "--file"} {
		if !strings.Contains(args, want) {
			t.Fatalf("mongosh args missing %q: %s", want, args)
		}
	}
	var scriptPath string
	lines := strings.Split(strings.TrimSpace(args), "\n")
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] == "--file" {
			scriptPath = lines[i+1]
			break
		}
	}
	if scriptPath == "" {
		t.Fatalf("script path missing in argv: %s", args)
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Fatalf("temporary mongosh script still exists: %v", err)
	}
	scriptData, err := os.ReadFile(scriptLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scriptData), secret) {
		t.Fatalf("fixture did not capture secret-bearing script: %q", scriptData)
	}
}

func TestRunScriptRedactsSecretFromFailure(t *testing.T) {
	clientPath := writeMongoshRunnerFixture(t,
		"printf 'authentication failed for top-secret\\n' >&2\nexit 2",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := runScript(ctx, Client{Path: clientPath}, "print('x')", "top-secret")
	if err == nil {
		t.Fatal("expected mongosh failure")
	}
	if strings.Contains(err.Error(), "top-secret") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("diagnostic redaction failed: %v", err)
	}
}

func TestMongoDiagnosticRedactionCoversURIEncodedSecret(t *testing.T) {
	secret := "p@ss/word?x=y z"
	userInfo := strings.TrimPrefix(url.UserPassword("taskdeck", secret).String(), "taskdeck:")
	candidates := []string{secret, url.PathEscape(secret), url.QueryEscape(secret), userInfo}
	text := strings.Join(candidates, " | ")
	redacted := redactMongoSecret(text, secret)
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(redacted, candidate) {
			t.Fatalf("MongoDB secret variant %q leaked after redaction: %s", candidate, redacted)
		}
	}
	if strings.Count(redacted, "[redacted]") < 2 {
		t.Fatalf("redaction markers missing: %s", redacted)
	}
}

func TestRunScriptHonorsContextCancellation(t *testing.T) {
	clientPath := writeMongoshRunnerFixture(t, "sleep 30")
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runScript(ctx, Client{Path: clientPath}, "print('x')", "")
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("error=%v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatalf("cancellation too slow: %v", time.Since(started))
	}
}
