package sftpclient

import (
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func TestBuildCommandAgentUsesBatchAndSafeSSHOptions(t *testing.T) {
	cmd, err := BuildCommand("/usr/bin/sftp", sshprofile.Profile{
		ID: "prod", Name: "Production", Host: "prod.example.com",
		Username: "deploy", AuthMethod: sshprofile.AuthAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cmd.Batch {
		t.Fatal("agent SFTP should use stdin batch mode")
	}
	joined := strings.Join(cmd.Args, "\n")
	for _, want := range []string{
		"-q", "-P", "22", "StrictHostKeyChecking=ask",
		"BatchMode=yes", "PreferredAuthentications=publickey",
		"-b", "deploy@prod.example.com",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %#v", want, cmd.Args)
		}
	}
}

func TestBuildCommandPasswordUsesAskpassCompatibleInteractiveInput(t *testing.T) {
	cmd, err := BuildCommand("/usr/bin/sftp", sshprofile.Profile{
		ID: "prod", Name: "Production", Host: "prod.example.com", Port: 2222,
		Username: "deploy", AuthMethod: sshprofile.AuthPassword,
		SecretRef: "ssh/prod/auth/secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Batch {
		t.Fatal("password SFTP must not force batch authentication")
	}
	joined := strings.Join(cmd.Args, "\n")
	for _, want := range []string{
		"2222", "StrictHostKeyChecking=accept-new", "PubkeyAuthentication=no",
		"PreferredAuthentications=password", "NumberOfPasswordPrompts=1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %#v", want, cmd.Args)
		}
	}
	if strings.Contains(joined, cmd.Args[0]+"\n-b\n") || strings.Contains(joined, "ssh/prod/auth/secret") {
		t.Fatalf("unsafe password SFTP args: %#v", cmd.Args)
	}
}
