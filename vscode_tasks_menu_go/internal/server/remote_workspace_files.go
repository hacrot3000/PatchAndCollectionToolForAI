package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	pathpkg "path"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
)

type remoteWorkspaceFileRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	Operation      string `json:"operation"`
	Path           string `json:"path,omitempty"`
	Content        string `json:"content,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

func remoteWorkspaceRelativePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." {
		return ".", nil
	}
	if strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("remote workspace path must be relative")
	}
	cleaned := pathpkg.Clean(value)
	if cleaned == "." {
		return ".", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("remote workspace path escapes root")
	}
	return cleaned, nil
}

func remoteWorkspaceResolvePath(root, relative string) (string, error) {
	root, err := normalizeRemoteRoot(root)
	if err != nil {
		return "", err
	}
	relative, err = remoteWorkspaceRelativePath(relative)
	if err != nil {
		return "", err
	}
	if relative == "." {
		return root, nil
	}
	resolved := pathpkg.Clean(pathpkg.Join(root, relative))
	if root == "/" {
		if !strings.HasPrefix(resolved, "/") {
			return "", errors.New("remote workspace path escapes root")
		}
		return resolved, nil
	}
	if resolved != root && !strings.HasPrefix(resolved, root+"/") {
		return "", errors.New("remote workspace path escapes root")
	}
	return resolved, nil
}

func (s *Server) canonicalRemoteWorkspacePath(ctx context.Context, workspace remoteWorkspaceProfile, profile filetransferprofile.Profile, remotePath string) (string, error) {
	root := remoteArchiveShellQuote(workspace.RemoteRoot)
	target := remoteArchiveShellQuote(remotePath)
	command := "set -eu; command -v realpath >/dev/null 2>&1; root=$(realpath -- "+root+"); target=$(realpath -- "+target+"); case \"$target\" in \"$root\"|\"$root\"/*) printf '%s\\n' \"$target\" ;; *) echo 'remote workspace path escapes canonical root' >&2; exit 42 ;; esac"
	output, err := s.runSFTPLinkedSSHCommand(ctx, profile, command)
	if err != nil {
		return "", fmt.Errorf("validate remote workspace canonical path: %w", err)
	}
	canonical := strings.TrimSpace(output)
	if canonical == "" || !strings.HasPrefix(canonical, "/") || strings.ContainsAny(canonical, "\x00\r\n") {
		return "", errors.New("remote workspace canonical path is invalid")
	}
	return canonical, nil
}

func (s *Server) resolveRemoteWorkspace(id string) (remoteWorkspaceProfile, filetransferprofile.Profile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return remoteWorkspaceProfile{}, filetransferprofile.Profile{}, errors.New("workspace_id is required")
	}
	remoteWorkspaceStoreMu.Lock()
	items, err := readRemoteWorkspaceProfiles()
	if err != nil {
		remoteWorkspaceStoreMu.Unlock()
		return remoteWorkspaceProfile{}, filetransferprofile.Profile{}, err
	}
	_, workspace, ok := findRemoteWorkspace(items, id)
	remoteWorkspaceStoreMu.Unlock()
	if !ok {
		return remoteWorkspaceProfile{}, filetransferprofile.Profile{}, errors.New("remote workspace not found")
	}
	workspace, err = normalizeRemoteWorkspaceProfile(s, workspace)
	if err != nil {
		return remoteWorkspaceProfile{}, filetransferprofile.Profile{}, err
	}
	profile, err := s.resolveFileTransferProfile(workspace.TransferProfileID)
	if err != nil {
		return remoteWorkspaceProfile{}, filetransferprofile.Profile{}, err
	}
	if profile.Protocol != filetransferprofile.ProtocolSFTP || profile.SSHProfileID != workspace.SSHProfileID {
		return remoteWorkspaceProfile{}, filetransferprofile.Profile{}, errors.New("remote workspace SFTP/SSH linkage is invalid")
	}
	return workspace, profile, nil
}

func (s *Server) listRemoteWorkspacePath(r *http.Request, workspace remoteWorkspaceProfile, profile filetransferprofile.Profile, relative string) (string, []fileTransferEntry, error) {
	remotePath, err := remoteWorkspaceResolvePath(workspace.RemoteRoot, relative)
	if err != nil {
		return "", nil, err
	}
	if _, err := s.canonicalRemoteWorkspacePath(r.Context(), workspace, profile, remotePath); err != nil {
		return "", nil, err
	}
	command, err := sftpclient.ListCommand(remotePath)
	if err != nil {
		return "", nil, err
	}
	result, err := s.runSFTP(r.Context(), profile, command+"quit\n", sftpclient.DefaultMaxOutputBytes)
	if err != nil {
		return "", nil, err
	}
	items, err := sftpclient.ParseLongList(result.Stdout)
	if err != nil {
		return "", nil, err
	}
	entries := make([]fileTransferEntry, 0, len(items))
	for _, item := range items {
		entry := fileTransferEntry(item)
		if entry.Name == "." || entry.Name == ".." {
			continue
		}
		entries = append(entries, entry)
	}
	return remotePath, entries, nil
}

func (s *Server) remoteWorkspaceFilesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req remoteWorkspaceFileRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectEditableLimit+(128<<10)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid remote workspace file request", http.StatusBadRequest)
		return
	}
	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	req.Operation = strings.ToLower(strings.TrimSpace(req.Operation))
	req.Path = strings.TrimSpace(req.Path)
	req.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(req.ExpectedSHA256))

	permission := identity.PermissionFilesRead
	transferPermission := identity.PermissionTransferRead
	switch req.Operation {
	case "list", "read":
	case "write":
		permission = identity.PermissionFilesWrite
		transferPermission = identity.PermissionTransferUpload
	default:
		http.Error(w, "unsupported remote workspace file operation", http.StatusBadRequest)
		return
	}
	resource := "remote_workspace:" + req.WorkspaceID
	if !s.requireSharedActionPermission(w, r, identity.PermissionSSHUse, "remote_workspace."+req.Operation, resource) {
		return
	}
	if !s.requireSharedActionPermission(w, r, transferPermission, "remote_workspace."+req.Operation, resource) {
		return
	}
	if !s.requireSharedActionPermission(w, r, permission, "remote_workspace."+req.Operation, resource) {
		return
	}

	workspace, profile, err := s.resolveRemoteWorkspace(req.WorkspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	remotePath, err := remoteWorkspaceResolvePath(workspace.RemoteRoot, req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Operation == "read" || req.Operation == "write" {
		if _, err := s.canonicalRemoteWorkspacePath(r.Context(), workspace, profile, remotePath); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}

	switch req.Operation {
	case "list":
		actualPath, entries, err := s.listRemoteWorkspacePath(r, workspace, profile, req.Path)
		if err != nil {
			s.auditSharedResult(r, "remote_workspace.list", "remote_workspace", workspace.ID, "error", nil)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		s.auditSharedSuccess(r, "remote_workspace.list", "remote_workspace", workspace.ID, nil)
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_id": workspace.ID,
			"path":         req.Path,
			"remote_path":  actualPath,
			"entries":      entries,
		})
	case "read":
		if req.Path == "" || req.Path == "." {
			http.Error(w, "file path is required", http.StatusBadRequest)
			return
		}
		content, err := s.readFileTransferText(r.Context(), profile, remotePath)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, errFileTransferTextTooLarge) {
				status = http.StatusRequestEntityTooLarge
			} else if errors.Is(err, errFileTransferTextUnsupported) {
				status = http.StatusUnsupportedMediaType
			}
			s.auditSharedResult(r, "remote_workspace.read", "remote_workspace", workspace.ID, "error", nil)
			http.Error(w, err.Error(), status)
			return
		}
		sha, err := s.backgroundRemoteSHA256(r.Context(), profile.ID, remotePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		s.auditSharedSuccess(r, "remote_workspace.read", "remote_workspace", workspace.ID, nil)
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_id": workspace.ID,
			"path":         req.Path,
			"content":      string(content),
			"sha256":       sha,
		})
	case "write":
		if req.Path == "" || req.Path == "." || !validCompareSHA256(req.ExpectedSHA256) {
			http.Error(w, "file path and expected_sha256 are required", http.StatusBadRequest)
			return
		}
		content := []byte(req.Content)
		if int64(len(content)) > projectEditableLimit {
			http.Error(w, errFileTransferTextTooLarge.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		if !projectTextBytesValid(content) {
			http.Error(w, errFileTransferTextUnsupported.Error(), http.StatusUnsupportedMediaType)
			return
		}
		currentSHA, err := s.backgroundRemoteSHA256(r.Context(), profile.ID, remotePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if !strings.EqualFold(currentSHA, req.ExpectedSHA256) {
			http.Error(w, "remote file changed after editor load", http.StatusConflict)
			return
		}
		if err := s.writeFileTransferText(r.Context(), profile, remotePath, content); err != nil {
			s.auditSharedResult(r, "remote_workspace.write", "remote_workspace", workspace.ID, "error", nil)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writtenSHA, err := s.backgroundRemoteSHA256(r.Context(), profile.ID, remotePath)
		if err != nil {
			http.Error(w, "remote write verification failed", http.StatusBadGateway)
			return
		}
		sum := sha256.Sum256(content)
		expectedWrittenSHA := hex.EncodeToString(sum[:])
		if !strings.EqualFold(writtenSHA, expectedWrittenSHA) {
			s.auditSharedResult(r, "remote_workspace.write", "remote_workspace", workspace.ID, "error", nil)
			http.Error(w, "remote write verification hash mismatch", http.StatusBadGateway)
			return
		}
		s.auditSharedSuccess(r, "remote_workspace.write", "remote_workspace", workspace.ID, nil)
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_id": workspace.ID,
			"path":         req.Path,
			"sha256":       writtenSHA,
			"content":      req.Content,
		})
	}
}
