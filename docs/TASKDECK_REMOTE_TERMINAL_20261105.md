# TaskDeck Remote Terminal PTY — Linux/macOS implementation (2026-11-05)

Branch: `feature/taskdeck-remote-access-architecture`.

## Code path

The independent native Remote GUI has a dedicated **Terminal PTY** tab.
Five precisely enumerated E2EE operations are implemented:

| Operation | Native UI | Host action |
| --- | --- | --- |
| `terminal.open` | Explicit confirm / Open | Spawn one `/bin/sh -i` Unix PTY in canonical workspace |
| `terminal.poll` | Read output | Return the next bounded text chunk from the host-owned output cursor |
| `terminal.write` | Type one line; Ctrl+C / Ctrl+D | Write at most 96 UTF-8 bytes to that same PTY |
| `terminal.resize` | Choose rows/columns | Apply a host PTY window size within strict limits |
| `terminal.close` | Explicit confirm / Close | Kill shell process group, close PTY and release the session-owned job |

All actions are encrypted end-to-end between the independently pinned native
client and the host Agent. PHP Portal/Relay never receive the terminal input,
terminal output, shell environment, TaskDeck password or a transferable
terminal token. A terminal has no externally selected ID or executable, no
reconnect token and no remote-selected working directory.

The shell runs only when `TASKDECK_REMOTE_TERMINAL=1` **on the host**,
with a live approved-client-key + TaskDeck password principal + Portal
grant, and **both** the `terminal.create` and `terminal.control_own`
permissions. Reading requires `terminal.view_own`; writing, resizing and
closing require `terminal.control_own`. The host rechecks local RBAC and
the uncached Portal grant on every action, including before/after mutation.
Portal viewer and operator roles alone cannot start a shell. Running
TaskDeck with effective UID 0 is deliberately refused; Windows and other
platforms fail closed.

The PTY has a 90-second absolute limit, one process per authenticated E2EE
session, a 16-KiB output capture, 128-byte maximum response chunks, and
the existing 16-round E2EE budget. When the channel, host Portal grant or
local TaskDeck session is closed, a watcher cancels the PTY and the Unix
process group. All mutations require host-local attempted audit *before*
the syscall, containing no command text or output. Shell environment and
program are fixed on host (`/bin/sh -i`, no secret-token forwarding).
The native UI always uses `textContent`, never host HTML insertion or
escape-code interpretation. Output and typed input are cleared on logout.

This is a **bounded native Remote Terminal**, not a full-bandwidth generic
HTTP/WS port forwarder. Its current UI requires user-initiated reads,
supports one-line input segments, and does not silently renew the
authenticated session or exceed quotas. For long-running or reconnectable
sessions, a separately reviewed streaming protocol would be required.

## Tests

The added Go/browser suites cover exact E2EE Terminal request/result
schemas, action-level permission gates, invalid control characters,
oversized input, dimensions, native-local origin/JSON validation, browser
confirmation and scrub, real Unix PTY I/O, resize, shell command execution,
host-local audit and process group cancellation after logout. Go CI runs
these tests with the race detector as a dedicated Terminal job as well as
the pre-existing Go/Portal security suites.

## Remaining feature backlog

- Advanced task dependencies, secrets and cross-session resumable output.
- More Git operations: fetch/log/diff, stage/unstage and branch management.
- Other GUI screens: editor, file interactions, patch and collections.
- Further remote features and production security/release gates.

The bounded Remote GUI task runner and fixed Git pull/commit/push operations
were implemented in earlier commits. Hardware/live acceptance and an
independent security review are deployment gates, **not unfinished code**.
