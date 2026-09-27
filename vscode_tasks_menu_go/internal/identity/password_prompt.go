package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// PromptNewPassword uses the already-required Python standard library to hide
// terminal input portably. getpass's echoing fallback is explicitly forbidden.
// The password travels back over a private stdout pipe, never argv or env.
func PromptNewPassword(ctx context.Context, input *os.File, prompts io.Writer) (string, error) {
	executable, prefix, err := resolvePythonSQLiteCommand()
	if err != nil {
		return "", err
	}
	args := append(append([]string(nil), prefix...), "-c", pythonPasswordPrompt)
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdin = input
	cmd.Stderr = prompts
	var output boundedTextBuffer
	output.limit = 32 * 1024
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("secure password prompt failed: %w", err)
	}
	var password string
	if err := json.Unmarshal([]byte(output.String()), &password); err != nil {
		return "", fmt.Errorf("invalid password prompt response")
	}
	return password, nil
}

const pythonPasswordPrompt = `
import getpass
import json
import sys
import warnings

if not sys.stdin.isatty():
    sys.exit("An interactive terminal is required; use --shared-admin-password-stdin for a private pipe.")
warnings.simplefilter("error", getpass.GetPassWarning)
try:
    password = getpass.getpass("New shared admin password: ")
    confirmation = getpass.getpass("Confirm password: ")
    if password != confirmation:
        sys.exit("Passwords do not match.")
    if len(password.encode("utf-8")) > 4096:
        sys.exit("Password is too long.")
    print(json.dumps(password))
except (EOFError, KeyboardInterrupt, getpass.GetPassWarning):
    sys.exit("Secure password input cancelled or unavailable.")
`
