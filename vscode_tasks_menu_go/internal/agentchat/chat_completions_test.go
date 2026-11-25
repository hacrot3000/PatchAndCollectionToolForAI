package agentchat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func chatProvider(base string) ProviderConfig {
	return ProviderConfig{ID: "chat", Name: "Chat", APIType: "chat_completions", BaseURL: base,
		Models: []ModelConfig{{ID: "model-x", Name: "Model X", ToolCalling: true}}}
}

func TestBuildChatCompletionsRequestMapsCanonicalHistoryAndToolResults(t *testing.T) {
	canonical, err := BuildResponsesRequest(RequestConfig{
		Model: "model-x", Instructions: "system policy", Mode: Stateless,
		History: []HistoryItem{
			{Type: "user", Content: "first"},
			{Type: "assistant", Content: "working"},
			{Type: "function_call", CallID: "call-1", Name: "taskdeck_submit_patch", Arguments: `{"name":"p","patch_python":"x"}`},
			{Type: "function_call_output", CallID: "call-1", Content: `{"status":"PASS"}`},
		},
		Next:            []HistoryItem{{Type: "user", Content: "continue"}},
		MaxOutputTokens: 123, EnablePatchTool: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := BuildChatCompletionsRequest(canonical)
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if req.Model != "model-x" || len(req.Messages) != 6 || req.Messages[0].Role != "system" || req.Messages[0].Content != "system policy" {
		t.Fatalf("system/history mapping: %+v", req)
	}
	if req.Messages[3].Role != "assistant" || len(req.Messages[3].ToolCalls) != 1 || req.Messages[3].ToolCalls[0].ID != "call-1" {
		t.Fatalf("tool call mapping: %+v", req.Messages[3])
	}
	if req.Messages[4].Role != "tool" || req.Messages[4].ToolCallID != "call-1" || !strings.Contains(req.Messages[4].Content, "PASS") {
		t.Fatalf("tool output mapping: %+v", req.Messages[4])
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "taskdeck_submit_patch" || req.Stream {
		t.Fatalf("tool schema/stream: %+v", req)
	}
}

func TestBuildChatCompletionsRejectsStatefulReferencesAndMalformedHistory(t *testing.T) {
	for _, cfg := range []RequestConfig{
		{Model: "m", Instructions: "i", Mode: PreviousResponse, PreviousResponseID: "r", Next: []HistoryItem{{Type: "user", Content: "n"}}},
		{Model: "m", Instructions: "i", Mode: Conversation, ConversationID: "c", Next: []HistoryItem{{Type: "user", Content: "n"}}},
	} {
		canonical, err := BuildResponsesRequest(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := BuildChatCompletionsRequest(canonical); err == nil {
			t.Fatal("unsupported or malformed context accepted")
		}
	}
	malformed := []byte(`{"model":"m","instructions":"i","input":[{"type":"function_call_output","call_id":"missing","output":"x"}]}`)
	if _, err := BuildChatCompletionsRequest(malformed); err == nil {
		t.Fatal("orphan tool output accepted")
	}
}

func TestParseChatCompletionsTextAndToolCalls(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-1","choices":[{"finish_reason":"tool_calls","message":{"content":"review","tool_calls":[{"id":"call-1","type":"function","function":{"name":"taskdeck_submit_patch","arguments":"{\"name\":\"p\",\"patch_python\":\"x\"}"}}]}}],"usage":{"prompt_tokens":9,"completion_tokens":4}}`)
	res, err := ParseChatCompletionsResult(body)
	if err != nil {
		t.Fatal(err)
	}
	if res.ResponseID != "chatcmpl-1" || res.Text != "review" || len(res.Calls) != 1 || res.Calls[0].CallID != "call-1" || res.InputTokens != 9 || res.OutputTokens != 4 {
		t.Fatalf("bad canonical result: %+v", res)
	}
}

func TestParseChatCompletionsRejectsMalformedOrUnsupportedFinish(t *testing.T) {
	cases := []string{
		`{bad`,
		`{"id":"x","choices":[{"finish_reason":"length","message":{"content":"partial"}}]}`,
		`{"id":"x","choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{"id":"c","type":"function","function":{"name":"x","arguments":"not json"}}]}}]}`,
		`{"id":"x","choices":[{"finish_reason":"stop","message":{"content":[]}}]}`,
	}
	for _, body := range cases {
		if _, err := ParseChatCompletionsResult([]byte(body)); err == nil {
			t.Errorf("accepted malformed response %q", body)
		}
	}
}

