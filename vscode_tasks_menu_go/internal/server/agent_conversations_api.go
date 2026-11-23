package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// agentConversationsAPI lists or creates conversations for the authenticated
// owner in the current project. Each conversation keeps its own history,
// status and run bindings, so several can run in parallel.
func (s *Server) agentConversationsAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
	permission := identity.PermissionSettingsRead
	if r.Method == http.MethodPost {
		permission = identity.PermissionSettingsWrite
	}
	if !s.requireSharedActionPermission(w, r, permission, "agent.conversations", "agent_conversations") {
		return
	}
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "conversation store unavailable", 500)
		return
	}
	switch r.Method {
	case http.MethodGet:
		sessions, err := store.List(owner)
		if err != nil {
			http.Error(w, "cannot list conversations", 500)
			return
		}
		summaries := make([]map[string]any, 0, len(sessions))
		for _, item := range sessions {
			summaries = append(summaries, map[string]any{
				"id": item.ID, "title": item.Title, "status": item.Status, "archived": item.Archived,
				"provider_id": item.ProviderID, "model_id": item.ModelID, "conversation_id": item.ConversationID,
				"revision": item.Revision, "updated_at": item.UpdatedAt, "unresolved": agentchat.HasUnresolvedWork(item),
			})
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"conversations": summaries})
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var request struct {
			ProviderID      string `json:"provider_id"`
			ModelID         string `json:"model_id"`
			Mode            string `json:"mode"`
			ReasoningEffort string `json:"reasoning_effort"`
			Title           string `json:"title,omitempty"`
			GroupID         string `json:"group_id,omitempty"`
			ForkFrom        string `json:"fork_from,omitempty"`
			ForkTurnID      string `json:"fork_turn_id,omitempty"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&request) != nil {
			http.Error(w, "invalid conversation request", 400)
			return
		}
		var trailing any
		if !errors.Is(dec.Decode(&trailing), io.EOF) {
			http.Error(w, "exactly one conversation object required", 400)
			return
		}
		if request.ForkFrom != "" {
			fork, err := s.ForkConversation(owner, request.ForkFrom, request.ForkTurnID)
			if err != nil {
				http.Error(w, "cannot fork conversation", 409)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusCreated, map[string]any{"conversation": fork, "forked_from": request.ForkFrom})
			return
		}
		if request.ProviderID == "" || request.ModelID == "" {
			http.Error(w, "provider and model are required", 400)
			return
		}
		mode := agentchat.ContextMode(request.Mode)
		if mode == "" {
			mode = agentchat.Stateless
		}
		project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
		if err != nil {
			http.Error(w, "owner project identity unavailable", 409)
			return
		}
		session, err := agentchat.NewSession(owner, s.agentOwnerWorkspace(), project.ID, request.ProviderID, request.ModelID, mode)
		if err != nil {
			http.Error(w, "invalid conversation binding", 400)
			return
		}
		session.Title = strings.TrimSpace(request.Title)
		session.ConversationID = session.ID
		session.ReasoningEffort = request.ReasoningEffort
		var groupRegistry *agentrun.ConversationRegistry
		var ownerWorkspaceID string
		if request.GroupID != "" {
			groupRegistry, err = s.agentConversationRegistry()
			if err != nil {
				http.Error(w, "conversation group registry unavailable", 500)
				return
			}
			ownerWorkspaceID, err = agentrunWorkspaceID(s)
			if err != nil {
				http.Error(w, "owner workspace is unavailable", 409)
				return
			}
			if _, err := groupRegistry.AddMember(request.GroupID, ownerWorkspaceID, project.ID, session.ID); err != nil {
				http.Error(w, "conversation group membership rejected", 409)
				return
			}
		}
		saved, err := store.Save(session, 0)
		if err != nil {
			if groupRegistry != nil {
				_, _ = groupRegistry.RemoveMember(request.GroupID, ownerWorkspaceID, project.ID, session.ID)
			}
			http.Error(w, "cannot persist conversation", 500)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, map[string]any{"conversation": saved})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

// agentConversationItemAPI exposes one conversation with its run bindings and
// coordination summary.
func (s *Server) agentConversationItemAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
	id := strings.TrimPrefix(r.URL.Path, "/api/agent/conversations/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "invalid conversation ID", 400)
		return
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionSettingsRead, "agent.conversation.inspect", id) {
		return
	}
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "conversation store unavailable", 500)
		return
	}
	session, err := store.Get(id, owner)
	if err != nil {
		http.Error(w, "conversation not found", 404)
		return
	}
	manager, err := s.agentRunManager(owner)
	if err != nil {
		http.Error(w, "Agent Run Manager unavailable", 503)
		return
	}
	runs := make([]map[string]any, 0)
	for _, record := range manager.List() {
		if record.Spec.ConversationID != session.ID {
			continue
		}
		runs = append(runs, map[string]any{"run_id": record.Spec.RunID, "status": record.Status,
			"attempt": record.Attempt, "execution_workspace_id": record.Spec.ExecutionWorkspaceID,
			"updated_at": record.UpdatedAt, "last_error": record.LastError})
	}
	hub, err := s.agentCoordinationHub()
	messages := []any{}
	if err == nil {
		if list, listErr := hub.PeerMessages(session.ID, ""); listErr == nil {
			for _, item := range list {
				messages = append(messages, item)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"conversation": session, "runs": runs, "peer_messages": messages})
}
