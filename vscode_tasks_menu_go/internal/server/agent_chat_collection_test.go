package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentCollectionProjectBoundReadOnly(t *testing.T) {
	root := t.TempDir()
	identity := patchtool.ProjectIdentity{Key: "M3", ID: "1ba64b25-0575-4f67-a6d3-cac508ef4a56"}
	if err := os.WriteFile(filepath.Join(root, "hello.php"), []byte("<?php echo 'Hello World';\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=do-not-leak"), 0600); err != nil {
		t.Fatal(err)
	}
	request := `{"id":"inspect-project-root-before-creating-hello-world-php","project":{"key":"M3","id":"1ba64b25-0575-4f67-a6d3-cac508ef4a56"},"actions":[{"type":"ls","path":".","max_entries":200},{"type":"read","path":"hello.php"}]}`
	args, err := parseAgentCollectionArguments(request, identity)
	if err != nil {
		t.Fatal(err)
	}
	fallback, ok := agentCollectionTextFallback(request, identity)
	if !ok || fallback.ID != args.ID {
		t.Fatal("valid project-bound legacy JSON was not recognized")
	}
	for _, wrapped := range []string{"> " + request, strings.Repeat("\x60",3) + "json\n" + request + "\n" + strings.Repeat("\x60",3)} {
		if recovered, ok := agentCollectionTextFallback(wrapped, identity); !ok || recovered.ID != args.ID {
			t.Fatalf("quoted/fenced Collection JSON not accepted: %s", wrapped)
		}
	}
	if _, ok := agentCollectionTextFallback("Please run this: " + request, identity); ok {
		t.Fatal("arbitrary prose was treated as an executable Collection command")
	}
	result := executeAgentCollection(root, identity, args)
	if !strings.Contains(result, "hello.php") || !strings.Contains(result, "Hello World") {
		t.Fatalf("Collection result did not include observed project file: %s", result)
	}
	if strings.Contains(result, "SECRET=do-not-leak") {
		t.Fatal("protected secret escaped in Collection result")
	}
	if _, err := os.Stat(filepath.Join(root, "hello.php")); err != nil {
		t.Fatal("read-only Collection removed the source file")
	}
	for _, bad := range []string{
		`{"project":{"key":"M4","id":"1ba64b25-0575-4f67-a6d3-cac508ef4a56"},"actions":[{"type":"ls","path":"."}]}`,
		`{"project":{"key":"M3","id":"1ba64b25-0575-4f67-a6d3-cac508ef4a56"},"actions":[{"type":"write","path":"hello.php"}]}`,
	} {
		if _, ok := agentCollectionTextFallback(bad, identity); ok {
			t.Fatalf("unsafe legacy JSON accepted: %s", bad)
		}
	}
	for _, rel := range []string{"../hello.php", "/etc/passwd", ".env"} {
		if _, err := agentCollectionRead(root, rel); err == nil {
			t.Fatalf("unsafe read succeeded: %s", rel)
		}
	}
	if err := os.Symlink(filepath.Join(root, "hello.php"), filepath.Join(root, "linked.php")); err == nil {
		if _, err := agentCollectionRead(root, "linked.php"); err == nil {
			t.Fatal("symlink source was followed")
		}
	}
}

func TestAgentChatLegacyCollectionFeedsPatchProposal(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	identity, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := json.Marshal(map[string]any{
		"id": "inspect-root", "project": identity,
		"actions": []map[string]any{{"type": "ls", "path": ".", "max_entries": 200}},
	})
	count := 0
	s.AgentChatCall = func(ctx context.Context, p agentchat.ProviderConfig, key string, payload []byte) (agentchat.ResponsesResult, error) {
		count++
		var request map[string]any
		if err := json.Unmarshal(payload, &request); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			tools := request["tools"].([]any)
			if len(tools) != 2 {
				t.Fatalf("missing Collection + Patch tools: %#v", tools)
			}
			return agentchat.ResponsesResult{ResponseID: "collect-raw", Text: string(legacy)}, nil
		}
		if count == 2 {
			items := request["input"].([]any)
			found := false
			for _, item := range items {
				text, _ := item.(map[string]any)["content"].(string)
				if strings.Contains(text, "VERIFIED observation") && strings.Contains(text, "results") {
					found = true
				}
			}
			if !found {
				t.Fatal("Collection result was not forwarded to model")
			}
			return agentchat.ResponsesResult{ResponseID: "patch-proposal", Calls: []agentchat.FunctionCall{{
				CallID: "patch-1", Name: "taskdeck_submit_patch",
				Arguments: `{"name":"Hello PHP","patch_python":"# TASKDECK_PROJECT_KEY: M3\n# TASKDECK_PROJECT_ID: 1ba64b25-0575-4f67-a6d3-cac508ef4a56\nprint('test')"}`,
			}}}, nil
		}
		t.Fatalf("unexpected provider call %d", count)
		return agentchat.ResponsesResult{}, nil
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "Create hello.php in project root", "confirm_external": true, "expected_revision": 1})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "waiting_tool") || count != 2 {
		t.Fatalf("two-turn Agent/Collection/Patch flow failed (%d calls): %d %s", count, w.Code, w.Body)
	}
	store, err := s.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Get(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.ToolCalls) != 1 || saved.ToolCalls[0].Status != "pending" {
		t.Fatalf("no pending human-approved Patch proposal: %+v", saved.ToolCalls)
	}
	if _, err := os.Stat(filepath.Join(s.Workspace, "hello.php")); !os.IsNotExist(err) {
		t.Fatal("AI proposal bypassed Patch approval and wrote the file")
	}
}

func TestAgentChatNativeCollectionFeedsModel(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	count := 0
	s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, body []byte) (agentchat.ResponsesResult, error) {
		count++
		if count == 1 {
			return agentchat.ResponsesResult{ResponseID: "native-read", Calls: []agentchat.FunctionCall{{
				CallID: "read-1", Name: "taskdeck_collect_workspace",
				Arguments: `{"actions":[{"type":"ls","path":"."}]}`,
			}}}, nil
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range payload["input"].([]any) {
			v := item.(map[string]any)
			if v["type"] == "function_call_output" && v["call_id"] == "read-1" {
				found = strings.Contains(v["output"].(string), "results")
			}
		}
		if !found {
			t.Fatal("typed Collection output was not forwarded by call_id")
		}
		return agentchat.ResponsesResult{ResponseID: "native-answer", Text: "Project root inspected."}, nil
	}
	w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
		map[string]any{"message": "Inspect project root", "confirm_external": true, "expected_revision": 1})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Project root inspected.") || count != 2 {
		t.Fatalf("native Collection flow failed (%d calls): %d %s", count, w.Code, w.Body)
	}
}
