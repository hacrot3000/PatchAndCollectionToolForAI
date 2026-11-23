package agentchat

import (
	"encoding/json"
	"strings"
	"testing"
)

func decodePayload(t *testing.T, c RequestConfig) map[string]any {
	t.Helper()
	b, err := BuildResponsesRequest(c)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBuildResponsesStatelessReplaysHistoryAndTools(t *testing.T) {
	c := RequestConfig{Model: "composer-2.5", Instructions: "Mandatory project identity and patch policy", Mode: Stateless,
		History: []HistoryItem{
			{Type: "user", Content: "Find bug"},
			{Type: "assistant", Content: "I found a bug"},
			{Type: "function_call", CallID: "call_1", Name: "taskdeck_submit_patch", Arguments: `{"patch_text":"..."}`},
			{Type: "function_call_output", CallID: "call_1", Content: "FAILED: exit 1"},
		},
		Next: []HistoryItem{{Type: "user", Content: "Fix it"}}}
	p := decodePayload(t, c)
	if p["store"] != false {
		t.Fatalf("stateless request should not rely on stored responses: %#v", p)
	}
	input := p["input"].([]any)
	if len(input) != 5 {
		t.Fatalf("history not replayed: %d", len(input))
	}
	if input[2].(map[string]any)["call_id"] != "call_1" || input[3].(map[string]any)["type"] != "function_call_output" {
		t.Fatal("tool items were not reconstructed")
	}
	if input[4].(map[string]any)["content"] != "Fix it" {
		t.Fatal("new message missing")
	}
}

func TestBuildResponsesPreviousResponseUsesDeltaAndRefreshesInstructions(t *testing.T) {
	c := RequestConfig{Model: "m", Instructions: "Must always be present", Mode: PreviousResponse, PreviousResponseID: "resp_previous",
		History: []HistoryItem{{Type: "user", Content: "large previous context"}},
		Next:    []HistoryItem{{Type: "user", Content: "continue"}}}
	p := decodePayload(t, c)
	if p["previous_response_id"] != "resp_previous" || p["instructions"] != c.Instructions || p["store"] != true {
		t.Fatalf("lost chain metadata: %#v", p)
	}
	input := p["input"].([]any)
	if len(input) != 1 || input[0].(map[string]any)["content"] != "continue" {
		t.Fatalf("unexpected replay with chained response: %#v", input)
	}
	c.PreviousResponseID = ""
	p = decodePayload(t, c)
	if _, ok := p["previous_response_id"]; ok {
		t.Fatal("must not send empty reference")
	}
	if len(p["input"].([]any)) != 2 {
		t.Fatal("missing history fallback")
	}
	if p["store"] != true {
		t.Fatal("first response must be storable for later chain")
	}
}

func TestBuildResponsesConversationDoesNotMixReferenceMethods(t *testing.T) {
	c := RequestConfig{Model: "m", Instructions: "policy", Mode: Conversation, ConversationID: "conv_1", PreviousResponseID: "resp_stale",
		History: []HistoryItem{{Type: "user", Content: "past"}},
		Next:    []HistoryItem{{Type: "user", Content: "now"}}, ReasoningEffort: "high", MaxOutputTokens: 500}
	p := decodePayload(t, c)
	if p["conversation"] != "conv_1" {
		t.Fatal("missing conversation ref")
	}
	if _, ok := p["previous_response_id"]; ok {
		t.Fatal("conversation must not mix response chaining")
	}
	if len(p["input"].([]any)) != 1 {
		t.Fatal("conversation should submit delta")
	}
	if p["max_output_tokens"] != float64(500) {
		t.Fatal("output token limit missing")
	}
	if p["reasoning"].(map[string]any)["effort"] != "high" {
		t.Fatal("reasoning missing")
	}
}

func TestResponsesRejectsMalformedInputs(t *testing.T) {
	base := RequestConfig{Model: "m", Instructions: "policy", Mode: Stateless, Next: []HistoryItem{{Type: "user", Content: "hello"}}}
	cases := []struct {
		name   string
		mutate func(*RequestConfig)
	}{
		{"missing instructions", func(c *RequestConfig) { c.Instructions = "" }},
		{"unsupported mode", func(c *RequestConfig) { c.Mode = "garbage" }},
		{"unsupported effort", func(c *RequestConfig) { c.ReasoningEffort = "ultra" }},
		{"missing next", func(c *RequestConfig) { c.Next = nil }},
		{"unknown history item", func(c *RequestConfig) { c.Next = []HistoryItem{{Type: "admin", Content: "inject"}} }},
		{"malformed tool arguments", func(c *RequestConfig) {
			c.Next = []HistoryItem{{Type: "function_call", Name: "tool", CallID: "call_1", Arguments: "{bad"}}
		}},
		{"oversize item", func(c *RequestConfig) {
			c.Next = []HistoryItem{{Type: "user", Content: strings.Repeat("x", maxItemBytes+1)}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			if _, err := BuildResponsesRequest(c); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestParseResponsesTextToolCallsAndConversation(t *testing.T) {
	arg, _ := json.Marshal(map[string]string{"patch_text": "test"})
	response := map[string]any{
		"id":           "resp_9",
		"status":       "completed",
		"conversation": map[string]string{"id": "conv_9"},
		"usage":        map[string]int{"input_tokens": 12, "output_tokens": 8},
		"output": []any{
			map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": "Checking patch"}}},
			map[string]string{"type": "function_call", "call_id": "call_9", "name": "taskdeck_submit_patch", "arguments": string(arg)},
		},
	}
	raw, _ := json.Marshal(response)
	result, err := ParseResponsesResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResponseID != "resp_9" || result.ConversationID != "conv_9" || result.Text != "Checking patch" || result.InputTokens != 12 {
		t.Fatalf("bad response: %+v", result)
	}
	if len(result.Calls) != 1 || result.Calls[0].CallID != "call_9" {
		t.Fatalf("lost call: %+v", result.Calls)
	}
}

func TestParseResponsesRefusesInvalidOrEmptyResults(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"id":"resp_1","status":"failed","output":[]}`,
		`{"id":"resp_1","status":"completed","output":[]}`,
		`{"id":"resp_1","status":"completed","output":[{"type":"function_call","call_id":"a","name":"run","arguments":"{}"},{"type":"function_call","call_id":"a","name":"run","arguments":"{}"}]}`,
		`{"id":"resp_1","status":"completed","output":[{"type":"function_call","call_id":"a","name":"run","arguments":"not JSON"}]}`,
	} {
		if _, err := ParseResponsesResult([]byte(body)); err == nil {
			t.Fatalf("accepted invalid response: %s", body)
		}
	}
}

func TestStatelessProviderContextRejectsMissingToolOutputBeforeHTTP(t *testing.T){
 orphan:=HistoryItem{Type:"function_call",CallID:"call_03541687963626754fe4b9e1",
  Name:"taskdeck_submit_patch",Arguments:`{"name":"stale","patch_python":"print('unverified')"}`}
 cfg:=RequestConfig{Model:"composer-2.5",Instructions:"TaskDeck policy",Mode:Stateless,
  History:[]HistoryItem{{Type:"user",Content:"original"},orphan},
  Next:[]HistoryItem{{Type:"user",Content:"continue"}}}
 if _,err:=BuildResponsesRequest(cfg);err==nil||
  !strings.Contains(err.Error(),"has no tool output"){
  t.Fatalf("orphan Patch must fail locally before HTTP: %v",err)
 }
 cfg.History=[]HistoryItem{{Type:"user",Content:"original"},
  {Type:"function_call_output",CallID:"not-a-call",Content:"PASS"}}
 if _,err:=BuildResponsesRequest(cfg);err==nil||
  !strings.Contains(err.Error(),"has no matching function call"){
  t.Fatalf("orphan tool feedback must fail before provider HTTP: %v",err)
 }
}

func TestStatelessProviderContextAllowsFreshOnceOnlyFeedback(t *testing.T){
 call:=HistoryItem{Type:"function_call",CallID:"call_pending",Name:"taskdeck_submit_patch",
  Arguments:`{"name":"valid","patch_python":"print('done')"}`}
 output:=HistoryItem{Type:"function_call_output",CallID:call.CallID,
  Content:`{"status":"FAIL","rc":1,"evidence":"stored once"}`}
 cfg:=RequestConfig{Model:"composer-2.5",Instructions:"TaskDeck policy",Mode:Stateless,
  History:[]HistoryItem{{Type:"user",Content:"original"},call},
  Next:[]HistoryItem{output}}
 req:=decodePayload(t,cfg)
 input:=req["input"].([]any)
 if len(input)!=3||input[1].(map[string]any)["call_id"]!=call.CallID||
  input[2].(map[string]any)["output"]!=output.Content{
  t.Fatalf("fresh once-only native Patch feedback was lost or modified: %#v",input)
 }
 // Stateful provider references need not carry the matching call again:
 // only the new tool output is sent when server-side history was verified.
 cfg.Mode=PreviousResponse
 cfg.PreviousResponseID="resp_existing"
 cfg.History=nil
 req=decodePayload(t,cfg)
 if len(req["input"].([]any))!=1||req["previous_response_id"]!="resp_existing"{
  t.Fatalf("valid stateful feedback was blocked: %#v",req)
 }
}
