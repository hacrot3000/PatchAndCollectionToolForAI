package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestUnifiedCommandPaletteUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalpalette.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"terminal-command-backdrop",
		"TaskDeck Command Palette · Ctrl/Cmd+Shift+P",
		"function activeTerminal()",
		"function buttonAction(view,selector,label",
		"function commandList()",
		"function dynamicTaskCommands()",
		"File: Quick Open…",
		"Project: Open Explorer",
		"Project: Search in Files…",
		"Project: Symbols…",
		"Project: Profiles…",
		"Project: Workspace Snapshots…",
		"Git: Open panel",
		"Git: Refresh status",
		"Connections: Open panel",
		"SSH: Open saved connections",
		"Database: Open saved connections",
		"SFTP/FTP: Open saved connections",
		"Settings: Open menu",
		"Settings: Edit tasks.json…",
		"globalThis.TaskMenuGitFiles?.open?.()",
		"globalThis.TaskMenuConnections?.open?.()",
		"globalThis.TaskMenuMenus?.open?.('settings')",
		"run:()=>app.startTask(task)",
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
		"globalThis.TaskMenuCommandPalette=paletteAPI",
		"globalThis.TaskMenuTerminalPalette=paletteAPI",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("unified command palette missing %q", want)
		}
	}

	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(next), "import '/featuremods/terminalpalette.js';") {
		t.Fatal("unified command palette module is not loaded")
	}
	menus, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(menus), "'.terminal-command-palette'") {
		t.Fatal("Session menu must expose unified command palette launcher")
	}
}
