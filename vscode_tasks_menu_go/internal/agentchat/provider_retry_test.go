package agentchat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type scriptedInvoker struct {
	calls    int
	provider []ProviderConfig
	keys     []string
	payloads [][]byte
	err      error
	result   ResponsesResult
}

func (i *scriptedInvoker) Invoke(_ context.Context, provider ProviderConfig, key string, payload []byte) (ResponsesResult, error) {
	i.calls++
	i.provider = append(i.provider, provider)
	i.keys = append(i.keys, key)
	i.payloads = append(i.payloads, append([]byte(nil), payload...))
	if i.calls == 1 && i.err != nil {
		return ResponsesResult{}, i.err
	}
	return i.result, nil
}

func TestConfiguredAdapterRetriesWithinPinnedProviderBinding(t *testing.T) {
	provider := ProviderConfig{ID: "composer", APIType: "responses", Models: []ModelConfig{{ID: "composer-2.5"}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-v1", Credential: "owner-key"}}
	invoker := &scriptedInvoker{err: &ProviderError{Code: "HTTP_503", HTTPStatus: 503, Retryable: true}, result: ResponsesResult{Text: "done"}}
	waits := 0
	adapter := ConfiguredProviderAdapter{
		ProviderID: "composer", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: invoker,
		Wait: func(_ context.Context, delay time.Duration) error {
			waits++
			if delay != 10*time.Second {
				t.Fatalf("provider retry delay = %v, want 10s", delay)
			}
			return nil
		},
	}
	spec := runSpecFor("composer", "composer-2.5", "run-101")
	spec.ProviderConfigVersion = "cfg-v1"
	spec.RetryMode = RetryLimited
	sink := &fixedProviderSink{}
	if err := adapter.Execute(context.Background(), spec, nil, sink); err != nil {
		t.Fatal(err)
	}
	if invoker.calls != 2 || waits != 1 || invoker.provider[0].ID != "composer" || invoker.provider[1].ID != "composer" || invoker.keys[0] != "owner-key" || invoker.keys[1] != "owner-key" || string(invoker.payloads[0]) != string(invoker.payloads[1]) {
		t.Fatalf("retry changed provider binding or payload: calls=%d waits=%d providers=%+v", invoker.calls, waits, invoker.provider)
	}
	if len(sink.events) != 3 || sink.events[0].Type != RunEventRetry || sink.events[1].Type != RunEventTextDelta {
		t.Fatalf("retry event/output sequence missing: %+v", sink.events)
	}
	var retryEvent struct {
		Attempt      int  `json:"attempt"`
		ResetPartial bool `json:"reset_partial_output"`
	}
	if err := json.Unmarshal(sink.events[0].Payload, &retryEvent); err != nil || retryEvent.Attempt != 1 || !retryEvent.ResetPartial {
		t.Fatalf("retry event did not identify/reset the failed provider attempt: %+v %v", retryEvent, err)
	}
}

func TestConfiguredAdapterRetriesIncompleteProviderStream(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Stream: true, Models: []ModelConfig{{ID: "gpt-model"}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-v1", Credential: "key", StreamingVerified: true}}
	invoker := &scriptedInvoker{err: io.ErrUnexpectedEOF, result: ResponsesResult{Text: "completed"}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: invoker,
		Wait: func(context.Context, time.Duration) error { return nil }}
	spec := runSpecFor("gpt", "gpt-model", "run-stream-retry")
	spec.ProviderConfigVersion = "cfg-v1"
	spec.RetryMode = RetryLimited
	if err := adapter.Execute(context.Background(), spec, nil, nil); err != nil {
		t.Fatal(err)
	}
	if invoker.calls != 2 {
		t.Fatalf("incomplete stream was not retried: calls=%d", invoker.calls)
	}
}

func TestConfiguredAdapterLimitedRetryCapsAtFiveAndOffNeverRetries(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Models: []ModelConfig{{ID: "gpt-model"}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-v1", Credential: "key"}}
	makeAdapter := func(calls *int) ConfiguredProviderAdapter {
		invoker := &alwaysRetryableInvoker{calls: calls}
		return ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: invoker,
			Wait: func(context.Context, time.Duration) error { return nil }}
	}
	spec := runSpecFor("gpt", "gpt-model", "run-limited")
	spec.ProviderConfigVersion = "cfg-v1"
	spec.RetryMode = RetryLimited
	limitedCalls := 0
	limitedSink := &fixedProviderSink{}
	err := makeAdapter(&limitedCalls).Execute(context.Background(), spec, nil, limitedSink)
	if err == nil || limitedCalls != 6 {
		t.Fatalf("limited policy made %d calls, want initial + five retries (err=%v)", limitedCalls, err)
	}
	retries := 0
	for _, event := range limitedSink.events {
		if event.Type == RunEventRetry {
			retries++
		}
	}
	if retries != 5 {
		t.Fatalf("limited policy emitted %d retry events, want 5", retries)
	}
	offCalls := 0
	spec.RetryMode = RetryOff
	err = makeAdapter(&offCalls).Execute(context.Background(), spec, nil, nil)
	if err == nil || offCalls != 1 {
		t.Fatalf("off policy made %d calls, want 1 (err=%v)", offCalls, err)
	}
}

type alwaysRetryableInvoker struct{ calls *int }

func (i *alwaysRetryableInvoker) Invoke(context.Context, ProviderConfig, string, []byte) (ResponsesResult, error) {
	*i.calls = *i.calls + 1
	return ResponsesResult{}, &ProviderError{Code: "HTTP_503", HTTPStatus: 503, Retryable: true}
}

func TestConfiguredAdapterNormalizesProviderErrorsWithoutLeakingCredentialText(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Models: []ModelConfig{{ID: "gpt-model"}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-v1", Credential: "api-secret"}}
	invoker := &fakeProviderInvoker{err: &ProviderError{Code: "AUTH_FAILURE", Stage: "provider_http", Message: "Bearer api-secret rejected", HTTPStatus: 401}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: invoker,
		Wait: func(context.Context, time.Duration) error { t.Fatal("permanent auth failure was retried"); return nil }}
	spec := runSpecFor("gpt", "gpt-model", "run-error")
	spec.ProviderConfigVersion = "cfg-v1"
	spec.RetryMode = RetryUnlimited
	sink := &fixedProviderSink{}
	err := adapter.Execute(context.Background(), spec, nil, sink)
	if err == nil || err.Error() == "Bearer api-secret rejected" || invoker.calls != 1 {
		t.Fatalf("provider error was not normalized: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0].Type != RunEventError || strings.Contains(string(sink.events[0].Payload), "api-secret") {
		t.Fatalf("provider error event leaked credentials or was not normalized: %+v", sink.events)
	}
}

type contextWaitingInvoker struct{}

func (contextWaitingInvoker) Invoke(ctx context.Context, _ ProviderConfig, _ string, _ []byte) (ResponsesResult, error) {
	<-ctx.Done()
	return ResponsesResult{}, ctx.Err()
}

func TestConfiguredAdapterBoundsEachProviderRequest(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Models: []ModelConfig{{ID: "gpt-model"}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-v1", Credential: "key"}}
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: contextWaitingInvoker{}, RequestTimeout: 10 * time.Millisecond}
	spec := runSpecFor("gpt", "gpt-model", "run-timeout")
	spec.ProviderConfigVersion = "cfg-v1"
	sink := &fixedProviderSink{}
	err := adapter.Execute(context.Background(), spec, nil, sink)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("provider call did not honor its per-request timeout: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0].Type != RunEventError || !strings.Contains(string(sink.events[0].Payload), "PROVIDER_TIMEOUT") {
		t.Fatalf("timeout was not emitted as a classified run error: %+v", sink.events)
	}
}

func TestConfiguredAdapterUnlimitedRetryStopsOnCancellation(t *testing.T) {
	provider := ProviderConfig{ID: "gpt", APIType: "responses", Models: []ModelConfig{{ID: "gpt-model"}}}
	resolver := &fakeProviderResolver{resolved: ResolvedProvider{Config: provider, ConfigVersion: "cfg-v1", Credential: "key"}}
	calls := 0
	adapter := ConfiguredProviderAdapter{ProviderID: "gpt", Resolver: resolver, Builder: &fakePayloadBuilder{}, Invoker: &alwaysRetryableInvoker{calls: &calls},
		Wait: func(context.Context, time.Duration) error { return context.Canceled }}
	spec := runSpecFor("gpt", "gpt-model", "run-unlimited")
	spec.ProviderConfigVersion = "cfg-v1"
	spec.RetryMode = RetryUnlimited
	err := adapter.Execute(context.Background(), spec, nil, &fixedProviderSink{})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("unlimited retry did not stop on cancel: calls=%d err=%v", calls, err)
	}
}
