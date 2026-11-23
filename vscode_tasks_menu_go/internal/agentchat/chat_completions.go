package agentchat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BuildChatCompletionsRequest translates the existing canonical Responses
// request into OpenAI-compatible Chat Completions messages. Stateful Responses
// references are rejected; the caller must provide complete canonical history.
func BuildChatCompletionsRequest(responsesBody []byte) ([]byte, error) {
	if len(responsesBody) == 0 || len(responsesBody) > maxRequestBytes || !json.Valid(responsesBody) {
		return nil, errors.New("invalid canonical request")
	}
	var in struct {
		Model        string            `json:"model"`
		Instructions string            `json:"instructions"`
		Input        []json.RawMessage `json:"input"`
		Tools        []map[string]any  `json:"tools"`
		MaxTokens    int               `json:"max_output_tokens"`
		Reasoning    struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
		PreviousID   string `json:"previous_response_id"`
		Conversation string `json:"conversation"`
	}
	if err := json.Unmarshal(responsesBody, &in); err != nil {
		return nil, errors.New("invalid canonical request")
	}
	if strings.TrimSpace(in.Model) == "" || strings.TrimSpace(in.Instructions) == "" {
		return nil, errors.New("model and instructions are required")
	}
	if in.PreviousID != "" || in.Conversation != "" {
		return nil, errors.New("Chat Completions supports stateless canonical history only")
	}
	if len(in.Input) == 0 || len(in.Input) > maxItems {
		return nil, errors.New("invalid canonical input length")
	}
	messages := make([]map[string]any, 0, len(in.Input)+1)
	messages = append(messages, map[string]any{"role": "system", "content": in.Instructions})
	seenCalls := make(map[string]bool)
	completedCalls := make(map[string]bool)
	for _, rawItem := range in.Input {
		var wire struct {
			Role      string `json:"role"`
			Type      string `json:"type"`
			Content   string `json:"content"`
			Output    string `json:"output"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := json.Unmarshal(rawItem, &wire); err != nil {
			return nil, errors.New("invalid canonical history item")
		}
		item := HistoryItem{Type: wire.Role, Content: wire.Content}
		if wire.Type != "" {
			item.Type = wire.Type
		}
		item.CallID, item.Name, item.Arguments = wire.CallID, wire.Name, wire.Arguments
		if item.Type == "function_call_output" && item.Content == "" {
			item.Content = wire.Output
		}
		if err := validHistoryItem(item); err != nil {
			return nil, err
		}
		if item.Type == "function_call" {
			if seenCalls[item.CallID] {
				return nil, errors.New("duplicate canonical tool call ID")
			}
			seenCalls[item.CallID] = true
		}
		if item.Type == "function_call_output" {
			if !seenCalls[item.CallID] || completedCalls[item.CallID] {
				return nil, errors.New("orphan or duplicate canonical tool output")
			}
			completedCalls[item.CallID] = true
		}
		switch item.Type {
		case "user", "assistant":
			messages = append(messages, map[string]any{"role": item.Type, "content": item.Content})
		case "function_call":
			messages = append(messages, map[string]any{"role": "assistant", "tool_calls": []map[string]any{{
				"id": item.CallID, "type": "function", "function": map[string]string{"name": item.Name, "arguments": item.Arguments},
			}}})
		case "function_call_output":
			messages = append(messages, map[string]any{"role": "tool", "tool_call_id": item.CallID, "content": item.Content})
		}
	}
	request := map[string]any{"model": in.Model, "messages": messages, "stream": false}
	if in.MaxTokens > 0 {
		request["max_tokens"] = in.MaxTokens
	}
	if in.Reasoning.Effort != "" {
		// Standard Chat Completions reasoning_effort extension. The server
		// only populates this for an explicitly configured model that
		// advertises the chosen effort; nonparticipating models omit it.
		request["reasoning_effort"] = in.Reasoning.Effort
	}
	if len(in.Tools) > 0 {
		out := make([]map[string]any, 0, len(in.Tools))
		for _, tool := range in.Tools {
			if tool["type"] != "function" {
				return nil, errors.New("unsupported Chat Completions tool type")
			}
			name, ok := tool["name"].(string)
			if !ok || !hasValidToolName(name) {
				return nil, errors.New("invalid function tool name")
			}
			parameters := tool["parameters"]
			if parameters == nil {
				parameters = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
			}
			out = append(out, map[string]any{"type": "function", "function": map[string]any{
				"name": name, "description": tool["description"], "parameters": parameters,
			}})
		}
		request["tools"] = out
		request["parallel_tool_calls"] = false
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if len(body) > maxRequestBytes {
		return nil, errors.New("Chat Completions request exceeds size limit")
	}
	return body, nil
}

// ParseChatCompletionsResult maps one completed OpenAI-compatible response to
// the same canonical assistant text/function-call representation as Responses.
func ParseChatCompletionsResult(body []byte) (ResponsesResult, error) {
	if len(body) == 0 || len(body) > maxRequestBytes {
		return ResponsesResult{}, errors.New("provider response missing or too large")
	}
	var raw struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   any `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ResponsesResult{}, fmt.Errorf("invalid Chat Completions JSON: %w", err)
	}
	if len(raw.Error) > 0 && string(raw.Error) != "null" {
		return ResponsesResult{}, errors.New("provider returned an API error")
	}
	if raw.ID == "" || len(raw.ID) > 256 || len(raw.Choices) != 1 {
		return ResponsesResult{}, errors.New("provider returned invalid Chat Completions response")
	}
	choice := raw.Choices[0]
	if choice.FinishReason != "stop" && choice.FinishReason != "tool_calls" {
		return ResponsesResult{}, fmt.Errorf("provider returned unsupported finish reason %q", choice.FinishReason)
	}
	text := ""
	switch content := choice.Message.Content.(type) {
	case string:
		text = content
	case nil:
		text = ""
	default:
		return ResponsesResult{}, errors.New("provider returned non-text assistant content")
	}
	result := ResponsesResult{ResponseID: raw.ID, Text: text, InputTokens: raw.Usage.PromptTokens, OutputTokens: raw.Usage.CompletionTokens, Status: "completed", FinishReason: choice.FinishReason, Model: raw.Model}
	seen := make(map[string]bool)
	for _, tc := range choice.Message.ToolCalls {
		if tc.Type != "function" || tc.ID == "" || len(tc.ID) > 256 || tc.Function.Name == "" || len(tc.Function.Name) > 128 || !json.Valid([]byte(tc.Function.Arguments)) || seen[tc.ID] {
			return ResponsesResult{}, errors.New("invalid or duplicate Chat Completions tool call")
		}
		seen[tc.ID] = true
		result.Calls = append(result.Calls, FunctionCall{CallID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	if choice.FinishReason == "tool_calls" && len(result.Calls) == 0 {
		return ResponsesResult{}, errors.New("tool_calls finish reason without tool calls")
	}
	if len(result.Calls) == 0 && strings.TrimSpace(result.Text) == "" {
		return ResponsesResult{}, errors.New("provider returned no text or tool calls")
	}
	return result, nil
}

// DoChatCompletions reuses the same hardened public-IP pinned transport as
// Responses. Streaming uses the separately bounded Chat Completions SSE parser.
func DoChatCompletions(ctx context.Context, p ProviderConfig, key string, canonicalBody []byte) (ResponsesResult, error) {
	client := newProviderClient(p)
	return doChatCompletionsWithClient(ctx, p, key, canonicalBody, client)
}

func doChatCompletionsWithClient(ctx context.Context, p ProviderConfig, key string, canonicalBody []byte, client *http.Client) (ResponsesResult, error) {
	if p.APIType != "chat_completions" {
		return ResponsesResult{}, &ProviderError{Code: "UNSUPPORTED_API_TYPE", Stage: "request_validation", Message: "Provider is not configured for Chat Completions"}
	}
	endpoint, err := p.APIURL("chat_completions")
	if err != nil {
		return ResponsesResult{}, classifyProviderPreflightError(err)
	}
	if key == "" || len(key) > 8192 {
		return ResponsesResult{}, classifyProviderPreflightError(errors.New("provider credential unavailable"))
	}
	body, err := BuildChatCompletionsRequest(canonicalBody)
	if err != nil {
		return ResponsesResult{}, err
	}
	req, err := newProviderRequest(ctx, endpoint, key, body)
	if err != nil {
		return ResponsesResult{}, err
	}
	RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.request", Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, RequestBody: body, RequestBytes: len(body)})
	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		code, message, ambiguous := classifyProviderTransportError(ctx, err)
		RecordProviderDiagnostic(ctx, DiagnosticEvent{Level: "error", Stage: "provider_http", Event: "http.error", Code: code, SafeMessage: message, Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, RequestBody: body, RequestBytes: len(body), Duration: time.Since(started)})
		return ResponsesResult{}, &ProviderError{Code: code, Stage: "provider_http", Message: message, Retryable: ambiguous, Ambiguous: ambiguous, BlockedSharedIP: blockedSharedIP(err)}
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRequestBytes+1))
	truncated := len(raw) > maxRequestBytes
	if truncated {
		raw = raw[:maxRequestBytes]
	}
	diagnostic := DiagnosticEvent{Level: "info", Stage: "provider_http", Event: "http.response", Endpoint: endpoint, APIType: p.APIType, ProviderID: p.ID, HTTPStatus: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), RetryAfter: resp.Header.Get("Retry-After"), ProviderRequestID: providerRequestID(resp.Header), ResponseBody: raw, RequestBytes: len(body), ResponseBytes: len(raw), RequestSHA256: BodyDigest(body), Truncated: truncated, Duration: time.Since(started)}
	if resp.StatusCode != http.StatusOK {
		pe := parseProviderHTTPError(resp.StatusCode, resp.Header.Get("Retry-After"), raw)
		diagnostic.Level = "error"
		diagnostic.Code = pe.Code
		diagnostic.SafeMessage = "Provider returned an HTTP error; inspect bounded diagnostics"
		RecordProviderDiagnostic(ctx, diagnostic)
		return ResponsesResult{}, pe
	}
	if readErr != nil {
		return ResponsesResult{}, &ProviderError{Code: "RESPONSE_READ_FAILED", Stage: "provider_http", Message: "Could not read provider response body", Ambiguous: true}
	}
	if truncated {
		return ResponsesResult{}, &ProviderError{Code: "RESPONSE_TOO_LARGE", Stage: "provider_http", Message: "Provider response exceeded the configured size limit"}
	}
	result, parseErr := ParseChatCompletionsResult(raw)
	if parseErr != nil {
		diagnostic.Level = "error"
		diagnostic.Stage = "provider_parse"
		diagnostic.Code = "JSON_PARSE_FAILED"
		diagnostic.SafeMessage = SafeProviderText(parseErr.Error())
		RecordProviderDiagnostic(ctx, diagnostic)
		return ResponsesResult{}, &ProviderError{Code: diagnostic.Code, Stage: diagnostic.Stage, Message: diagnostic.SafeMessage}
	}
	RecordProviderDiagnostic(ctx, diagnostic)
	return result, nil
}
