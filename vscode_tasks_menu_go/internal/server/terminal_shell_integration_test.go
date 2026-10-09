package server

import (
	"os"
	"os/exec"
	"runtime"
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
		"]133;C",
		"PS0=",
		"BASH_VERSINFO",
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

func TestShellIntegrationFrontendTracksCommandExecutionNotShellLifetime(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/shellintegration.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"activeCommand:'',executing:false,startedAt:0,finishedAt:0,exitCode:null,completedCount:0",
		"function beginExecution(view,source)",
		"if(code==='C'){beginExecution(view,'shell-preexec')",
		"function fallbackInput(view,data)",
		"const typed=commandText(view,state.commandRow,state.commandCol).text",
		"if(typed)beginExecution(view,'interactive-enter')",
		"const wasExecuting=state.executing",
		"state.activeCommand=state.commandRow==null?'':commandText(view,state.commandRow,state.commandCol).text",
		"state.executing=false;state.activeCommand='';"
		"if(wasExecuting){",
		"state.executing=false;state.activeCommand='';state.finishedAt=Date.now();state.exitCode=status;state.completedCount++",
		"emit(view,'execution-finished'",
		"view.term.onData(data=>fallbackInput(view,data))",
	} {
		if !strings.Contains(js,want) { t.Fatalf("terminal command state missing %q",want) }
	}
}

func TestBashShellIntegrationEmitsRealCommandStartAndExitStatus(t *testing.T) {
	if runtime.GOOS=="windows" { t.Skip("Bash integration requires a POSIX shell") }
	if _,err:=exec.LookPath("bash");err!=nil { t.Skip("bash is unavailable") }
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	spec:=tasks.Execution{Command:"bash",Preview:"bash"}
	if err:=configureTerminalShellIntegration(&spec);err!=nil {t.Fatal(err)}
	cmd:=exec.Command("bash","--noprofile",spec.Args[0],spec.Args[1],spec.Args[2])
	cmd.Stdin=strings.NewReader("printf 'taskdeck-command-marker-test\\n'\nfalse\nexit 0\n")
	output,err:=cmd.CombinedOutput()
	if err!=nil {t.Fatalf("interactive Bash terminated unexpectedly: %v output=%q",err,output)}
	text:=string(output)
	for _,marker:=range []string{
		"taskdeck-command-marker-test",
		"\x1b]133;A\x07",

		"\x1b]133;C\x07",
		"\x1b]133;D;1\x07",
	} {
		if !strings.Contains(text,marker) {t.Fatalf("Bash missing %q in output %q",marker,text)}
	}
}

func TestSSHTransientBashRCEmulatesLoginProfilesWithoutDoubleBashrc(t *testing.T) {
	rc := taskDeckSSHLoginBashRC()
	for _,want:=range []string{
		"if [ -r /etc/profile ]; then . /etc/profile; fi",
		`if [ -r "$HOME/.bash_profile" ]; then`,
		`elif [ -r "$HOME/.bash_login" ]; then`,
		`elif [ -r "$HOME/.profile" ]; then`,
		"__taskdeck_prompt_marker",
		"]133;C",
		"]133;D;%d",
	} {
		if !strings.Contains(rc,want) {t.Errorf("remote SSH login RC missing %q",want)}
	}
	if strings.Contains(rc,`if [ -r "$HOME/.bashrc" ]; then`) {
		t.Fatal("remote SSH RC must not source bashrc again after a login profile that may already source it")
	}
	if strings.Contains(rc,"taskdeck-bashrc") {
		t.Fatal("remote SSH RC must not assume the local TaskDeck RC file exists on remote")
	}
}
