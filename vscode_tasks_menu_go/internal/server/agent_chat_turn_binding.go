package server

import (
	"errors"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// agentSessionAcceptsUserMessage reports whether a new Legacy user turn may start.
func agentSessionAcceptsUserMessage(status string) bool {
	switch status {
	case "created", "ready", "completed", "failed", "cancelled", "tool_rejected":
		return true
	default:
		return false
	}
}

// agentApplyNextTurnSelection updates session-level composer selection for the
// next admitted turn. It never mutates an active TurnBinding.
func agentApplyNextTurnSelection(x *agentchat.Session, providerID, modelID, effort string) error {
	if x == nil {
		return agentchat.ErrInvalidSession
	}
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	effort = strings.TrimSpace(effort)
	if providerID == "" && modelID == "" && effort == "" {
		return nil
	}
	if providerID != "" {
		x.ProviderID = providerID
	}
	if modelID != "" {
		x.ModelID = modelID
	}
	if effort != "" {
		x.ReasoningEffort = effort
	}
	return nil
}

// agentResolveTurnModel loads the configured provider/model for a message turn.
func (s *Server) agentResolveTurnModel(owner string, x *agentchat.Session, providerID, modelID, effort string) (agentchat.ProviderConfig, agentchat.ModelConfig, string, error) {
	if err := agentApplyNextTurnSelection(x, providerID, modelID, effort); err != nil {
		return agentchat.ProviderConfig{}, agentchat.ModelConfig{}, "", err
	}
	registry, err := s.agentChatProviders()
	if err != nil {
		return agentchat.ProviderConfig{}, agentchat.ModelConfig{}, "", err
	}
	p, err := registry.Get(owner, x.ProviderID)
	if err != nil {
		return agentchat.ProviderConfig{}, agentchat.ModelConfig{}, "", errors.New("Agent Responses provider not configured")
	}
	model, exists := p.FindModel(x.ModelID)
	if !exists {
		return agentchat.ProviderConfig{}, agentchat.ModelConfig{}, "", errors.New("Agent model is no longer configured")
	}
	resolvedEffort := agentProviderReasoningEffort(p, x.ReasoningEffort, model)
	if effort != "" {
		advertised := false
		for _, item := range model.ReasoningEfforts {
			if item == effort {
				advertised = true
				break
			}
		}
		if !advertised {
			return agentchat.ProviderConfig{}, agentchat.ModelConfig{}, "", errors.New("reasoning effort is not advertised by this model")
		}
		resolvedEffort = effort
		x.ReasoningEffort = effort
	}
	return p, model, resolvedEffort, nil
}

// agentPinActiveTurnBinding records immutable admission metadata for the turn.
func agentPinActiveTurnBinding(x *agentchat.Session, mode agentchat.ContextMode, p agentchat.ProviderConfig, model agentchat.ModelConfig, effort string, credential []byte, payload []byte, requestID string) error {
	if x == nil || x.ProviderRun == nil {
		return agentchat.ErrInvalidSession
	}
	binding, err := agentchat.BuildTurnBinding(x.ProviderRun.ID, x.ProviderRun.ID, requestID, mode, p, model, effort, credential, payload)
	if err != nil {
		return err
	}
	x.TurnBinding = &binding
	return nil
}

func agentClearActiveTurnBinding(x *agentchat.Session) {
	if x != nil {
		x.TurnBinding = nil
	}
}

func writeAgentQueuedTurn(w http.ResponseWriter, id string, revision uint64) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, map[string]any{
		"session_id": id, "revision": revision, "status": "queued",
		"queued": true, "execution_enabled": false,
	})
}
