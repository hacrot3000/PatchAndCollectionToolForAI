package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

	case "copy":
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
		lease, ok := s.acquireSharedMutation(w, r, "file.copy", oldRel)
		if !ok {
			return
		}
		defer s.releaseSharedMutation(lease)
		if err := copyProjectPath(source, target, sourceInfo); err != nil {
			_ = os.RemoveAll(target)
			http.Error(w, "cannot copy project item: "+err.Error(), http.StatusConflict)
			return
		}
		kind := "file"
		if sourceInfo.IsDir() {
			kind = "directory"
		}
		s.auditSharedSuccess(r, "file.copy", kind, oldRel, map[string]any{"new_path": newRel})
		writeJSON(w, http.StatusCreated, map[string]any{"path": newRel, "source_path": oldRel, "type": kind})

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

func copyProjectPath(source, target string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return os.ErrInvalid
	}
	if info.Mode().IsRegular() {
		return copyProjectRegularFile(source, target, info.Mode().Perm())
	}
	if !info.IsDir() {
		return os.ErrInvalid
	}
	if err := os.Mkdir(target, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourceChild := filepath.Join(source, entry.Name())
		targetChild := filepath.Join(target, entry.Name())
		childInfo, err := os.Lstat(sourceChild)
		if err != nil {
			return err
		}
		if childInfo.Mode()&os.ModeSymlink != 0 {
			return os.ErrInvalid
		}
		if childInfo.IsDir() {
			if err := copyProjectPath(sourceChild, targetChild, childInfo); err != nil {
				return err
			}
			continue
		}
		if !childInfo.Mode().IsRegular() {
			return os.ErrInvalid
		}
		if err := copyProjectRegularFile(sourceChild, targetChild, childInfo.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyProjectRegularFile(source, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = output.Close()
		if !ok {
			_ = os.Remove(target)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
