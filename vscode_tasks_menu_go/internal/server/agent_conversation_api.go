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

func (s *Server) agentCoordinationHub(owner string) (*agentrun.CoordinationHub, error) {
	ownerWorkspace := s.agentOwnerWorkspace()
	privatePath, err := aiPrivatePath(ownerWorkspace)
	if err != nil {
		return nil, err
	}
	project, err := patchtool.ReadProjectIdentity(ownerWorkspace)
	if err != nil {
		return nil, err
	}
	workspaceHash := aiWorkspaceHash(ownerWorkspace)
	legacyPath := filepath.Join(filepath.Dir(privatePath), "agent-coordination-"+workspaceHash, "coordination.json")
	dir := filepath.Join(filepath.Dir(privatePath), "agent-coordination-"+workspaceHash+"-"+aiOwnerHash(owner))
	hubPath := filepath.Join(dir, "coordination.json")
	if err := s.migrateLegacyCoordinationJournal(legacyPath, hubPath, owner); err != nil {
		return nil, err
	}
	return agentrun.NewCoordinationHub(hubPath, owner, project.ID)
}

func (s *Server) agentConversationIDsForOwner(owner string) (map[string]bool, error) {
	store, err := s.agentChatStore()
	if err != nil {
		return nil, err
	}
	sessions, err := store.List(owner)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		out[session.ID] = true
	}
	return out, nil
}

// agentConversationGroupIDsForOwner returns only groups whose ownership has
// been verified for this principal. Readable (shared) groups are deliberately
// excluded: a group that was shared with a principal is not proof that the
// principal owns the group's contracts or artifacts.
func (s *Server) agentConversationGroupIDsForOwner(owner string) (map[string]bool, error) {
	registry, err := s.agentConversationRegistry()
	if err != nil {
		return nil, err
	}
	ownerID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		return nil, err
	}
	project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
	if err != nil {
		return nil, err
	}
	groups, err := registry.Groups(ownerID, project.ID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(groups))
	for _, group := range groups {
		if group.PrincipalOwner == owner {
			out[group.ID] = true
		}
	}
	return out, nil
}

// agentConversationGroupIDsAccessibleToOwner returns readable groups for
// access checks. It must not be used as ownership evidence.
func (s *Server) agentConversationGroupIDsAccessibleToOwner(owner string) (map[string]bool, error) {
	registry, err := s.agentConversationRegistry()
	if err != nil {
		return nil, err
	}
	ownerID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		return nil, err
	}
	project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
	if err != nil {
		return nil, err
	}
	groups, err := registry.AccessibleGroups(ownerID, project.ID, owner, s.agentGroupMemberOwnerResolver(owner))
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(groups))
	for _, group := range groups {
		out[group.ID] = true
	}
	return out, nil
}

