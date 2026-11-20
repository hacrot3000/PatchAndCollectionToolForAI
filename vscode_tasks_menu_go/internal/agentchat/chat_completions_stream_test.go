package agentchat

import (
	"strings"
	"testing"
)

func TestChatCompletionsStreamAccumulatesFragmentedTextAndToolArguments(t *testing.T) {
	a := newChatStreamAccumulator()
	chunks := []string{
		`{"id":"chatcmpl-s","choices":[{"delta":{"content":"hello "},"finish_reason":null}]}`,
		`{"id":"chatcmpl-s","choices":[{"delta":{"content":"world"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-s","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"taskdeck_submit_patch","arguments":"{\"name\":\"p\","}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-s","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"patch_python\":\"x\"}"}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-s","choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	for _, chunk := range chunks {
		if err := a.apply([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.apply([]byte("[DONE]")); err != nil {
		t.Fatal(err)
	}
	res, err := a.result()
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "hello world" || len(res.Calls) != 1 || res.Calls[0].CallID != "call-1" || !strings.Contains(res.Calls[0].Arguments, "patch_python") {
		t.Fatalf("bad streaming result: %+v", res)
	}
}

func TestChatCompletionsStreamRejectsMissingFinishAndInvalidFragments(t *testing.T) {
	a := newChatStreamAccumulator()
	if err := a.apply([]byte(`{"id":"c","choices":[{"delta":{"content":"partial"},"finish_reason":null}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.result(); err == nil {
		t.Fatal("stream without finish reason accepted")
	}
	bad := newChatStreamAccumulator()
	if err := bad.apply([]byte(`{"id":"c","choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"f","arguments":"{bad"}}]},"finish_reason":"tool_calls"}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := bad.result(); err == nil {
		t.Fatal("malformed fragmented arguments accepted")
	}
	changed := newChatStreamAccumulator()
	_ = changed.apply([]byte(`{"id":"c","choices":[{"delta":{"tool_calls":[{"index":0,"id":"one","type":"function","function":{"name":"f","arguments":"{}"}}]},"finish_reason":null}]}`))
	if err := changed.apply([]byte(`{"id":"c","choices":[{"delta":{"tool_calls":[{"index":0,"id":"two","type":"function","function":{"arguments":""}}]},"finish_reason":"tool_calls"}]}`)); err == nil {
		t.Fatal("tool call ID changed between fragments")
	}
}
