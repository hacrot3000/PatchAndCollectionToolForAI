package sftpclient

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

type Command struct {
	Executable  string
	Args        []string
	Destination string
	Batch       bool
}

func FindOpenSFTP() (string, error) {
	path, err := exec.LookPath("sftp")
	if err != nil {
		return "", errors.New("OpenSSH sftp client not found in PATH; install the system OpenSSH client")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve OpenSSH sftp path: %w", err)
	}
	return filepath.Clean(path), nil
}

func BuildCommand(executable string, profile sshprofile.Profile) (Command, error) {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return Command{}, errors.New("OpenSSH sftp client path is required")
	}
	p, err := sshprofile.Normalize(profile)
	if err != nil {
		return Command{}, err
	}

	hostKeyPolicy := "ask"
	if p.SecretRef != "" {
		hostKeyPolicy = "accept-new"
	}
	args := []string{
		"-q",
		"-P", strconv.Itoa(p.Port),
		"-o", "ConnectTimeout=" + strconv.Itoa(p.ConnectTimeoutSeconds),
		"-o", "ServerAliveInterval=" + strconv.Itoa(p.ServerAliveIntervalSeconds),
		"-o", "ServerAliveCountMax=" + strconv.Itoa(p.ServerAliveCountMax),
		"-o", "StrictHostKeyChecking=" + hostKeyPolicy,
	}
	if p.ProxyJump != "" {
		args = append(args, "-J", p.ProxyJump)
	}

	batch := p.SecretRef == ""
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

	if batch {
		// A batchfile of "-" means commands come from stdin. We only use this
		// when the SSH profile requires no interactive password/passphrase.
		args = append(args, "-b", "-")
	}
	destination := p.Username + "@" + p.Host
	args = append(args, destination)
	return Command{Executable: executable, Args: args, Destination: destination, Batch: batch}, nil
}
