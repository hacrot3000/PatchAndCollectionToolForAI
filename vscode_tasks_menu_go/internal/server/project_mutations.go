package server

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/state"
)

type projectMutationRequest struct {
	Action  string `json:"action"`
	Path    string `json:"path"`
	NewPath string `json:"new_path,omitempty"`
	Token   string `json:"token,omitempty"`
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
	req.Token = strings.TrimSpace(req.Token)

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

	case "trash":
		if req.Path == "" {
			http.Error(w, "project path is required", http.StatusBadRequest)
			return
		}
		rel, source, sourceInfo, err := s.resolveHostWorkspaceEntry(req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		token, trashPath, err := s.newProjectTrashPath()
		if err != nil {
			http.Error(w, "cannot prepare project trash", http.StatusInternalServerError)
			return
		}
		lease, ok := s.acquireSharedMutation(w, r, "file.trash", rel)
		if !ok {
			return
		}
		defer s.releaseSharedMutation(lease)
		if err := moveProjectPath(source, trashPath, sourceInfo); err != nil {
			http.Error(w, "cannot move project item to trash", http.StatusConflict)
			return
		}
		kind := "file"
		if sourceInfo.IsDir() {
			kind = "directory"
		}
		s.auditSharedSuccess(r, "file.trash", kind, rel, map[string]any{"token": token})
		writeJSON(w, http.StatusOK, map[string]any{"path": rel, "token": token, "type": kind})

	case "restore":
		if req.Path == "" || req.Token == "" {
			http.Error(w, "project path and trash token are required", http.StatusBadRequest)
			return
		}
		rel, target, err := s.resolveHostWorkspaceDestination(req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		trashPath, info, err := s.projectTrashItem(req.Token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		lease, ok := s.acquireSharedMutation(w, r, "file.restore", rel)
		if !ok {
			return
		}
		defer s.releaseSharedMutation(lease)
		if err := moveProjectPath(trashPath, target, info); err != nil {
			http.Error(w, "cannot restore project item", http.StatusConflict)
			return
		}
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		s.auditSharedSuccess(r, "file.restore", kind, rel, map[string]any{"token": req.Token})
		writeJSON(w, http.StatusOK, map[string]any{"path": rel, "token": req.Token, "type": kind})

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

func (s *Server) projectTrashDir() (string, error) {
	dir := filepath.Join(state.Dir(s.Workspace), "project-trash")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func (s *Server) newProjectTrashPath() (string, string, error) {
	dir, err := s.projectTrashDir()
	if err != nil {
		return "", "", err
	}
	for attempt := 0; attempt < 8; attempt++ {
		raw := make([]byte, 16)
		if _, err := cryptorand.Read(raw); err != nil {
			return "", "", err
		}
		token := hex.EncodeToString(raw)
		path := filepath.Join(dir, token)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return token, path, nil
		}
	}
	return "", "", fmt.Errorf("cannot allocate trash token")
}

func (s *Server) projectTrashItem(token string) (string, os.FileInfo, error) {
	token = strings.TrimSpace(token)
	if len(token) != 32 {
		return "", nil, fmt.Errorf("invalid trash token")
	}
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 16 {
		return "", nil, fmt.Errorf("invalid trash token")
	}
	dir, err := s.projectTrashDir()
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, token)
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, fmt.Errorf("trash item not found")
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return "", nil, fmt.Errorf("invalid trash item")
	}
	return path, info, nil
}

func moveProjectPath(source, target string, info os.FileInfo) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	}
	if err := copyProjectPath(source, target, info); err != nil {
		_ = os.RemoveAll(target)
		return err
	}
	var err error
	if info.IsDir() {
		err = os.RemoveAll(source)
	} else {
		err = os.Remove(source)
	}
	if err != nil {
		_ = os.RemoveAll(target)
		return err
	}
	return nil
}
