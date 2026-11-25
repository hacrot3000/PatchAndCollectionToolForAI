package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestAgentCoordinationUIIsLoadedAndUsesExistingAPIs(t *testing.T) {
	module, err := webassets.Files.ReadFile("featuremods/agentchat_coordination.js")
	if err != nil {
		t.Fatal(err)
	}
	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/agentchat_coordination.js';") {
		t.Fatal("Agent coordination module is not loaded by the frontend")
	}
	for _, endpoint := range []string{
		"/api/agent/conversations", "/api/agent/conversation-groups",
		"/api/agent/coordination", "add_member", "remove_member",
		"update_instructions", "peer_progress", "peer_message", "message_kind", "contract_publish", "contract_advance", "artifact_publish", "dependency_register",
	} {
		if !strings.Contains(string(module), endpoint) {
			t.Errorf("Agent coordination UI does not use %q", endpoint)
		}
	}
	for _, unsafe := range []string{"innerHTML", "eval(", "new Function("} {
		if strings.Contains(string(module), unsafe) {
			t.Errorf("Agent coordination UI uses unsafe rendering through %q", unsafe)
		}
	}
}
