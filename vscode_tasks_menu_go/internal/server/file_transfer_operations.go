package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/ftpclient"
	"bletonfc/vscode_tasks_menu/internal/sftpclient"
	"bletonfc/vscode_tasks_menu/internal/sshaskpass"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
	"bletonfc/vscode_tasks_menu/internal/state"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

type fileTransferOperationRequest struct {
	ProfileID string                      `json:"profile_id"`
	Path      string                      `json:"path,omitempty"`
	Profile   *fileTransferProfileRequest `json:"profile,omitempty"`
}

type fileTransferTestResult struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type fileTransferEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

func decodeFileTransferOperationRequest(w http.ResponseWriter, r *http.Request) (fileTransferOperationRequest, error) {
	var req fileTransferOperationRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return fileTransferOperationRequest{}, errors.New("invalid JSON")
	}
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	req.Path = strings.TrimSpace(req.Path)
	return req, nil
}

func (s *Server) resolveFileTransferProfile(profileID string) (filetransferprofile.Profile, error) {
	store, err := s.fileTransferProfileStore()
	if err != nil {
		return filetransferprofile.Profile{}, err
	}
	profile, err := store.Get(strings.TrimSpace(profileID))
	if err != nil {
		if errors.Is(err, filetransferprofile.ErrProfileNotFound) {
			return filetransferprofile.Profile{}, errors.New("file-transfer profile not found")
		}
		return filetransferprofile.Profile{}, err
	}
	return profile, nil
}

func (s *Server) resolveFileTransferTestProfile(req fileTransferOperationRequest) (filetransferprofile.Profile, *string, error) {
	if req.Profile == nil {
		if req.ProfileID == "" {
			return filetransferprofile.Profile{}, nil, errors.New("file-transfer profile id or profile is required")
		}
		profile, err := s.resolveFileTransferProfile(req.ProfileID)
		return profile, nil, err
	}
	draft := *req.Profile
	id := "test"
	var secretRef string
	var ephemeralSecret *string
	if draft.Protocol == filetransferprofile.ProtocolFTP {
		if draft.Secret != nil {
			if err := validateFileTransferSecret(*draft.Secret); err != nil {
				return filetransferprofile.Profile{}, nil, err
			}
			ephemeralSecret = draft.Secret
		} else if req.ProfileID != "" {
			existing, err := s.resolveFileTransferProfile(req.ProfileID)
			if err != nil {
				return filetransferprofile.Profile{}, nil, err
			}
			if existing.Protocol == filetransferprofile.ProtocolFTP {
				secretRef = existing.SecretRef
			}
		}
	}
	profile, err := filetransferprofile.Normalize(draft.profile(id, secretRef))
	if err != nil {
		return filetransferprofile.Profile{}, nil, err
	}
	if err := s.validateFileTransferProfileReference(profile); err != nil {
		return filetransferprofile.Profile{}, nil, err
	}
	return profile, ephemeralSecret, nil
}

func (s *Server) ftpPassword(profile filetransferprofile.Profile, ephemeral *string) (string, error) {
	if ephemeral != nil {
		return *ephemeral, nil
	}
	if profile.SecretRef == "" {
		return "", nil
	}
	secrets, err := s.connectionSecretStore()
	if err != nil {
		return "", err
	}
	value, err := secrets.Get(profile.SecretRef)
	if err != nil {
		return "", fmt.Errorf("load ftp password: %w", err)
	}
	return string(value), nil
}

func (s *Server) withFTPClient(ctx context.Context, profile filetransferprofile.Profile, ephemeral *string, fn func(*ftpclient.Client) error) error {
	password, err := s.ftpPassword(profile, ephemeral)
	if err != nil {
		return err
	}
	timeout := time.Duration(profile.ConnectTimeoutSeconds) * time.Second
	address := net.JoinHostPort(profile.Host, fmt.Sprintf("%d", profile.Port))
	client, err := ftpclient.Dial(ctx, address, profile.Username, password, timeout)
	if err != nil {
		return err
	}
	defer client.Close()
	return fn(client)
}

func (s *Server) sftpExecutable() (string, error) {
	if strings.TrimSpace(s.SFTPExecutable) != "" {
		return s.SFTPExecutable, nil
	}
	return sftpclient.FindOpenSFTP()
}

