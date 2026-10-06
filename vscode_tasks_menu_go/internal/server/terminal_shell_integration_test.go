package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/tasks"
	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestConfigureTerminalShellIntegrationForBash(t *testing.T) {
	configHome := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# user bashrc\n"), 0o600); err != nil { t.Fatal(err) }

	spec := tasks.Execution{Command: "/bin/bash", Preview: "/bin/bash"}
	if err := configureTerminalShellIntegration(&spec); err != nil { t.Fatal(err) }
	if len(spec.Args) != 3 || spec.Args[0] != "--rcfile" || spec.Args[2] != "-i" { t.Fatalf("args=%v", spec.Args) }
	rcPath := spec.Args[1]
	data, err := os.ReadFile(rcPath)
	if err != nil { t.Fatal(err) }
	content := string(data)
	for _, want := range []string{
		"if [ -r \"$HOME/.bashrc\" ]",
		"__taskdeck_emit_osc7",
		"]7;file://",
		"]133;D;%d",
		"]133;A",
		"]133;B",
		"PROMPT_COMMAND",
	} {
		if !strings.Contains(content, want) { t.Fatalf("bash integration rc missing %q\n%s", want, content) }
	}
	info, err := os.Stat(rcPath)
	if err != nil { t.Fatal(err) }
	if info.Mode().Perm()&0o077 != 0 { t.Fatalf("bash integration rc permissions=%o", info.Mode().Perm()) }
}

func TestConfigureTerminalShellIntegrationLeavesNonBashUntouched(t *testing.T) {
	spec := tasks.Execution{Command: "/bin/zsh", Args: []string{"-l"}, Preview: "/bin/zsh -l"}
	if err := configureTerminalShellIntegration(&spec); err != nil { t.Fatal(err) }
	if len(spec.Args) != 1 || spec.Args[0] != "-l" || spec.Preview != "/bin/zsh -l" { t.Fatalf("spec=%+v", spec) }
}

func TestShellIntegrationBrowserModuleContracts(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/shellintegration.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"registerOscHandler(7",
		"registerOscHandler(133",
		"taskmenu:shell-integration",
		"command-finished",
		"exitCode",
		"view.shellCwd",
		"TaskDeckShellIntegration",
		"maxCommands=200",
		"function replaceCommands(id,commands)",
		"replaceCommands,cwd",
	} {
		if !strings.Contains(js, want) { t.Fatalf("shellintegration.js missing %q", want) }
	}
	if strings.Contains(js, "innerHTML") || strings.Contains(js, "eval(") { t.Fatal("shell integration must not use dynamic HTML/eval") }
}

func TestShellIntegrationLoadsAfterTerminalCWD(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	cwd := strings.Index(js, "featuremods/terminalcwd.js")
	shell := strings.Index(js, "featuremods/shellintegration.js")
	if cwd < 0 || shell < 0 || shell < cwd { t.Fatalf("load order cwd=%d shell=%d", cwd, shell) }
}
