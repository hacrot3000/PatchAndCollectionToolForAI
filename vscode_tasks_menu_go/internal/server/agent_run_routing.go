package server

import (
	"context"
	"errors"
	"net/http"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

type internalAgentRunRequestKey struct{}

func markInternalAgentRunRequest(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), internalAgentRunRequestKey{}, true))
}

func isInternalAgentRunRequest(r *http.Request) bool {
	internal, _ := r.Context().Value(internalAgentRunRequestKey{}).(bool)
	return internal
}

func (s *Server) agentRunForToolCall(owner, conversationID, callID string) (agentrun.Record, bool, error) {
	manager, err := s.agentRunManager(owner)
	if err != nil {
		return agentrun.Record{}, false, err
	}
	ownerID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		return agentrun.Record{}, false, err
	}
	var match *agentrun.Record
	for _, record := range manager.List() {
		if record.Spec.ConversationID != conversationID {
			continue
		}
		found := false
		for _, item := range record.History {
			if item.Type == "function_call" && item.CallID == callID {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		if record.Spec.ExecutionWorkspaceID != ownerID || record.Status == agentrun.WaitingTool {
			if match != nil {
				return agentrun.Record{}, false, errors.New("ambiguous Agent Run tool-call binding")
			}
			copy := record
			match = &copy
		}
	}
	if match == nil {
		return agentrun.Record{}, false, nil
	}
	return *match, true, nil
}

func (s *Server) conversationHasExecutionWorktree(owner, conversationID string) bool {
	manager, err := s.agentRunManager(owner)
	if err != nil {
		return false
	}
	ownerID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		return true
	}
	for _, record := range manager.List() {
		if record.Spec.ConversationID == conversationID && record.Spec.ExecutionWorkspaceID != ownerID {
			return true
		}
	}
	return false
}

func agentRunHasCall(record agentrun.Record, callID string) bool {
	for _, item := range record.History {
		if item.Type == "function_call" && item.CallID == callID && item.Name == "taskdeck_submit_patch" {
			return true
		}
	}
	return false
}
