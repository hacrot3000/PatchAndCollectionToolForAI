package server

import (
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "testing"
)

func TestRemoteWorkspaceGitPermissionRequiresWriteForMutations(t *testing.T) {
    for _, action := range []string{"stage", "unstage", "commit", "fetch", "pull"} {
        got, err := remoteWorkspaceGitPermission(action)
        if err != nil || got != "git.write" {
            t.Fatalf("%s permission = %q, %v; want git.write", action, got, err)
        }
    }
    if permission, err := remoteWorkspaceGitPermission("diff_staged"); err != nil || permission != "git.diff" {
        t.Fatalf("diff_staged permission = %q, %v", permission, err)
    }
}

func TestRemoteWorkspaceGitRequestRejectsPathAndActionSmuggling(t *testing.T) {
    valid := []remoteWorkspaceGitRequest{
        {WorkspaceID: "abc", Action: "status"},
        {WorkspaceID: "abc", Action: "diff_staged"},
        {WorkspaceID: "abc", Action: "stage", Path: "src/main.go"},
        {WorkspaceID: "abc", Action: "unstage", Path: "literal[1].txt"},
        {WorkspaceID: "abc", Action: "commit", Message: "Fix parsing in src/main.go"},
    }
    for _, req := range valid {
        if err := validateRemoteWorkspaceGitRequest(&req); err != nil {
            t.Fatalf("valid %+v: %v", req, err)
        }
    }
    invalid := []remoteWorkspaceGitRequest{
        {WorkspaceID: "", Action: "status"},
        {WorkspaceID: "abc", Action: "execute"},
        {WorkspaceID: "abc", Action: "status", Path: "src/file"},
        {WorkspaceID: "abc", Action: "status", Message: "surprise"},
        {WorkspaceID: "abc", Action: "stage", Path: ""},
        {WorkspaceID: "abc", Action: "stage", Path: "."},
        {WorkspaceID: "abc", Action: "stage", Path: "../secret"},
        {WorkspaceID: "abc", Action: "stage", Path: "src/../../secret"},
        {WorkspaceID: "abc", Action: "stage", Path: "/etc/passwd"},
        {WorkspaceID: "abc", Action: "stage", Path: "src/../README.md"},
        {WorkspaceID: "abc", Action: "stage", Path: "a\\b"},
        {WorkspaceID: "abc", Action: "stage", Path: "file\nname"},
        {WorkspaceID: "abc", Action: "stage", Path: "README", Message: "ignored"},
        {WorkspaceID: "abc", Action: "commit"},
        {WorkspaceID: "abc", Action: "commit", Path: "file", Message: "msg"},
        {WorkspaceID: "abc", Action: "commit", Message: "message\nsecond"},
        {WorkspaceID: "abc", Action: "commit", Message: " message "},
        {WorkspaceID: "abc", Action: "commit", Message: strings.Repeat("A", 2001)},
    }
    for _, req := range invalid {
        if err := validateRemoteWorkspaceGitRequest(&req); err == nil {
            t.Fatalf("invalid %+v unexpectedly accepted", req)
        }
    }
}

func TestRemoteWorkspaceGitLiteralPathAndCommitAreShellSafe(t *testing.T) {
    if runtime.GOOS == "windows" { t.Skip("POSIX shell integration") }
    if _, err := exec.LookPath("git"); err != nil { t.Skip("git unavailable") }
    dir := filepath.Join(t.TempDir(), "repo' with spaces")
    if err := os.Mkdir(dir, 0700); err != nil { t.Fatal(err) }

    run := func(command string) string {
        t.Helper()
        cmd := exec.Command("sh", "-c", command)
        output, err := cmd.CombinedOutput()
        if err != nil { t.Fatalf("command failed: %v, output=%q", err, output) }
        return string(output)
    }
    run("git init -q " + remoteArchiveShellQuote(dir))
    run("git -C " + remoteArchiveShellQuote(dir) + " config user.name Test")
    run("git -C " + remoteArchiveShellQuote(dir) + " config user.email test@example.invalid")

    // Bracket metacharacters must be interpreted as an exact filename, not
    // a pathspec glob. A quote inside the remote root must not escape shell.
    a := "literal[1].txt"
    b := "literal1.txt"
    if err := os.WriteFile(filepath.Join(dir, a), []byte("A\n"), 0600); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(dir, b), []byte("B\n"), 0600); err != nil { t.Fatal(err) }
    command, err := remoteWorkspaceGitCommandForRequest(dir, remoteWorkspaceGitRequest{
        WorkspaceID:"abc", Action:"stage", Path:a,
    })
    if err != nil { t.Fatal(err) }
    run(command)
    if got := run("git -C " + remoteArchiveShellQuote(dir) + " diff --cached --name-only"); strings.TrimSpace(got) != a {
        t.Fatalf("unexpected staged paths: %q", got)
    }

    command, err = remoteWorkspaceGitCommandForRequest(dir, remoteWorkspaceGitRequest{
        WorkspaceID:"abc", Action:"unstage", Path:a,
    })
    if err != nil { t.Fatal(err) }
    // restore --staged requires HEAD, so establish the initial commit first.
    run("git -C " + remoteArchiveShellQuote(dir) + " commit -qm initial")
    if err := os.WriteFile(filepath.Join(dir, a), []byte("Changed\n"), 0600); err != nil { t.Fatal(err) }
    command, _ = remoteWorkspaceGitCommandForRequest(dir, remoteWorkspaceGitRequest{
        WorkspaceID:"abc", Action:"stage", Path:a,
    })
    run(command)
    run("git -C " + remoteArchiveShellQuote(dir) + " diff --cached --name-only")
    run(commandForRemoteGitTest(t, dir, "unstage", a, ""))
    if got := run("git -C " + remoteArchiveShellQuote(dir) + " diff --cached --name-only"); strings.TrimSpace(got) != "" {
        t.Fatalf("unstage left staged data: %q", got)
    }

    run(commandForRemoteGitTest(t, dir, "stage", a, ""))
    message := "Safe quote: Paul's change; echo NOT_A_SHELL_COMMAND"
    run(commandForRemoteGitTest(t, dir, "commit", "", message))
    if got := run("git -C " + remoteArchiveShellQuote(dir) + " log -1 --format=%s"); strings.TrimSpace(got) != message {
        t.Fatalf("commit message changed: %q", got)
    }
}

func commandForRemoteGitTest(t *testing.T, root, action, path, message string) string {
    t.Helper()
    command, err := remoteWorkspaceGitCommandForRequest(root, remoteWorkspaceGitRequest{
        WorkspaceID: "abc", Action:action, Path:path, Message:message,
    })
    if err != nil { t.Fatal(err) }
    return command
}
