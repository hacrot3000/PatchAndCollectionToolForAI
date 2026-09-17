package server

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceTerminalExecutionUsesProjectRoot(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("SHELL", "/bin/sh")

	spec, err := workspaceTerminalExecution(workspace)
	if err != nil {
		t.Fatalf("workspaceTerminalExecution: %v", err)
	}
	if spec.TaskID != 0 {
		t.Fatalf("terminal task id = %d, want 0", spec.TaskID)
	}
	if spec.Label != "Terminal" {
		t.Fatalf("terminal label = %q, want Terminal", spec.Label)
	}
	if spec.Cwd != filepath.Clean(workspace) {
		t.Fatalf("terminal cwd = %q, want %q", spec.Cwd, filepath.Clean(workspace))
	}
	if spec.Command != "/bin/sh" {
		t.Fatalf("terminal command = %q, want /bin/sh", spec.Command)
	}
	if spec.Preview != "/bin/sh" {
		t.Fatalf("terminal preview = %q, want /bin/sh", spec.Preview)
	}
	joined := strings.Join(spec.Env, "\n")
	if !strings.Contains(joined, "TERM=") {
		t.Fatalf("terminal environment does not include TERM")
	}
	if !strings.Contains(joined, "COLORTERM=truecolor") {
		t.Fatalf("terminal environment does not include COLORTERM=truecolor")
	}
}
