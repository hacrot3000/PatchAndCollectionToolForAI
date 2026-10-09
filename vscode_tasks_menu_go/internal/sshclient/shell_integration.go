package sshclient

import (
	"errors"
	"fmt"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

// Remote Bash integration is delivered in-memory over SSH as a quoted here-doc.
// It neither writes to the remote filesystem nor changes the user's dotfiles.
// Non-Bash login shells retain the prior interactive-login behavior.
const remoteBashRCDelimiter = "TASKDECK_SSH_BASH_RC_V1"

// BuildInstrumentedInteractiveCommand preserves all existing SSH connection
// settings, configured startup commands, explicit CWD and port forwards. Only
// Bash login shells with /dev/fd support receive temporary OSC 7/133 hooks.
// Other shells still work normally and may provide their own OSC markers.
func BuildInstrumentedInteractiveCommand(executable string, profile sshprofile.Profile, remoteCWD string, bashRC string) (Command, error) {
	if bashRC == "" || len(bashRC) > 16384 ||
		strings.Contains(bashRC, "\x00") ||
		strings.Contains(bashRC, "\n"+remoteBashRCDelimiter+"\n") ||
		strings.HasSuffix(bashRC, "\n"+remoteBashRCDelimiter) {
		return Command{}, errors.New("invalid SSH Bash shell integration")
	}
	command, err := BuildInteractiveCommandAt(executable, profile, remoteCWD)
	if err != nil {
		return Command{}, err
	}
	if len(command.Args) == 0 {
		return Command{}, errors.New("SSH command has no destination")
	}
	// BuildInteractiveCommandAt may append a remote bootstrap after the
	// destination (custom home, presets or an explicit remote CWD).
	var bootstrap string
	last := len(command.Args) - 1
	if command.Args[last] != command.Destination {
		bootstrap = command.Args[last]
		command.Args = command.Args[:last]
		const loginShell = `exec "${SHELL:-/bin/sh}" -l`
		if !strings.HasSuffix(bootstrap, loginShell) {
			return Command{}, fmt.Errorf("unexpected SSH interactive bootstrap")
		}
		bootstrap = strings.TrimSuffix(bootstrap, loginShell)
	}
	// Let SSH start its usual remote command shell. For Bash, the final
	// interactive shell reads hooks from fd 3, not from an on-disk file.
	// Quoting the heredoc delimiter prevents client-side or remote-shell
	// expansion of RC contents (including $HOME, backticks and ${...}).
	var remote strings.Builder
	if bootstrap != "" {
		remote.WriteString(bootstrap)
		remote.WriteByte('\n')
	}
	remote.WriteString(`if [ "${SHELL##*/}" = "bash" ] && command -v bash >/dev/null 2>&1 && [ -d /dev/fd ]; then
  exec bash --rcfile /dev/fd/3 -i 3<<'`)
	remote.WriteString(remoteBashRCDelimiter)
	remote.WriteString("'\n")
	remote.WriteString(bashRC)
	if !strings.HasSuffix(bashRC, "\n") {
		remote.WriteByte('\n')
	}
	remote.WriteString(remoteBashRCDelimiter)
	remote.WriteString("\nelse\n")
	remote.WriteString(`  exec "${SHELL:-/bin/sh}" -l`)
	remote.WriteString("\nfi")
	command.Args = append(command.Args, remote.String())
	return command, nil
}
