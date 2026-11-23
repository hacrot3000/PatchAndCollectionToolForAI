package agentchat

import "errors"

var (
	ErrProviderCapabilityMismatch = errors.New("provider model cannot represent required run context")
	ErrProviderContextBudget      = errors.New("canonical provider payload exceeds the conservative model input budget")
)

type ProviderPromptOptions interface {
	Options(RunSpec, ProviderConfig, ModelConfig) (RequestConfig, error)
}

type ProviderHistoryVerifier interface {
	VerifyHistory(RunSpec, []HistoryItem) error
}

// CanonicalProviderPayloadBuilder converts an immutable canonical snapshot to
// a provider-specific request. It deliberately omits opaque provider session
// references; implementations may add them only after proving matching lineage.
type CanonicalProviderPayloadBuilder struct {
	Options ProviderPromptOptions
}

// ToolSupport describes which provider tools a pinned run may advertise.
type ToolSupport struct {
	// PatchTool is true only when the model has verified tool-calling evidence
	// and the run's workspace binding grants approval-controlled Patch.
	PatchTool bool
	// CollectionTool is true only when the run's workspace binding grants
	// read-only workspace inspection.
	CollectionTool bool
}

type ProviderToolSupportProvider interface {
	ToolSupport(RunSpec, ProviderConfig, ModelConfig) (ToolSupport, error)
}

// ToolAwarePromptOptions is an optional extension of ProviderPromptOptions. It
// lets the server decide whether a pinned run may advertise TaskDeck tools.
type ToolAwarePromptOptions interface {
	ProviderPromptOptions
	ProviderToolSupportProvider
}

// ProviderToolSchema advertises the TaskDeck tool surface for a run. The
// provider adapter never executes a tool; it only announces the schema and
// normalizes the returned function call.
func (b CanonicalProviderPayloadBuilder) ToolSupport(spec RunSpec, provider ProviderConfig, model ModelConfig) (ToolSupport, error) {
	aware, ok := b.Options.(ToolAwarePromptOptions)
	if !ok {
		return ToolSupport{}, nil
	}
	return aware.ToolSupport(spec, provider, model)
}

func (b CanonicalProviderPayloadBuilder) Build(spec RunSpec, provider ProviderConfig, model ModelConfig, history []HistoryItem) ([]byte, error) {
	if b.Options == nil {
		return nil, errors.New("provider prompt options are unavailable")
	}
	if provider.ID != spec.ProviderID || model.ID != spec.ModelID {
		return nil, ErrProviderBindingMismatch
	}
	if verifier, ok := b.Options.(ProviderHistoryVerifier); ok {
		if err := verifier.VerifyHistory(spec, history); err != nil {
			return nil, err
		}
	}
	options, err := b.Options.Options(spec, provider, model)
	if err != nil {
		return nil, err
	}
	tools, err := b.ToolSupport(spec, provider, model)
	if err != nil {
		return nil, err
	}
	options.EnablePatchTool = tools.PatchTool
	options.EnableCollectionTool = tools.CollectionTool
	if !model.ToolCalling && (options.EnablePatchTool || options.EnableCollectionTool || historyRequiresTools(history) || historyRequiresTools(options.Next)) {
		return nil, ErrProviderCapabilityMismatch
	}
	options.Model = model.ID
	if len(options.Next) == 0 {
		// The canonical snapshot ends with the run's submitted user turn. After a
		// tool continuation it ends with the tool output, so the user turn that
		// started this attempt is located by its pinned parent turn.
		userTurn := -1
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Type == "user" && (spec.ParentTurnID == "" || history[i].ParentTurnID == spec.ParentTurnID) {
				userTurn = i
				break
			}
		}
		if userTurn < 0 {
			return nil, errors.New("run snapshot does not contain its submitted user turn")
		}
		options.History = append([]HistoryItem(nil), history[:userTurn]...)
		options.Next = []HistoryItem{history[userTurn]}
	} else {
		options.History = append([]HistoryItem(nil), history...)
	}
	options.Mode = Stateless
	options.PreviousResponseID = ""
	options.ConversationID = ""
	canonical, err := BuildResponsesRequest(options)
	if err != nil {
		return nil, err
	}
	var payload []byte
	if provider.APIType == "chat_completions" {
		payload, err = BuildChatCompletionsRequest(canonical)
	} else if provider.APIType == "responses" {
		payload = canonical
	} else {
		return nil, errors.New("unsupported provider API type")
	}
	if err != nil {
		return nil, err
	}
	// Serialized bytes are a conservative upper bound, not an exact tokenizer.
	if model.MaxInputTokens > 0 && len(payload) > model.MaxInputTokens {
		return nil, ErrProviderContextBudget
	}
	return payload, nil
}

func historyRequiresTools(history []HistoryItem) bool {
	for _, item := range history {
		if item.Type == "function_call" || item.Type == "function_call_output" {
			return true
		}
	}
	return false
}
