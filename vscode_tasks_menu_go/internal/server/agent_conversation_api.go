package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func (s *Server) agentConversationRegistry() (*agentrun.ConversationRegistry, error) {
	privatePath, err := aiPrivatePath(s.agentOwnerWorkspace())
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(filepath.Dir(privatePath), "agent-conversations-"+aiWorkspaceHash(s.agentOwnerWorkspace())+"-"+aiOwnerHash("shared"))
	return agentrun.NewConversationRegistry(filepath.Join(dir, "conversations.json"))
}

func (s *Server) agentCoordinationHub() (*agentrun.CoordinationHub, error) {
	privatePath, err := aiPrivatePath(s.agentOwnerWorkspace())
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(filepath.Dir(privatePath), "agent-coordination-"+aiWorkspaceHash(s.agentOwnerWorkspace()))
	return agentrun.NewCoordinationHub(filepath.Join(dir, "coordination.json"))
}

func (s *Server) agentConversationGroupsAPI(w http.ResponseWriter, r *http.Request) {
	permission := identity.PermissionSettingsRead
	if r.Method == http.MethodPost {
		permission = identity.PermissionSettingsWrite
	}
	if !s.requireSharedActionPermission(w, r, permission, "agent.conversation_groups", "agent_conversation_groups") {
		return
	}
	registry, err := s.agentConversationRegistry()
	if err != nil {
		http.Error(w, "conversation registry unavailable", 500)
		return
	}
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		http.Error(w, "owner workspace is unavailable", 409)
		return
	}
	project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
	if err != nil {
		http.Error(w, "owner project identity unavailable", 409)
		return
	}
	switch r.Method {
	case http.MethodGet:
		groups, err := registry.Groups(ownerWorkspaceID, project.ID)
		if err != nil {
			http.Error(w, "cannot list conversation groups", 500)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"groups": groups})
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var request struct {
			Title string `json:"title"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&request) != nil {
			http.Error(w, "invalid conversation group", 400)
			return
		}
		var trailing any
		if !errors.Is(dec.Decode(&trailing), io.EOF) {
			http.Error(w, "exactly one conversation group object required", 400)
			return
		}
		group, err := registry.CreateGroup(ownerWorkspaceID, project.ID, request.Title)
		if err != nil {
			http.Error(w, "conversation group rejected", 400)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, map[string]any{"group": group})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) agentConversationGroupItemAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
	id := strings.TrimPrefix(r.URL.Path, "/api/agent/conversation-groups/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "invalid conversation group ID", 400)
		return
	}
	permission := identity.PermissionSettingsRead
	if r.Method == http.MethodPost {
		permission = identity.PermissionSettingsWrite
	}
	if !s.requireSharedActionPermission(w, r, permission, "agent.conversation_group.inspect", id) {
		return
	}
	registry, err := s.agentConversationRegistry()
	if err != nil {
		http.Error(w, "conversation registry unavailable", 500)
		return
	}
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		http.Error(w, "owner workspace is unavailable", 409)
		return
	}
	project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
	if err != nil {
		http.Error(w, "owner project identity unavailable", 409)
		return
	}
	switch r.Method {
	case http.MethodGet:
		groups, err := registry.Groups(ownerWorkspaceID, project.ID)
		if err != nil {
			http.Error(w, "cannot list conversation groups", 500)
			return
		}
		for _, group := range groups {
			if group.ID == id {
				w.Header().Set("Cache-Control", "no-store")
				writeJSON(w, 200, map[string]any{"group": group})
				return
			}
		}
		http.Error(w, "conversation group not found", 404)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		var request struct {
			Action           string `json:"action"`
			ConversationID   string `json:"conversation_id"`
			ExpectedRevision uint64 `json:"expected_revision"`
			Instructions     string `json:"instructions"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&request) != nil {
			http.Error(w, "invalid conversation group request", 400)
			return
		}
		var trailing any
		if !errors.Is(dec.Decode(&trailing), io.EOF) {
			http.Error(w, "exactly one conversation group object required", 400)
			return
		}
		switch request.Action {
		case "add_member":
			if err := s.validateAgentConversationOwner(owner, request.ConversationID); err != nil {
				http.Error(w, "conversation not found for owner project", 404)
				return
			}
			group, err := registry.AddMember(id, ownerWorkspaceID, project.ID, request.ConversationID)
			if err != nil {
				http.Error(w, "conversation group member rejected", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"group": group})
		case "remove_member":
			group, err := registry.RemoveMember(id, ownerWorkspaceID, project.ID, request.ConversationID)
			if err != nil {
				http.Error(w, "conversation group member rejected", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"group": group})
		case "update_instructions":
			group, err := registry.UpdateGroupInstructions(id, ownerWorkspaceID, project.ID, request.ExpectedRevision, request.Instructions)
			if err != nil {
				http.Error(w, "conversation group revision changed or update invalid", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"group": group})
		default:
			http.Error(w, "unknown conversation group action", 400)
		}
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func agentrunWorkspaceID(s *Server) (string, error) {
	return agentrun.WorkspaceID(s.agentOwnerWorkspace())
}

