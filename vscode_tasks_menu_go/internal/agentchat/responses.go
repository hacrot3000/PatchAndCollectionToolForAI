// Package agentchat contains the provider-independent, non-executing building
// blocks for TaskDeck Agent Chat. No function in this package executes tools.
package agentchat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ContextMode defines how the provider retains previous turns. A provider
// advertising Responses compatibility does not necessarily support state.
type ContextMode string

const (
	Stateless        ContextMode = "stateless"
	PreviousResponse ContextMode = "previous_response"
	Conversation     ContextMode = "conversation"
)

// HistoryItem is the canonical replay format, including tool calls/results.
// For a normal message, Type is "user" or "assistant", and Content is text.
// For a tool call, Type is "function_call" and CallID/Name/Arguments are set.
// For a tool result, Type is "function_call_output" and CallID/Content are set.
type HistoryItem struct {
	Type                  string `json:"type"`
	Content               string `json:"content,omitempty"`
	CallID                string `json:"call_id,omitempty"`
	Name                  string `json:"name,omitempty"`
	Arguments             string `json:"arguments,omitempty"`
	OriginRunID           string `json:"origin_run_id,omitempty"`
	ProviderID            string `json:"provider_id,omitempty"`
	ModelID               string `json:"model_id,omitempty"`
	ParentTurnID          string `json:"parent_turn_id,omitempty"`
	ExcludedPendingRunIDs string `json:"excluded_pending_run_ids,omitempty"`
	RequestEffort string `json:"request_effort,omitempty"`
	RequestContext string `json:"request_context,omitempty"`
	RequestAt string `json:"request_at,omitempty"`
	RequestInputTokens int `json:"request_input_tokens,omitempty"`
}

type RequestConfig struct {
	Model                string
	PromptCacheKey string
	PromptCacheRetention string
	PromptCacheTTL string
	Instructions         string
	CurrentInstructions string // trusted, mutable server guidance; appended after stable native contract
	Mode                 ContextMode
	PreviousResponseID   string
	ConversationID       string
	History              []HistoryItem
	Next                 []HistoryItem
	MaxOutputTokens      int
	ReasoningEffort      string
	EnablePatchTool      bool // gated, approval-required native Patch function
	EnableCollectionTool bool // bounded, read-only project inspection
	EnableAskUserTool bool // explicitly interactive legacy conversation tool
	NativeCollectionContract string // current installed native schema document, advisory only
	// ProbeToolName advertises a single harmless function with no required
	// arguments. It exists only for a user-triggered capability probe and can
	// never dispatch a Patch or any other side effect.
	ProbeToolName string
}

const (
	maxInstructionsBytes = 1 << 20
	maxItemBytes         = 512 << 10
	maxItems             = 256
	maxRequestBytes      = 4 << 20
)

func validHistoryItem(item HistoryItem) error {
	if len(item.Content) > maxItemBytes || len(item.Arguments) > maxItemBytes {
		return errors.New("history item exceeds limit")
	}
	switch item.Type {
	case "user", "assistant":
		if item.Content == "" || item.CallID != "" || item.Name != "" || item.Arguments != "" {
			return errors.New("invalid text message")
		}
	case "function_call":
		if item.CallID == "" || item.Name == "" || item.Arguments == "" || item.Content != "" {
			return errors.New("invalid function call")
		}
		if !json.Valid([]byte(item.Arguments)) {
			return errors.New("invalid function call arguments")
		}
	case "function_call_output":
		if item.CallID == "" || item.Content == "" || item.Name != "" || item.Arguments != "" {
			return errors.New("invalid function call result")
		}
	default:
		return errors.New("unknown history item type")
	}
	if len(item.CallID) > 256 || len(item.Name) > 128 {
		return errors.New("history item ID exceeds limit")
	}
	return nil
}

