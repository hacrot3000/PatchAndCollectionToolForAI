package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func TestSSHTerminalExecutionUsesExistingTerminalProcessModel(t *testing.T) {
	workspace := t.TempDir()
	profileStore, err := sshprofile.NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profileStore.Create(sshprofile.Profile{
		ID:            "prod",
		Name:          "Production",
		Host:          "prod.example.com",
		Username:      "deploy",
		AuthMethod:    sshprofile.AuthAgent,
		CustomHomeDir: "/srv/app",
		PresetCommands: []sshprofile.PresetCommand{
			{ID: "env", Name: "Environment", Command: "export APP_ENV=prod"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	sshPath := filepath.Join(binDir, "ssh")
	if err := os.WriteFile(sshPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	s := &Server{Workspace: workspace, SSHProfiles: profileStore}
	spec, err := s.sshTerminalExecution(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command != sshPath {
		t.Fatalf("command = %q, want %q", spec.Command, sshPath)
	}
	joined := strings.Join(spec.Args, "\n")
	for _, want := range []string{
		"deploy@prod.example.com",
		"StrictHostKeyChecking=ask",
		"cd -- '/srv/app'",
		"export APP_ENV=prod",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ssh args missing %q: %#v", want, spec.Args)
		}
	}
	if spec.Cwd != workspace {
		t.Fatalf("local ssh process cwd = %q, want workspace %q", spec.Cwd, workspace)
	}
	if !strings.Contains(spec.Label, "Production") {
		t.Fatalf("label = %q", spec.Label)
	}
}

func TestSSHTerminalExecutionFailsClosedForStoredSecretUntilAskpass(t *testing.T) {
	profileStore, err := sshprofile.NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = profileStore.Create(sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "prod.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthPassword,
		SecretRef:  "ssh/prod/auth/test",
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{Workspace: t.TempDir(), SSHProfiles: profileStore}
	_, err = s.sshTerminalExecution("prod")
	if err == nil || !strings.Contains(err.Error(), "askpass broker") {
		t.Fatalf("error = %v", err)
	}
}
