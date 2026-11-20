package sshclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

const probeTimeout = 3 * time.Second

type Command struct {
	Executable  string
	Args        []string
	Destination string
}

func FindOpenSSH() (string, error) {
	path, err := exec.LookPath("ssh")
	if err != nil {
		return "", errors.New("OpenSSH client not found in PATH; install the system ssh client")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve OpenSSH client path: %w", err)
	}
	return filepath.Clean(path), nil
}

func ProbeOpenSSH(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("OpenSSH client path is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-V")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", errors.New("OpenSSH version probe timed out")
	}
	version := strings.TrimSpace(string(output))
	if err != nil {
		if version == "" {
			return "", fmt.Errorf("OpenSSH version probe failed: %w", err)
		}
		return "", fmt.Errorf("OpenSSH version probe failed: %s", version)
	}
	if version == "" {
		return "", errors.New("OpenSSH version probe returned no version")
	}
	if len(version) > 4096 {
		version = version[:4096]
	}
	return version, nil
}

func BuildCommand(executable string, profile sshprofile.Profile) (Command, error) {
	return buildCommand(executable, profile, "-tt", false, true)
}

func BuildProbeCommand(executable string, profile sshprofile.Profile) (Command, error) {
	return buildCommand(executable, profile, "-T", true, false)
}

func BuildRemoteCommand(executable string, profile sshprofile.Profile, remoteCommand string) (Command, error) {
	remoteCommand = strings.TrimSpace(remoteCommand)
	if remoteCommand == "" {
		return Command{}, errors.New("remote SSH command is required")
	}
	if len(remoteCommand) > 32768 || strings.ContainsRune(remoteCommand, '\x00') {
		return Command{}, errors.New("remote SSH command is invalid")
	}
	command, err := buildCommand(executable, profile, "-T", false, false)
	if err != nil {
		return Command{}, err
	}
	command.Args = append(command.Args, remoteCommand)
	return command, nil
}

func buildCommand(executable string, profile sshprofile.Profile, ttyFlag string, probe bool, includeProfileForwardings bool) (Command, error) {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return Command{}, errors.New("OpenSSH client path is required")
	}
	p, err := sshprofile.Normalize(profile)
	if err != nil {
		return Command{}, err
	}

	hostKeyPolicy := "ask"
	if p.SecretRef != "" {
		// SSH_ASKPASS_REQUIRE=force also captures confirmation prompts. Use
		// OpenSSH's TOFU mode for stored-secret sessions: new keys are accepted,
		// but changed host keys remain rejected.
		hostKeyPolicy = "accept-new"
	}
	args := []string{
		ttyFlag,
		"-p", strconv.Itoa(p.Port),
		"-o", "ConnectTimeout=" + strconv.Itoa(p.ConnectTimeoutSeconds),
		"-o", "ServerAliveInterval=" + strconv.Itoa(p.ServerAliveIntervalSeconds),
		"-o", "ServerAliveCountMax=" + strconv.Itoa(p.ServerAliveCountMax),
		"-o", "StrictHostKeyChecking=" + hostKeyPolicy,
	}
	if p.ProxyJump != "" {
		args = append(args, "-J", p.ProxyJump)
	}

	if includeProfileForwardings && len(p.Forwardings) != 0 {
		args = append(args, "-o", "ExitOnForwardFailure=yes")
		for _, forwarding := range p.Forwardings {
			bind := net.JoinHostPort(forwarding.BindHost, strconv.Itoa(forwarding.BindPort))
			switch forwarding.Kind {
			case sshprofile.ForwardLocal:
				target := net.JoinHostPort(forwarding.TargetHost, strconv.Itoa(forwarding.TargetPort))
				args = append(args, "-L", bind+":"+target)
			case sshprofile.ForwardRemote:
				target := net.JoinHostPort(forwarding.TargetHost, strconv.Itoa(forwarding.TargetPort))
				args = append(args, "-R", bind+":"+target)
			case sshprofile.ForwardDynamic:
				args = append(args, "-D", bind)
			}
		}
	}

	switch p.AuthMethod {
	case sshprofile.AuthAgent:
		args = append(args,
			"-o", "BatchMode=yes",
			"-o", "PreferredAuthentications=publickey",
		)
	case sshprofile.AuthPrivateKey:
		args = append(args,
			"-i", p.IdentityFile,
			"-o", "IdentitiesOnly=yes",
			"-o", "PreferredAuthentications=publickey",
		)
		if p.SecretRef == "" {
			args = append(args, "-o", "BatchMode=yes")
		}
	case sshprofile.AuthPassword:
		args = append(args,
			"-o", "PubkeyAuthentication=no",
			"-o", "PreferredAuthentications=password",
			"-o", "NumberOfPasswordPrompts=1",
		)
	default:
		return Command{}, fmt.Errorf("unsupported ssh authentication method %q", p.AuthMethod)
	}

	destination := p.Username + "@" + p.Host
	args = append(args, destination)
	if probe {
		args = append(args, "true")
	}
	return Command{
		Executable:  executable,
		Args:        args,
		Destination: destination,
	}, nil
}
