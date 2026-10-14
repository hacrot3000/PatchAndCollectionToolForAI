# SSH Terminal Auto-Reconnect — Implementation and Operation

Branch: `main` (SSH reconnect module and websocket recovery available in main).
Date: 2026-10-14.

## Behavior

TaskDeck has separate layers of connection state:

- **Browser WebSocket → TaskDeck**: if TaskDeck's terminal session is still running, the existing frontend WebSocket reconnect mechanism is responsible. This does not create a new SSH login.
  - If the WebSocket and the status API are both temporarily unavailable, the tab retries attaching the **same session**, using delays starting at 600 ms and capped at 15 seconds, without retrying SSH itself.
  - The current terminal output stays visible during failed attachment attempts. Terminal scrollback resets only after the replacement socket opens and replays server output.
- **TaskDeck SSH client → remote host**: if OpenSSH process finishes, the SSH tab receives session metadata with `status`, `exit_code`, `target_type=ssh` and `target_profile_id`. The new SSH reconnect module handles this case.

Reconnection eligibility requires ALL of the following:

1. This is an actual SSH terminal with a known profile, not a local terminal, task or Patch Tool process.
2. The browser observed the SSH session running before it exited; a stale, already-exited session restored on page load does not trigger background retries.
3. The session stopped unexpectedly, `status=exited`, and the process exit code is **255**.
4. The user did **not** deliberately enter `exit`, `logout`, `exit 255`, or press Ctrl+D at an empty prompt, and did not click Stop/Close/Cancel retries.
5. There is no detected SSH host-key/authentication failure requiring user intervention.
6. The browser still owns a valid active view, **has control permission for that specific SSH session** (not merely read-only visibility), and is authorized to open that SSH connection.

**Exit code 0 is always treated as normal SSH termination and never auto-reconnected**, as requested. Other non-255 codes are not automatically retried. A 255 is a *candidate* transport error, not proof; explicit user intent and fatal-error checks take precedence.

## Retry strategy

- Enabled by default, with a per-workspace **Auto reconnect** checkbox visible in the SSH terminal panel. Preference is stored in this browser's localStorage.
- At most **five attempts**, using delays of 2, 5, 10, 20 and 30 seconds. No infinite retry loop.
- The original SSH tab, scrollback and process exit status remain available. Once a new SSH terminal session is running, the **replacement opens in a separate tab**. It is activated only if the user is still focused on the failed tab; unrelated active tabs are not interrupted.
- Reopened sessions use the **same saved SSH profile**. Authentication, host-key verification, permissions and settings are enforced through the existing `POST /api/sessions` server API. There is no automatic host-key override or arbitrary command execution.
- Fast connection failures are examined before adding the new tab; unsuccessful new sessions are cleaned up. If the originating tab is closed or retries are cancelled during creation, the unused candidate is stopped/removed when possible.
- `Reconnect now` opens a visible terminal immediately, including profiles that require an interactive password or key passphrase. Unstored credentials are never submitted in the background.
- A new SSH session is a new login; unsaved shell state, jobs, variables and manual `cd` operations are not guaranteed to be restored. **Do not auto-rerun unfinished server commands**, especially writes such as crontab, database changes, restarts or deployment scripts.

## Visible SSH tab controls

`Auto reconnect`, `Reconnect now`, `Cancel retries`, and a status line showing countdown, attempt count, connected replacement, or why automatic retry was blocked. A user who deliberately logs out sees the completed terminal and may choose manual reconnect; nothing happens automatically.

## Security / limits

- The client-side command-intent recognizer is best-effort and observes submitted interactive text and Shell Integration command-start markers. It catches conventional `exit`, `logout`, optional numeric exit arguments and Ctrl+D at empty input. Exotic scripts or nested shell commands can be ambiguous.
- Exit 255 might also result from host authentication or other SSH-specific failures. Known fatal texts suppress automatic retry. Every path has a finite attempt budget. **Never** disable strict SSH host-key verification or assume retries fix authentication.
- Browser reload clears in-memory retry state. A running SSH session can be reattached normally, but an old exited session is not auto-restarted solely because a tab was restored.
- Stored browser preference is per workspace/browser; it does not modify the server-wide SSH configuration.
- In multi-user mode, the existing server-side session-create authorization still checks `ssh.use`, plus profile availability and user permissions. Auto reconnect never skips those checks.
- A replacement SSH tab uses a fresh PTY/terminal instance and does not preserve active remote processes from the previous disconnected SSH session. Whether server-side processes survive a disconnect depends on the remote server's process management.

## Validation

- `test_ssh_reconnect.cjs`: policy tests for exit 0/255/other codes, explicit logout, dangerous auth/host-key failures, terminal identity, bounded delays and UI wiring.
- `test_ssh_reconnect_runtime.cjs`: mocked frontend event lifecycle for normal exit, explicit `exit 255`, Ctrl+D, transport timeout 255, host-key failure, opt-out and already-exited restored tab.
- `test_ssh_websocket_recovery.cjs`: regression for metadata/proxy outage during WebSocket closure, same-session recovery, deferred reset of terminal screen, and SSH exit-code handoff.
- Both tests are registered in `.github/workflows/vscode_tasks_menu_go.yml`.
- Manual testing is still required against a real SSH host: (1) login then `exit` produces code 0 and no retry; (2) kill network or SSH transport to produce 255 and observe retry; (3) check Cancel, no saved password, changed host key, tab close during retry and browser reconnect.