func (s *Server) agentConversationGroupsAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
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
		groups, err := registry.AccessibleGroups(ownerWorkspaceID, project.ID, owner, s.agentGroupMemberOwnerResolver(owner))
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
		group, err := registry.CreateGroupForPrincipal(ownerWorkspaceID, project.ID, request.Title, owner)
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
		group, access, err := s.agentConversationGroupAccess(owner, id)
		if err != nil || !access.Read {
			http.Error(w, "conversation group not found", 404)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"group": group, "access": access})
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		var request struct {
			Action           string   `json:"action"`
			ConversationID   string   `json:"conversation_id"`
			ExpectedRevision uint64   `json:"expected_revision"`
			Instructions     string   `json:"instructions"`
			Managers         []string `json:"managers,omitempty"`
			Shared           []string `json:"shared_principals,omitempty"`
			ClaimPrincipal   string   `json:"claim_principal,omitempty"`
			ConversationIDs  []string `json:"conversation_ids,omitempty"`
			ClaimReason      string   `json:"reason,omitempty"`
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
		case "claim_owner":
			// Recovering a legacy group requires explicit administrative
			// authority plus verifiable evidence. An ownerless group alone is
			// never treated as permission to take it over.
			claim := agentrun.GroupOwnerClaim{Principal: request.ClaimPrincipal, ConversationIDs: request.ConversationIDs, Reason: request.ClaimReason}
			if claim.Principal == "" {
				claim.Principal = owner
			}
			if !s.agentGroupAdministrator(r, claim.Principal) {
				http.Error(w, "group ownership claim requires project administrator", 403)
				return
			}
			group, err := registry.ClaimGroupOwner(id, ownerWorkspaceID, project.ID, claim,
				func(principal string) bool { return s.agentGroupAdministrator(r, principal) }, s.groupConversationOwnerCheck())
			if err != nil {
				http.Error(w, "group ownership claim rejected", 403)
				return
			}
			writeJSON(w, 200, map[string]any{"group": group})
		case "add_member":
			if err := s.validateAgentConversationOwner(owner, request.ConversationID); err != nil {
				http.Error(w, "conversation not found for owner project", 404)
				return
			}
			// Membership changes require management rights, so a principal can
			// never add its own conversation to a group it may not manage.
			if err := s.validateAgentConversationGroupManage(owner, id); err != nil {
				http.Error(w, "conversation group management is not permitted", 403)
				return
			}
			group, err := registry.AddMember(id, ownerWorkspaceID, project.ID, request.ConversationID)
			if err != nil {
				http.Error(w, "conversation group member rejected", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"group": group})
		case "remove_member":
			if err := s.validateAgentConversationGroupManage(owner, id); err != nil {
				http.Error(w, "conversation group management is not permitted", 403)
				return
			}
			group, err := registry.RemoveMember(id, ownerWorkspaceID, project.ID, request.ConversationID)
			if err != nil {
				http.Error(w, "conversation group member rejected", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"group": group})
		case "update_instructions":
			if err := s.validateAgentConversationGroupManage(owner, id); err != nil {
				http.Error(w, "conversation group management is not permitted", 403)
				return
			}
			group, err := registry.UpdateGroupInstructions(id, ownerWorkspaceID, project.ID, request.ExpectedRevision, request.Instructions)
			if err != nil {
				http.Error(w, "conversation group revision changed or update invalid", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"group": group})
		case "set_managers":
			if err := s.validateAgentConversationGroupManage(owner, id); err != nil {
				http.Error(w, "conversation group management is not permitted", 403)
				return
			}
			group, err := registry.SetGroupManagers(id, ownerWorkspaceID, project.ID, request.Managers)
			if err != nil {
				http.Error(w, "conversation group manager update rejected", 409)
				return
			}
			writeJSON(w, 200, map[string]any{"group": group})
		case "share":
			if err := s.validateAgentConversationGroupManage(owner, id); err != nil {
				http.Error(w, "conversation group management is not permitted", 403)
				return
			}
			group, err := registry.ShareGroup(id, ownerWorkspaceID, project.ID, request.Shared)
			if err != nil {
				http.Error(w, "conversation group sharing update rejected", 409)
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

func (s *Server) conversationGroupForOwner(owner, groupID string) (agentrun.ConversationGroup, error) {
	registry, err := s.agentConversationRegistry()
	if err != nil {
		return agentrun.ConversationGroup{}, err
	}
	ownerID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		return agentrun.ConversationGroup{}, err
	}
	project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
	if err != nil {
		return agentrun.ConversationGroup{}, err
	}
	groups, err := registry.Groups(ownerID, project.ID)
	if err != nil {
		return agentrun.ConversationGroup{}, err
	}
	for _, group := range groups {
		if group.ID == groupID {
			return group, nil
		}
	}
	return agentrun.ConversationGroup{}, errors.New("conversation group not found")
}

// agentGroupMemberOwnerResolver reports whether the requesting owner owns the
// conversation with the given ID. It lets group authorization follow the
// conversation registry instead of the workspace that hosts the project.
func (s *Server) agentGroupMemberOwnerResolver(owner string) agentrun.MemberOwnerResolver {
	store, err := s.agentChatStore()
	if err != nil {
		return func(string) bool { return false }
	}
	sessions, err := store.List(owner)
	if err != nil {
		return func(string) bool { return false }
	}
	owned := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		owned[session.ID] = true
	}
	return func(conversationID string) bool { return owned[conversationID] }
}

