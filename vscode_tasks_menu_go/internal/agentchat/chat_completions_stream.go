package agentchat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxChatSSEEventBytes = 256 << 10
	maxChatSSETotalBytes = 8 << 20
	maxChatSSEEvents     = 20000
)

type chatStreamAccumulator struct {
	responseID string
	model      string
	text       strings.Builder
	calls      []FunctionCall
	callIndex  map[int]int
	args       []strings.Builder
	events     int
	totalBytes int
	finished   bool
	done       bool
	finish     string
	input      int
	cacheUsage ProviderCacheUsage
	output     int
}

func newChatStreamAccumulator() *chatStreamAccumulator {
	return &chatStreamAccumulator{callIndex: make(map[int]int)}
}

func (a *chatStreamAccumulator) apply(payload []byte, callbacks ...func(StreamEvent) error) error {
	emit := func(event StreamEvent) error {
		if len(callbacks) > 0 && callbacks[0] != nil {
			return callbacks[0](event)
		}
		return nil
	}
	if string(payload) == "[DONE]" {
		if !a.finished {
			return errors.New("Chat Completions stream ended without a finish reason")
		}
		a.done = true
		return nil
	}
	a.events++
	if a.events > maxChatSSEEvents || len(payload) > maxChatSSEEventBytes {
		return errors.New("Chat Completions stream exceeded event limit")
	}
	var chunk struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			FinishReason *string `json:"finish_reason"`
			Delta        struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
		Usage *struct {
			CacheUsageFields
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return errors.New("invalid Chat Completions stream chunk")
	}
	if chunk.ID != "" {
		a.responseID = chunk.ID
	}
	if chunk.Model != "" {
		a.model = chunk.Model
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			if a.text.Len()+len(choice.Delta.Content) > maxSSETextBytes {
				return errors.New("Chat Completions stream text exceeded limit")
			}
			a.text.WriteString(choice.Delta.Content)
			if err := emit(StreamEvent{Type: "assistant.delta", TextDelta: choice.Delta.Content}); err != nil {
				return err
			}
		}
		for _, tc := range choice.Delta.ToolCalls {
			idx, ok := a.callIndex[tc.Index]
			if !ok {
				if tc.ID == "" || tc.Type != "function" {
					return errors.New("first Chat Completions tool chunk must include ID and type")
				}
				a.calls = append(a.calls, FunctionCall{CallID: tc.ID})
				a.args = append(a.args, strings.Builder{})
				idx = len(a.calls) - 1
				a.callIndex[tc.Index] = idx
			} else if tc.ID != "" && a.calls[idx].CallID != tc.ID {
				return errors.New("Chat Completions tool call ID changed between chunks")
			}
			if tc.Function.Name != "" {
				if a.calls[idx].Name != "" && a.calls[idx].Name != tc.Function.Name {
					return errors.New("Chat Completions function name changed between chunks")
				}
				a.calls[idx].Name = tc.Function.Name
			}
			if a.args[idx].Len()+len(tc.Function.Arguments) > maxItemBytes {
				return errors.New("Chat Completions tool arguments exceeded limit")
			}
			a.args[idx].WriteString(tc.Function.Arguments)
			// The accumulated name is published with every fragment so a run can
			// assemble the complete call even when the name arrives later.
			if tc.Function.Name != "" || tc.Function.Arguments != "" {
				if err := emit(StreamEvent{Type: "tool.call.delta", CallID: a.calls[idx].CallID, CallName: a.calls[idx].Name, Arguments: tc.Function.Arguments}); err != nil {
					return err
				}
			}
		}
		if choice.FinishReason != nil {
			a.finish = *choice.FinishReason
			if a.finish != "stop" && a.finish != "tool_calls" {
				return fmt.Errorf("unsupported Chat Completions finish reason %q", a.finish)
			}
			a.finished = true
		}
	}
	if chunk.Usage != nil {
		a.input, a.output = chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
		if usage:=chunk.Usage.CacheUsageFields.normalized(a.input);usage.Reported(){a.cacheUsage=usage}
	}
	return nil
}

func (a *chatStreamAccumulator) result() (ResponsesResult, error) {
	if !a.finished || !a.done || a.finish == "" {
		return ResponsesResult{}, &ProviderError{Code: "SSE_INCOMPLETE", Stage: "provider_stream", Message: "Chat Completions stream ended before completion/finish reason", Retryable: true, Ambiguous: true}
	}
	if a.responseID == "" || len(a.responseID) > 256 {
		return ResponsesResult{}, errors.New("Chat Completions stream missing response ID")
	}
	for i := range a.calls {
		args := a.args[i].String()
		if a.calls[i].CallID == "" || a.calls[i].Name == "" || !json.Valid([]byte(args)) {
			return ResponsesResult{}, errors.New("incomplete Chat Completions function call")
		}
		a.calls[i].Arguments = args
	}
	if a.finish == "tool_calls" && len(a.calls) == 0 {
		return ResponsesResult{}, errors.New("tool_calls finish without tool calls")
	}
	if len(a.calls) == 0 && strings.TrimSpace(a.text.String()) == "" {
		return ResponsesResult{}, errors.New("empty Chat Completions stream")
	}
	return ResponsesResult{ResponseID: a.responseID, Text: a.text.String(), Calls: a.calls, InputTokens: a.input, OutputTokens: a.output, CacheUsage:a.cacheUsage, Status: "completed", FinishReason: a.finish, Model: a.model}, nil
}

