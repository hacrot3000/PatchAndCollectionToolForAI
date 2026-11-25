package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func (s *Server) agentContextGroupStore(owner string) (*agentrun.ContextGroupStore, string, string, error) {
	privatePath, err := aiPrivatePath(s.agentOwnerWorkspace())
	if err != nil {
		return nil, "", "", err
	}
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.agentOwnerWorkspace())
	if err != nil {
		return nil, "", "", err
	}
	identity, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace())
	if err != nil {
		return nil, "", "", err
	}
	dir := filepath.Join(filepath.Dir(privatePath), "agent-context-groups-"+aiWorkspaceHash(s.agentOwnerWorkspace())+"-"+aiOwnerHash(owner))
	store, err := agentrun.NewContextGroupStore(dir)
	return store, ownerWorkspaceID, identity.ID, err
}

func (s *Server) agentContextGroupsAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
	permission := identity.PermissionSettingsRead
	if r.Method == http.MethodPost {
		permission = identity.PermissionSettingsWrite
	}
	if !s.requireSharedActionPermission(w, r, permission, "agent.context_group", "agent_context_groups") {
		return
	}
	store, ownerID, projectID, err := s.agentContextGroupStore(owner)
	if err != nil {
		http.Error(w, "context group store unavailable", 503)
		return
	}
	switch r.Method {
	case http.MethodGet:
		groups, err := store.List(ownerID, projectID)
		if err != nil {
			http.Error(w, "cannot list context groups", 500)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"groups": groups})
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 40<<10)
		var request struct {
			Title        string `json:"title"`
			Instructions string `json:"instructions"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&request); err != nil {
			http.Error(w, "invalid context group", 400)
			return
		}
		var trailing any
		if !errors.Is(dec.Decode(&trailing), io.EOF) {
			http.Error(w, "exactly one context group object required", 400)
			return
		}
		group, err := store.Create(ownerID, projectID, request.Title, request.Instructions)
		if err != nil {
			http.Error(w, "context group rejected", 400)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, map[string]any{"group": group})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) agentContextGroupItemAPI(w http.ResponseWriter, r *http.Request) {
	owner := aiPlanOwner(r)
	id := strings.TrimPrefix(r.URL.Path, "/api/agent/context-groups/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "invalid context group ID", 400)
		return
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionSettingsRead, "agent.context_group.inspect", id) {
		return
	}
	store, ownerID, projectID, err := s.agentContextGroupStore(owner)
	if err != nil {
		http.Error(w, "context group store unavailable", 503)
		return
	}
	if r.Method == http.MethodGet {
		group, err := store.Get(id, ownerID, projectID)
		if err != nil {
			http.Error(w, "context group not found", 404)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"group": group})
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.requireSharedActionPermission(w, r, identity.PermissionSettingsWrite, "agent.context_group.update", id) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 40<<10)
	var request struct {
		Title            string `json:"title"`
		Instructions     string `json:"instructions"`
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		http.Error(w, "invalid context group update", 400)
		return
	}
	var trailing any
	if !errors.Is(dec.Decode(&trailing), io.EOF) {
		http.Error(w, "exactly one context group object required", 400)
		return
	}
	group, err := store.Update(id, ownerID, projectID, request.ExpectedRevision, request.Title, request.Instructions)
	if err != nil {
		http.Error(w, "context group revision changed or update invalid", 409)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"group": group})
}