func (s *Server) runSFTP(ctx context.Context, profile filetransferprofile.Profile, commands string, maxOutput int) (sftpclient.Result, error) {
	sshStore, err := s.sshProfileStore()
	if err != nil {
		return sftpclient.Result{}, err
	}
	sshProfile, err := sshStore.Get(profile.SSHProfileID)
	if err != nil {
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
			return sftpclient.Result{}, errors.New("referenced SSH profile not found")
		}
		return sftpclient.Result{}, err
	}
	executable, err := s.sftpExecutable()
	if err != nil {
		return sftpclient.Result{}, err
	}
	command, err := sftpclient.BuildCommand(executable, sshProfile)
	if err != nil {
		return sftpclient.Result{}, err
	}

	envSpec := tasks.Execution{Env: os.Environ()}
	var ticket *sshaskpass.Ticket
	if sshProfile.SecretRef != "" {
		secrets, err := s.connectionSecretStore()
		if err != nil {
			return sftpclient.Result{}, err
		}
		ticket, err = sshaskpass.Prepare(secrets, sshProfile.SecretRef, state.Dir(s.Workspace))
		if err != nil {
			return sftpclient.Result{}, err
		}
		defer ticket.Close()
		helper, err := os.Executable()
		if err != nil {
			return sftpclient.Result{}, fmt.Errorf("resolve TaskDeck askpass helper: %w", err)
		}
		if err := tasks.ApplyEnvironmentOverrides(&envSpec, map[string]string{
			"DISPLAY":                     "taskdeck-ssh-askpass",
			"SSH_ASKPASS":                 helper,
			"SSH_ASKPASS_REQUIRE":         "force",
			"TASKDECK_SSH_ASKPASS":        "1",
			"TASKDECK_SSH_ASKPASS_SOCKET": ticket.SocketPath,
			"TASKDECK_SSH_ASKPASS_TOKEN":  ticket.Token,
		}); err != nil {
			return sftpclient.Result{}, err
		}
	}
	timeout := time.Duration(sshProfile.ConnectTimeoutSeconds+30) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return sftpclient.Run(runCtx, command, commands, envSpec.Env, s.Workspace, maxOutput)
}

func (s *Server) testFileTransferProfile(ctx context.Context, profile filetransferprofile.Profile, ephemeral *string) error {
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		return s.withFTPClient(ctx, profile, ephemeral, func(client *ftpclient.Client) error {
			_, err := client.Pwd(ctx)
			return err
		})
	case filetransferprofile.ProtocolSFTP:
		_, err := s.runSFTP(ctx, profile, sftpclient.PwdCommand()+"quit\n", 64<<10)
		return err
	default:
		return fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
}

func (s *Server) fileTransferTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeFileTransferOperationRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	profile, ephemeral, err := s.resolveFileTransferTestProfile(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	started := time.Now()
	err = s.testFileTransferProfile(r.Context(), profile, ephemeral)
	result := fileTransferTestResult{ElapsedMS: time.Since(started).Milliseconds()}
	if err != nil {
		result.Message = err.Error()
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer_connection", Action: "test", ProfileID: req.ProfileID, Success: false})
		writeJSON(w, http.StatusOK, result)
		return
	}
	result.OK = true
	result.Message = strings.ToUpper(string(profile.Protocol)) + " connection succeeded"
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer_connection", Action: "test", ProfileID: req.ProfileID, Success: true})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) fileTransferList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeFileTransferOperationRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ProfileID == "" || req.Profile != nil {
		http.Error(w, "saved file-transfer profile id is required", http.StatusBadRequest)
		return
	}
	profile, err := s.resolveFileTransferProfile(req.ProfileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	path := req.Path
	if path == "" {
		path = profile.InitialPath
	}
	var entries []fileTransferEntry
	switch profile.Protocol {
	case filetransferprofile.ProtocolFTP:
		err = s.withFTPClient(r.Context(), profile, nil, func(client *ftpclient.Client) error {
			items, listErr := client.List(r.Context(), path)
			if listErr != nil {
				return listErr
			}
			entries = make([]fileTransferEntry, 0, len(items))
			for _, item := range items {
				entries = append(entries, fileTransferEntry(item))
			}
			return nil
		})
	case filetransferprofile.ProtocolSFTP:
		command, commandErr := sftpclient.ListCommand(path)
		if commandErr != nil {
			err = commandErr
			break
		}
		result, runErr := s.runSFTP(r.Context(), profile, command+"quit\n", sftpclient.DefaultMaxOutputBytes)
		if runErr != nil {
			err = runErr
			break
		}
		items, parseErr := sftpclient.ParseLongList(result.Stdout)
		if parseErr != nil {
			err = parseErr
			break
		}
		entries = make([]fileTransferEntry, 0, len(items))
		for _, item := range items {
			entries = append(entries, fileTransferEntry(item))
		}
	default:
		err = fmt.Errorf("unsupported file-transfer protocol %q", profile.Protocol)
	}
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "list", ProfileID: profile.ID, Success: false})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer", Action: "list", ProfileID: profile.ID, Success: true})
	writeJSON(w, http.StatusOK, map[string]any{
		"profile_id": profile.ID,
		"protocol":   profile.Protocol,
		"path":       path,
		"entries":    entries,
	})
}
