package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

type remoteWorkspaceGitRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Action      string `json:"action"`
}

func remoteWorkspaceGitPermission(action string) (string,error) {
	switch action {
	case "status": return identity.PermissionGitStatus,nil
	case "branches","log": return identity.PermissionGitLog,nil
	case "diff": return identity.PermissionGitDiff,nil
	case "fetch","pull": return identity.PermissionGitWrite,nil
	case "push": return identity.PermissionGitPush,nil
	default:return "",errors.New("unsupported remote Git action")
	}
}

func remoteWorkspaceGitCommand(root,action string) (string,error) {
	root=remoteArchiveShellQuote(root)
	prefix:="set -eu; command -v git >/dev/null 2>&1; git -C "+root+" "
	switch action {
	case "status":return prefix+"status --short --branch",nil
	case "branches":return prefix+"branch --all --verbose --no-abbrev",nil
	case "log":return prefix+"log --oneline --decorate -n 50",nil
	case "diff":return prefix+"diff --no-ext-diff --",nil
	case "fetch":return prefix+"fetch --progress",nil
	case "pull":return prefix+"pull --ff-only",nil
	case "push":return prefix+"push --progress",nil
	default:return "",errors.New("unsupported remote Git action")
	}
}

func (s *Server) remoteWorkspaceGitAPI(w http.ResponseWriter,r *http.Request) {
	if r.Method!=http.MethodPost{http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
	var req remoteWorkspaceGitRequest
	decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,64<<10));decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&req);err!=nil{http.Error(w,"invalid remote Git request",http.StatusBadRequest);return}
	req.WorkspaceID=strings.TrimSpace(req.WorkspaceID);req.Action=strings.ToLower(strings.TrimSpace(req.Action))
	if req.WorkspaceID==""{http.Error(w,"workspace_id is required",http.StatusBadRequest);return}
	permission,err:=remoteWorkspaceGitPermission(req.Action);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
	if !s.requireSharedActionPermission(w,r,identity.PermissionSSHUse,"remote_git."+req.Action,"remote_workspace:"+req.WorkspaceID){return}
	if !s.requireSharedActionPermission(w,r,permission,"remote_git."+req.Action,"remote_workspace:"+req.WorkspaceID){return}

	remoteWorkspaceStoreMu.Lock()
	items,err:=readRemoteWorkspaceProfiles()
	if err!=nil{remoteWorkspaceStoreMu.Unlock();http.Error(w,err.Error(),http.StatusInternalServerError);return}
	_,workspace,ok:=findRemoteWorkspace(items,req.WorkspaceID)
	remoteWorkspaceStoreMu.Unlock()
	if !ok{http.NotFound(w,r);return}
	workspace,err=normalizeRemoteWorkspaceProfile(s,workspace);if err!=nil{http.Error(w,err.Error(),http.StatusConflict);return}
	profile,err:=s.resolveFileTransferProfile(workspace.TransferProfileID)
	if err!=nil{http.Error(w,err.Error(),http.StatusNotFound);return}
	if profile.Protocol!=filetransferprofile.ProtocolSFTP||profile.SSHProfileID!=workspace.SSHProfileID{
		http.Error(w,"remote workspace SFTP/SSH linkage is invalid",http.StatusConflict);return
	}
	command,err:=remoteWorkspaceGitCommand(workspace.RemoteRoot,req.Action);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
	output,err:=s.runSFTPLinkedSSHCommand(r.Context(),profile,command)
	if err!=nil{
		s.auditSharedResult(r,"remote_git."+req.Action,"remote_workspace",workspace.ID,"error",nil)
		http.Error(w,err.Error(),http.StatusBadGateway);return
	}
	s.auditSharedSuccess(r,"remote_git."+req.Action,"remote_workspace",workspace.ID,nil)
	writeJSON(w,http.StatusOK,map[string]any{
		"workspace_id":workspace.ID,"action":req.Action,"remote_root":workspace.RemoteRoot,"output":output,
	})
}