func TestDoChatCompletionsUsesSharedHardenedTransport(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-2","choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	// Do not install the test server's private CA in production transport;
	// verify that loopback/private endpoints are rejected before network I/O.
	p := chatProvider("https://127.0.0.1/v1")
	canonical, _ := BuildResponsesRequest(RequestConfig{Model: "model-x", Instructions: "i", Mode: Stateless, Next: []HistoryItem{{Type: "user", Content: "hi"}}})
	if _, err := DoChatCompletions(context.Background(), p, "key", canonical); err == nil {
		t.Fatal("loopback provider was accepted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDoChatCompletionsUsesChatEndpointBearerAndCanonicalParser(t *testing.T) {
	p := chatProvider("https://api.example.invalid/v1")
	canonical, err := BuildResponsesRequest(RequestConfig{Model: "model-x", Instructions: "policy", Mode: Stateless,
		Next: []HistoryItem{{Type: "user", Content: "hello"}}, MaxOutputTokens: 20})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://api.example.invalid/v1/chat/completions" {
			t.Fatalf("wrong endpoint: %s", req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing provider credential")
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "model-x" || payload["stream"] != false {
			t.Fatalf("bad payload: %+v", payload)
		}
		messages := payload["messages"].([]any)
		if messages[0].(map[string]any)["role"] != "system" || messages[1].(map[string]any)["content"] != "hello" {
			t.Fatalf("bad canonical mapping: %+v", messages)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"chat-id","model":"model-x","choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))}, nil
	})}
	result, err := doChatCompletionsWithClient(context.Background(), p, "secret", canonical, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResponseID != "chat-id" || result.Model != "model-x" || result.FinishReason != "stop" || result.InputTokens != 3 || result.OutputTokens != 2 {
		t.Fatalf("bad result: %+v", result)
	}
}


func TestChatCompletionsForwardsConfiguredReasoningEffort(t *testing.T){
	for _,effort:=range []string{"low","medium","high","xhigh","max"} {
		canonical,err:=BuildResponsesRequest(RequestConfig{
			Model:"gpt-6.1-sol",Instructions:"taskdeck instructions",Mode:Stateless,
			Next:[]HistoryItem{{Type:"user",Content:"make a plan"}},
			ReasoningEffort:effort,
		})
		if err!=nil{t.Fatal(err)}
		body,err:=BuildChatCompletionsRequest(canonical)
		if err!=nil{t.Fatal(err)}
		var payload map[string]any
		if err:=json.Unmarshal(body,&payload);err!=nil{t.Fatal(err)}
		if payload["reasoning_effort"]!=effort {
			t.Fatalf("%s absent from Chat Completions request: %s",effort,body)
		}
	}
	canonical,err:=BuildResponsesRequest(RequestConfig{
		Model:"ordinary-model",Instructions:"policy",Mode:Stateless,
		Next:[]HistoryItem{{Type:"user",Content:"hi"}},
	})
	if err!=nil{t.Fatal(err)}
	body,err:=BuildChatCompletionsRequest(canonical)
	if err!=nil{t.Fatal(err)}
	var payload map[string]any
	if err:=json.Unmarshal(body,&payload);err!=nil{t.Fatal(err)}
	if _,present:=payload["reasoning_effort"];present {
		t.Fatalf("unconfigured model should not receive reasoning override: %s",body)
	}
}


// Exercise the actual transport with the Run Manager's payload builder, rather
// than testing a mock invoker that accepts any JSON. This catches conversion
// at both layers before any network I/O is attempted.
func TestRunPayloadReachesChatCompletionsTransport(t *testing.T) {
	p := chatProvider("https://api.example.invalid/v1")
	spec := runSpecFor(p.ID, p.Models[0].ID, "run-chat-transport")
	spec.ParentTurnID = "turn-1"
	history := []HistoryItem{
		{Type: "user", Content: "inspect and continue", ParentTurnID: "turn-1"},
		{Type: "function_call", Name: "taskdeck_collect_workspace", CallID: "collect-1", Arguments: `{"actions":[{"type":"ls","path":"."}]}`},
		{Type: "function_call_output", CallID: "collect-1", Content: `{"status":"PASS","marker":"read-result"}`},
	}
	builder := CanonicalProviderPayloadBuilder{Options: fixedPromptOptions{config: RequestConfig{
		Instructions: "pinned run policy", MaxOutputTokens: 1024,
	}}}
	canonical, err := builder.Build(spec, p, p.Models[0], history)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.String() != "https://api.example.invalid/v1/chat/completions" || req.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("wrong provider binding")
		}
		var body struct {
			Model string `json:"model"`
			Input json.RawMessage `json:"input"`
			Messages []struct {
				Role string `json:"role"`
				Content string `json:"content"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != p.Models[0].ID || len(body.Input) != 0 || len(body.Messages) != 4 ||
			body.Messages[0].Content != "pinned run policy" || body.Messages[1].Content != "inspect and continue" ||
			body.Messages[3].Role != "tool" || body.Messages[3].ToolCallID != "collect-1" ||
			!strings.Contains(body.Messages[3].Content, "read-result") {
			t.Fatalf("unexpected wire request: %+v", body)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"id":"chat-run","model":"model-x","choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`))}, nil
	})}
	result, err := doChatCompletionsWithClient(context.Background(), p, "test-key", canonical, client)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || result.Text != "ok" {
		t.Fatalf("request did not reach transport exactly once: requests=%d result=%+v", requests, result)
	}
}
