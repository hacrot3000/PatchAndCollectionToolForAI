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
)

const (
	maxChatSSEEventBytes = 256 << 10
	maxChatSSETotalBytes = 8 << 20
	maxChatSSEEvents     = 20000
)

type chatStreamAccumulator struct {
	responseID string
	text       strings.Builder
	calls      []FunctionCall
	callIndex  map[int]int
	args       []strings.Builder
	events     int
	totalBytes int
	finished   bool
	finish     string
	input      int
	output     int
}

func newChatStreamAccumulator() *chatStreamAccumulator {
	return &chatStreamAccumulator{callIndex: make(map[int]int)}
}

func (a *chatStreamAccumulator) apply(payload []byte) error {
	if string(payload) == "[DONE]" {
		a.finished = true
		return nil
	}
	a.events++
	if a.events > maxChatSSEEvents || len(payload) > maxChatSSEEventBytes {
		return errors.New("Chat Completions stream exceeded event limit")
	}
	var chunk struct {
		ID      string `json:"id"`
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
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			if a.text.Len()+len(choice.Delta.Content) > maxSSETextBytes {
				return errors.New("Chat Completions stream text exceeded limit")
			}
			a.text.WriteString(choice.Delta.Content)
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
	}
	return nil
}

func (a *chatStreamAccumulator) result() (ResponsesResult, error) {
	if !a.finished {
		return ResponsesResult{}, errors.New("Chat Completions stream ended before completion")
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
	return ResponsesResult{ResponseID: a.responseID, Text: a.text.String(), Calls: a.calls, InputTokens: a.input, OutputTokens: a.output, Status: "completed"}, nil
}

// DoChatCompletionsStream sends a stateless Chat Completions request through
// the shared hardened transport and accepts only a complete SSE response.
func DoChatCompletionsStream(ctx context.Context, p ProviderConfig, key string, canonicalBody []byte) (ResponsesResult, error) {
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
	if resp.StatusCode != http.StatusOK {
		return ResponsesResult{}, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	if !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return ResponsesResult{}, errors.New("provider did not return an event stream")
	}
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
		return acc.apply([]byte(payload))
	}
	for {
		line, readErr := reader.ReadString('\n')
		acc.totalBytes += len(line)
		if acc.totalBytes > maxChatSSETotalBytes {
			return ResponsesResult{}, errors.New("Chat Completions stream exceeded size limit")
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
			if acc.finished {
				return acc.result()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				if err := dispatch(); err != nil {
					return ResponsesResult{}, err
				}
				if acc.finished {
					return acc.result()
				}
				return ResponsesResult{}, errors.New("Chat Completions stream ended before completion")
			}
			if ctx.Err() != nil {
				return ResponsesResult{}, ctx.Err()
			}
			return ResponsesResult{}, errors.New("cannot read Chat Completions stream")
		}
	}
}
