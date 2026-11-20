package sshclient

import (
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

const remoteShellTestRC = "# synthetic TaskDeck shell hooks\n" +
	"PROMPT_COMMAND='printf \\047\\033]133;A\\007\\047'\n" +
	"PS0='\\033]133;C\\007'\n"

func TestBuildInstrumentedInteractiveSSHUsesEphemeralRemoteBashRC(t *testing.T) {
	profile := sshprofile.Profile{
		ID: "prod", Name: "Production", Host: "example.com", Username: "deploy",
		AuthMethod: sshprofile.AuthAgent,
		CustomHomeDir: "/srv/app's project",
		PresetCommands: []sshprofile.PresetCommand{
			{ID: "env", Name: "Env", Command: "export APP_ENV=production"},
		},
		Forwardings: []sshprofile.PortForwarding{
			{Kind: sshprofile.ForwardLocal, BindPort: 8080, TargetHost: "127.0.0.1", TargetPort: 80},
		},
	}
	command, err := BuildInstrumentedInteractiveCommand("/usr/bin/ssh", profile, "", remoteShellTestRC)
	if err != nil { t.Fatal(err) }
	if command.Destination != "deploy@example.com" { t.Fatalf("destination=%q",command.Destination) }
	joined := strings.Join(command.Args, "\n")
	for _, want := range []string{
		"-tt", "ExitOnForwardFailure=yes", "127.0.0.1:8080:127.0.0.1:80",
		"cd -- '/srv/app'\"'\"'s project'",
		"export APP_ENV=production",
		`[ "${SHELL##*/}" = "bash" ]`,
		"exec bash --rcfile /dev/fd/3 -i 3<<'",
		remoteBashRCDelimiter,
		remoteShellTestRC,
		`exec "${SHELL:-/bin/sh}" -l`,
	} {
		if !strings.Contains(joined,want) { t.Errorf("instrumented SSH missing %q in %s",want,joined) }
	}
	if strings.Contains(joined,"/tmp/taskdeck") || strings.Contains(joined,"/home/deploy/.bashrc >") {
		t.Fatal("remote shell integration must not write to remote config files")
	}
	if command.Args[len(command.Args)-2]!=command.Destination {
		t.Fatalf("SSH destination must precede one remote bootstrap arg: %v",command.Args)
	}
}

func TestBuildInstrumentedSSHRemoteCWDPreservesExactSemantics(t *testing.T) {
	profile := sshprofile.Profile{
		ID: "prod", Name: "Production", Host: "example.com", Username: "deploy",
		AuthMethod: sshprofile.AuthAgent,
		CustomHomeDir: "/srv/initial",
		PresetCommands: []sshprofile.PresetCommand{{ID:"one",Name:"One",Command:"touch /tmp/unwanted"}},
	}
	cmd, err := BuildInstrumentedInteractiveCommand("/usr/bin/ssh",profile,"/var/lib/my project's work",remoteShellTestRC)
	if err != nil { t.Fatal(err) }
	script:=cmd.Args[len(cmd.Args)-1]
	if !strings.Contains(script,`cd -- '/var/lib/my project'"'"'s work' || exit 1`) {
		t.Fatalf("remote CWD not securely quoted: %s",script)
	}
	if strings.Contains(script,"touch /tmp/unwanted") || strings.Contains(script,"/srv/initial") {
		t.Fatalf("explicit CWD unexpectedly runs profile presets: %s",script)
	}
	for _,cwd := range []string{"relative/path","/ok\nrm -rf /","/ok\x00bad"} {
		if _,err:=BuildInstrumentedInteractiveCommand("/usr/bin/ssh",profile,cwd,remoteShellTestRC);err==nil{
			t.Fatalf("invalid remote cwd %q was accepted",cwd)
		}
	}
}

func TestBuildInstrumentedSSHRefusesInjectedHereDocAndInvalidRC(t *testing.T) {
	profile := sshprofile.Profile{ID:"prod",Name:"Production",Host:"example.com",Username:"deploy",AuthMethod:sshprofile.AuthAgent}
	for _,rc := range []string{"","x\x00y","x\n"+remoteBashRCDelimiter+"\nexit 0",strings.Repeat("a",16385)} {
		if _,err:=BuildInstrumentedInteractiveCommand("/usr/bin/ssh",profile,"",rc);err==nil {
			t.Fatalf("unsafe remote shell RC unexpectedly accepted (len=%d)",len(rc))
		}
	}
}
