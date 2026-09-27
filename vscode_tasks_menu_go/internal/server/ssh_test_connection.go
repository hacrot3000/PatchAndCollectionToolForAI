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

func (s *Server) testSSHProfile(profileID string) sshConnectionTestResult {
	started := time.Now()
	result := sshConnectionTestResult{}

	store, err := s.sshProfileStore()
	if err != nil {
		result.Message = err.Error()
		return result
	}
	profile, err := store.Get(strings.TrimSpace(profileID))
	if err != nil {
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
			result.Message = "ssh profile not found"
		} else {
			result.Message = err.Error()
		}
		return result
	}

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
		secrets, secretErr := s.connectionSecretStore()
		if secretErr != nil {
			result.Message = secretErr.Error()
			return result
		}
		ticket, secretErr = sshaskpass.Prepare(secrets, profile.SecretRef, state.Dir(s.Workspace))
		if secretErr != nil {
			result.Message = secretErr.Error()
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
		ProfileID string `json:"profile_id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ProfileID) == "" {
		http.Error(w, "ssh profile id is required", http.StatusBadRequest)
		return
	}
	result := s.testSSHProfile(req.ProfileID)
	s.auditConnection(r, ConnectionAuditEvent{
		Kind:      "ssh_connection",
		Action:    "test",
		ProfileID: strings.TrimSpace(req.ProfileID),
		Success:   result.OK,
	})
	writeJSON(w, http.StatusOK, result)
}
