package server

import (
    "context"
    "errors"
    "os"
    "os/exec"
    "path/filepath"
    "strings"

    "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// readRemoteGitStatus is intentionally NOT a generic Git RPC. Only the
// TaskDeck host chooses the repository: its exact, canonical workspace root.
// No PHP, Relay or remote client parameter can select a path or command.
//
// Git status is read-only and produces at most 256 bytes of accepted output.
// No shell, aliases, hooks, filters, credential helpers, remote network
// operations, mutating git subcommands or user-provided arguments are run.
func (s *Server) readRemoteGitStatus(ctx context.Context) (string, error) {
    denied := remotetransport.ErrRemoteActionDenied
    // A separately opted-in host capability: existing deployments that
    // allowed only session.ping must never silently expose repository state.
    // This environment variable is read exclusively by the trusted host
    // daemon, never from a Portal ticket or encrypted client arguments.
    if os.Getenv("TASKDECK_REMOTE_GIT_STATUS") != "1" {
        return "", denied
    }
    if s == nil || ctx == nil || ctx.Err() != nil || s.Workspace == "" {
        return "", denied
    }
    root, err := filepath.EvalSymlinks(s.Workspace)
    if err != nil { return "", denied }
    root, err = filepath.Abs(root)
    if err != nil { return "", denied }
    info, err := os.Stat(root)
    if err != nil || !info.IsDir() { return "", denied }

    // Use a minimal environment, not the Portal/Relay process environment.
    // Local repository config remains under the host's control, but fsmonitor
    // and optional Git index writes are disabled for this read-only request.
    gitEnv := []string{
        "PATH=" + os.Getenv("PATH"),
        "HOME=" + os.Getenv("HOME"),
        "LC_ALL=C",
        "GIT_OPTIONAL_LOCKS=0",
        "GIT_CONFIG_NOSYSTEM=1",
        "GIT_CONFIG_GLOBAL=" + os.DevNull,
        "GIT_CONFIG_SYSTEM=" + os.DevNull,
    }
    run := func(limit int, args ...string) (string,error) {
        command := exec.CommandContext(ctx, "git", args...)
        command.Dir = root
        command.Env = gitEnv
        output := &boundedCommandOutput{remaining: limit}
        command.Stdout = output
        // Do not forward remote repo errors, credential helper hints or
        // host paths as unauthenticated Relay-visible error messages.
        command.Stderr = nil
        if err := command.Run(); err != nil || ctx.Err() != nil || output.truncated {
            return "", denied
        }
        return output.String(), nil
    }
    const prefix = "-c"
    // First reject subdirectories of other repositories, including repos
    // in parent directories. A Project must explicitly select the repo root.
    top, err := run(4096, prefix, "core.fsmonitor=false", "-C", root,
        "rev-parse", "--show-toplevel")
    if err != nil { return "", err }
    top = strings.TrimSpace(top)
    canonicalTop, err := filepath.EvalSymlinks(top)
    if err != nil { return "", denied }
    canonicalTop, err = filepath.Abs(canonicalTop)
    if err != nil || filepath.Clean(canonicalTop) != filepath.Clean(root) {
        return "", denied
    }

    // Even read-only git status may run repository-defined filter.clean.
    // Reject executable filters before touching tracked worktree data.
    if !remoteGitNoExecutableFilters(ctx,root,gitEnv){return "",denied}
    status, err := run(remotetransport.MaxRemoteGitStatusText,
        prefix, "core.fsmonitor=false",
        prefix, "core.quotePath=true",
        "-C", root, "--no-optional-locks",
        "status", "--short", "--branch", "--untracked-files=no")
    if err != nil { return "", err }
    if strings.TrimSpace(status) == "" { return "clean", nil }
    // The AEAD serializer further rejects ANSI/NUL/control sequences and
    // oversized output; do not send such data to a native terminal.
    if strings.ContainsRune(status, 0) || errors.Is(ctx.Err(), context.Canceled) {
        return "", denied
    }
    return status, nil
}
