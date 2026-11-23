package agentchat

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeProviderResolver struct {
	owner, provider, version string
	resolved                 ResolvedProvider
	err                      error
}

func (r *fakeProviderResolver) Resolve(_ context.Context, spec RunSpec) (ResolvedProvider, error) {
	r.owner, r.provider, r.version = spec.OwnerWorkspaceID, spec.ProviderID, spec.ProviderConfigVersion
	return r.resolved, r.err
}

type fakePayloadBuilder struct {
	spec    RunSpec
	history []HistoryItem
}

func (b *fakePayloadBuilder) Build(spec RunSpec, _ ProviderConfig, _ ModelConfig, history []HistoryItem) ([]byte, error) {
	b.spec = spec
	b.history = history
	return []byte("canonical-payload"), nil
}

type fakeProviderInvoker struct {
	provider ProviderConfig
	key      string
	payload  []byte
	result   ResponsesResult
	err      error
	calls    int
}

func (i *fakeProviderInvoker) Invoke(_ context.Context, provider ProviderConfig, key string, payload []byte) (ResponsesResult, error) {
	i.calls++
	i.provider, i.key, i.payload = provider, key, append([]byte(nil), payload...)
	return i.result, i.err
}

type fixedProviderSink struct{ events []RunEvent }

func (s *fixedProviderSink) Publish(_ context.Context, event RunEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestConfiguredProviderAdapterUsesOwnerBindingAndEmitsNormalizedResult(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Models: []ModelConfig{{ID: "gpt-6.1-sol"}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-5", Credential: "secret-never-in-event"}}
	builder, invoker := &fakePayloadBuilder{}, &fakeProviderInvoker{result: ResponsesResult{Text: "answer", InputTokens: 11, OutputTokens: 4}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: builder, Invoker: invoker}
	spec := runSpecFor("gpt", "gpt-6.1-sol", "run-102")
	spec.OwnerWorkspaceID = "owner-workspace"
	spec.ProviderConfigVersion = "cfg-5"
	history := []HistoryItem{{Type: "user", Content: "continue"}}
	sink := &fixedProviderSink{}
	if err := adapter.Execute(context.Background(), spec, history, sink); err != nil {
		t.Fatal(err)
	}
	if resolver.owner != spec.OwnerWorkspaceID || resolver.provider != spec.ProviderID || resolver.version != spec.ProviderConfigVersion {
		t.Fatalf("configuration did not resolve from immutable owner binding: %+v", resolver)
	}
	if builder.spec.ExecutionWorkspaceID != spec.ExecutionWorkspaceID || len(builder.history) != 1 {
		t.Fatal("payload builder did not receive the pinned run context and snapshot history")
	}
	if invoker.key != "secret-never-in-event" || string(invoker.payload) != "canonical-payload" {
		t.Fatal("provider invocation did not use the owner-resolved credential and canonical payload")
	}
	if len(sink.events) != 2 || sink.events[0].Type != RunEventTextDelta || sink.events[1].Type != RunEventUsage {
		t.Fatalf("unexpected normalized provider events: %+v", sink.events)
	}
	for _, event := range sink.events {
		if string(event.Payload) == invoker.key {
			t.Fatal("provider credential leaked into normalized events")
		}
	}
}

type fakeStreamingInvoker struct {
	result ResponsesResult
}

func (i fakeStreamingInvoker) Invoke(context.Context, ProviderConfig, string, []byte) (ResponsesResult, error) {
	return i.result, nil
}

func (i fakeStreamingInvoker) InvokeStream(_ context.Context, _ ProviderConfig, _ string, _ []byte, emit func(StreamEvent) error) (ResponsesResult, error) {
	for _, delta := range []string{"hel", "lo"} {
		if err := emit(StreamEvent{Type: "assistant.delta", TextDelta: delta}); err != nil {
			return ResponsesResult{}, err
		}
	}
	if err := emit(StreamEvent{Type: "tool.call.delta", CallID: "call-1", CallName: "taskdeck_collect_workspace", Arguments: `{"type":"read"`}); err != nil {
		return ResponsesResult{}, err
	}
	if err := emit(StreamEvent{Type: "tool.call.delta", CallID: "call-1", Arguments: `,"path":"a.txt"}`}); err != nil {
		return ResponsesResult{}, err
	}
	return i.result, nil
}

// partialToolStreamInvoker streams a tool call whose arguments never become
// valid JSON, so the adapter must withhold it.
type partialToolStreamInvoker struct{}

func (partialToolStreamInvoker) Invoke(context.Context, ProviderConfig, string, []byte) (ResponsesResult, error) {
	return ResponsesResult{}, nil
}

func (partialToolStreamInvoker) InvokeStream(_ context.Context, _ ProviderConfig, _ string, _ []byte, emit func(StreamEvent) error) (ResponsesResult, error) {
	if err := emit(StreamEvent{Type: "tool.call.delta", CallID: "call-partial", CallName: "taskdeck_submit_patch", Arguments: `{"name":"x"`}); err != nil {
		return ResponsesResult{}, err
	}
	return ResponsesResult{Text: "hello"}, nil
}

func TestConfiguredProviderAdapterPublishesStreamDeltasBeforeCompletion(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Stream: true, Models: []ModelConfig{{ID: "gpt-6.1-sol"}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-5", Credential: "key", StreamingVerified: true}}
	invoker := fakeStreamingInvoker{result: ResponsesResult{Text: "hello", InputTokens: 2, OutputTokens: 1, Calls: []FunctionCall{{CallID: "call-1", Name: "taskdeck_collect_workspace", Arguments: `{"type":"read"}`}}}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: invoker}
	sink := &fixedProviderSink{}
	spec := runSpecFor("gpt", "gpt-6.1-sol", "run-stream")
	spec.ProviderConfigVersion = "cfg-5"
	if err := adapter.Execute(context.Background(), spec, nil, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 6 || string(sink.events[0].Payload) != "hel" || string(sink.events[1].Payload) != "lo" ||
		sink.events[0].Type != RunEventTextDelta || sink.events[1].Type != RunEventTextDelta ||
		sink.events[2].Type != RunEventToolCallDelta || sink.events[3].Type != RunEventToolCallDelta ||
		sink.events[4].Type != RunEventToolCall {
		t.Fatalf("stream deltas/final tool call were not published in order: %+v", sink.events)
	}
	var call FunctionCall
	if err := json.Unmarshal(sink.events[4].Payload, &call); err != nil {
		t.Fatal(err)
	}
	if call.CallID != "call-1" || call.Name != "taskdeck_collect_workspace" || !json.Valid([]byte(call.Arguments)) {
		t.Fatalf("streamed tool call was not normalized: %+v", call)
	}
	if sink.events[5].Type != RunEventUsage {
		t.Fatalf("usage event missing after tool call: %+v", sink.events)
	}
}

func TestConfiguredProviderAdapterWithholdsIncompleteStreamedToolCall(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Stream: true, Models: []ModelConfig{{ID: "gpt-6.1-sol"}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-5", Credential: "key", StreamingVerified: true}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{},
		Invoker: partialToolStreamInvoker{}}
	sink := &fixedProviderSink{}
	spec := runSpecFor("gpt", "gpt-6.1-sol", "run-partial-tool")
	spec.ProviderConfigVersion = "cfg-5"
	if err := adapter.Execute(context.Background(), spec, nil, sink); err != nil {
		t.Fatal(err)
	}
	for _, event := range sink.events {
		if event.Type == RunEventToolCall {
			t.Fatalf("incomplete streamed tool call was published: %+v", event)
		}
	}
}

func TestConfiguredProviderAdapterRejectsChangedProviderConfiguration(t *testing.T) {
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{
		Config:        ProviderConfig{ID: "gpt", Models: []ModelConfig{{ID: "gpt-6.1-sol"}}},
		ConfigVersion: "new-version", Credential: "key",
	}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: &fakeProviderInvoker{}}
	err := adapter.Execute(context.Background(), runSpecFor("gpt", "gpt-6.1-sol", "run-1"), nil, nil)
	if !errors.Is(err, ErrProviderBindingMismatch) {
		t.Fatalf("changed provider config did not fail closed: %v", err)
	}
}