// agentGroupAdministrator reports documented project-administration authority.
// The implicit local owner of a single-user installation is the administrator
// of its own groups; a shared installation requires the explicit permission.
func (s *Server) agentGroupAdministrator(r *http.Request, principal string) bool {
	if !s.Config.SharedServerEnabled {
		return principal == "local"
	}
	requestPrincipal, ok := PrincipalFromContext(r.Context())
	if !ok || !requestPrincipal.Allowed(identity.PermissionProjectAdmin) {
		return false
	}
	return aiPlanOwner(r) == principal
}

// groupConversationOwnerCheck checks conversation ownership against the store
// for an arbitrary principal. It is used only for verified ownership claims.
func (s *Server) groupConversationOwnerCheck() agentrun.ConversationOwnerCheck {
	store, err := s.agentChatStore()
	if err != nil {
		return func(string, string) bool { return false }
	}
	return func(principal, conversationID string) bool {
		_, err := store.Get(conversationID, principal)
		return err == nil
	}
}

// agentConversationGroupAccess resolves the effective access of the requesting
// owner to a conversation group.
func (s *Server) agentConversationGroupAccess(owner, groupID string) (agentrun.ConversationGroup, agentrun.GroupAccess, error) {
	registry, err := s.agentConversationRegistry()
	if err != nil {
		return agentrun.ConversationGroup{}, agentrun.GroupAccess{}, err
	}
	ownerID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		return agentrun.ConversationGroup{}, agentrun.GroupAccess{}, err
	}
	project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
	if err != nil {
		return agentrun.ConversationGroup{}, agentrun.GroupAccess{}, err
	}
	group, ok := registry.Group(groupID)
	if !ok || group.OwnerWorkspaceID != ownerID || group.ProjectID != project.ID {
		return agentrun.ConversationGroup{}, agentrun.GroupAccess{}, agentrun.ErrGroupNotFound
	}
	return group, group.Access(owner, s.agentGroupMemberOwnerResolver(owner)), nil
}

// validateAgentConversationGroupAccess requires read access to a group. A
// group owned by another owner is reported as not found so it never leaks.
func (s *Server) validateAgentConversationGroupAccess(owner, groupID string) error {
	_, access, err := s.agentConversationGroupAccess(owner, groupID)
	if err != nil {
		return err
	}
	if !access.Read {
		return agentrun.ErrGroupNotAuthorized
	}
	return nil
}

// validateAgentConversationGroupManage requires management rights over a group.
func (s *Server) validateAgentConversationGroupManage(owner, groupID string) error {
	_, access, err := s.agentConversationGroupAccess(owner, groupID)
	if err != nil {
		return err
	}
	if !access.Manage {
		return agentrun.ErrGroupNotAuthorized
	}
	return nil
}

// validateAgentConversationGroupMember requires that the conversation belongs
// to the requesting owner and is a member of the group.
func (s *Server) validateAgentConversationGroupMember(owner, groupID, conversationID string) error {
	if err := s.validateAgentConversationOwner(owner, conversationID); err != nil {
		return err
	}
	group, access, err := s.agentConversationGroupAccess(owner, groupID)
	if err != nil {
		return err
	}
	if !access.Read {
		return agentrun.ErrGroupNotAuthorized
	}
	for _, member := range group.Members {
		if member == conversationID {
			return nil
		}
	}
	return errors.New("conversation is not a member of the group")
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
