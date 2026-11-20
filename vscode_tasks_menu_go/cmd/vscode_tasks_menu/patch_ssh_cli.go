package main

// PATCH remote SSH is a narrowly scoped, same-OS-user CLI bridge. It reuses
// TaskDeck's saved SSH profiles and secret store, never takes an SSH address,
// password, script, raw shell string or interpreter from the PATCH package.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"bletonfc/vscode_tasks_menu/internal/patchsshpolicy"
	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshaskpass"
	"bletonfc/vscode_tasks_menu/internal/sshclient"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
	"bletonfc/vscode_tasks_menu/internal/state"
)

type patchSSHCommandRequest struct {
	ProfileName string   `json:"profile_name"`
	Argv []string `json:"argv"`
	TimeoutSeconds int `json:"timeout_seconds"`
	DryRun bool `json:"dry_run"`
}

func parsePatchSSHRequest(in io.Reader) (patchSSHCommandRequest, string, error) {
	var request patchSSHCommandRequest
	decoder := json.NewDecoder(io.LimitReader(in, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, "", fmt.Errorf("invalid PATCH SSH request: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return request, "", errors.New("PATCH SSH request contains trailing data")
	}
	if request.ProfileName == "" || strings.TrimSpace(request.ProfileName) != request.ProfileName ||
		len(request.ProfileName) > 128 {
		return request, "", errors.New("exact saved SSH profile name is required (1..128 bytes)")
	}
	for _, ch := range request.ProfileName {
		if unicode.IsControl(ch) {
			return request, "", errors.New("SSH profile name contains control characters")
		}
	}
	if request.TimeoutSeconds < 1 || request.TimeoutSeconds > 120 {
		return request, "", errors.New("remote SSH timeout must be 1..120 seconds")
	}
	command, err := patchsshpolicy.Validate(request.Argv)
	if err != nil {
		return request, "", err
	}
	return request, command, nil
}

func patchSSHProfileByName(profiles []sshprofile.Profile, name string) (sshprofile.Profile, error) {
	var found sshprofile.Profile
	matches := 0
	for _, profile := range profiles {
		if profile.Name == name {
			found = profile
			matches++
		}
	}
	if matches != 1 {
		return sshprofile.Profile{}, fmt.Errorf("SSH profile name %q has %d matches; use one unique saved profile name", name, matches)
	}
	return found, nil
}

type patchSSHOutput struct {
	writer io.Writer
	remaining int
}

func (w *patchSSHOutput) Write(data []byte) (int, error) {
	if len(data) > w.remaining {
		if w.remaining > 0 {
			_, _ = w.writer.Write(data[:w.remaining])
			w.remaining = 0
		}
		return 0, errors.New("SSH remote output exceeded 64 KiB")
	}
	n, err := w.writer.Write(data)
	w.remaining -= n
	return n, err
}

// runPatchSSHCLI validates the profile/command before an SSH process is spawned.
// The remote login shell only sees whitelisted ASCII tokens, never user-provided
// shell syntax. Shared multi-user server mode is explicitly forbidden here.
func runPatchSSHCLI(workspace string, input io.Reader, output, diagnostics io.Writer) error {
	request, remoteCommand, err := parsePatchSSHRequest(input)
	if err != nil {
		return err
	}
	root, err := readCollectWorkspace(workspace)
	if err != nil {
		return fmt.Errorf("PATCH SSH workspace rejected: %w", err)
	}
	store, err := sshprofile.DefaultStore()
	if err != nil { return err }
	profiles, err := store.Load()
	if err != nil { return fmt.Errorf("SSH profile store unavailable: %w", err) }
	profile, err := patchSSHProfileByName(profiles, request.ProfileName)
	if err != nil { return err }
	executable, err := sshclient.FindOpenSSH()
	if err != nil { return err }
	command, err := sshclient.BuildRemoteCommand(executable, profile, remoteCommand)
	if err != nil { return err }
	if request.DryRun {
		fmt.Fprintf(output, "PATCH SSH PREFLIGHT PASS: profile=%s; command=%s\n", profile.Name, remoteCommand)
		return nil
	}

	// Disable stdin/interactive remote prompts. Password/private-key passphrase
	// profiles reuse the same protected TaskDeck askpass broker as SSH terminals.
	var ticket *sshaskpass.Ticket
	env := append([]string{}, os.Environ()...)
	if profile.SecretRef != "" {
		secrets, err := secretstore.NewDefaultFileStore()
		if err != nil { return fmt.Errorf("SSH credential store unavailable: %w", err) }
		ticket, err = sshaskpass.Prepare(secrets, profile.SecretRef, state.Dir(root))
		if err != nil { return fmt.Errorf("SSH credential unavailable: %w", err) }
		defer ticket.Close()
		helper, err := os.Executable()
		if err != nil { return err }
		env = append(env,
			"DISPLAY=taskdeck-ssh-askpass",
			"SSH_ASKPASS="+helper,
			"SSH_ASKPASS_REQUIRE=force",
			"TASKDECK_SSH_ASKPASS=1",
			"TASKDECK_SSH_ASKPASS_SOCKET="+ticket.SocketPath,
			"TASKDECK_SSH_ASKPASS_TOKEN="+ticket.Token,
		)
	}
	duration := time.Duration(request.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	cmd := exec.CommandContext(ctx, command.Executable, command.Args...)
	cmd.Env = env
	cmd.Dir = filepath.Clean(root)
	cmd.Stdin = nil
	cmd.Stdout = &patchSSHOutput{writer:output, remaining:64<<10}
	cmd.Stderr = &patchSSHOutput{writer:diagnostics, remaining:64<<10}
	fmt.Fprintf(diagnostics, "PATCH SSH: profile=%s; command=%s; timeout=%ds\n", profile.Name, remoteCommand, request.TimeoutSeconds)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil { return errors.New("PATCH SSH command timed out") }
		return fmt.Errorf("PATCH SSH remote command failed: %w", err)
	}
	return nil
}