func encodeHistory(items []HistoryItem) ([]map[string]string, error) {
	if len(items) > maxItems {
		return nil, errors.New("too many history items")
	}
	out := make([]map[string]string, 0, len(items))
	for _, item := range items {
		if err := validHistoryItem(item); err != nil {
			return nil, err
		}
		switch item.Type {
		case "user", "assistant":
			out = append(out, map[string]string{"role": item.Type, "content": item.Content})
		case "function_call":
			out = append(out, map[string]string{"type": "function_call", "call_id": item.CallID, "name": item.Name, "arguments": item.Arguments})
		case "function_call_output":
			out = append(out, map[string]string{"type": "function_call_output", "call_id": item.CallID, "output": item.Content})
		}
	}
	return out, nil
}

// validateStatelessToolPairs rejects malformed provider-facing history
// BEFORE any HTTP call. It never invents a Patch/COLLECT result and never
// modifies the canonical journal. A previous_response_id/conversation may
// legitimately contain the matching call server-side, so validate only full
// stateless replays.
func validateStatelessToolPairs(items []HistoryItem) error {
 calls:=map[string]bool{}
 outputs:=map[string]bool{}
 for _,item:=range items {
  switch item.Type {
  case "function_call":
   if calls[item.CallID] {return fmt.Errorf("duplicate stateless function call %q",item.CallID)}
   calls[item.CallID]=true
  case "function_call_output":
   if !calls[item.CallID] {return fmt.Errorf("stateless tool output %q has no matching function call",item.CallID)}
   if outputs[item.CallID] {return fmt.Errorf("duplicate stateless tool output %q",item.CallID)}
   outputs[item.CallID]=true
  }
 }
 for id:=range calls{
  if !outputs[id]{return fmt.Errorf("stateless function call %q has no tool output",id)}
 }
 return nil
}

