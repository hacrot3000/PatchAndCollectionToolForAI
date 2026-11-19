package agentchat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	maxSSEEventBytes = 256 << 10
	maxSSETotalBytes = 8 << 20
	maxSSEEvents     = 20000
	maxSSETextBytes  = 1 << 20
)

// StreamEvent is a normalized, bounded view of one Responses SSE event. Only
// recognized event types are exposed; unknown types are ignored rather than
// interpreted as instructions or tool calls.
type StreamEvent struct {
	Type         string
	TextDelta    string
	CallID       string
	CallName     string
	Arguments    string
	ResponseID   string
	Conversation string
	Done         bool
	InputTokens  int
	OutputTokens int
}

// streamAccumulator reuses the same final parsing rules as the JSON path: an
// incomplete stream cannot produce a trusted result, and function calls are
// only accepted with a complete call ID/name/JSON arguments.
type streamAccumulator struct {
	text         strings.Builder
	callByItem   map[string]int
	calls        []FunctionCall
	argBuffers   []strings.Builder
	responseID   string
	conversation string
	inputTokens  int
	outputTokens int
	sawDone      bool
	totalBytes   int
	eventCount   int
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{callByItem: make(map[string]int)}
}

func (a *streamAccumulator) apply(ev StreamEvent) error {
	a.eventCount++
	if a.eventCount > maxSSEEvents {
		return errors.New("provider stream exceeded event limit")
	}
	if ev.TextDelta != "" {
		if a.text.Len()+len(ev.TextDelta) > maxSSETextBytes {
			return errors.New("provider stream text exceeded limit")
		}
		a.text.WriteString(ev.TextDelta)
	}
	if ev.ResponseID != "" {
		a.responseID = ev.ResponseID
	}
	if ev.Conversation != "" {
		a.conversation = ev.Conversation
	}
	if ev.InputTokens > 0 {
		a.inputTokens = ev.InputTokens
	}
	if ev.OutputTokens > 0 {
		a.outputTokens = ev.OutputTokens
	}
	if ev.Type == "response.output_item.added" && ev.CallID != "" {
		a.callByItem[ev.CallID] = -1
	}
	if ev.CallID != "" && ev.CallName != "" {
		if _, ok := a.callByItem[ev.CallID]; !ok {
			a.callByItem[ev.CallID] = -1
		}
	}
	if ev.Arguments != "" && ev.CallID != "" {
		idx, ok := a.callByItem[ev.CallID]
		if !ok {
			return errors.New("streamed arguments without a function call")
		}
		if idx < 0 {
			a.calls = append(a.calls, FunctionCall{CallID: ev.CallID, Name: ev.CallName})
			idx = len(a.calls) - 1
			a.callByItem[ev.CallID] = idx
			a.argBuffers = append(a.argBuffers, strings.Builder{})
		}
		if a.calls[idx].Name == "" && ev.CallName != "" {
			a.calls[idx].Name = ev.CallName
		}
		if a.argBuffers[idx].Len()+len(ev.Arguments) > maxItemBytes {
			return errors.New("streamed function arguments exceeded limit")
		}
		a.argBuffers[idx].WriteString(ev.Arguments)
	}
	if ev.Done {
		a.sawDone = true
	}
	return nil
}

func (a *streamAccumulator) result() (ResponsesResult, error) {
	if !a.sawDone {
		return ResponsesResult{}, errors.New("provider stream ended before response completion")
	}
	if a.responseID == "" || len(a.responseID) > 256 {
		return ResponsesResult{}, errors.New("streamed response missing valid ID")
	}
	for i := range a.calls {
		args := a.argBuffers[i].String()
		if a.calls[i].CallID == "" || a.calls[i].Name == "" || !json.Valid([]byte(args)) {
			return ResponsesResult{}, errors.New("streamed function call is incomplete")
		}
		a.calls[i].Arguments = args
	}
	out := ResponsesResult{
		ResponseID: a.responseID, ConversationID: a.conversation,
		Text: a.text.String(), Calls: a.calls,
		InputTokens: a.inputTokens, OutputTokens: a.outputTokens, Status: "completed",
	}
	if len(out.Calls) == 0 && strings.TrimSpace(out.Text) == "" {
		return ResponsesResult{}, errors.New("provider stream returned no text or tool calls")
	}
	return out, nil
}

