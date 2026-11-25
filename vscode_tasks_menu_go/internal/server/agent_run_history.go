package server

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func verifyAgentRunSnapshotBinding(spec agentchat.RunSpec, snapshot agentrun.ContextSnapshot) error {
	if snapshot.Policy == nil || snapshot.Policy.RunBinding == nil {
		return errors.New("context snapshot does not contain an immutable run binding")
	}
	binding := snapshot.Policy.RunBinding
	if binding.RunID != spec.RunID || binding.ConversationID != spec.ConversationID ||
		binding.OwnerWorkspaceID != spec.OwnerWorkspaceID || binding.ExecutionWorkspaceID != spec.ExecutionWorkspaceID ||
		binding.ParentTurnID != spec.ParentTurnID || binding.ProviderID != spec.ProviderID || binding.ModelID != spec.ModelID ||
		binding.ProviderConfigVersion != spec.ProviderConfigVersion || binding.CredentialFingerprint != spec.CredentialFingerprint ||
		binding.ExcludedPendingRunIDs != spec.ExcludedPendingRunIDs || binding.RetryMode != spec.RetryMode {
		return errors.New("run spec differs from its immutable snapshot binding")
	}
	return nil
}

func completedAgentRunHistory(base []agentchat.HistoryItem, records []agentrun.Record, conversationID string) []agentchat.HistoryItem {
	included := make(map[string]bool)
	for _, item := range base {
		if item.OriginRunID != "" {
			included[item.OriginRunID] = true
		}
	}
	conversationRuns := make([]agentrun.Record, 0, len(records))
	for _, record := range records {
		if record.Spec.ConversationID == conversationID && !included[record.Spec.RunID] {
			conversationRuns = append(conversationRuns, record)
		}
	}
	sort.Slice(conversationRuns, func(i, j int) bool {
		if conversationRuns[i].CreatedAt.Equal(conversationRuns[j].CreatedAt) {
			return conversationRuns[i].Spec.RunID < conversationRuns[j].Spec.RunID
		}
		return conversationRuns[i].CreatedAt.Before(conversationRuns[j].CreatedAt)
	})
	out := append([]agentchat.HistoryItem(nil), base...)
	for _, record := range conversationRuns {
		var prompt string
		// A server-generated auto-continuation is not the original user
		// request. Preserve the first submitted turn for this run lineage.
		for _, item := range record.History {
			if item.Type == "user" && item.ParentTurnID == record.Spec.ParentTurnID && item.OriginRunID != record.Spec.RunID {
				prompt = item.Content
				break
			}
		}
		if prompt == "" {
			for i := len(record.History) - 1; i >= 0; i-- {
				if record.History[i].Type == "user" {
					prompt = record.History[i].Content
					break
				}
			}
		}
		if prompt == "" {
			continue
		}
		spec := record.Spec
		out = append(out, agentchat.HistoryItem{
			Type: "user", Content: prompt, OriginRunID: spec.RunID,
			ProviderID: spec.ProviderID, ModelID: spec.ModelID, ParentTurnID: spec.ParentTurnID,
		})
		if record.Status != agentrun.Completed {
			if record.Status == agentrun.Failed || record.Status == agentrun.Stopped || record.Status == agentrun.Interrupted {
				out = append(out, agentchat.HistoryItem{
					Type: "assistant", Content: "The prior Agent Run ended as " + string(record.Status) + " without a confirmed final response. Do not assume it changed the workspace.",
					OriginRunID: spec.RunID, ProviderID: spec.ProviderID, ModelID: spec.ModelID, ParentTurnID: spec.ParentTurnID,
				})
			}
			included[spec.RunID] = true
			continue
		}
		var response strings.Builder
		toolRequest := false
		attemptID := ""
		for _, event := range record.Events {
			if event.AttemptID != "" && event.AttemptID != attemptID {
				response.Reset()
				attemptID = event.AttemptID
			}
			switch event.Type {
			case agentchat.RunEventTextDelta:
				response.Write(event.Payload)
			case agentchat.RunEventRetry:
				var retry struct {
					ResetPartial bool `json:"reset_partial_output"`
				}
				if json.Unmarshal(event.Payload, &retry) == nil && retry.ResetPartial {
					response.Reset()
				}
			case agentchat.RunEventToolCall:
				toolRequest = true
			}
		}
		answer := response.String()
		if answer == "" && toolRequest {
			answer = "A tool request was recorded for this run but was not executed; do not assume it changed the workspace. Review the run card before continuing."
		}
		if answer != "" {
			out = append(out, agentchat.HistoryItem{
				Type: "assistant", Content: answer, OriginRunID: spec.RunID,
				ProviderID: spec.ProviderID, ModelID: spec.ModelID, ParentTurnID: spec.ParentTurnID,
			})
		}
		included[spec.RunID] = true
	}
	return out
}

func latestAgentRunID(records []agentrun.Record, conversationID string) string {
	var latest *agentrun.Record
	for i := range records {
		if records[i].Spec.ConversationID != conversationID {
			continue
		}
		if latest == nil || records[i].CreatedAt.After(latest.CreatedAt) ||
			(records[i].CreatedAt.Equal(latest.CreatedAt) && records[i].Spec.RunID > latest.Spec.RunID) {
			latest = &records[i]
		}
	}
	if latest == nil {
		return ""
	}
	return latest.Spec.RunID
}

func (s *Server) completedAgentRuns(owner, conversationID string) ([]agentrun.Record, error) {
	if s == nil {
		return nil, nil
	}
	s.agentRunManagersMu.Lock()
	manager := s.agentRunManagers[owner]
	s.agentRunManagersMu.Unlock()
	var records []agentrun.Record
	if manager != nil {
		records = manager.List()
	} else {
		runDir, err := s.agentRunJournalDir(owner)
		if err != nil {
			return nil, err
		}
		records, err = agentrun.ReadRecords(runDir)
		if err != nil {
			return nil, err
		}
	}
	filtered := make([]agentrun.Record, 0, len(records))
	for _, record := range records {
		if record.Spec.ConversationID == conversationID {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}
