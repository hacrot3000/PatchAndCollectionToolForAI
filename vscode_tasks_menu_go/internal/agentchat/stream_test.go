package agentchat

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func streamOf(t *testing.T, lines ...string) ResponsesResult {
	t.Helper()
	acc := newStreamAccumulator()
	for _, raw := range lines {
		ev, err := parseSSEEvent([]byte(raw))
		if err != nil {
			t.Fatalf("parse %s: %v", raw, err)
		}
		if err := acc.apply(ev); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	res, err := acc.result()
	if err != nil {
		t.Fatalf("result: %v", err)
	}
	return res
}

func TestStreamAccumulatorCollectsTextAndToolCall(t *testing.T) {
	res := streamOf(t,
		`{"type":"response.output_text.delta","delta":"Hel"}`,
		`{"type":"response.output_text.delta","delta":"lo"}`,
		`{"type":"response.output_item.added","item":{"type":"function_call","call_id":"c1","name":"taskdeck_submit_patch"}}`,
		`{"type":"response.function_call_arguments.delta","call_id":"c1","name":"taskdeck_submit_patch","delta":"{\"name\":\"x\","}`,
		`{"type":"response.function_call_arguments.delta","call_id":"c1","delta":"\"patch_python\":\"y\"}"}`,
		`{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":5}}}`,
	)
	if res.Text != "Hello" || res.ResponseID != "resp_1" {
		t.Fatalf("unexpected stream result: %+v", res)
	}
	if len(res.Calls) != 1 || res.Calls[0].CallID != "c1" || !strings.Contains(res.Calls[0].Arguments, "patch_python") {
		t.Fatalf("tool call not assembled: %+v", res.Calls)
	}
	if res.InputTokens != 10 || res.OutputTokens != 5 {
		t.Fatalf("usage not captured: %+v", res)
	}
}

func TestStreamAccumulatorRefusesIncompleteOrFailed(t *testing.T) {
	acc := newStreamAccumulator()
	ev, _ := parseSSEEvent([]byte(`{"type":"response.output_text.delta","delta":"partial"}`))
	if err := acc.apply(ev); err != nil {
		t.Fatal(err)
	}
	if _, err := acc.result(); err == nil {
		t.Fatal("incomplete stream treated as success")
	}
	if _, err := parseSSEEvent([]byte(`{"type":"response.failed"}`)); err == nil {
		t.Fatal("provider failure event accepted")
	}
	if _, err := parseSSEEvent([]byte(`{not json`)); err == nil {
		t.Fatal("malformed event accepted")
	}
	if _, err := parseSSEEvent([]byte(`{"type":"response.function_call_arguments.delta","delta":"{}"}`)); err == nil {
		t.Fatal("arguments without call ID accepted")
	}
	over := newStreamAccumulator()
	big := strings.Repeat("a", maxSSETextBytes+1)
	ev2, _ := parseSSEEvent([]byte(`{"type":"response.output_text.delta","delta":"` + big + `"}`))
	if err := over.apply(ev2); err == nil {
		t.Fatal("oversize stream text accepted")
	}
}

func TestStreamAccumulatorRequiresValidFunctionArguments(t *testing.T) {
	acc := newStreamAccumulator()
	for _, raw := range []string{
		`{"type":"response.output_item.added","item":{"type":"function_call","call_id":"c2","name":"taskdeck_submit_patch"}}`,
		`{"type":"response.function_call_arguments.delta","call_id":"c2","delta":"{not-json"}`,
		`{"type":"response.completed","response":{"id":"r2","status":"completed"}}`,
	} {
		ev, err := parseSSEEvent([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := acc.apply(ev); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := acc.result(); err == nil {
		t.Fatal("invalid function arguments accepted from stream")
	}
}

func TestStreamCancellationAndRedirectPolicy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := ProviderConfig{ID: "p", Name: "P", APIType: "responses", BaseURL: "https://example.invalid/v1",
		Models: []ModelConfig{{ID: "m", Name: "m"}}}
	body := []byte(`{"model":"m","instructions":"i","input":[{"role":"user","content":"hi"}]}`)
	if _, err := DoResponsesStream(ctx, p, "k", body); err == nil {
		t.Fatal("cancelled stream returned success")
	}
	// A private/local endpoint must be refused before any network attempt.
	p.BaseURL = "https://127.0.0.1/v1"
	if _, err := DoResponsesStream(ctx, p, "k", body); err == nil {
		t.Fatal("loopback endpoint accepted for streaming")
	}
	if _, err := DoResponsesStream(context.Background(), p, "", body); !errors.Is(err, err) || err == nil {
		t.Fatal("missing key accepted")
	}
}
