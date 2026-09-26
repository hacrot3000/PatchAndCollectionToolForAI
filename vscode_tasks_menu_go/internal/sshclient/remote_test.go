package sshclient

import (
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func TestRemoteBootstrapEmptyKeepsNormalSSHLogin(t *testing.T) {
	got, err := RemoteBootstrap(sshprofile.Profile{
		ID: "prod", Name: "Production", Host: "prod.example.com", Username: "deploy", AuthMethod: sshprofile.AuthAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("bootstrap = %q, want empty", got)
	}
}

func TestRemoteBootstrapAppliesHomePresetsAndLoginShell(t *testing.T) {
	got, err := RemoteBootstrap(sshprofile.Profile{
		ID: "prod", Name: "Production", Host: "prod.example.com", Username: "deploy", AuthMethod: sshprofile.AuthAgent,
		CustomHomeDir: "/srv/app's current",
		PresetCommands: []sshprofile.PresetCommand{
			{ID: "env", Name: "Environment", Command: "export APP_ENV=prod"},
			{ID: "logs", Name: "Logs", Cwd: "/srv/logs", Command: "pwd"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`cd -- '/srv/app'\"'\"'s current' || exit 1`,
		"export APP_ENV=prod",
		`cd -- '/srv/logs' || exit 1`,
		"pwd",
		`exec "${SHELL:-/bin/sh}" -l`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("bootstrap missing %q: %s", want, got)
		}
	}
}

func TestBuildInteractiveCommandAppendsBootstrapAfterDestination(t *testing.T) {
	cmd, err := BuildInteractiveCommand("/usr/bin/ssh", sshprofile.Profile{
		ID: "prod", Name: "Production", Host: "prod.example.com", Username: "deploy", AuthMethod: sshprofile.AuthAgent,
		CustomHomeDir: "/srv/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) < 2 {
		t.Fatalf("args too short: %#v", cmd.Args)
	}
	if cmd.Args[len(cmd.Args)-2] != "deploy@prod.example.com" {
		t.Fatalf("destination not before bootstrap: %#v", cmd.Args)
	}
	if !strings.Contains(cmd.Args[len(cmd.Args)-1], "cd -- '/srv/app'") {
		t.Fatalf("bootstrap missing from final arg: %#v", cmd.Args)
	}
}
