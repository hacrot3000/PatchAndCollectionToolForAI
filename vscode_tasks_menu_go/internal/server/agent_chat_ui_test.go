package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestAgentChatUIHasNoExecutionAndWiresProviderSessions(t *testing.T) {
	files := []string{"featuremods/agentchat.js", "featuremods/next.js", "featuremods/menus.js"}
	content := map[string]string{}
	for _, path := range files {
		b, err := webassets.Files.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content[path] = string(b)
	}
	js := content[files[0]]
	for _, needle := range []string{
		"'/api/agent/providers'",
		"'/api/agent/providers/test'",
		"'/api/agent/providers/probe'",
		"'/cancel'",
		"'/metadata'",
		"'/api/agent/sessions'",
		"'/messages'",
		"confirm_external:true",
		"expected_revision:s.revision",
		"Approve & run in Patch Tool",
		"Check native Patch status",
		"/execution",
		"/pack-preview",
		"+ Text file",
		"/auto-feedback",
		"/events",
		"syncAgentEvents",
		"renderAgentTabs",
		"closeAgentTab",
		"agentDrafts",
		"Auto-send results",
		"requesting_ai",
		"attachments}),",
		"request_id:requestID",
		"View console",
		"Send verified result to AI",
		"confirm_execution:action==='approve'",
		"agentSessionBusy",
		"tool_calling=verified",
		"expected_revision:session.revision",
		"globalThis.TaskMenuAgentChat=",
		"API key",
		"settingsOverlay",
	} {
		if !strings.Contains(js, needle) {
			t.Errorf("Agent Chat UI lacks %q", needle)
		}
	}
	for _, unsafe := range []string{"innerHTML", "eval(", "new Function(", "/api/patch/paste", "/api/terminal/command"} {
		if strings.Contains(js, unsafe) {
			t.Errorf("Agent Chat must not execute commands or insert HTML through %q", unsafe)
		}
	}
	if !strings.Contains(content[files[1]], "import '/featuremods/agentchat.js';") {
		t.Error("Agent Chat JS not loaded into web frontend")
	}
	if !strings.Contains(content[files[2]], "Open AI Agent Chat") || !strings.Contains(content[files[2]], "AI Agent providers / API keys") {
		t.Error("Settings menu lacks Agent Chat launch actions")
	}
}

// Regression gate: Agent Chat must stay an independent surface. Advisory
// Smart Terminal must keep no execution path, and the native Patch panel must
// not gain an Agent-execution shortcut.
func TestAgentChatDoesNotEscalateAdvisoryOrNativePatchSurfaces(t *testing.T) {
	agentJS, err := webassets.Files.ReadFile("featuremods/agentchat.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"execCommand", "child_process", "/api/terminal/", "/api/patch/upload"} {
		if strings.Contains(string(agentJS), forbidden) {
			t.Errorf("Agent Chat UI reaches an unrelated execution surface: %q", forbidden)
		}
	}
	advice, err := webassets.Files.ReadFile("featuremods/aiassistant.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/api/agent/sessions", "agentChatCall", "taskdeck_submit_patch"} {
		if strings.Contains(string(advice), forbidden) {
			t.Errorf("advisory Smart Terminal gained Agent execution surface: %q", forbidden)
		}
	}
	panel, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/api/agent/sessions", "taskdeck_submit_patch"} {
		if strings.Contains(string(panel), forbidden) {
			t.Errorf("native Patch panel gained an Agent shortcut: %q", forbidden)
		}
	}
}
