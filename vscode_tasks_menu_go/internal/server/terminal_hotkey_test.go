package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalHotkeyCreatesNewTerminal(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalhotkey.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const primary=event.ctrlKey||event.metaKey",
		"event.shiftKey",
		"!event.altKey",
		"event.code==='Backquote'",
		"event.preventDefault()",
		"event.stopPropagation()",
		"if(event.repeat)return",
		"app.startTerminal().catch(app.showError)",
		"window.addEventListener('keydown',openTerminalShortcut,true)",
		"shortcut:'Ctrl+Shift+`'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminalhotkey.js missing %q", want)
		}
	}
}

func TestTerminalHotkeyFeatureIsLoaded(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	cwd := strings.Index(js, "import '/featuremods/terminalcwd.js';")
	hotkey := strings.Index(js, "import '/featuremods/terminalhotkey.js';")
	if cwd < 0 || hotkey < 0 || hotkey < cwd {
		t.Fatalf("terminal hotkey must load after terminal cwd support: cwd=%d hotkey=%d", cwd, hotkey)
	}
}