// parseSSEEvent converts one SSE `data:` payload into a bounded StreamEvent.
func parseSSEEvent(payload []byte) (StreamEvent, error) {
	var raw struct {
		Type         string          `json:"type"`
		ResponseID   string          `json:"response_id"`
		Delta        string          `json:"delta"`
		CallID       string          `json:"call_id"`
		Name         string          `json:"name"`
		Arguments    string          `json:"arguments"`
		Conversation json.RawMessage `json:"conversation"`
		Response     *struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Usage  struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Output []struct {
				Type      string `json:"type"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"output"`
		} `json:"response"`
		Item *struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"item"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return StreamEvent{}, errors.New("invalid provider stream event")
	}
	ev := StreamEvent{Type: raw.Type, ResponseID: raw.ResponseID, CallID: raw.CallID, CallName: raw.Name, Arguments: raw.Arguments}
	if ev.CallID == "" && raw.Item != nil {
		ev.CallID = raw.Item.CallID
		if ev.CallName == "" {
			ev.CallName = raw.Item.Name
		}
		if ev.Arguments == "" {
			ev.Arguments = raw.Item.Arguments
		}
	}
	if raw.Conversation != nil && string(raw.Conversation) != "null" {
		if raw.Conversation[0] == '"' {
			_ = json.Unmarshal(raw.Conversation, &ev.Conversation)
		} else {
			var ref struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(raw.Conversation, &ref) == nil {
				ev.Conversation = ref.ID
			}
		}
	}
	if len(ev.Conversation) > 256 {
		return StreamEvent{}, errors.New("streamed conversation ID too large")
	}
	switch raw.Type {
	case "response.output_text.delta":
		ev.TextDelta = raw.Delta
	case "response.function_call_arguments.delta":
		if ev.CallID == "" {
			return StreamEvent{}, errors.New("function arguments event missing call ID")
		}
		// Responses puts the argument fragment in "delta", not "arguments".
		ev.Arguments = raw.Delta
	case "response.output_item.added":
		if raw.Item == nil || ev.CallID == "" {
			return StreamEvent{}, errors.New("function call added without identity")
		}
	case "response.completed":
		if raw.Response == nil {
			return StreamEvent{}, errors.New("completion event missing response")
		}
		ev.Done = true
		ev.ResponseID = raw.Response.ID
		ev.InputTokens = raw.Response.Usage.InputTokens
		ev.OutputTokens = raw.Response.Usage.OutputTokens
		if strings.TrimSpace(ev.ResponseID) == "" {
			return StreamEvent{}, errors.New("completion event missing response ID")
		}
	case "response.failed", "error":
		return StreamEvent{}, errors.New("provider reported a stream failure")
	}
	return ev, nil
}

// DoResponsesStream performs a Responses streaming request. It reuses the
// hardened DNS/redirect policy, enforces per-event and total byte limits and
// returns a result only after a completion event. Cancellation propagates via
// ctx. An EOF without completion is an error, never a partial success.
func DoResponsesStream(ctx context.Context, p ProviderConfig, key string, body []byte) (ResponsesResult, error) {
	endpoint, err := validateProviderCall(p, key, body)
	if err != nil {
		return ResponsesResult{}, err
	}
	client := newProviderClient()
	defer client.CloseIdleConnections()
	req, err := newProviderRequest(ctx, endpoint, key, body)
	if err != nil {
		return ResponsesResult{}, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		return ResponsesResult{}, errors.New("provider request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ResponsesResult{}, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); !strings.HasPrefix(ct, "text/event-stream") {
		return ResponsesResult{}, errors.New("provider did not return an event stream")
	}
	acc := newStreamAccumulator()
	reader := bufio.NewReaderSize(io.LimitReader(resp.Body, maxSSETotalBytes+1), 32<<10)
	var data strings.Builder
	dispatch := func() error {
		if data.Len() == 0 {
			return nil
		}
		payload := strings.TrimSpace(data.String())
		data.Reset()
		if payload == "" || payload == "[DONE]" {
			return nil
		}
		ev, err := parseSSEEvent([]byte(payload))
		if err != nil {
			return err
		}
		return acc.apply(ev)
	}
	for {
		line, err := reader.ReadString('\n')
		acc.totalBytes += len(line)
		if acc.totalBytes > maxSSETotalBytes {
			return ResponsesResult{}, errors.New("provider stream exceeded size limit")
		}
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(trimmed, "data:"):
			chunk := strings.TrimPrefix(trimmed, "data:")
			chunk = strings.TrimPrefix(chunk, " ")
			if data.Len()+len(chunk) > maxSSEEventBytes {
				return ResponsesResult{}, errors.New("provider stream event exceeded limit")
			}
			data.WriteString(chunk)
			data.WriteByte('\n')
		case trimmed == "":
			if dispatchErr := dispatch(); dispatchErr != nil {
				return ResponsesResult{}, dispatchErr
			}
			if acc.sawDone {
				return acc.result()
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if dispatchErr := dispatch(); dispatchErr != nil {
					return ResponsesResult{}, dispatchErr
				}
				if acc.sawDone {
					return acc.result()
				}
				return ResponsesResult{}, errors.New("provider stream ended before completion")
			}
			if ctx.Err() != nil {
				return ResponsesResult{}, ctx.Err()
			}
			return ResponsesResult{}, errors.New("cannot read provider stream")
		}
	}
}
