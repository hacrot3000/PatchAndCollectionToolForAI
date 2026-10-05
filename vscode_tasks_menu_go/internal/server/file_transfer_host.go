package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/ftpclient"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
)

type fileTransferHostToRemoteRequest struct {
	ProfileID string `json:"profile_id"`
	HostPath  string `json:"host_path"`
	RemotePath string `json:"remote_path"`
}

type fileTransferRemoteToHostRequest struct {
	ProfileID string `json:"profile_id"`
	RemotePath string `json:"remote_path"`
	HostDir   string `json:"host_dir"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

func decodeFileTransferJSON(w http.ResponseWriter, r *http.Request, value any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return errors.New("invalid JSON")
	}
	return nil
}

func (s *Server) fileTransferHostToRemote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferHostToRemoteRequest
	if err := decodeFileTransferJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.HostPath = strings.TrimSpace(req.HostPath)
	req.RemotePath = strings.TrimSpace(req.RemotePath)
	if req.ProfileID == "" || req.HostPath == "" || req.RemotePath == "" {
		http.Error(w, "profile_id, host_path and remote_path are required", http.StatusBadRequest)
		return
	}

	hostPath, err := s.resolveProjectPath(req.HostPath, false, false)
	if err != nil {
		http.Error(w, "host file not found inside workspace", http.StatusNotFound)
		return
	}
	info, err := os.Stat(hostPath)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "host file unavailable", http.StatusNotFound)
		return
	}
	if info.Size() > maxFileTransferBytes {
		http.Error(w, fmt.Sprintf("file transfer exceeds %d bytes", maxFileTransferBytes), http.StatusRequestEntityTooLarge)
		return
	}
	profile, err := s.resolveFileTransferProfile(req.ProfileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		file, openErr := os.Open(hostPath)
		if openErr != nil {
			err = openErr
			break
		}
		defer file.Close()
		err = s.withFTPClient(r.Context(), profile, nil, func(client *ftpclient.Client) error {
			return client.Store(r.Context(), req.RemotePath, file)
		})
	case filetransferprofile.ProtocolSFTP:
		var command string
		command, err = sftpclient.PutCommand(hostPath, req.RemotePath)
		if err == nil {
			_, err = s.runSFTP(r.Context(), profile, command+"quit\n", 64<<10)
		}
	default:
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "host_to_remote", ProfileID: profile.ID, Success: false})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "host_to_remote", ProfileID: profile.ID, Success: true})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fileTransferRemoteToHost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferRemoteToHostRequest
	if err := decodeFileTransferJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.RemotePath = strings.TrimSpace(req.RemotePath)
	req.HostDir = strings.TrimSpace(req.HostDir)
	if req.ProfileID == "" || req.RemotePath == "" {
		http.Error(w, "profile_id and remote_path are required", http.StatusBadRequest)
		return
	}
	if req.HostDir == "" {
		req.HostDir = "."
	}
	hostDir, err := s.resolveProjectPath(req.HostDir, true, true)
	if err != nil {
		http.Error(w, "host destination directory not found inside workspace", http.StatusNotFound)
		return
	}
	name := remoteDownloadName(req.RemotePath)
	target := filepath.Join(hostDir, name)
	if err := validateHostTransferTarget(target, req.Overwrite); err != nil {
		status := http.StatusConflict
		if errors.Is(err, os.ErrPermission) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	profile, err := s.resolveFileTransferProfile(req.ProfileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	tmp, err := os.CreateTemp(hostDir, ".taskdeck-file-transfer-*")
	if err != nil {
		http.Error(w, "cannot create host transfer temp file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	defer cleanup()
	if err := tmp.Chmod(0o600); err != nil {
		http.Error(w, "cannot protect host transfer temp file", http.StatusInternalServerError)
		return
	}

	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		writer := &transferLimitWriter{dst: tmp, remaining: maxFileTransferBytes}
		err = s.withFTPClient(r.Context(), profile, nil, func(client *ftpclient.Client) error {
			return client.Retrieve(r.Context(), req.RemotePath, writer)
		})
		if syncErr := tmp.Sync(); err == nil {
			err = syncErr
		}
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
	case filetransferprofile.ProtocolSFTP:
		if closeErr := tmp.Close(); closeErr != nil {
			err = closeErr
			break
		}
		var command string
		command, err = sftpclient.GetCommand(req.RemotePath, tmpPath)
		if err == nil {
			_, err = s.runSFTP(r.Context(), profile, command+"quit\n", 64<<10)
		}
	default:
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "remote_to_host", ProfileID: profile.ID, Success: false})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	info, err := os.Stat(tmpPath)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "downloaded host temp file unavailable", http.StatusInternalServerError)
		return
	}
	if info.Size() > maxFileTransferBytes {
		http.Error(w, fmt.Sprintf("file transfer exceeds %d bytes", maxFileTransferBytes), http.StatusRequestEntityTooLarge)
		return
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		http.Error(w, "cannot finalize host transfer permissions", http.StatusInternalServerError)
		return
	}
	if err := validateHostTransferTarget(target, req.Overwrite); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	root, rootErr := s.projectRoot()
	if rootErr != nil {
		http.Error(w, "project root unavailable", http.StatusInternalServerError)
		return
	}
	rel, relErr := filepath.Rel(root, target)
	if relErr != nil {
		rel = name
	}
	resourceID := filepath.ToSlash(rel)
	lease, ok := s.acquireSharedMutation(w, r, "file.transfer", resourceID)
	if !ok {
		return
	}
	defer s.releaseSharedMutation(lease)

	if err := os.Rename(tmpPath, target); err != nil {
		http.Error(w, "cannot finalize host transfer", http.StatusInternalServerError)
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "remote_to_host", ProfileID: profile.ID, Success: true})
	writeJSON(w, http.StatusCreated, map[string]any{
		"path": resourceID,
		"name": name,
		"size": info.Size(),
	})
}

func validateHostTransferTarget(target string, overwrite bool) error {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.New("host destination unavailable")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("host destination is not a regular file")
	}
	if !overwrite {
		return errors.New("host destination file already exists")
	}
	if info.Mode().Perm()&0o222 == 0 {
		return os.ErrPermission
	}
	return nil
}


type fileTransferHostMutationRequest struct {
	Action  string `json:"action"`
	Path    string `json:"path"`
	NewPath string `json:"new_path,omitempty"`
}

func (s *Server) resolveHostWorkspaceEntry(requested string) (string, string, os.FileInfo, error) {
	rootView, rootRelative, err := s.projectRootForVirtualPath(requested)
	if err != nil {
		return "", "", nil, err
	}
	root := rootView.Path
	rel, err := cleanProjectRelativePath(rootRelative, false)
	if err != nil {
		return "", "", nil, err
	}
	candidate := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(candidate)
	if err != nil {
		return "", "", nil, fmt.Errorf("host item not found")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", "", nil, fmt.Errorf("host symlink mutations are not allowed")
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil || !pathWithin(root, resolved) {
		return "", "", nil, fmt.Errorf("host item is outside workspace")
	}
	info, err = os.Stat(resolved)
	if err != nil {
		return "", "", nil, fmt.Errorf("host item not found")
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return "", "", nil, fmt.Errorf("host item type is unsupported")
	}
	return workspaceVirtualPath(rootView.ID, filepath.ToSlash(rel)), resolved, info, nil
}

func (s *Server) resolveHostWorkspaceDestination(requested string) (string, string, error) {
	rootView, rootRelative, err := s.projectRootForVirtualPath(requested)
	if err != nil {
		return "", "", err
	}
	root := rootView.Path
	rel, err := cleanProjectRelativePath(rootRelative, false)
	if err != nil {
		return "", "", err
	}
	clean := filepath.FromSlash(rel)
	parent := filepath.Dir(clean)
	base := filepath.Base(clean)
	if base == "" || base == "." || base == ".." {
		return "", "", fmt.Errorf("invalid host destination")
	}
	parentPath := root
	if parent != "." {
		parentPath = filepath.Join(root, parent)
	}
	resolvedParent, err := filepath.EvalSymlinks(parentPath)
	if err != nil || !pathWithin(root, resolvedParent) {
		return "", "", fmt.Errorf("host destination parent not found")
	}
	parentInfo, err := os.Stat(resolvedParent)
	if err != nil || !parentInfo.IsDir() {
		return "", "", fmt.Errorf("host destination parent not found")
	}
	target := filepath.Join(resolvedParent, base)
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("host destination symlink is not allowed")
		}
		return "", "", fmt.Errorf("host destination already exists")
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("host destination unavailable")
	}
	return workspaceVirtualPath(rootView.ID, filepath.ToSlash(rel)), target, nil
}

func (s *Server) fileTransferHostMutate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req fileTransferHostMutationRequest
	if err := decodeFileTransferJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.Path = strings.TrimSpace(req.Path)
	req.NewPath = strings.TrimSpace(req.NewPath)

	switch req.Action {
	case "mkdir":
		if req.Path == "" {
			http.Error(w, "host path is required", http.StatusBadRequest)
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
			http.Error(w, "cannot create host folder", http.StatusConflict)
			return
		}
		s.auditSharedSuccess(r, "file.mkdir", "directory", rel, nil)
		w.WriteHeader(http.StatusNoContent)
	case "rename":
		if req.Path == "" || req.NewPath == "" {
			http.Error(w, "host path and new_path are required", http.StatusBadRequest)
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
			http.Error(w, "cannot rename host item", http.StatusConflict)
			return
		}
		kind := "file"
		if sourceInfo.IsDir() {
			kind = "directory"
		}
		s.auditSharedSuccess(r, "file.rename", kind, oldRel, map[string]any{"new_path": newRel})
		w.WriteHeader(http.StatusNoContent)
	case "delete":
		if req.Path == "" {
			http.Error(w, "host path is required", http.StatusBadRequest)
			return
		}
		rel, target, info, err := s.resolveHostWorkspaceEntry(req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		lease, ok := s.acquireSharedMutation(w, r, "file.delete", rel)
		if !ok {
			return
		}
		defer s.releaseSharedMutation(lease)
		if err := os.Remove(target); err != nil {
			message := "cannot delete host file"
			if info.IsDir() {
				message = "cannot delete host folder; it must be empty"
			}
			http.Error(w, message, http.StatusConflict)
			return
		}
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		s.auditSharedSuccess(r, "file.delete", kind, rel, nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unsupported host mutation action", http.StatusBadRequest)
	}
}
