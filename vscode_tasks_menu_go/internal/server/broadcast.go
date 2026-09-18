package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/session"
)

type broadcastRequest struct {
	Action    string `json:"action"`
	Mode      string `json:"mode,omitempty"`
	GroupID   string `json:"group_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Preset    string `json:"preset,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	SourceID  string `json:"source_id,omitempty"`
	Data      string `json:"data,omitempty"`
}

func (s *Server) broadcastStateAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		state, err := loadBroadcastState(s.Workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, state)
	case http.MethodPost:
		var req broadcastRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		switch strings.TrimSpace(req.Action) {
		case "set_mode":
			s.broadcastSetMode(w, req)
		case "create_group":
			s.broadcastCreateGroup(w, req)
		case "update_group":
			s.broadcastUpdateGroup(w, req)
		case "delete_group":
			s.broadcastDeleteGroup(w, req)
		case "assign":
			s.broadcastAssign(w, req)
		case "input":
			s.broadcastInput(w, req)
		default:
			http.Error(w, "unknown broadcast action", http.StatusBadRequest)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) broadcastSetMode(w http.ResponseWriter, req broadcastRequest) {
	mode := strings.TrimSpace(req.Mode)
	if !validBroadcastMode(mode) {
		http.Error(w, "invalid broadcast mode", http.StatusBadRequest)
		return
	}
	state, err := mutateBroadcastState(s.Workspace, func(state *broadcastState) error {
		state.Mode = mode
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) broadcastCreateGroup(w http.ResponseWriter, req broadcastRequest) {
	name := normalizeBroadcastGroupName(req.Name)
	preset := strings.TrimSpace(req.Preset)
	if name == "" {
		http.Error(w, "group name is required", http.StatusBadRequest)
		return
	}
	if !validBroadcastPreset(preset) {
		http.Error(w, "invalid group color preset", http.StatusBadRequest)
		return
	}
	id, err := newBroadcastGroupID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := mutateBroadcastState(s.Workspace, func(state *broadcastState) error {
		state.Groups = append(state.Groups, broadcastGroup{ID: id, Name: name, Preset: preset})
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, state)
}

func (s *Server) broadcastUpdateGroup(w http.ResponseWriter, req broadcastRequest) {
	id := strings.TrimSpace(req.GroupID)
	name := normalizeBroadcastGroupName(req.Name)
	preset := strings.TrimSpace(req.Preset)
	if id == "" || name == "" {
		http.Error(w, "group id and name are required", http.StatusBadRequest)
		return
	}
	if !validBroadcastPreset(preset) {
		http.Error(w, "invalid group color preset", http.StatusBadRequest)
		return
	}
	state, err := mutateBroadcastState(s.Workspace, func(state *broadcastState) error {
		group, ok := findBroadcastGroup(state, id)
		if !ok {
			return errBroadcastGroupNotFound
		}
		group.Name, group.Preset = name, preset
		return nil
	})
	if err != nil {
		if err == errBroadcastGroupNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) broadcastDeleteGroup(w http.ResponseWriter, req broadcastRequest) {
	id := strings.TrimSpace(req.GroupID)
	if id == "" {
		http.Error(w, "group id is required", http.StatusBadRequest)
		return
	}
	state, err := mutateBroadcastState(s.Workspace, func(state *broadcastState) error {
		found := false
		groups := state.Groups[:0]
		for _, group := range state.Groups {
			if group.ID == id {
				found = true
				continue
			}
			groups = append(groups, group)
		}
		if !found {
			return errBroadcastGroupNotFound
		}
		state.Groups = groups
		for sessionID, groupID := range state.Assignments {
			if groupID == id {
				delete(state.Assignments, sessionID)
			}
		}
		return nil
	})
	if err != nil {
		if err == errBroadcastGroupNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) broadcastAssign(w http.ResponseWriter, req broadcastRequest) {
	sessionID := strings.TrimSpace(req.SessionID)
	groupID := strings.TrimSpace(req.GroupID)
	if sessionID == "" {
		http.Error(w, "session id is required", http.StatusBadRequest)
		return
	}
	if _, ok := s.Sessions.Metadata(sessionID); !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	state, err := mutateBroadcastState(s.Workspace, func(state *broadcastState) error {
		if groupID == "" {
			delete(state.Assignments, sessionID)
			return nil
		}
		if _, ok := findBroadcastGroup(state, groupID); !ok {
			return errBroadcastGroupNotFound
		}
		state.Assignments[sessionID] = groupID
		return nil
	})
	if err != nil {
		if err == errBroadcastGroupNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) broadcastInput(w http.ResponseWriter, req broadcastRequest) {
	sourceID := strings.TrimSpace(req.SourceID)
	if sourceID == "" {
		http.Error(w, "source session id is required", http.StatusBadRequest)
		return
	}
	source, ok := s.Sessions.Metadata(sourceID)
	if !ok {
		http.Error(w, "source session not found", http.StatusNotFound)
		return
	}
	if source.Status != "running" || req.Data == "" {
		writeJSON(w, http.StatusOK, map[string]any{"delivered": 0})
		return
	}
	state, err := loadBroadcastState(s.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if state.Mode == broadcastModeNone {
		writeJSON(w, http.StatusOK, map[string]any{"delivered": 0})
		return
	}

	sourceGroup := ""
	if state.Mode == broadcastModeGroup {
		sourceGroup = state.Assignments[sourceID]
		if sourceGroup == "" {
			writeJSON(w, http.StatusOK, map[string]any{"delivered": 0})
			return
		}
	}

	delivered := 0
	failed := make([]string, 0)
	for _, meta := range s.Sessions.List() {
		if meta.ID == sourceID || meta.Status != "running" {
			continue
		}
		if state.Mode == broadcastModeGroup && state.Assignments[meta.ID] != sourceGroup {
			continue
		}
		if err := s.Sessions.Input(meta.ID, []byte(req.Data)); err != nil {
			failed = append(failed, meta.ID)
			continue
		}
		delivered++
	}
	writeJSON(w, http.StatusOK, map[string]any{"delivered": delivered, "failed": failed})
}

var errBroadcastGroupNotFound = broadcastStateError("broadcast group not found")

type broadcastStateError string

func (e broadcastStateError) Error() string { return string(e) }

func broadcastGroupForSession(state broadcastState, sessionID string) (broadcastGroup, bool) {
	groupID := state.Assignments[sessionID]
	if groupID == "" {
		return broadcastGroup{}, false
	}
	for _, group := range state.Groups {
		if group.ID == groupID {
			return group, true
		}
	}
	return broadcastGroup{}, false
}

func broadcastLiveSessions(items []session.Metadata) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, meta := range items {
		out[meta.ID] = meta.Status == "running"
	}
	return out
}
