package sshclient

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func BuildTunnelCommand(
	executable string,
	profile sshprofile.Profile,
	localPort int,
	remoteHost string,
	remotePort int,
) (Command, error) {
	if localPort < 1 || localPort > 65535 {
		return Command{}, errors.New("SSH tunnel local port must be between 1 and 65535")
	}
	if remotePort < 1 || remotePort > 65535 {
		return Command{}, errors.New("SSH tunnel remote port must be between 1 and 65535")
	}
	remoteHost = strings.TrimSpace(remoteHost)
	if remoteHost == "" {
		return Command{}, errors.New("SSH tunnel remote host is required")
	}
	if strings.ContainsAny(remoteHost, " \t/\x00\r\n") {
		return Command{}, errors.New("SSH tunnel remote host must not contain whitespace or slash")
	}

	command, err := buildCommand(executable, profile, "-T", false, false)
	if err != nil {
		return Command{}, err
	}
	if len(command.Args) == 0 || command.Args[len(command.Args)-1] != command.Destination {
		return Command{}, errors.New("SSH tunnel command has invalid destination layout")
	}

	forwardHost := remoteHost
	if ip := net.ParseIP(strings.Trim(remoteHost, "[]")); ip != nil && strings.Contains(ip.String(), ":") {
		forwardHost = "[" + strings.Trim(remoteHost, "[]") + "]"
	}
	forward := "127.0.0.1:" + strconv.Itoa(localPort) + ":" + forwardHost + ":" + strconv.Itoa(remotePort)

	args := append([]string(nil), command.Args[:len(command.Args)-1]...)
	args = append(args,
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-L", forward,
		command.Destination,
	)
	command.Args = args
	return command, nil
}
