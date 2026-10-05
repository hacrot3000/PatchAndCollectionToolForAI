package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

type projectMutationRequest struct {
	Action  string `json:"action"`
	Path    string `json:"path"`
	NewPath string `json:"new_path,omitempty"`
}

func (s *Server) projectMutate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectMutationRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.Path = strings.TrimSpace(req.Path)
	req.NewPath = strings.TrimSpace(req.NewPath)

	switch req.Action {
	case "create_file":
		if req.Path == "" {
			http.Error(w, "project path is required", http.StatusBadRequest)
			return
		}
		rel, target, err := s.resolveHostWorkspaceDestination(req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		lease, ok := s.acquireSharedMutation(w, r, "file.create", rel)
		if !ok {
			return
		}
		defer s.releaseSharedMutation(lease)
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			http.Error(w, "cannot create project file", http.StatusConflict)
			return
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(target)
			http.Error(w, "cannot finalize project file", http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "file.create", "file", rel, nil)
		writeJSON(w, http.StatusCreated, map[string]any{"path": rel, "type": "file"})

	case "mkdir":
		if req.Path == "" {
			http.Error(w, "project path is required", http.StatusBadRequest)
			return
		}
		rel, target, err := s.resolveHostWorkspaceDestination(req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		lease, ok := s.acquireSharedMutation(w, r, "file.mkdir", rel)
		if !ok {
			return
		}
		defer s.releaseSharedMutation(lease)
		if err := os.Mkdir(target, 0o755); err != nil {
			http.Error(w, "cannot create project folder", http.StatusConflict)
			return
		}
		s.auditSharedSuccess(r, "file.mkdir", "directory", rel, nil)
		writeJSON(w, http.StatusCreated, map[string]any{"path": rel, "type": "dir"})

	case "rename":
		if req.Path == "" || req.NewPath == "" {
			http.Error(w, "project path and new_path are required", http.StatusBadRequest)
			return
		}
		oldRel, source, sourceInfo, err := s.resolveHostWorkspaceEntry(req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		newRel, target, err := s.resolveHostWorkspaceDestination(req.NewPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		lease, ok := s.acquireSharedMutation(w, r, "file.rename", oldRel)
		if !ok {
			return
		}
		defer s.releaseSharedMutation(lease)
		if err := os.Rename(source, target); err != nil {
			http.Error(w, "cannot rename project item", http.StatusConflict)
			return
		}
		kind := "file"
		if sourceInfo.IsDir() {
			kind = "directory"
		}
		s.auditSharedSuccess(r, "file.rename", kind, oldRel, map[string]any{"new_path": newRel})
		writeJSON(w, http.StatusOK, map[string]any{"path": newRel, "old_path": oldRel, "type": kind})
	default:
		http.Error(w, "unsupported project mutation action", http.StatusBadRequest)
	}
}
