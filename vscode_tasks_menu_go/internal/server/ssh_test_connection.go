package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshaskpass"
	"bletonfc/vscode_tasks_menu/internal/sshclient"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
	"bletonfc/vscode_tasks_menu/internal/state"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

const maxSSHTestOutputBytes = 16 << 10

type boundedCommandOutput struct {
	buf       bytes.Buffer
	remaining int
	truncated bool
}

func (w *boundedCommandOutput) Write(p []byte) (int, error) {
	original := len(p)
	if w.remaining <= 0 {
		w.truncated = w.truncated || original > 0
		return original, nil
	}
	if len(p) > w.remaining {
		p = p[:w.remaining]
		w.truncated = true
	}
	_, _ = w.buf.Write(p)
	w.remaining -= len(p)
	return original, nil
}

func (w *boundedCommandOutput) String() string {
	text := strings.TrimSpace(w.buf.String())
	if w.truncated {
		if text != "" {
			text += "\n"
		}
		text += "[output truncated]"
	}
	return text
}

type sshConnectionTestResult struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type sshEphemeralSecretStore struct {
	ref    string
	secret []byte
}

func (s *sshEphemeralSecretStore) Put(string, []byte) error {
	return errors.New("ssh test secret store is read-only")
}

func (s *sshEphemeralSecretStore) Delete(string) error {
	return nil
}

func (s *sshEphemeralSecretStore) Get(id string) ([]byte, error) {
	if s == nil || id != s.ref {
		return nil, secretstore.ErrNotFound
	}
	return append([]byte(nil), s.secret...), nil
}

func (s *Server) testSSHProfile(profileID string) sshConnectionTestResult {
	store, err := s.sshProfileStore()
	if err != nil {
		return sshConnectionTestResult{Message: err.Error()}
	}
	profile, err := store.Get(strings.TrimSpace(profileID))
	if err != nil {
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
			return sshConnectionTestResult{Message: "ssh profile not found"}
		}
		return sshConnectionTestResult{Message: err.Error()}
	}
	return s.testSSHConnection(profile, nil)
}

func (s *Server) testSSHProfileOverride(profileID string, req sshProfileRequest) sshConnectionTestResult {
	store, err := s.sshProfileStore()
	if err != nil {
		return sshConnectionTestResult{Message: err.Error()}
	}
	existing, err := store.Get(strings.TrimSpace(profileID))
	if err != nil {
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
			return sshConnectionTestResult{Message: "ssh profile not found"}
		}
		return sshConnectionTestResult{Message: err.Error()}
	}

	secretRef := ""
	if req.Secret == nil && req.AuthMethod == existing.AuthMethod {
		secretRef = existing.SecretRef
	}
	candidate := req.profile("test", secretRef)
	if strings.TrimSpace(candidate.Name) == "" {
		candidate.Name = "Connection test"
	}
	return s.testSSHConnection(candidate, req.Secret)
}

func (s *Server) testSSHConnection(profile sshprofile.Profile, secret *string) sshConnectionTestResult {
	started := time.Now()
	result := sshConnectionTestResult{}

	var secrets secretstore.Store
	if secret != nil {
		if err := validateSSHAuthenticationSecret(*secret); err != nil {
			result.Message = err.Error()
			return result
		}
		if profile.AuthMethod == sshprofile.AuthAgent {
			result.Message = "agent authentication must not include a secret"
			return result
		}
		profile.SecretRef = "ssh-test/ephemeral"
		secrets = &sshEphemeralSecretStore{ref: profile.SecretRef, secret: []byte(*secret)}
	}

	normalized, err := sshprofile.Normalize(profile)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	profile = normalized

	executable, err := sshclient.FindOpenSSH()
	if err != nil {
		result.Message = err.Error()
		return result
	}
	command, err := sshclient.BuildProbeCommand(executable, profile)
	if err != nil {
		result.Message = err.Error()
		return result
	}

	spec := tasks.Execution{Env: os.Environ()}
	var ticket *sshaskpass.Ticket
	if profile.SecretRef != "" {
		if secrets == nil {
			secrets, err = s.connectionSecretStore()
			if err != nil {
				result.Message = err.Error()
				return result
			}
		}
		ticket, err = sshaskpass.Prepare(secrets, profile.SecretRef, state.Dir(s.Workspace))
		if err != nil {
			result.Message = err.Error()
			return result
		}
		defer ticket.Close()

		helper, helperErr := os.Executable()
		if helperErr != nil {
			result.Message = fmt.Sprintf("resolve TaskDeck askpass helper: %v", helperErr)
			return result
		}
		if envErr := tasks.ApplyEnvironmentOverrides(&spec, map[string]string{
			"DISPLAY":                     "taskdeck-ssh-askpass",
			"SSH_ASKPASS":                 helper,
			"SSH_ASKPASS_REQUIRE":         "force",
			"TASKDECK_SSH_ASKPASS":        "1",
			"TASKDECK_SSH_ASKPASS_SOCKET": ticket.SocketPath,
			"TASKDECK_SSH_ASKPASS_TOKEN":  ticket.Token,
		}); envErr != nil {
			result.Message = envErr.Error()
			return result
		}
	}

	timeout := time.Duration(profile.ConnectTimeoutSeconds+5) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command.Executable, command.Args...)
	cmd.Env = spec.Env
	cmd.Dir = s.Workspace
	output := &boundedCommandOutput{remaining: maxSSHTestOutputBytes}
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Run()
	result.ElapsedMS = time.Since(started).Milliseconds()

	if ctx.Err() == context.DeadlineExceeded {
		result.Message = "SSH connection test timed out"
		return result
	}
	if err != nil {
		message := output.String()
		if message == "" {
			message = err.Error()
		}
		result.Message = message
		return result
	}

	result.OK = true
	result.Message = "SSH connection succeeded"
	return result
}

func (s *Server) sshTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ProfileID string             `json:"profile_id,omitempty"`
		Profile   *sshProfileRequest `json:"profile,omitempty"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	profileID := strings.TrimSpace(req.ProfileID)
	var result sshConnectionTestResult
	switch {
	case req.Profile != nil && profileID != "":
		result = s.testSSHProfileOverride(profileID, *req.Profile)
	case req.Profile != nil:
		candidate := req.Profile.profile("test", "")
		if strings.TrimSpace(candidate.Name) == "" {
			candidate.Name = "Connection test"
		}
		result = s.testSSHConnection(candidate, req.Profile.Secret)
	case profileID != "":
		result = s.testSSHProfile(profileID)
	default:
		http.Error(w, "ssh profile id or profile is required", http.StatusBadRequest)
		return
	}

	s.auditConnection(r, ConnectionAuditEvent{
		Kind:      "ssh_connection",
		Action:    "test",
		ProfileID: profileID,
		Success:   result.OK,
	})
	writeJSON(w, http.StatusOK, result)
}
