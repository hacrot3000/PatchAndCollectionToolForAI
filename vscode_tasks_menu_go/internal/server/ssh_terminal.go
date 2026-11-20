package server

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/sshaskpass"
	"bletonfc/vscode_tasks_menu/internal/sshclient"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
	"bletonfc/vscode_tasks_menu/internal/state"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func (s *Server) sshTerminalExecution(profileID string) (tasks.Execution, error) {
	return s.sshTerminalExecutionAt(profileID, "")
}

func (s *Server) sshTerminalExecutionAt(profileID, remoteCWD string) (tasks.Execution, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return tasks.Execution{}, errors.New("ssh profile id is required")
	}
	store, err := s.sshProfileStore()
	if err != nil {
		return tasks.Execution{}, err
	}
	profile, err := store.Get(profileID)
	if err != nil {
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
			return tasks.Execution{}, fmt.Errorf("ssh profile not found")
		}
		return tasks.Execution{}, err
	}

	executable, err := sshclient.FindOpenSSH()
	if err != nil {
		return tasks.Execution{}, err
	}
	remoteCWD = strings.TrimSpace(remoteCWD)
	// Deliver the same Bash OSC 133 command-boundary hooks as local terminals
	// over SSH fd 3. The remote host's Bash rc and SSH authentication are not
	// changed, and other remote login shells retain their previous behavior.
	command, err := sshclient.BuildInstrumentedInteractiveCommand(executable, profile, remoteCWD, taskDeckSSHLoginBashRC())
	if err != nil {
		return tasks.Execution{}, err
	}

	rawArgs := make([]any, len(command.Args))
	for i, arg := range command.Args {
		rawArgs[i] = arg
	}
	spec, err := tasks.ResolveExecution(tasks.Task{
		ID:        -1,
		Label:     "SSH · " + profile.Name,
		MenuLabel: "SSH · " + profile.Name,
		Detail:    "Remote terminal " + command.Destination + func() string { if remoteCWD != "" { return " · " + remoteCWD }; return "" }(),
		Type:      "process",
		Command:   command.Executable,
		Args:      rawArgs,
	}, s.Workspace)
	if err != nil {
		return tasks.Execution{}, err
	}
	spec.TargetType = "ssh"
	spec.TargetProfileID = profile.ID
	// The remote RC is an implementation detail, not a user command. Never
	// dump its multi-line source into the terminal header or session metadata.
	spec.Preview = "SSH · " + command.Destination
	if remoteCWD != "" {
		spec.Preview += " · " + remoteCWD
	}
	spec.Preview += " · interactive"

	if profile.SecretRef != "" {
		secrets, err := s.connectionSecretStore()
		if err != nil {
			return tasks.Execution{}, err
		}
		ticket, err := sshaskpass.Prepare(secrets, profile.SecretRef, state.Dir(s.Workspace))
		if err != nil {
			return tasks.Execution{}, err
		}
		helper, err := os.Executable()
		if err != nil {
			ticket.Close()
			return tasks.Execution{}, fmt.Errorf("resolve TaskDeck askpass helper: %w", err)
		}
		if err := tasks.ApplyEnvironmentOverrides(&spec, map[string]string{
			"DISPLAY":                     "taskdeck-ssh-askpass",
			"SSH_ASKPASS":                 helper,
			"SSH_ASKPASS_REQUIRE":         "force",
			"TASKDECK_SSH_ASKPASS":        "1",
			"TASKDECK_SSH_ASKPASS_SOCKET": ticket.SocketPath,
			"TASKDECK_SSH_ASKPASS_TOKEN":  ticket.Token,
		}); err != nil {
			ticket.Close()
			return tasks.Execution{}, err
		}
	}
	return spec, nil
}
