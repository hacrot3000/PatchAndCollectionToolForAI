package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalCloneUIUsesServerSideClone(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalclone.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function localTerminal(view)",
		"function cloneAllowed(view)",
		"async function cloneTerminal(view)",
		"Only local terminals can be duplicated",
		"app.hasPermission?.('terminal.create')",
		"app.canControlSession?.(view.meta)",
		"'/api/sessions/'+encodeURIComponent(view.meta.id)+'/clone'",
		"method:'POST'",
		"body:'{}'",
		"return app.attachSession(meta,true)",
		"button.className='session-clone'",
		"button.textContent='Duplicate terminal here'",
		"live CWD and launch environment",
		"globalThis.TaskMenuTerminalClone={cloneTerminal,decorate,localTerminal,cloneAllowed}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminal clone UI missing %q", want)
		}
	}

	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(next), "import '/featuremods/terminalclone.js';") {
		t.Fatal("terminal clone module is not loaded")
	}

	menus, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(menus), "['PROCESS',['.terminal-command-palette','.session-clone','.session-force-restart','.terminal-rename','.session-process-tree']]") {
		t.Fatal("Session menu does not expose terminal clone action")
	}
}
