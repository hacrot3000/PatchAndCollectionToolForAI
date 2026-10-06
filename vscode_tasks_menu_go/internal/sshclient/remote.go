package sshclient

import (
	"errors"
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

func BuildInteractiveCommandAt(executable string, profile sshprofile.Profile, remoteCWD string) (Command, error) {
	p, err := sshprofile.Normalize(profile)
	if err != nil {
		return Command{}, err
	}
	remoteCWD = strings.TrimSpace(remoteCWD)
	if remoteCWD == "" {
		return BuildInteractiveCommand(executable, p)
	}
	if len(remoteCWD) > 4096 || strings.ContainsAny(remoteCWD, "\x00\r\n") || !strings.HasPrefix(remoteCWD, "/") {
		return Command{}, errors.New("remote SSH cwd must be an absolute path without control characters")
	}
	command, err := BuildCommand(executable, p)
	if err != nil {
		return Command{}, err
	}
	command.Args = append(command.Args, "cd -- "+remoteShellQuote(remoteCWD)+" || exit 1; exec \"${SHELL:-/bin/sh}\" -l")
	return command, nil
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
