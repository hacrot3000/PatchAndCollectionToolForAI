package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/sshaskpass"
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
	if spec.TargetType != "ssh" || spec.TargetProfileID != profile.ID {
		t.Fatalf("terminal target = %q/%q, want ssh/%q", spec.TargetType, spec.TargetProfileID, profile.ID)
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

func TestSSHTerminalExecutionUsesOneTimeAskpassForStoredSecret(t *testing.T) {
	workspace := t.TempDir()
	profileStore, err := sshprofile.NewStore(filepath.Join(t.TempDir(), "ssh_profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	const secretRef = "ssh/prod/auth/test"
	const secretValue = "correct-horse"
	profile, err := profileStore.Create(sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "prod.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthPassword,
		SecretRef:  secretRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	secrets := newMemorySecretStore()
	if err := secrets.Put(secretRef, []byte(secretValue)); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	sshPath := filepath.Join(binDir, "ssh")
	if err := os.WriteFile(sshPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	s := &Server{Workspace: workspace, SSHProfiles: profileStore, ConnectionSecrets: secrets}
	spec, err := s.sshTerminalExecution(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(spec.Args, "\n")
	if !strings.Contains(joined, "StrictHostKeyChecking=accept-new") {
		t.Fatalf("stored-secret ssh must use accept-new host-key policy: %#v", spec.Args)
	}
	if strings.Contains(joined, secretValue) || strings.Contains(joined, secretRef) {
		t.Fatalf("stored secret leaked into argv: %#v", spec.Args)
	}

	env := map[string]string{}
	for _, item := range spec.Env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	for _, key := range []string{
		"SSH_ASKPASS",
		"SSH_ASKPASS_REQUIRE",
		"TASKDECK_SSH_ASKPASS",
		"TASKDECK_SSH_ASKPASS_SOCKET",
		"TASKDECK_SSH_ASKPASS_TOKEN",
	} {
		if env[key] == "" {
			t.Fatalf("missing askpass environment %s", key)
		}
	}
	if env["SSH_ASKPASS_REQUIRE"] != "force" || env["TASKDECK_SSH_ASKPASS"] != "1" {
		t.Fatalf("unexpected askpass environment: %#v", env)
	}
	if strings.Contains(strings.Join(spec.Env, "\n"), secretValue) || strings.Contains(strings.Join(spec.Env, "\n"), secretRef) {
		t.Fatalf("stored secret leaked into environment: %#v", spec.Env)
	}

	var output strings.Builder
	if err := sshaskpass.RunHelper(
		env["TASKDECK_SSH_ASKPASS_SOCKET"],
		env["TASKDECK_SSH_ASKPASS_TOKEN"],
		"Password:",
		&output,
	); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != secretValue+"\n" {
		t.Fatalf("askpass output = %q", got)
	}
	if err := sshaskpass.RunHelper(
		env["TASKDECK_SSH_ASKPASS_SOCKET"],
		env["TASKDECK_SSH_ASKPASS_TOKEN"],
		"Password:",
		&strings.Builder{},
	); err == nil {
		t.Fatal("askpass ticket unexpectedly allowed a second read")
	}
}
