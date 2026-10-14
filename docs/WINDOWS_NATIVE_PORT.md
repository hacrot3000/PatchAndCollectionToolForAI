# Windows native TaskDeck (Go) — compatibility audit / ConPTY plan

Date: 2026-10-14 · branch `main`.
**This is not a claim that native Windows backend works.** Windows support shipped
in this phase is the Linux backend under Docker Desktop or WSL2, plus a native
WinForms launcher. Native Go is gated separately.

## Verified blockers in current repository

| Module | Existing implementation | Required Windows-native adapter |
|---|---|---|
| `internal/session/session.go` | `pty.StartWithSize`, `pty.Setsize`, Unix process-group signals via `syscall.Kill` | ConPTY session creation, resizing, I/O streaming, Ctrl+C/Ctrl+Break semantics, graceful shutdown and job object cleanup |
| `third_party/github.com/creack/pty` | Unix/Linux PTY code and ioctl | Windows ConPTY dependency (audited/vendor-pinned) or platform-specific internal adapter |
| `internal/terminal/menu.go` | `stty -g`, `stty raw -echo` | Win32 console mode / terminal handling |
| `internal/broker/server.go` | `syscall.Flock` and `net.Listen("unix", path)` | Windows named pipe or authenticated loopback broker, explicit ACL, per-workspace ownership |
| `internal/session/process_tree_windows.go` | Returns “process tree inspection is not available on Windows” | Windows Job Objects / process tree enumeration and safe cleanup |
| `internal/server/gitjobs_process_windows.go` | Kills immediate Git process only | Kill entire Git job tree; cancellation tests for nested git hooks/processes |
| `cmd/vscode_tasks_menu/main.go` | Unix signals for reload/shutdown, daemon controls | Windows named event/service hooks and staged updater |
| `internal/session/session.go` Patch protocol | Uses `ExtraFiles` / inherited FD3 & FD4; explicitly rejects Windows command pipe | Named pipe / authenticated IPC with protocol framing and lifecycle, not silent fallback |
| Runtime/paths | Bash commands and POSIX paths, chmod 0600, symlink handling | PowerShell/CMD command model, NTFS ACLs, Unicode and long paths, case and CRLF policy |

## Target architecture

Keep the browser UI and Git/Patch/SSH/database business logic shared.
Introduce `internal/platform/terminal` with a small `Session` interface
(`Start, Read, Write, Resize, Signal, Close, ProcessID`) and implementations
for `!windows` (existing PTY) and `windows` (ConPTY + Job Objects).
Use separate `internal/platform/broker` IPC adapters; do not make Windows
behavior a scattered runtime `if GOOS==windows` inside feature handlers.
Windows-only tasks must not be silently run inside Bash containers.

## Phased acceptance gates

1. **Native compile gate:** `GOOS=windows go build ./cmd/vscode_tasks_menu` PASS on real Windows runner; all Windows build tags resolved.
2. **Terminal gate:** PowerShell, CMD, ANSI, Unicode, interactive stdin, tab/arrow keys, Ctrl+C, resize, concurrent sessions and clean process tree close PASS using ConPTY.
3. **Broker/security gate:** per-workspace IPC authentication/permissions; restart, one-user and shared-server controls; no untrusted listener; no credential leakage.
4. **Git/Editor/FTP/SFTP gate:** checkout/merge/diff, upload/download large files, file watcher, normalized path handling, multi-repo and locking tests on NTFS.
5. **Patch gate:** Windows native Patch protocol must support event and command channels; failures are explicit, not silently lost.
6. **Updater/release gate:** staged EXE replacement after shutdown, rollback, MSIX/MSI signed distribution, clean installation on Windows 10/11.
7. **Publish gate:** native Windows engine release only after Windows integration smoke and migration tests pass.

## What is already usable

- `Dockerfile.taskdeck`: Linux `runtime` and `dev` image.
- `deploy/windows/taskdeck.ps1`, `taskdeck.cmd`, optional WinForms
  `TaskDeckLauncher.exe`: Windows front end with Docker Linux backend.
- `deploy/windows/taskdeck-wsl.ps1`: Linux TaskDeck backend via WSL2.
- Python Patch Tool has an existing Windows launcher and test lane;
  it does **not** prove native Go TaskDeck supports ConPTY.

Do not remove Unix PTY or weaken existing broker access checks to make
Windows cross-compilation appear successful; tests above are mandatory.
