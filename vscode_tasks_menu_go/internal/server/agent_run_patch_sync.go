package server

import (
	"context"
	"errors"
	"reflect"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func (s *Server) syncAgentRunConversation(owner string, run agentrun.Record, executionWorkspace string) (agentchat.Session, error) {
	ownerWorkspace := s.agentOwnerWorkspace()
	store, err := s.agentChatStore()
	if err != nil {
		return agentchat.Session{}, err
	}
	session, err := store.Get(run.Spec.ConversationID, owner)
	if err != nil {
		return agentchat.Session{}, err
	}
	identity, err := patchtool.ReadProjectIdentity(ownerWorkspace)
	if err != nil || session.Workspace != ownerWorkspace || session.ProjectID != identity.ID {
		return agentchat.Session{}, errors.New("run conversation owner binding mismatch")
	}
	executionIdentity, err := patchtool.ReadProjectIdentity(executionWorkspace)
	if err != nil || executionIdentity.ID != session.ProjectID {
		return agentchat.Session{}, errors.New("execution workspace project identity mismatch")
	}
	// F03: a late sync from a stale run must not hide history already recorded
	// from a newer sibling run, nor mirror an older status over it.
	if agentRunHistorySnapshotIsStale(session.History, run.History, run.Spec.RunID) {
		return agentchat.Session{}, errors.New("run history is stale relative to current conversation history")
	}
	// The run history may include already-reconciled Collection calls and
	// historical calls from other turns. Only its current pending Patch
	// calls may create approval records.
	resolved := map[string]bool{}
	for _, item := range run.History {
		if item.Type == "function_call_output" { resolved[item.CallID] = true }
	}
	changed := !reflect.DeepEqual(session.History, run.History) || session.Status != string(run.Status)
	session.History = append([]agentchat.HistoryItem(nil), run.History...)
	session.Status = string(run.Status)
	for _, item := range run.History {
		if item.Type != "function_call" || resolved[item.CallID] {
			continue
		}
		if item.Name != "taskdeck_submit_patch" {
			return agentchat.Session{}, errors.New("run proposed a tool without an execution bridge")
		}
		exists := false
		for _, current := range session.ToolCalls {
			if current.CallID == item.CallID {
				if current.AgentRunID != run.Spec.RunID || current.Name != item.Name ||
					current.ArgumentsSHA256 != agentPatchPayloadSHA256(item.Arguments) {
					return agentchat.Session{}, errors.New("native tool call ID belongs to a different or unverified run")
				}
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		if _, err := parseAgentPatchArguments(item.Arguments); err != nil {
			return agentchat.Session{}, err
		}
		snapshot, err := agentWorkspaceSnapshotOf(context.Background(), executionWorkspace)
		if err != nil {
			return agentchat.Session{}, err
		}
		expires := time.Now().UTC().Add(agentApprovalTTL)
		session.ToolCalls = append(session.ToolCalls, agentchat.ToolCallRecord{
			CallID: item.CallID, AgentRunID: run.Spec.RunID, Name: "taskdeck_submit_patch", Status: "pending",
			ArgumentsSHA256: agentPatchPayloadSHA256(item.Arguments), ExpectedHead: snapshot.Head,
			ExpectedBranch: snapshot.Branch, WorkspaceStatusSHA256: snapshot.StatusSHA256,
			GitIsRepo: snapshot.IsRepo, ApprovalExpiresAt: &expires, ManualRetryOnly: true,
		})
		session.AppendEvent("tool_proposed", "waiting_tool", item.CallID, "agent_run")
		changed = true
	}
	if !changed {
		return session, nil
	}
	saved, err := store.Save(session, session.Revision)
	if err != nil {
		return agentchat.Session{}, err
	}
	return saved, nil
}

// agentRunHistorySnapshotIsStale reports whether applying run's history
// snapshot to a session would hide history the session already holds from a
// different, superseding run. A snapshot is stale when the session carries
// a final-answer item (assistant text or tool result) whose OriginRunID is
// neither this run nor one already represented in the snapshot itself.
// Prior runs present in the snapshot were part of the run's immutable
// context and are therefore not treated as stale.
func agentRunHistorySnapshotIsStale(sessionHistory, runHistory []agentchat.HistoryItem, runID string) bool {
	inSnapshot := make(map[string]bool, len(runHistory))
	for _, item := range runHistory {
		if item.OriginRunID != "" {
			inSnapshot[item.OriginRunID] = true
		}
	}
	for _, item := range sessionHistory {
		if item.OriginRunID == "" || item.OriginRunID == runID || inSnapshot[item.OriginRunID] {
			continue
		}
		if item.Type == "assistant" || item.Type == "function_call_output" {
			return true
		}
	}
	return false
}
