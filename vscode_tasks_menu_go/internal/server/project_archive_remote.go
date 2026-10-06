package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
	"bletonfc/vscode_tasks_menu/internal/sshaskpass"
	"bletonfc/vscode_tasks_menu/internal/sshclient"
	"bletonfc/vscode_tasks_menu/internal/state"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

type projectArchiveRemoteExtractRequest struct {
	ProfileID string `json:"profile_id"`
	HostPath string `json:"host_path"`
	RemoteArchive string `json:"remote_archive"`
	RemoteDestination string `json:"remote_destination"`
}

func remoteArchiveShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (s *Server) runSFTPLinkedSSHCommand(ctx context.Context, profile filetransferprofile.Profile, remoteCommand string) (string, error) {
	sshStore, err := s.sshProfileStore()
	if err != nil {
		return "", err
	}
	sshProfile, err := sshStore.Get(profile.SSHProfileID)
	if err != nil {
		return "", errors.New("referenced SSH profile not found")
	}
	executable, err := sshclient.FindOpenSSH()
	if err != nil {
		return "", err
	}
	command, err := sshclient.BuildProbeCommand(executable, sshProfile)
	if err != nil {
		return "", err
	}
	if len(command.Args) == 0 || command.Args[len(command.Args)-1] != "true" {
		return "", errors.New("unexpected SSH probe command shape")
	}
	command.Args[len(command.Args)-1] = remoteCommand

	spec := tasks.Execution{Env: os.Environ()}
	var ticket *sshaskpass.Ticket
	if sshProfile.SecretRef != "" {
		secrets, err := s.connectionSecretStore()
		if err != nil {
			return "", err
		}
		ticket, err = sshaskpass.Prepare(secrets, sshProfile.SecretRef, state.Dir(s.Workspace))
		if err != nil {
			return "", err
		}
		defer ticket.Close()
		helper, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("resolve TaskDeck askpass helper: %w", err)
		}
		if err := tasks.ApplyEnvironmentOverrides(&spec, map[string]string{
			"DISPLAY": "taskdeck-ssh-askpass",
			"SSH_ASKPASS": helper,
			"SSH_ASKPASS_REQUIRE": "force",
			"TASKDECK_SSH_ASKPASS": "1",
			"TASKDECK_SSH_ASKPASS_SOCKET": ticket.SocketPath,
			"TASKDECK_SSH_ASKPASS_TOKEN": ticket.Token,
		}); err != nil {
			return "", err
		}
	}

	timeout := time.Duration(sshProfile.ConnectTimeoutSeconds+120) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command.Executable, command.Args...)
	cmd.Env = spec.Env
	cmd.Dir = s.Workspace
	output := &boundedCommandOutput{remaining: 64 << 10}
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Run()
	if runCtx.Err() == context.DeadlineExceeded {
		return output.String(), errors.New("remote archive extraction timed out")
	}
	if err != nil {
		if text := output.String(); text != "" {
			return text, errors.New(text)
		}
		return "", err
	}
	return output.String(), nil
}

func remoteArchiveExtractCommand(format, archivePath, destination string) (string, error) {
	archive := remoteArchiveShellQuote(archivePath)
	dest := remoteArchiveShellQuote(destination)
	prefix := "set -eu; test ! -e " + dest + "; mkdir -p -- " + dest + "; "
	switch format {
	case "zip":
		return prefix + "command -v unzip >/dev/null 2>&1; unzip -q -- " + archive + " -d " + dest, nil
	case "tar.gz":
		return prefix + "command -v tar >/dev/null 2>&1; tar -xzf " + archive + " -C " + dest + " --no-same-owner --no-same-permissions", nil
	default:
		return "", errors.New("unsupported remote archive format")
	}
}

func (s *Server) projectArchiveRemoteExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectArchiveRemoteExtractRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.HostPath = strings.TrimSpace(req.HostPath)
	req.RemoteArchive = strings.TrimSpace(req.RemoteArchive)
	req.RemoteDestination = strings.TrimSpace(req.RemoteDestination)
	if req.ProfileID == "" || req.HostPath == "" || req.RemoteArchive == "" || req.RemoteDestination == "" {
		http.Error(w, "profile_id, host_path, remote_archive and remote_destination are required", http.StatusBadRequest)
		return
	}
	if strings.ContainsAny(req.RemoteArchive+req.RemoteDestination, "\x00\r\n") {
		http.Error(w, "remote paths must not contain NUL or line breaks", http.StatusBadRequest)
		return
	}
	format := archiveFormatFromPath(req.HostPath)
	if format == "" {
		http.Error(w, "host_path must reference a .zip, .tar.gz, or .tgz archive", http.StatusBadRequest)
		return
	}
	hostPath, err := s.resolveProjectPath(req.HostPath, false, false)
	if err != nil {
		http.Error(w, "project archive not found", http.StatusNotFound)
		return
	}
	if _, err := previewProjectArchive(hostPath, req.HostPath, format); err != nil {
		http.Error(w, "archive failed safe preflight: "+err.Error(), http.StatusBadRequest)
		return
	}
	profile, err := s.resolveFileTransferProfile(req.ProfileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if profile.Protocol != filetransferprofile.ProtocolSFTP {
		http.Error(w, "remote extract requires an SFTP profile linked to SSH; FTP has no portable remote shell extraction capability", http.StatusBadRequest)
		return
	}

	put, err := sftpclient.PutCommand(hostPath, req.RemoteArchive)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.runSFTP(r.Context(), profile, put+"quit\n", 64<<10); err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind:"file_transfer", Action:"archive_upload_extract", ProfileID:profile.ID, Success:false})
		http.Error(w, "archive upload failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	command, err := remoteArchiveExtractCommand(format, req.RemoteArchive, req.RemoteDestination)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	output, err := s.runSFTPLinkedSSHCommand(r.Context(), profile, command)
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind:"file_transfer", Action:"archive_upload_extract", ProfileID:profile.ID, Success:false})
		http.Error(w, "remote extract failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind:"file_transfer", Action:"archive_upload_extract", ProfileID:profile.ID, Success:true})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"profile_id": profile.ID,
		"format": format,
		"remote_archive": req.RemoteArchive,
		"remote_destination": req.RemoteDestination,
		"output": output,
	})
}
