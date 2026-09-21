package server

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/tasks"
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

func TestConfigureTerminalGitTextconv(t *testing.T) {
	workspace := t.TempDir()
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("HOME", t.TempDir())

	spec := tasks.Execution{Env: os.Environ()}
	if err := configureTerminalGitTextconv(workspace, &spec); err != nil {
		t.Fatalf("configureTerminalGitTextconv: %v", err)
	}

	env := make(map[string]string, len(spec.Env))
	for _, item := range spec.Env {
		if key, value, ok := strings.Cut(item, "="); ok {
			env[key] = value
		}
	}
	count, err := strconv.Atoi(env["GIT_CONFIG_COUNT"])
	if err != nil || count < 3 {
		t.Fatalf("GIT_CONFIG_COUNT=%q err=%v", env["GIT_CONFIG_COUNT"], err)
	}

	configs := map[string]string{}
	for i := 0; i < count; i++ {
		configs[env["GIT_CONFIG_KEY_"+strconv.Itoa(i)]] = env["GIT_CONFIG_VALUE_"+strconv.Itoa(i)]
	}
	attributesPath := configs["core.attributesFile"]
	if attributesPath == "" {
		t.Fatal("terminal Git config missing core.attributesFile")
	}
	data, err := os.ReadFile(attributesPath)
	if err != nil {
		t.Fatal(err)
	}
	attributes := string(data)
	for _, want := range []string{"*.java diff=taskmenu-cr", "*.go diff=taskmenu-cr", "*.js diff=taskmenu-cr"} {
		if !strings.Contains(attributes, want) {
			t.Fatalf("managed attributes missing %q", want)
		}
	}
	if !strings.Contains(configs["diff.taskmenu-cr.textconv"], "--git-textconv") {
		t.Fatalf("textconv command=%q", configs["diff.taskmenu-cr.textconv"])
	}
	if configs["diff.taskmenu-cr.cachetextconv"] != "false" {
		t.Fatalf("cachetextconv=%q want false", configs["diff.taskmenu-cr.cachetextconv"])
	}
}
