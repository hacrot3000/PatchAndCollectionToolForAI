package sshclient

import (
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func TestBuildTunnelCommandBindsLoopbackAndKeepsDestinationLast(t *testing.T) {
	command, err := BuildTunnelCommand("/usr/bin/ssh", sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "jump.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthAgent,
	}, 43123, "db.internal", 3306)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command.Args, "\n")
	for _, want := range []string{
		"-T",
		"-N",
		"ExitOnForwardFailure=yes",
		"127.0.0.1:43123:db.internal:3306",
		"BatchMode=yes",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("tunnel args missing %q: %#v", want, command.Args)
		}
	}
	if command.Args[len(command.Args)-1] != "deploy@jump.example.com" {
		t.Fatalf("destination is not final argv item: %#v", command.Args)
	}
}

func TestBuildTunnelCommandFormatsIPv6RemoteHost(t *testing.T) {
	command, err := BuildTunnelCommand("/usr/bin/ssh", sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "jump.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthAgent,
	}, 43123, "2001:db8::25", 3306)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(command.Args, "\n"), "127.0.0.1:43123:[2001:db8::25]:3306") {
		t.Fatalf("IPv6 forward missing: %#v", command.Args)
	}
}

func TestBuildTunnelCommandKeepsStoredSecretHostKeyPolicy(t *testing.T) {
	command, err := BuildTunnelCommand("/usr/bin/ssh", sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "jump.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthPassword,
		SecretRef:  "ssh/prod/auth/secret",
	}, 43123, "127.0.0.1", 3306)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command.Args, "\n")
	if !strings.Contains(joined, "StrictHostKeyChecking=accept-new") {
		t.Fatalf("stored-secret tunnel host-key policy missing: %#v", command.Args)
	}
	if strings.Contains(joined, "ssh/prod/auth/secret") {
		t.Fatalf("secret reference leaked into tunnel argv: %#v", command.Args)
	}
}

func TestBuildTunnelCommandRejectsUnsafeEndpoints(t *testing.T) {
	profile := sshprofile.Profile{
		ID: "prod", Name: "Production", Host: "jump.example.com", Username: "deploy", AuthMethod: sshprofile.AuthAgent,
	}
	for _, tc := range []struct {
		localPort  int
		remoteHost string
		remotePort int
	}{
		{0, "db", 3306},
		{43123, "", 3306},
		{43123, "db host", 3306},
		{43123, "db/host", 3306},
		{43123, "db", 0},
		{43123, "db", 70000},
	} {
		if _, err := BuildTunnelCommand("/usr/bin/ssh", profile, tc.localPort, tc.remoteHost, tc.remotePort); err == nil {
			t.Fatalf("unsafe endpoint unexpectedly accepted: %+v", tc)
		}
	}
}