// DoChatCompletionsStream sends a stateless Chat Completions request through
// the shared hardened transport and accepts only a complete SSE response.
func DoChatCompletionsStream(ctx context.Context, p ProviderConfig, key string, canonicalBody []byte) (ResponsesResult, error) {
	return DoChatCompletionsStreamWithEvents(ctx, p, key, canonicalBody, nil)
}

// DoChatCompletionsStreamWithEvents emits validated text/tool argument deltas
// without weakening the complete-result checks used by the regular API.
func DoChatCompletionsStreamWithEvents(ctx context.Context, p ProviderConfig, key string, canonicalBody []byte, onEvent func(StreamEvent) error) (ResponsesResult, error) {
	if p.APIType != "chat_completions" {
		return ResponsesResult{}, errors.New("provider is not configured for Chat Completions")
	}
	endpoint, err := p.APIURL("chat_completions")
	if err != nil {
		return ResponsesResult{}, err
	}
	if key == "" || len(key) > 8192 {
		return ResponsesResult{}, errors.New("provider credential unavailable")
	}
	body, err := BuildChatCompletionsRequest(canonicalBody)
	if err != nil {
		return ResponsesResult{}, err
	}
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return ResponsesResult{}, err
	}
	request["stream"] = true
	body, err = json.Marshal(request)
	if err != nil || len(body) > maxRequestBytes {
		return ResponsesResult{}, errors.New("Chat Completions stream request exceeds limit")
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
	recordProviderBackend(ctx,resp.Header)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10+1))
		if len(raw) > 256<<10 {
			raw = raw[:256<<10]
		}
		pe := parseProviderHTTPError(resp.StatusCode, resp.Header.Get("Retry-After"), raw)
		RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "error", Stage: "provider_http", Event: "http.response", Code: pe.Code, Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, HTTPStatus: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), RetryAfter: resp.Header.Get("Retry-After"), ProviderRequestID: providerRequestID(resp.Header), ResponseBody: raw, RequestBytes: len(body), RequestSHA256: BodyDigest(body), ResponseBytes: len(raw), Duration: time.Since(started), Timing: timing.snapshot()})
		return ResponsesResult{}, pe
	}
	if !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "error", Stage: "provider_parse", Event: "sse.invalid_content_type", Code: "SSE_INVALID_CONTENT_TYPE", Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, HTTPStatus: resp.StatusCode, ContentType: resp.Header.Get("Content-Type")})
		return ResponsesResult{}, &ProviderError{Code: "SSE_INVALID_CONTENT_TYPE", Stage: "provider_parse", Message: "Provider did not return an event stream", HTTPStatus: resp.StatusCode}
	}
	RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.response", Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, HTTPStatus: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), ProviderRequestID: providerRequestID(resp.Header), RequestBytes: len(body), RequestSHA256: BodyDigest(body), Duration: time.Since(started), Timing: timing.snapshot()})
	reader := bufio.NewReaderSize(io.LimitReader(resp.Body, maxChatSSETotalBytes+1), 32<<10)
	acc := newChatStreamAccumulator()
	var data strings.Builder
	dispatch := func() error {
		if data.Len() == 0 {
			return nil
		}
		payload := strings.TrimSpace(data.String())
		data.Reset()
		if payload == "" {
			return nil
		}
		return acc.apply([]byte(payload), onEvent)
	}
	for {
		line, readErr := reader.ReadString('\n')
		acc.totalBytes += len(line)
		if acc.totalBytes > maxChatSSETotalBytes {
			return ResponsesResult{}, errors.New("Chat Completions stream exceeded size limit")
		}
		if line != "" {
			RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "debug", Stage: "provider_stream", Event: "sse.frame", StreamFrame: []byte(line + "\n"), ResponseBytes: len(line) + 1})
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data:") {
			chunk := strings.TrimPrefix(line, "data:")
			chunk = strings.TrimPrefix(chunk, " ")
			if data.Len()+len(chunk) > maxChatSSEEventBytes {
				return ResponsesResult{}, errors.New("Chat Completions event exceeded limit")
			}
			data.WriteString(chunk)
			data.WriteByte('\n')
		} else if line == "" {
			if err := dispatch(); err != nil {
				return ResponsesResult{}, err
			}
			if acc.done {
				return acc.result()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				if err := dispatch(); err != nil {
					return ResponsesResult{}, err
				}
				if acc.done {
					return acc.result()
				}
				return ResponsesResult{}, &ProviderError{Code: "SSE_INCOMPLETE", Stage: "provider_stream", Message: "Chat Completions stream ended before completion", Retryable: true, Ambiguous: true}
			}
			if ctx.Err() != nil {
				return ResponsesResult{}, ctx.Err()
			}
			return ResponsesResult{}, errors.New("cannot read Chat Completions stream")
		}
	}
}
