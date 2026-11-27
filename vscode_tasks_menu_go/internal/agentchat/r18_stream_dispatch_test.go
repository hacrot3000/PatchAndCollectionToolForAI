package agentchat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// R18 end-to-end: a partially streamed taskdeck_submit_patch call followed by
// an abrupt stream failure must NOT yield an executable native tool call.
// This drives the real HTTPProviderInvoker -> DoChatCompletionsStreamWithEvents
// adapter boundary, not just the accumulator parser. No approval, dispatch or
// native execution intent may be derived from an incomplete call.
func TestR18PartialStreamedPatchCallNeverBecomesExecutable(t *testing.T) {
	const partial = "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-r18\",\"type\":\"function\",\"function\":{\"name\":\"taskdeck_submit_patch\",\"arguments\":\"{\\\"name\\\":\\\"p\\\",\"}}]},\"finish_reason\":null}]}\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(partial))
		// Abrupt end: no finish_reason=tool_calls and no [DONE].
	}))
	defer srv.Close()

	provider := ProviderConfig{ID: "r18", Name: "R18", APIType: "chat_completions", BaseURL: srv.URL,
		Models: []ModelConfig{{ID: "r18-model", ToolCalling: true}}}
	body, err := BuildResponsesRequest(RequestConfig{Model: "r18-model", Instructions: "policy",
		Mode: Stateless, Next: []HistoryItem{{Type: "user", Content: "apply patch"}}, EnablePatchTool: true})
	if err != nil { t.Fatal(err) }

	var deltas []StreamEvent
	invoker := HTTPProviderInvoker{}
	res, invokeErr := invoker.InvokeStream(context.Background(), provider, "test-key", body,
		func(ev StreamEvent) error { deltas = append(deltas, ev); return nil })
	if invokeErr == nil {
		t.Fatalf("partial stream was accepted as complete: %+v", res)
	}
	// The stream may emit a delta, but the final result must carry no complete
	// executable call. Any caller requiring len(Calls)==1 && valid JSON must
	// therefore refuse dispatch.
	if len(res.Calls) != 0 {
		t.Fatalf("incomplete stream yielded executable calls: %+v", res.Calls)
	}
	for _, ev := range deltas {
		if ev.Type == "tool.call.delta" && strings.Contains(ev.Arguments, "taskdeck_submit_patch") {
			// Deltas are advisory stream fragments, never execution proof.
			continue
		}
	}
	if res.Text != "" { t.Fatal("partial tool stream produced final assistant text") }
}
