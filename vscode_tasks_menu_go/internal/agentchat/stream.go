package agentchat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
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

type streamEventCallbackError struct{ err error }

func (e streamEventCallbackError) Error() string { return e.err.Error() }
func (e streamEventCallbackError) Unwrap() error { return e.err }

func (a *streamAccumulator) apply(ev StreamEvent, callbacks ...func(StreamEvent) error) error {
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
	if len(callbacks) > 0 && callbacks[0] != nil {
		if err := callbacks[0](ev); err != nil {
			return streamEventCallbackError{err: err}
		}
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
		// A provider may stream the complete call in the item itself.
		if ev.CallName == "" && raw.Item.Name != "" {
			ev.CallName = raw.Item.Name
		}
	case "response.output_item.done":
		if raw.Item == nil || ev.CallID == "" {
			return StreamEvent{}, errors.New("function call completed without identity")
		}
		if ev.CallName == "" && raw.Item.Name != "" {
			ev.CallName = raw.Item.Name
		}
		if ev.Arguments == "" && raw.Item.Arguments != "" {
			ev.Arguments = raw.Item.Arguments
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

// responsesStreamBody is pure: keep the durable canonical request untouched,
// but send stream:true on the wire (the Accept header alone is insufficient).
func responsesStreamBody(body []byte)([]byte,error){
 var wire map[string]json.RawMessage
 if err:=json.Unmarshal(body,&wire);err!=nil||wire==nil{
  return nil,errors.New("invalid Responses stream request")
 }
 wire["stream"]=json.RawMessage("true")
 encoded,err:=json.Marshal(wire)
 if err!=nil||len(encoded)>maxRequestBytes{
  return nil,errors.New("stream request payload exceeds limit")
 }
 return encoded,nil
}

// DoResponsesStream performs a Responses streaming request. It reuses the
// hardened DNS/redirect policy, enforces per-event and total byte limits and
// returns a result only after a completion event. Cancellation propagates via
// ctx. An EOF without completion is an error, never a partial success.
func DoResponsesStream(ctx context.Context, p ProviderConfig, key string, body []byte) (ResponsesResult, error) {
	return DoResponsesStreamWithEvents(ctx, p, key, body, nil)
}

// DoResponsesStreamWithEvents emits validated provider deltas while preserving
// the existing complete-response validation and hardened transport behavior.
func DoResponsesStreamWithEvents(ctx context.Context, p ProviderConfig, key string, body []byte, onEvent func(StreamEvent) error) (ResponsesResult, error) {
	endpoint, err := validateProviderCall(p, key, body)
	if err != nil {
		return ResponsesResult{}, err
	}
	// Responses SSE requires the JSON stream flag, not just an Accept header.
	// Only a separately capability-verified provider reaches this adapter.
	body, err = responsesStreamBody(body)
	if err != nil {
		return ResponsesResult{}, classifyProviderPreflightError(err)
	}
	client := newProviderClient(p)
	defer client.CloseIdleConnections()
	req, err := newProviderRequest(ctx, endpoint, key, body)
	if err != nil {
		return ResponsesResult{}, err
	}
	req.Header.Set("Accept", "text/event-stream")
	RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.request", Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, RequestBody: body, RequestBytes: len(body)})
	started := time.Now()
	req, timing := withProviderTiming(req)
	resp, err := client.Do(req)
	if err != nil {
		code, message, ambiguous := classifyProviderTransportError(ctx, err)
		RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "error", Stage: "provider_http", Event: "http.error", Code: code, SafeMessage: message, Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, RequestBytes: len(body), RequestSHA256: BodyDigest(body), Duration: time.Since(started), Timing: timing.snapshot()})
		return ResponsesResult{}, &ProviderError{Code: code, Stage: "provider_http", Message: message, Retryable: ambiguous, Ambiguous: ambiguous, BlockedSharedIP: blockedSharedIP(err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10+1))
		if len(raw) > 256<<10 {
			raw = raw[:256<<10]
		}
		pe := parseProviderHTTPError(resp.StatusCode, resp.Header.Get("Retry-After"), raw)
		RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "error", Stage: "provider_http", Event: "http.response", Code: pe.Code, Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, HTTPStatus: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), RetryAfter: resp.Header.Get("Retry-After"), ProviderRequestID: providerRequestID(resp.Header), ResponseBody: raw, RequestBytes: len(body), RequestSHA256: BodyDigest(body), ResponseBytes: len(raw), Duration: time.Since(started), Timing: timing.snapshot()})
		return ResponsesResult{}, pe
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); !strings.HasPrefix(ct, "text/event-stream") {
		RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "error", Stage: "provider_parse", Event: "sse.invalid_content_type", Code: "SSE_INVALID_CONTENT_TYPE", Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, HTTPStatus: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), RequestBytes: len(body), RequestSHA256: BodyDigest(body), Duration: time.Since(started), Timing: timing.snapshot()})
		return ResponsesResult{}, &ProviderError{Code: "SSE_INVALID_CONTENT_TYPE", Stage: "provider_parse", Message: "Provider did not return an event stream", HTTPStatus: resp.StatusCode}
	}
	RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.response", Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, HTTPStatus: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), ProviderRequestID: providerRequestID(resp.Header), RequestBytes: len(body), RequestSHA256: BodyDigest(body), Duration: time.Since(started), Timing: timing.snapshot()})
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
			RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "error", Stage: "provider_parse", Event: "sse.parse_error", Code: "SSE_PARSE_FAILED", SafeMessage: "Provider stream frame could not be parsed"})
			return &ProviderError{Code: "SSE_PARSE_FAILED", Stage: "provider_parse", Message: "Provider stream frame could not be parsed"}
		}
		if err = acc.apply(ev, onEvent); err != nil {
			var callbackErr streamEventCallbackError
			if errors.As(err, &callbackErr) {
				return callbackErr.err
			}
			RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "error", Stage: "provider_parse", Event: "sse.accumulate_error", Code: "SSE_INCOMPLETE", SafeMessage: "Provider stream did not contain a complete result"})
			return &ProviderError{Code: "SSE_INCOMPLETE", Stage: "provider_parse", Message: "Provider stream did not contain a complete result", Retryable: true, Ambiguous: true}
		}
		return nil
	}
	for {
		line, err := reader.ReadString('\n')
		acc.totalBytes += len(line)
		if acc.totalBytes > maxSSETotalBytes {
			return ResponsesResult{}, errors.New("provider stream exceeded size limit")
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if line != "" {
			RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "debug", Stage: "provider_stream", Event: "sse.frame", StreamFrame: []byte(line), ResponseBytes: len(line)})
		}
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
				return ResponsesResult{}, &ProviderError{Code: "SSE_INCOMPLETE", Stage: "provider_stream", Message: "Provider stream ended before completion", Retryable: true, Ambiguous: true}
			}
			if ctx.Err() != nil {
				return ResponsesResult{}, ctx.Err()
			}
			return ResponsesResult{}, errors.New("cannot read provider stream")
		}
	}
}