func (s *Server) validateAgentConversationOwner(owner, conversationID string) error {
	if strings.TrimSpace(conversationID) == "" {
		return errors.New("conversation ID is required")
	}
	store, err := s.agentChatStore()
	if err != nil {
		return err
	}
	session, err := store.Get(conversationID, owner)
	if err != nil {
		return err
	}
	project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
	if err != nil {
		return err
	}
	if session.ProjectID != project.ID {
		return errors.New("conversation belongs to another project")
	}
	return nil
}

func (s *Server) validateAgentConversationGroupMember(owner, groupID, conversationID string) error {
	if err := s.validateAgentConversationOwner(owner, conversationID); err != nil {
		return err
	}
	if strings.TrimSpace(groupID) == "" {
		return errors.New("conversation group is required")
	}
	registry, err := s.agentConversationRegistry()
	if err != nil {
		return err
	}
	ownerID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		return err
	}
	project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
	if err != nil {
		return err
	}
	groups, err := registry.Groups(ownerID, project.ID)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.ID != groupID {
			continue
		}
		for _, member := range group.Members {
			if member == conversationID {
				return nil
			}
		}
		return errors.New("conversation is not a member of the group")
	}
	return errors.New("conversation group not found")
}

// ForkConversation creates a new conversation that inherits the ancestor prefix
// of the source conversation up to the fork point. Later local turns of the
// source are never copied into the fork.
func (s *Server) ForkConversation(owner, sourceID, forkTurnID string) (agentchat.Session, error) {
	store, err := s.agentChatStore()
	if err != nil {
		return agentchat.Session{}, err
	}
	source, err := store.Get(sourceID, owner)
	if err != nil {
		return agentchat.Session{}, err
	}
	registry, err := s.agentConversationRegistry()
	if err != nil {
		return agentchat.Session{}, err
	}
	fork, err := agentchat.NewSession(source.Owner, source.Workspace, source.ProjectID, source.ProviderID, source.ModelID, source.Mode)
	if err != nil {
		return agentchat.Session{}, err
	}
	fork.Title = source.Title
	fork.ReasoningEffort = source.ReasoningEffort
	fork.RetryMode = source.RetryMode
	fork.ConversationID = fork.ID
	fork.Status = "created"
	runRecords, err := s.completedAgentRuns(owner, sourceID)
	if err != nil {
		return agentchat.Session{}, err
	}
	sourceHistory := completedAgentRunHistory(source.History, runRecords, sourceID)
	if _, err := agentrun.ForkHistoryThroughTurn(sourceHistory, forkTurnID); err != nil {
		return agentchat.Session{}, err
	}
	if _, err := registry.RecordFork(fork.ID, sourceID, forkTurnID, ""); err != nil {
		return agentchat.Session{}, err
	}
	prefix, ok := registry.SharedSnapshotFor(fork.ID, sourceHistory)
	if !ok {
		return agentchat.Session{}, errors.New("fork boundary is not present in source history")
	}
	fork.History = prefix
	saved, err := store.Save(fork, 0)
	if err != nil {
		return agentchat.Session{}, err
	}
	return saved, nil
}

func (s *Server) agentConversationForkAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionSettingsWrite, "agent.conversation.fork", "agent_conversations") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var request struct {
		SourceConversationID string `json:"source_conversation_id"`
		ForkTurnID           string `json:"fork_turn_id,omitempty"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&request) != nil {
		http.Error(w, "invalid fork request", 400)
		return
	}
	var trailing any
	if !errors.Is(dec.Decode(&trailing), io.EOF) {
		http.Error(w, "exactly one fork object required", 400)
		return
	}
	if strings.TrimSpace(request.SourceConversationID) == "" {
		http.Error(w, "source conversation is required", 400)
		return
	}
	fork, err := s.ForkConversation(owner, request.SourceConversationID, request.ForkTurnID)
	if err != nil {
		http.Error(w, "cannot fork conversation", 409)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"session": fork, "forked_from": request.SourceConversationID})
}
