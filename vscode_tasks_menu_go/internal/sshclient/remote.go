package sshclient

import (
	"strings"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func RemoteBootstrap(profile sshprofile.Profile) (string, error) {
	p, err := sshprofile.Normalize(profile)
	if err != nil {
		return "", err
	}
	if p.CustomHomeDir == "" && len(p.PresetCommands) == 0 {
		return "", nil
	}

	lines := make([]string, 0, 2+len(p.PresetCommands)*2)
	if p.CustomHomeDir != "" {
		lines = append(lines, "cd -- "+remoteShellQuote(p.CustomHomeDir)+" || exit 1")
	}
	for _, preset := range p.PresetCommands {
		if preset.Cwd != "" {
			lines = append(lines, "cd -- "+remoteShellQuote(preset.Cwd)+" || exit 1")
		}
		lines = append(lines, preset.Command)
	}
	lines = append(lines, `exec "${SHELL:-/bin/sh}" -l`)
	return strings.Join(lines, "; "), nil
}

func remoteShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func BuildInteractiveCommand(executable string, profile sshprofile.Profile) (Command, error) {
	command, err := BuildCommand(executable, profile)
	if err != nil {
		return Command{}, err
	}
	bootstrap, err := RemoteBootstrap(profile)
	if err != nil {
		return Command{}, err
	}
	if bootstrap != "" {
		command.Args = append(command.Args, bootstrap)
	}
	return command, nil
}
