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
		"+ File",
		"/automation",
		"Delete conversation",
		"Auto apply Patch",
		"Auto Collection",
		"function deleteAgentSession(",
		"function makeAgentHeaderGroup(",
		"agentPatchSyncInFlight",
		"agentHistoryItemEqual",
		"agentRenderedNodes",
		"renderAgentTabs",
		"closeAgentTab",
		"agentDrafts",
		"Auto-send results",
		"stageContext",
		"attachProjectFile",
		"Attach file to Chat",
		"Add selection to chat",
		"attachments:wireAttachments",
		"agentAttachmentDrafts.set(sessionID,attachments)",
		"normalizeStagedAttachments",
		"agentPendingAttachments",
		"editorSelectionContexts",
		"project_file_summary",
		"view.dirty?'unsaved_editor_buffer':'editor_buffer'",
		"/api/agent/task-manifests/validate",
		"Planning only",
		"Validate / Preview",
		"Download canonical JSON",
		"canonical_json",
		"settingsAPIType",
		"Chat Completions API",
		"requesting_ai",
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
		"AI Debug",
		"/api/agent/debug/report",
		"Copy log",
		"Automatic safe capture",
	} {
		if !strings.Contains(js, needle) {
			t.Errorf("Agent Chat UI lacks %q", needle)
		}
	}
	// The Attachment Compactor replaces the raw full-file snapshot path.
	// Ensure the UI stages the verified bounded source outline, while exact
	// editor selections remain an independently capped quotation.
	if !strings.Contains(js,"content_source:'project_file_summary'") ||
		!strings.Contains(js,"compacted:true") {
		t.Error("Agent Chat must stage bounded project-file summaries, not full-file snapshots")
	}
	for _, otherPath := range []string{"featuremods/projectfileactions.js", "featuremods/tabcontext.js", "featuremods/editor.js", "featuremods/markdownpreview.js", "featuremods/structuredview.js"} {
		b, err := webassets.Files.ReadFile(otherPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "globalThis.TaskMenuAgentChat") && otherPath == "featuremods/projectfileactions.js" && (!strings.Contains(string(b), "Attach to Chat") || !strings.Contains(string(b), "attachProjectFile")) {
			t.Errorf("%s lacks file attachment action", otherPath)
		}
		if otherPath == "featuremods/tabcontext.js" && (!strings.Contains(string(b), "editorActions") || !strings.Contains(string(b), "Add selection to chat")) {
			t.Errorf("%s lacks editor attachment action", otherPath)
		}
		if otherPath == "featuremods/editor.js" && !strings.Contains(string(b), "getTextSelectionContext") {
			t.Errorf("%s cannot map viewer selection", otherPath)
		}
	}
	for _, unsafe := range []string{"innerHTML", "eval(", "new Function(", "/api/patch/paste", "/api/terminal/command"} {
		if strings.Contains(js, unsafe) {
			t.Errorf("Agent Chat must not execute commands or insert HTML through %q", unsafe)
		}
	}
	if strings.Contains(js,"if(!window.confirm('Send this request") {
		t.Error("redundant external-provider Send confirmation modal remains")
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
