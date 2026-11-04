# TaskDeck Remote GUI: bounded task.run (2026-11-04)

## Scope and threat boundary

This checkpoint adds an E2EE **single-shot short task** operation. It does **not**
enable generic Remote Control, arbitrary shell, background jobs, long-running
streaming tasks, user-supplied command/arguments, input prompts, secrets,
workflow dependencies, Git writes, Terminal or remote file mutation APIs.

PHP Portal / Relay remain untrusted ciphertext couriers. The trusted native
client verifies the pinned Agent identity, TaskDeck password and an approved
software client key. Every action is bound to that Agent-owned principal,
single E2EE channel and live, revocable Portal grant.

## Host opt-in and authorization

Remote access is OFF by default. Task run is independently OFF unless the
host process explicitly sets BOTH:

- TASKDECK_REMOTE_TASKS_PREVIEW=1
- TASKDECK_REMOTE_TASKS_RUN=1

The TaskDeck principal must independently hold tasks.run permission for the
authorized project, and Portal must grant operator or admin. Viewer cannot run
tasks even with both environment flags enabled. Availability shown by
session.features is advisory; host rechecks privileges and opt-in before each
run. \`remote_control_available=false\` continues to mean unrestricted remote
control remains disabled.

## Request and execution

1. Native GUI reads the host-owned task preview, receiving sanitized labels
   paired with stable Task.IDs. The host retains a snapshot hash and the
   exact IDs actually published to this authenticated E2EE session.
2. Clicking **Chạy task** sends only one canonical numeric ID via localhost
   capability-protected POST to native TaskDeck. E2EE \`tasks.run\` uses
   the same fixed 512-byte authenticated plaintext limit and one of the
   existing 16 one-at-a-time actions.
3. On the host, the dispatcher checks PermissionTasksRun and the live Portal
   grant, verifies the issued ID against the unchanged workspace and
   \`.vscode/tasks.json\` source hash, and reloads the actual task definition.
   It refuses ambiguous stable IDs, unsafe labels, tasks with input prompts,
   secrets, dependencies, and working directories outside workspace.
4. The host writes a persistent TaskDeck-local \`attempted\` audit record
   **before** launching the process. Audit includes numeric Task.ID and
   authenticated local principal, but not commands, output, secrets, tokens
   or credentials.
5. A new process group runs host-authored command/args with a 6-second
   deadline. Revocation, timeout and logout cancel execution. stdout/stderr
   are drained to a bounded buffer; the authenticated response returns exit
   code 0–255, up to 192 bytes of sanitized UTF-8 and a truncation flag.
   A nonzero exit code is a normal task completion result, not a grant.
6. The dispatcher rechecks current local TaskDeck RBAC, account, client key
   and Portal revocation before accepting the result. The native GUI renders
   output using textContent only.

## Current limits and remaining priorities

- This **short-task subfeature is implemented in code**. It is not a full
  remote task runner: tasks requiring inputs, secrets, dependencies, long
  execution, detached sessions, progress/output streaming or interactive
  cancellation controls remain unsupported.
- Do not rely solely on source CI to approve production; device end-to-end
  testing and separate review are still needed. Production LIVE remains
  NO-GO, with Remote disabled by default.
- Follow agreed priority: finish Remote GUI task execution; Git pull /
  commit / push; interactive Terminal; other Git commands; other GUI;
  remaining remote features. Tasks with code complete but only physical
  execution untested should be tracked separately, not marked code pending.

## Security test coverage

Typed E2EE binding of request and result to selected Task.ID; refusal of
malformed result, wrong task ID, stale snapshot, viewer role and host opt-out;
output buffer truncation; authenticated audit and nonzero exit-code behavior.
CI also checks native-only GUI localhost capability, rejection of command
payloads and PHP Origin, and its task-preview/inspection controls.

## 2026-11-04 continuation — one fixed pickString input

- Added one-parameter `pickString` support. Up to six unique options,
  each at most 32 ASCII bytes, are loaded only from the host's
  `.vscode/tasks.json`; unsupported promptString, secrets, multiple
  inputs and dependency workflows remain denied.
- The native GUI must inspect the selected host-issued task ID first.
  Inspection returns only the non-secret input identifier and its fixed
  options through E2EE. Native GUI shows a local `select`; execution
  sends a **1-based choice index**, never user-entered values or shell
  arguments. The host re-reads the task definition/snapshot and resolves
  the selected option with TaskDeck's existing ResolveExecutionWithInputs.
- Requests with choices on non-run operations, duplicate/unsafe values,
  missing options, out-of-range indices or changed source snapshots are
  rejected. Inspection metadata is cleared on preview refresh, logout,
  expiry and channel revocation. Both result and native-local route have
  fixed schemas and bounded lengths.
- No stored password, shell command, input content, environment or
  secret is sent to the PHP Portal; the 512-byte AEAD frame and local
  permission/grant rechecks remain unchanged.

**Still unsupported:** promptString/free-form entry, multiple inputs,
secret-dependent tasks, workflow dependencies, long-duration tasks,
output streaming, interactive stop/cancel, Windows whole-process-tree
execution, Git writes and terminal. This is a code implementation,
not a production go-live approval.

## 2026-11-04 — native task stop (Linux/macOS)

- The trusted native GUI now exposes **Dừng task và ngắt phiên**
  only while an authenticated `tasks.run` request is in progress.
- Selecting Stop immediately posts to the existing localhost-only
  `/api/close` endpoint using the already verified local capability.
  The native session revokes E2EE and cancels the active action; the Agent
  terminates the task's Unix process group, not only the parent shell.
  The GUI erases the selected tasks and rejects any delayed result.
- **This is cancellation by ending the session**, not independent job
  management. Resuming work requires a new authenticated session.
- Regression coverage: browser pending-response stop, context cancellation,
  cleanup of background descendants on normal task exit, and the actual
  authenticated session Close during execution. Unix tests target Linux
  and macOS; no Windows-specific changes are included or planned now.
- Pending P0: independent cancel without closing an E2EE session, execution
  beyond the 6-second bounded mode, incremental output/status, promptString,
  multiple inputs, secrets and dependency graph support. Git writes remain
  the next priority after the task GUI code is complete.

## 2026-11-04 — one bounded process-only promptString input

The trusted native GUI additionally supports exactly one `promptString`
parameter when the task on the host has `type:"process"`, not shell.

- Input must be a nonempty 1–64 byte ASCII token, starting with a letter
  or digit; remaining bytes may be letters, digits, underscore, hyphen or
  period. Spaces, separators, control bytes, quotation marks, pipes and
  shell metacharacters are rejected. This is intentionally narrower than
  generic VS Code `promptString` and is not suitable for passwords.
- `tasks.inspect` discloses only the host-authored input ID and type
  through E2EE after the current per-session snapshot check; it never
  discloses command/arguments or environment.
- The client sends the approved token in the JSON body of a localhost-only
  `POST /api/tasks-run-input/<host-task-id>` with the native local
  capability. The token **never enters the URL**, PHP, audit or logs.
  Native validates the exact one-key JSON object and then sends the value
  within the authenticated fixed-size `tasks.run` E2EE action.
- The host reloads the exact verified task source before every attempt.
  For promptString, it denies multiple inputs, task shell type, known
  shell executables, dynamic executable names, inputs used in cwd/env
  options, secrets and dependency workflows; the value may only become
  part of an argument list of the host-defined process.
- Cancelling a task still closes the E2EE session and terminates its Unix
  process group. PHP Portal never needs to store the token.
- Supported Linux and macOS code paths only. **No Windows-specific
  implementation is being added in this phase**.

Still unsupported: multiple inputs, arbitrary free-form strings or paths,
secret values, shell prompt substitution, long/streamed tasks, an
independently cancelable persistent task session, and dependency workflows.
Production LIVE remains NO-GO pending the existing security release gates.
