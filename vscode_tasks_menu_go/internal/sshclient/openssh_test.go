package sshclient

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func TestBuildCommandAgentUsesSafeDefaults(t *testing.T) {
	cmd, err := BuildCommand("/usr/bin/ssh", sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "prod.example.com",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, "\n")
	for _, want := range []string{
		"-tt",
		"ConnectTimeout=10",
		"ServerAliveInterval=15",
		"ServerAliveCountMax=3",
		"StrictHostKeyChecking=ask",
		"BatchMode=yes",
		"PreferredAuthentications=publickey",
		"deploy@prod.example.com",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %#v", want, cmd.Args)
		}
	}
}

func TestBuildCommandPasswordDoesNotExposeSecret(t *testing.T) {
	const secretRef = "secret/ssh/prod-password"
	cmd, err := BuildCommand("/usr/bin/ssh", sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "prod.example.com",
		Port:       2222,
		Username:   "deploy",
		AuthMethod: sshprofile.AuthPassword,
		SecretRef:  secretRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, "\n")
	if strings.Contains(joined, secretRef) {
		t.Fatalf("secret reference leaked into ssh args: %#v", cmd.Args)
	}
	for _, want := range []string{
		"2222",
		"PubkeyAuthentication=no",
		"PreferredAuthentications=password",
		"NumberOfPasswordPrompts=1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %#v", want, cmd.Args)
		}
	}
	if strings.Contains(joined, "BatchMode=yes") {
		t.Fatalf("password auth unexpectedly disabled prompts: %#v", cmd.Args)
	}
}

func TestBuildCommandPrivateKeyUsesIdentityOnly(t *testing.T) {
	cmd, err := BuildCommand("/usr/bin/ssh", sshprofile.Profile{
		ID:           "prod",
		Name:         "Production",
		Host:         "prod.example.com",
		Username:     "deploy",
		AuthMethod:   sshprofile.AuthPrivateKey,
		IdentityFile: "/home/deploy/.ssh/id_ed25519",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, "\n")
	for _, want := range []string{
		"/home/deploy/.ssh/id_ed25519",
		"IdentitiesOnly=yes",
		"PreferredAuthentications=publickey",
		"BatchMode=yes",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %#v", want, cmd.Args)
		}
	}
}

func TestBuildCommandIncludesProxyJumpAsSingleArgument(t *testing.T) {
	cmd, err := BuildCommand("/usr/bin/ssh", sshprofile.Profile{
		ID:         "prod",
		Name:       "Production",
		Host:       "db.internal",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthAgent,
		ProxyJump:  "jump@example.com:2222",
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i := 0; i+1 < len(cmd.Args); i++ {
		if cmd.Args[i] == "-J" && cmd.Args[i+1] == "jump@example.com:2222" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("proxy jump pair not found: %#v", cmd.Args)
	}
}

func TestBuildCommandAddsProfilePortForwardings(t *testing.T) {
	cmd, err := BuildCommand("/usr/bin/ssh", sshprofile.Profile{
		ID: "prod", Name: "Production", Host: "prod.example.com", Username: "deploy", AuthMethod: sshprofile.AuthAgent,
		Forwardings: []sshprofile.PortForwarding{
			{Kind: sshprofile.ForwardLocal, BindHost: "127.0.0.1", BindPort: 8080, TargetHost: "app.internal", TargetPort: 80},
			{Kind: sshprofile.ForwardRemote, BindHost: "127.0.0.1", BindPort: 9000, TargetHost: "127.0.0.1", TargetPort: 9001},
			{Kind: sshprofile.ForwardDynamic, BindHost: "::1", BindPort: 1080},
		},
	})
	if err != nil { t.Fatal(err) }
	joined := strings.Join(cmd.Args, "\n")
	for _, want := range []string{
		"ExitOnForwardFailure=yes",
		"127.0.0.1:8080:app.internal:80",
		"127.0.0.1:9000:127.0.0.1:9001",
		"[::1]:1080",
	} {
		if !strings.Contains(joined, want) { t.Fatalf("args missing %q: %#v", want, cmd.Args) }
	}
	for _, flag := range []string{"-L", "-R", "-D"} {
		if !strings.Contains(joined, flag) { t.Fatalf("args missing forwarding flag %q: %#v", flag, cmd.Args) }
	}
}

func TestBuildProbeCommandDisablesTTYAndSkipsRemoteBootstrap(t *testing.T) {
	cmd, err := BuildProbeCommand("/usr/bin/ssh", sshprofile.Profile{
		ID:            "prod",
		Name:          "Production",
		Host:          "prod.example.com",
		Username:      "deploy",
		AuthMethod:    sshprofile.AuthAgent,
		CustomHomeDir: "/srv/app",
		PresetCommands: []sshprofile.PresetCommand{
			{ID: "env", Name: "Environment", Command: "export APP_ENV=prod"},
		},
		Forwardings: []sshprofile.PortForwarding{
			{Kind: sshprofile.ForwardLocal, BindPort: 8080, TargetHost: "app.internal", TargetPort: 80},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) < 3 {
		t.Fatalf("probe args too short: %#v", cmd.Args)
	}
	if cmd.Args[0] != "-T" {
		t.Fatalf("probe tty flag = %q, want -T", cmd.Args[0])
	}
	if cmd.Args[len(cmd.Args)-2] != "deploy@prod.example.com" || cmd.Args[len(cmd.Args)-1] != "true" {
		t.Fatalf("probe destination/command = %#v", cmd.Args)
	}
	joined := strings.Join(cmd.Args, "\n")
	if strings.Contains(joined, "/srv/app") || strings.Contains(joined, "APP_ENV") || strings.Contains(joined, "8080:app.internal:80") {
		t.Fatalf("probe unexpectedly runs terminal bootstrap/forwarding: %#v", cmd.Args)
	}
}

func TestProbeOpenSSH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	path := filepath.Join(t.TempDir(), "ssh")
	script := "#!/bin/sh\necho 'OpenSSH_fixture_1.0' >&2\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ProbeOpenSSH(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "OpenSSH_fixture_1.0" {
		t.Fatalf("version = %q", got)
	}
}