// BuildResponsesRequest builds a Responses API JSON request. Required
// instructions are sent EVERY time, including when previous_response_id is
// used (top-level instructions are not inherited in response chains).
//
// In the absence of a verified reference, stateful modes fail closed by
// replaying local history rather than assuming the provider remembers it.
func BuildResponsesRequest(c RequestConfig) ([]byte, error) {
	if strings.TrimSpace(c.Instructions)=="" {return nil,errors.New("mandatory instructions required")}
	var collectionParameters map[string]any
	if c.EnableCollectionTool || c.NativeCollectionContract != "" {
		var guidance string
		collectionParameters, guidance = collectionToolContract(c.NativeCollectionContract)
		c.Instructions += "\n\n" + guidance
	}
	c.Instructions += c.CurrentInstructions
	if strings.TrimSpace(c.Model) == "" || len(c.Model) > 128 {
		return nil, errors.New("invalid model")
	}
	if strings.TrimSpace(c.Instructions) == "" || len(c.Instructions) > maxInstructionsBytes {
		return nil, errors.New("mandatory instructions required or too large")
	}
	if len(c.Next) == 0 {
		return nil, errors.New("next message or tool result required")
	}
	if len(c.Next) > maxItems {
		return nil, errors.New("too many next input items")
	}
	if c.MaxOutputTokens < 0 || c.MaxOutputTokens > 200000 {
		return nil, errors.New("invalid output token limit")
	}
	if c.ReasoningEffort != "" {
		switch c.ReasoningEffort {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return nil, errors.New("unsupported reasoning effort value")
		}
	}
	if c.Mode != Stateless && c.Mode != PreviousResponse && c.Mode != Conversation {
		return nil, errors.New("unsupported context mode")
	}
	// Input uses only the current delta when a verified server-side reference
	// is available. Never mix conversation and previous_response_id.
	hasPrevious := c.Mode == PreviousResponse && c.PreviousResponseID != ""
	hasConversation := c.Mode == Conversation && c.ConversationID != ""
	items := c.Next
	if !hasPrevious && !hasConversation {
		items = append(append([]HistoryItem{}, c.History...), c.Next...)
		if err:=validateStatelessToolPairs(items);err!=nil {
			return nil, fmt.Errorf("invalid stateless provider context: %w",err)
		}
	}
	input, err := encodeHistory(items)
	if err != nil {
		return nil, err
	}
	req := map[string]any{
		"model":        c.Model,
		"instructions": c.Instructions,
		"input":        input,
	}
	if hasPrevious {
		req["previous_response_id"] = c.PreviousResponseID
		req["store"] = true
	} else if hasConversation {
		req["conversation"] = c.ConversationID
	} else if c.Mode == PreviousResponse {
		// Retain the first response so that it can be referenced next turn.
		req["store"] = true
	} else {
		req["store"] = false
	}
	if !validPromptCacheLifetime(c.PromptCacheRetention,c.PromptCacheTTL){return nil,errors.New("invalid prompt cache lifetime")}
	if c.PromptCacheRetention!=""{req["prompt_cache_retention"]=c.PromptCacheRetention}
	if c.PromptCacheTTL!=""{req["prompt_cache_options"]=map[string]string{"ttl":c.PromptCacheTTL}}
	if c.PromptCacheKey!="" {
		if !validAffinityKey(c.PromptCacheKey){return nil,errors.New("invalid prompt cache key")}
		req["prompt_cache_key"]=c.PromptCacheKey
	}
	if c.MaxOutputTokens > 0 {
		req["max_output_tokens"] = c.MaxOutputTokens
	}
	if c.ReasoningEffort != "" {
		req["reasoning"] = map[string]string{"effort": c.ReasoningEffort}
	}
	if c.ProbeToolName != "" {
		if !hasValidToolName(c.ProbeToolName) {
			return nil, errors.New("invalid probe tool name")
		}
		req["tools"] = []any{map[string]any{
			"type":        "function",
			"name":        c.ProbeToolName,
			"description": "Harmless TaskDeck capability probe. Do not call it. Reply with a short text answer.",
			"strict":      true,
			"parameters": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"required":             []string{},
				"additionalProperties": false,
			},
		}}
		req["parallel_tool_calls"] = false
	}
	if c.EnableCollectionTool {
		// This is only a permissive transport schema. The EXISTING Python
		// Patch Tool COLLECT_ACTION_SCHEMA.json validates every native action,
		// limit and exact project identity before any source inspection.
		// Never reintroduce a second hard-coded Go Collection action parser.
		req["tools"] = []any{map[string]any{
			"type": "function", "name": "taskdeck_collect_workspace",
			"description": "Run the SAME native Python Patch Tool COLLECT as its manual Queue/Paste text workflow. Use current COLLECT_ACTION_SCHEMA.json. Actions include overview, find, search, file, range, head, tail, symbol, references, dependencies, directory, decompile, ida, ghidra, database_select, git, command and pack. For a 150 MB decompile/source dump prefer targeted decompile/address/symbol/search instead of pack. Tool produces ZIP + TXT, and INCOMPLETE rc=3 retains usable evidence; never assume missing matches prove absence. Do not print Collection JSON as chat text.",
			"strict": false,
			"parameters": collectionParameters,
		}}
		req["parallel_tool_calls"]=false
	}
	if c.EnablePatchTool {
		tools, _ := req["tools"].([]any)
		req["tools"] = append(tools, map[string]any{
			"type":        "function",
			"name":        "taskdeck_submit_patch",
			"description": "Submit a Python Patch OR run an eligible existing project-local build/test/diagnostic script via native command-only PATCH. Prefer the command-only lane to asking the user to run an eligible script manually. Set exactly one of patch_python and patch_manifest nonempty; set the other to an empty string. A command-only manifest contains one structured argv command (for example bash scripts/check.sh), never inline shell. Native validation, project identity, script origin and approval gates still apply. Never claim submission means execution.",
			"strict":      true,
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":         map[string]string{"type": "string", "description": "Short descriptive patch name"},
					"patch_python": map[string]string{"type": "string", "description": "Complete TaskDeck Python patch with identity headers, or empty when submitting a command-only manifest"},
					"patch_manifest": map[string]string{"type": "string", "description": "JSON string of a native command-only PATCH_TOOL_MANIFEST.json, or empty for Python. Exactly one existing project-local command, legacy_strict, no payload or predecessor override."},
				},
				"required":             []string{"name", "patch_python", "patch_manifest"},
				"additionalProperties": false,
			},
		})
		req["parallel_tool_calls"] = false
	}
	if c.EnableAskUserTool {
		tools,_:=req["tools"].([]any)
		req["tools"]=append(tools,AskUserToolSchema())
		req["parallel_tool_calls"]=false
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if len(b) > maxRequestBytes {
		return nil, errors.New("AI request exceeds 4 MiB context limit")
	}
	return b, nil
}

