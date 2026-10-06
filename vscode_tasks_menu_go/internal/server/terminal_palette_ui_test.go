package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalCommandPaletteUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalpalette.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"terminal-command-backdrop",
		"Terminal Command Palette · Ctrl/Cmd+Shift+P",
		"function activeTerminal()",
		"function buttonAction(view,selector,label",
		"function commandList()",
		"Terminal: New terminal",
		"Terminal: Duplicate terminal here",
		"Terminal: Search all terminal scrollback",
		"'.console-search-btn'",
		"'.console-save-btn'",
		"'.session-split-vertical'",
		"'.session-split-horizontal'",
		"'.session-move-split-group'",
		"'.session-process-tree'",
		"'.terminal-rename'",
		"'.session-clear-console'",
		"globalThis.TaskMenuTerminalClone?.cloneTerminal?.(view)",
		"globalThis.TaskMenuTerminalSearchAll?.open?.()",
		"event.ctrlKey||event.metaKey",
		"!event.shiftKey",
		"String(event.key||'').toLowerCase()!=='p'",
		"event.stopPropagation()",
		"button.className='terminal-command-palette'",
		"globalThis.TaskMenuTerminalPalette={open,close,commandList,decorate",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminal command palette missing %q", want)
		}
	}

	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(next), "import '/featuremods/terminalpalette.js';") {
		t.Fatal("terminal command palette module is not loaded")
	}
	menus, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(menus), "'.terminal-command-palette'") {
		t.Fatal("Session menu must expose terminal command palette")
	}
}
