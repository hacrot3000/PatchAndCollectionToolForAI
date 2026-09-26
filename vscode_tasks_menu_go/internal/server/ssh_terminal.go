package server

import (
	"errors"
	"fmt"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/sshclient"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func (s *Server) sshTerminalExecution(profileID string) (tasks.Execution, error) {
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

	// Stored password/private-key passphrase must not be placed on argv or in a
	// long-lived environment variable. Until the one-time askpass broker is
	// installed, fail closed instead of silently prompting for a saved secret.
	if profile.SecretRef != "" {
		return tasks.Execution{}, errors.New("ssh stored-secret authentication is not available until the askpass broker is enabled")
	}

	executable, err := sshclient.FindOpenSSH()
	if err != nil {
		return tasks.Execution{}, err
	}
	command, err := sshclient.BuildInteractiveCommand(executable, profile)
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
		Detail:    "Remote terminal " + command.Destination,
		Type:      "process",
		Command:   command.Executable,
		Args:      rawArgs,
	}, s.Workspace)
	if err != nil {
		return tasks.Execution{}, err
	}
	return spec, nil
}
