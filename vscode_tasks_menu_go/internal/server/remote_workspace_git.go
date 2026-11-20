package server

import (
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "strings"

    "bletonfc/vscode_tasks_menu/internal/filetransferprofile"
    "bletonfc/vscode_tasks_menu/internal/identity"
)

type remoteWorkspaceGitRequest struct {
    WorkspaceID string `json:"workspace_id"`
    Action      string `json:"action"`
    Path        string `json:"path,omitempty"`
    Message     string `json:"message,omitempty"`
}

func remoteWorkspaceGitPermission(action string) (string, error) {
    switch action {
    case "status":
        return identity.PermissionGitStatus, nil
    case "branches", "log":
        return identity.PermissionGitLog, nil
    case "diff", "diff_staged":
        return identity.PermissionGitDiff, nil
    case "fetch", "pull", "stage", "unstage", "commit":
        return identity.PermissionGitWrite, nil
    case "push":
        return identity.PermissionGitPush, nil
    default:
        return "", errors.New("unsupported remote Git action")
    }
}

// Do not silently normalize user-supplied paths or accept optional arguments
// on an unrelated action. Stage/unstage paths are literal Git pathspecs, not
// shell snippets, directory traversal, or Git wildcard expressions.
func validateRemoteWorkspaceGitRequest(req *remoteWorkspaceGitRequest) error {
    req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
    req.Action = strings.ToLower(strings.TrimSpace(req.Action))
    if req.WorkspaceID == "" {
        return errors.New("workspace_id is required")
    }
    if _, err := remoteWorkspaceGitPermission(req.Action); err != nil {
        return err
    }
    switch req.Action {
    case "stage", "unstage":
        if req.Message != "" || req.Path == "" || len(req.Path) > 2048 {
            return errors.New("stage/unstage requires one relative path and no commit message")
        }
        cleaned, err := remoteWorkspaceRelativePath(req.Path)
        if err != nil || cleaned == "." || cleaned != req.Path ||
            strings.ContainsAny(cleaned, "\x00\r\n\t") {
            return errors.New("Git file path must be a canonical relative path")
        }
    case "commit":
        if req.Path != "" || req.Message == "" || len(req.Message) > 2000 ||
            strings.TrimSpace(req.Message) != req.Message ||
            strings.ContainsAny(req.Message, "\x00\r\n") {
            return errors.New("commit requires a one-line message of at most 2000 bytes")
        }
    default:
        if req.Path != "" || req.Message != "" {
            return errors.New("this Git action does not accept a path or commit message")
        }
    }
    return nil
}

func remoteWorkspaceGitCommand(root, action string) (string, error) {
    root = remoteArchiveShellQuote(root)
    prefix := "set -eu; command -v git >/dev/null 2>&1; git -C " + root + " "
    switch action {
    case "status":
        return prefix + "status --short --branch", nil
    case "branches":
        return prefix + "branch --all --verbose --no-abbrev", nil
    case "log":
        return prefix + "log --oneline --decorate -n 50", nil
    case "diff":
        return prefix + "diff --no-ext-diff --", nil
    case "diff_staged":
        return prefix + "diff --cached --no-ext-diff --", nil
    case "fetch":
        return prefix + "fetch --progress", nil
    case "pull":
        return prefix + "pull --ff-only", nil
    case "push":
        return prefix + "push --progress", nil
    default:
        return "", errors.New("unsupported remote Git action")
    }
}

func remoteWorkspaceGitCommandForRequest(root string, req remoteWorkspaceGitRequest) (string, error) {
    if err := validateRemoteWorkspaceGitRequest(&req); err != nil {
        return "", err
    }
    switch req.Action {
    case "stage", "unstage":
        // The :(literal) magic prevents Git pathspec glob expansion and
        // user-supplied :(top), :(exclude) or other pathspec modifiers.
        literalPath := remoteArchiveShellQuote(":(literal)" + req.Path)
        prefix := "set -eu; command -v git >/dev/null 2>&1; git -C " +
            remoteArchiveShellQuote(root) + " "
        if req.Action == "stage" {
            return prefix + "add -A -- " + literalPath, nil
        }
        return prefix + "restore --staged -- " + literalPath, nil
    case "commit":
        // Preserve Git's normal hooks/signing. Do not construct a shell
        // command from unquoted input or add remote shell escape features.
        return "set -eu; command -v git >/dev/null 2>&1; git -C " +
            remoteArchiveShellQuote(root) + " commit -m " +
            remoteArchiveShellQuote(req.Message), nil
    default:
        return remoteWorkspaceGitCommand(root, req.Action)
    }
}

func (s *Server) remoteWorkspaceGitAPI(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    var req remoteWorkspaceGitRequest
    decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
    decoder.DisallowUnknownFields()
    if err := decoder.Decode(&req); err != nil {
        http.Error(w, "invalid remote Git request", http.StatusBadRequest)
        return
    }
    var extra any
    if err := decoder.Decode(&extra); err != io.EOF {
        http.Error(w, "trailing data in remote Git request", http.StatusBadRequest)
        return
    }
    if err := validateRemoteWorkspaceGitRequest(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    permission, _ := remoteWorkspaceGitPermission(req.Action)
    resource := "remote_workspace:" + req.WorkspaceID
    if !s.requireSharedActionPermission(w, r, identity.PermissionSSHUse, "remote_git."+req.Action, resource) {
        return
    }
    if !s.requireSharedActionPermission(w, r, permission, "remote_git."+req.Action, resource) {
        return
    }

    workspace, profile, err := s.resolveRemoteWorkspace(req.WorkspaceID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    if profile.Protocol != filetransferprofile.ProtocolSFTP ||
        profile.SSHProfileID != workspace.SSHProfileID {
        http.Error(w, "remote workspace SFTP/SSH linkage is invalid", http.StatusConflict)
        return
    }
    command, err := remoteWorkspaceGitCommandForRequest(workspace.RemoteRoot, req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    output, err := s.runSFTPLinkedSSHCommand(r.Context(), profile, command)
    if err != nil {
        s.auditSharedResult(r, "remote_git."+req.Action, "remote_workspace", workspace.ID, "error", nil)
        http.Error(w, err.Error(), http.StatusBadGateway)
        return
    }
    s.auditSharedSuccess(r, "remote_git."+req.Action, "remote_workspace", workspace.ID, nil)
    writeJSON(w, http.StatusOK, map[string]any{
        "workspace_id": workspace.ID,
        "action": req.Action,
        "remote_root": workspace.RemoteRoot,
        "output": output,
    })
}
