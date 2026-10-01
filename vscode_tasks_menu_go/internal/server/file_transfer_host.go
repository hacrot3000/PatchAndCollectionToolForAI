package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	rel, relErr := filepath.Rel(s.Workspace, target)
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

var _ io.Reader = (*os.File)(nil)
