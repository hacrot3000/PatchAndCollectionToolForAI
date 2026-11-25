package agentchat

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fixedPromptOptions struct{ config RequestConfig }

func (o fixedPromptOptions) Options(RunSpec, ProviderConfig, ModelConfig) (RequestConfig, error) {
	return o.config, nil
}

func TestCanonicalPayloadBuilderMapsHistoryAndDropsOpaqueSessionState(t *testing.T) {
	builder := CanonicalProviderPayloadBuilder{Options: fixedPromptOptions{config: RequestConfig{
		Instructions: "Task policy", Mode: Conversation, PreviousResponseID: "composer-response-secret",
		ConversationID: "composer-thread-secret", MaxOutputTokens: 100,
		Next: []HistoryItem{{Type: "user", Content: "continue"}},
	}}}
	provider := ProviderConfig{ID: "gpt", APIType: "chat_completions", Models: []ModelConfig{{ID: "gpt-model"}}}
	spec := runSpecFor("gpt", "gpt-model", "run-102")
	history := []HistoryItem{{Type: "assistant", Content: "visible result", ProviderID: "composer", OriginRunID: "run-101"}}
	body, err := builder.Build(spec, provider, provider.Models[0], history)
	if err != nil {
		t.Fatal(err)
	}
	// The run builder must return canonical input, not the wire body.
	var canonical map[string]any
	if err := json.Unmarshal(body, &canonical); err != nil {
		t.Fatal(err)
	}
	if canonical["instructions"] != "Task policy" || canonical["input"] == nil || canonical["messages"] != nil {
		t.Fatalf("run payload was prematurely converted: %s", body)
	}
	body, err = BuildChatCompletionsRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["model"] != "gpt-model" {
		t.Fatalf("wrong pinned model: %#v", payload["model"])
	}
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) != 3 {
		t.Fatalf("canonical history and next turn not mapped: %#v", payload["messages"])
	}
	encoded, _ := json.Marshal(payload)
	for _, forbidden := range []string{"composer-response-secret", "composer-thread-secret", "origin_run_id", "provider_id"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("opaque/provider metadata leaked to GPT payload: %s", encoded)
		}
	}
	if !strings.Contains(string(encoded), "visible result") || !strings.Contains(string(encoded), "continue") {
		t.Fatalf("visible history did not cross provider boundary: %s", encoded)
	}
}

func TestCanonicalPayloadBuilderReplaysToolContinuationForBothProviderAPIs(t *testing.T) {
	history := []HistoryItem{
		{Type: "assistant", Content: "prior context"},
		{Type: "user", Content: "make the requested change", ParentTurnID: "turn-1"},
		{Type: "function_call", CallID: "call-native-1", Name: "taskdeck_submit_patch", Arguments: `{"name":"change","patch_python":"print(1)"}`},
		{Type: "function_call_output", CallID: "call-native-1", Content: "Native Patch PASS: changed one file"},
	}
	for _, apiType := range []string{"responses", "chat_completions"} {
		t.Run(apiType, func(t *testing.T) {
			builder := CanonicalProviderPayloadBuilder{Options: fixedPromptOptions{config: RequestConfig{Instructions: "Task policy"}}}
			provider := ProviderConfig{ID: "gpt", APIType: apiType, Models: []ModelConfig{{ID: "gpt-model", ToolCalling: true}}}
			spec := runSpecFor("gpt", "gpt-model", "run-continuation")
			spec.ParentTurnID = "turn-1"
			body, err := builder.Build(spec, provider, provider.Models[0], history)
			if err != nil {
				t.Fatal(err)
			}
			// Exactly the same conversion the production HTTP transport performs.
			if apiType == "chat_completions" {
				body, err = BuildChatCompletionsRequest(body)
				if err != nil {
					t.Fatal(err)
				}
			}
			var payload struct {
				Input    []map[string]any `json:"input"`
				Messages []map[string]any `json:"messages"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			if apiType == "responses" {
				if len(payload.Input) != 4 || payload.Input[1]["role"] != "user" || payload.Input[2]["call_id"] != "call-native-1" || payload.Input[3]["call_id"] != "call-native-1" || payload.Input[3]["output"] != "Native Patch PASS: changed one file" {
					t.Fatalf("Responses continuation omitted/reordered canonical tool history: %s", body)
				}
				return
			}
			if len(payload.Messages) != 5 || payload.Messages[2]["role"] != "user" || payload.Messages[3]["role"] != "assistant" || payload.Messages[4]["role"] != "tool" || payload.Messages[4]["tool_call_id"] != "call-native-1" || payload.Messages[4]["content"] != "Native Patch PASS: changed one file" {
				t.Fatalf("Chat Completions continuation omitted/reordered canonical tool history: %s", body)
			}
		})
	}
}

func TestCanonicalPayloadBuilderRefusesToSilentlyExceedModelInputBudget(t *testing.T) {
	builder := CanonicalProviderPayloadBuilder{Options: fixedPromptOptions{config: RequestConfig{
		Instructions: "Long system context", Next: []HistoryItem{{Type: "user", Content: "a prompt"}},
	}}}
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Models: []ModelConfig{{ID: "gpt-model", MaxInputTokens: 8}}}
	spec := runSpecFor("gpt", "gpt-model", "run-budget")
	if _, err := builder.Build(spec, provider, provider.Models[0], nil); !errors.Is(err, ErrProviderContextBudget) {
		t.Fatalf("over-budget context was not rejected explicitly: %v", err)
	}
}

func TestCanonicalPayloadBuilderRefusesUnsupportedToolHistory(t *testing.T) {
	builder := CanonicalProviderPayloadBuilder{Options: fixedPromptOptions{config: RequestConfig{
		Instructions: "Task policy", Next: []HistoryItem{{Type: "user", Content: "continue"}},
	}}}
	provider := ProviderConfig{ID: "plain", APIType: "responses", Models: []ModelConfig{{ID: "plain-model", ToolCalling: false}}}
	spec := runSpecFor("plain", "plain-model", "run-1")
	history := []HistoryItem{{Type: "function_call", CallID: "call-1", Name: "inspect", Arguments: `{}`}}
	if _, err := builder.Build(spec, provider, provider.Models[0], history); !errors.Is(err, ErrProviderCapabilityMismatch) {
		t.Fatalf("unsupported tool history was not rejected: %v", err)
	}
}
