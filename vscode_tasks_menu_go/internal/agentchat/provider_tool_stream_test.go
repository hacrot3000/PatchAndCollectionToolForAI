package agentchat

import (
	"context"
	"encoding/json"
	"testing"
)

type scriptedStreamInvoker struct {
	events []StreamEvent
	result ResponsesResult
}

func (i scriptedStreamInvoker) Invoke(context.Context, ProviderConfig, string, []byte) (ResponsesResult, error) {
	return i.result, nil
}

func (i scriptedStreamInvoker) InvokeStream(_ context.Context, _ ProviderConfig, _ string, _ []byte, emit func(StreamEvent) error) (ResponsesResult, error) {
	for _, event := range i.events {
		if err := emit(event); err != nil {
			return ResponsesResult{}, err
		}
	}
	return i.result, nil
}

func TestStreamedResponsesToolCallIsPublishedOnceWhenComplete(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Stream: true, Models: []ModelConfig{{ID: "gpt-model", ToolCalling: true}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-1", Credential: "key", StreamingVerified: true}}
	invoker := scriptedStreamInvoker{events: []StreamEvent{
		{Type: "response.output_item.added", CallID: "call-1", CallName: "taskdeck_submit_patch"},
		{Type: "response.function_call_arguments.delta", CallID: "call-1", Arguments: `{"name":"n"`},
		{Type: "response.function_call_arguments.delta", CallID: "call-1", Arguments: `,"patch_python":"p"}`},
	}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: invoker}
	sink := &fixedProviderSink{}
	spec := runSpecFor("gpt", "gpt-model", "run-stream-tool")
	spec.ProviderConfigVersion = "cfg-1"
	if err := adapter.Execute(context.Background(), spec, nil, sink); err != nil {
		t.Fatal(err)
	}
	toolEvents := 0
	for _, event := range sink.events {
		if event.Type != RunEventToolCall {
			continue
		}
		toolEvents++
		var call FunctionCall
		if err := json.Unmarshal(event.Payload, &call); err != nil {
			t.Fatal(err)
		}
		if call.CallID != "call-1" || call.Name != "taskdeck_submit_patch" || !json.Valid([]byte(call.Arguments)) {
			t.Fatalf("streamed tool call was not normalized: %+v", call)
		}
	}
	if toolEvents != 1 {
		t.Fatalf("expected exactly one canonical tool-call event, got %d", toolEvents)
	}
}

func TestStreamedChatCompletionsToolCallAssemblesNameAndArguments(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "chat_completions", Stream: true, Models: []ModelConfig{{ID: "gpt-model", ToolCalling: true}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-1", Credential: "key", StreamingVerified: true}}
	invoker := scriptedStreamInvoker{events: []StreamEvent{
		{Type: "tool.call.delta", CallID: "call-2", CallName: "taskdeck_submit_patch", Arguments: `{"name":"n"`},
		{Type: "tool.call.delta", CallID: "call-2", Arguments: `,"patch_python":"p"}`},
	}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: invoker}
	sink := &fixedProviderSink{}
	spec := runSpecFor("gpt", "gpt-model", "run-chat-tool")
	spec.ProviderConfigVersion = "cfg-1"
	if err := adapter.Execute(context.Background(), spec, nil, sink); err != nil {
		t.Fatal(err)
	}
	toolEvents := 0
	for _, event := range sink.events {
		if event.Type != RunEventToolCall {
			continue
		}
		toolEvents++
		var call FunctionCall
		if err := json.Unmarshal(event.Payload, &call); err != nil {
			t.Fatal(err)
		}
		if call.CallID != "call-2" || call.Name != "taskdeck_submit_patch" || !json.Valid([]byte(call.Arguments)) {
			t.Fatalf("Chat Completions tool call was not normalized: %+v", call)
		}
	}
	if toolEvents != 1 {
		t.Fatalf("expected exactly one canonical tool-call event, got %d", toolEvents)
	}
}

func TestStreamedToolCallWithoutNameIsWithheld(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Stream: true, Models: []ModelConfig{{ID: "gpt-model", ToolCalling: true}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-1", Credential: "key", StreamingVerified: true}}
	invoker := scriptedStreamInvoker{events: []StreamEvent{
		{Type: "response.function_call_arguments.delta", CallID: "call-3", Arguments: `{"name":"n"}`},
	}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: invoker}
	sink := &fixedProviderSink{}
	spec := runSpecFor("gpt", "gpt-model", "run-nameless-tool")
	spec.ProviderConfigVersion = "cfg-1"
	if err := adapter.Execute(context.Background(), spec, nil, sink); err != nil {
		t.Fatal(err)
	}
	for _, event := range sink.events {
		if event.Type == RunEventToolCall {
			t.Fatalf("tool call without a function name was published: %+v", event)
		}
	}
}
