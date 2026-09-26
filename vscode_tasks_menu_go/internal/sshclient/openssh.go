package sshclient

import (
	"context"
	"errors"
	"fmt"
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
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return Command{}, errors.New("OpenSSH client path is required")
	}
	p, err := sshprofile.Normalize(profile)
	if err != nil {
		return Command{}, err
	}

	args := []string{
		"-tt",
		"-p", strconv.Itoa(p.Port),
		"-o", "ConnectTimeout=" + strconv.Itoa(p.ConnectTimeoutSeconds),
		"-o", "ServerAliveInterval=" + strconv.Itoa(p.ServerAliveIntervalSeconds),
		"-o", "ServerAliveCountMax=" + strconv.Itoa(p.ServerAliveCountMax),
		"-o", "StrictHostKeyChecking=ask",
	}
	if p.ProxyJump != "" {
		args = append(args, "-J", p.ProxyJump)
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
	return Command{
		Executable:  executable,
		Args:        args,
		Destination: destination,
	}, nil
}