type FunctionCall struct {
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ResponsesResult struct {
	RequestDisplay RequestDisplay // trusted local request metadata; never parsed from provider output
	ResponseID     string
	ConversationID string
	Text           string
	Calls          []FunctionCall
	InputTokens    int
	CacheUsage ProviderCacheUsage
	OutputTokens   int
	Status         string
	FinishReason   string
	Model          string
}

// ErrEmptyProviderResponse identifies a parsed response with no usable
// assistant output. It is not a successful turn or a native tool result.
var ErrEmptyProviderResponse = errors.New("provider returned no text or tool calls")

// ParseResponsesResult extracts text and function_call items without accepting
// arbitrary output text as an executable tool call.
func ParseResponsesResult(body []byte) (ResponsesResult, error) {
	if len(body) == 0 || len(body) > maxRequestBytes {
		return ResponsesResult{}, errors.New("provider response missing or too large")
	}
	var raw struct {
		ID           string          `json:"id"`
		Status       string          `json:"status"`
		Conversation json.RawMessage `json:"conversation"`
		Output       []struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			CacheUsageFields
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ResponsesResult{}, fmt.Errorf("invalid Responses JSON: %w", err)
	}
	if raw.ID == "" || len(raw.ID) > 256 {
		return ResponsesResult{}, errors.New("response missing valid ID")
	}
	if raw.Status != "" && raw.Status != "completed" {
		return ResponsesResult{}, fmt.Errorf("provider returned status %q", raw.Status)
	}
	res := ResponsesResult{ResponseID: raw.ID, Status: raw.Status, InputTokens: raw.Usage.InputTokens, OutputTokens: raw.Usage.OutputTokens, CacheUsage:raw.Usage.CacheUsageFields.normalized(raw.Usage.InputTokens)}
	if len(raw.Conversation) != 0 && string(raw.Conversation) != "null" {
		if raw.Conversation[0] == '"' {
			if err := json.Unmarshal(raw.Conversation, &res.ConversationID); err != nil {
				return ResponsesResult{}, errors.New("invalid conversation ID")
			}
		} else {
			var ref struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(raw.Conversation, &ref); err != nil {
				return ResponsesResult{}, errors.New("invalid conversation")
			}
			res.ConversationID = ref.ID
		}
		if len(res.ConversationID) > 256 {
			return ResponsesResult{}, errors.New("conversation ID too large")
		}
	}
	seen := make(map[string]bool)
	var textParts []string
	for _, item := range raw.Output {
		switch item.Type {
		case "message":
			for _, segment := range item.Content {
				if segment.Type == "output_text" && segment.Text != "" {
					textParts = append(textParts, segment.Text)
				}
			}
		case "function_call":
			if item.CallID == "" || len(item.CallID) > 256 || item.Name == "" || len(item.Name) > 128 || !json.Valid([]byte(item.Arguments)) || seen[item.CallID] {
				return ResponsesResult{}, errors.New("invalid or duplicate function_call")
			}
			seen[item.CallID] = true
			res.Calls = append(res.Calls, FunctionCall{CallID: item.CallID, Name: item.Name, Arguments: item.Arguments})
		}
	}
	res.Text = strings.Join(textParts, "\n")
	if len(res.Calls) == 0 && strings.TrimSpace(res.Text) == "" {
		return ResponsesResult{}, ErrEmptyProviderResponse
	}
	return res, nil
}
