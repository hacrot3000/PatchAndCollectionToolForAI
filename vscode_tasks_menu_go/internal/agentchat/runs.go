package agentchat

import (
	"context"
	"errors"
	"strings"
)

// RunSpec is the immutable provider binding captured when a request is sent.
// It intentionally contains identifiers only, never credentials.
type RunSpec struct {
	RunID                 string    `json:"run_id"`
	ConversationID        string    `json:"conversation_id"`
	ParentTurnID          string    `json:"parent_turn_id,omitempty"`
	OwnerWorkspaceID      string    `json:"owner_workspace_id"`
	ExecutionWorkspaceID  string    `json:"execution_workspace_id"`
	ProviderID            string    `json:"provider_id"`
	ModelID               string    `json:"model_id"`
	ProviderConfigVersion string    `json:"provider_config_version"`
	CredentialFingerprint string    `json:"credential_fingerprint,omitempty"`
	ContextSnapshotID     string    `json:"context_snapshot_id"`
	ExcludedPendingRunIDs string    `json:"excluded_pending_run_ids,omitempty"`
	AttemptID             string    `json:"attempt_id"`
	IdempotencyKey        string    `json:"idempotency_key"`
	RetryMode             RetryMode `json:"retry_mode,omitempty"`
}

func (s RunSpec) Validate() error {
	for _, value := range []string{s.RunID, s.ConversationID, s.OwnerWorkspaceID, s.ExecutionWorkspaceID,
		s.ProviderID, s.ModelID, s.ProviderConfigVersion, s.ContextSnapshotID, s.AttemptID, s.IdempotencyKey} {
		if strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return errors.New("invalid or missing run binding field")
		}
	}
	if len(s.ParentTurnID) > 256 || strings.ContainsAny(s.ParentTurnID, "\x00\r\n") {
		return errors.New("invalid parent turn ID")
	}
	if len(s.ExcludedPendingRunIDs) > 1300 {
		return errors.New("excluded pending run IDs exceed limit")
	}
	if s.ExcludedPendingRunIDs != "" {
		ids := strings.Split(s.ExcludedPendingRunIDs, ",")
		if len(ids) > 5 {
			return errors.New("too many excluded pending run IDs")
		}
		seenExcluded := make(map[string]bool, len(ids))
		for _, runID := range ids {
			if strings.TrimSpace(runID) == "" || len(runID) > 256 || strings.ContainsAny(runID, "\x00\r\n") || seenExcluded[runID] {
				return errors.New("invalid excluded pending run ID")
			}
			seenExcluded[runID] = true
		}
	}
	if s.CredentialFingerprint != "" && (len(s.CredentialFingerprint) != 64 || strings.Trim(s.CredentialFingerprint, "0123456789abcdef") != "") {
		return errors.New("invalid provider credential fingerprint")
	}
	if s.RetryMode != "" && s.RetryMode != RetryOff && s.RetryMode != RetryLimited && s.RetryMode != RetryUnlimited {
		return errors.New("invalid provider retry mode")
	}
	return nil
}

// ValidateHistoryItemForRun exposes the canonical-history validator to the
// run manager while keeping its implementation private to Agent Chat.
func ValidateHistoryItemForRun(item HistoryItem) error {
	if err := validHistoryItem(item); err != nil {
		return err
	}
	for _, value := range []string{item.OriginRunID, item.ProviderID, item.ModelID, item.ParentTurnID, item.ExcludedPendingRunIDs} {
		if len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return errors.New("invalid history provenance")
		}
	}
	return nil
}

type RunEventType string

const (
	RunEventTextDelta     RunEventType = "assistant.delta"
	RunEventToolCallDelta RunEventType = "tool.call.delta"
	RunEventToolCall      RunEventType = "tool.call"
	RunEventToolResult    RunEventType = "tool.result"
	RunEventUsage         RunEventType = "usage"
	RunEventRetry         RunEventType = "provider.retry"
	RunEventError         RunEventType = "error"
	RunEventTerminal      RunEventType = "terminal"
)

// RunEvent is normalized and correlated to the originating immutable run.
type RunEvent struct {
	RunID      string       `json:"run_id"`
	AttemptID  string       `json:"attempt_id"`
	ProviderID string       `json:"provider_id"`
	Sequence   uint64       `json:"seq"`
	Type       RunEventType `json:"type"`
	Payload    []byte       `json:"payload,omitempty"`
}

type EventSink interface {
	Publish(context.Context, RunEvent) error
}

// ProviderAdapter binds invocation and cancellation to the run spec; adapters
// must not consult mutable UI selection state.
type ProviderAdapter interface {
	Execute(context.Context, RunSpec, []HistoryItem, EventSink) error
	Cancel(context.Context, RunSpec) error
}
