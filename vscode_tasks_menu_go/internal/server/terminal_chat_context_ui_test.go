package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalChatContextModuleUsesRequiredStageContract(t *testing.T) {
	module, err := webassets.Files.ReadFile("featuremods/terminalchatcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(module)
	for _, required := range []string{
		"app.consoleText(view)",
		"view.term.getSelection?.()",
		"globalThis.TaskMenuAgentChat",
		"kind:payload.kind,name:payload.name,mime:payload.mime,text:payload.text,source:payload.source",
		"focusComposer:true",
		"Attach console to Chat",
		"Attach selection to Chat",
		"CAPTURE_MODE: ",
		"OMITTED_PREFIX_BYTES: ",
		"taskmenu:session-retargeted",
	} {
		if !strings.Contains(s, required) {
			t.Errorf("terminal chat module missing contract %q", required)
		}
	}
	for _, forbidden := range []string{"/api/terminal/command", "BroadcastGroup", "TaskMenuAgentChat.send", "fetch('/api/agent/sessions"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("terminal attach module must not execute/send/aggregate via %q", forbidden)
		}
	}
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(next), "import '/featuremods/terminalchatcontext.js';") {
		t.Fatal("terminalchatcontext.js is not loaded by the frontend bundle")
	}
}
