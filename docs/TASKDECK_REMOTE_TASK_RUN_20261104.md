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

## 2026-11-04 — two host-authored pickString inputs (Linux/macOS)

- The bounded task runner now allows **up to two** task inputs when both
  are `pickString`, each with 1–6 sanitized unique host-defined options.
  Mixed `pickString`/`promptString`, multiple free-form inputs, secret
  input and dependency workflows remain unsupported.
- The Agent returns both input IDs and their option labels through the
  authenticated `tasks.inspect` reply. The native UI shows two local
  selectors, with no raw option values sent in the command request.
- Execution carries one 1-based combined choice index in the existing
  E2EE `tasks.run` request; in host-defined input order, its value is
  `(firstChoiceIndex-1)*secondOptionCount + secondChoiceIndex`.
  The maximum is 36. A single `pickString` retains its original 1–6
  indexing and `promptString` continues to use its separate encrypted
  short-token request.
- Host re-reads the unchanged `.vscode/tasks.json`, validates distinct
  input identifiers, both option lists, snapshot digest and project RBAC,
  and resolves both values with TaskDeck's existing task resolver.
  Input choice is not a generic shell/command/argument API.
- Existing 512-byte authenticated plaintext envelope, 16-action quota,
  6-second short-task limit, persistent host-side audit, output bounds,
  session revocation and Unix child-process cleanup remain enforced.
  Tasks with excessively large option metadata cannot fit the envelope
  and are rejected rather than truncating a choice silently.
- Regression coverage checks E2EE typed inspection, combined selection,
  invalid indices, stale updates to second global input, operator
  execution and the native GUI's two-selector behavior.

No Windows-specific implementation is included in this phase.

## 2026-11-04 — terminal task outcome audit

- The original **mandatory** local attempted audit remains committed before
  any process is started. If it fails, the task cannot run.
- After the process finishes or is killed, TaskDeck now writes a second
  best-effort, host-local `remote.action.tasks_run_finished` record:
  `completed`, `failed`, `cancelled`, or `timed_out`.
- Outcome metadata contains only the Task.ID, numeric exit code
  (or -1 when unavailable), and elapsed milliseconds. It does not record
  command strings, process output, input values, environment, secrets or
  credential material.
- Cancellation and Portal/session revocation still attempt outcome
  logging using a bounded internal context, because the original request
  context has already been revoked. A failure of the *post-execution*
  outcome audit does not undo the mandatory pre-execution attempt audit.
- Regression tests cover successful exit, nonzero exit and cancellation
  during an actual authenticated Unix child-process execution.

No Windows-specific work in this change.

## 2026-11-05 — staged session-owned long-task engine (not yet exposed to GUI)

New *host-only* primitives implement a bounded long-running job lifecycle,
without exposing any new Relay, PHP, browser or E2EE operation in this commit:

- Maximum 90-second execution with the existing Unix process-group cleanup.
- At most **one** in-memory job per authenticated E2EE session; must not
  replace an existing job without explicitly finishing and clearing it.
- Maximum 16 KiB captured stdout/stderr, always drained, sanitized, with
  maximum 160-byte Unicode-safe incremental chunks.
- A session-owned polling method can wait at most 7 seconds for output;
  independent cancel does not require dropping the session.
- The job terminates on authenticated session close, E2EE channel loss,
  Portal grant revocation or timeout. Job state is never stored in Portal.
- The exact host task source, stable issued ID, runtime opt-in, workspace
  directory boundary and mandatory pre-start audit are checked before spawn.
  Existing input constraints still apply.
- Linux/macOS regression tests cover incremental output, simultaneous-job
  rejection, cancel without logout, session-close child-process cleanup,
  stale task definitions, default-off opt-in and memory cap.

**Release status:** This is foundation code only. Its methods are intentionally
unmounted; do not describe long task streaming, independent remote cancel or
polling as available to users yet. E2EE typed actions, native GUI controls,
live permission rechecks on each job action, and transport-level tests are
required before enabling. Set `TASKDECK_REMOTE_TASKS_LONG=1` only for
approved development tests; default remains disabled. No Windows work.

## 2026-11-05 — E2EE + trusted native GUI long-task controls

The previously staged session-owned long-task engine is now wired through
four **enumerated** AEAD operations on the Agent and an independently bundled
native-local GUI:

- `tasks.start` — one previously issued and reverified host Task.ID plus
  the same narrow zero/one/two-input selection rules as bounded `tasks.run`.
  Host requires `TASKDECK_REMOTE_TASKS_PREVIEW=1`,
  `TASKDECK_REMOTE_TASKS_RUN=1`, and the **independent opt-in**
  `TASKDECK_REMOTE_TASKS_LONG=1` (off by default).
- `tasks.poll` — host-owned incremental output/status, at most 160 bytes
  per authenticated reply. Poll may wait up to 7 seconds for fresh output;
  terminal statuses include completed/failed/cancelled/timed_out.
- `tasks.cancel` — asks the Agent to stop its Unix task process group but
  **does not revoke the E2EE session**. The final status can be polled.
- `tasks.clear` — releases an already completed/cancelled job so a
  subsequent task may start within the same session.
- Only one job per session; 90-second maximum job duration and 16-KiB total
  host-memory output cap. Job dies on session logout, grant revocation,
  closed channel or expiry. No PHP Portal persistent job storage.
- Every operation requires the TaskDeck project `tasks.run` permission,
  approved native key, local password principal and **fresh Portal
  authorization before and after** execution. Mutations are audited locally
  before cancellation or clearing; job start has a mandatory pre-start
  audit and completion records only sanitized metadata.
- Trusted native GUI offers **Chạy task dài**, **Nhận output tiếp**,
  **Hủy job (giữ phiên)**, **Dọn job đã kết thúc** with text-only output
  rendering. There is no general remote shell or arbitrary HTTP proxy.
- Output retrieval is **manual incremental polling**, not yet continuous
  automatic streaming. The shared E2EE quota of 16 application actions per
  session means long/noisy jobs may exhaust the quota; the UI then requires
  a fresh login. Do not claim unlimited streaming.
- Fixed a native GUI metadata parity defect: forwarding `input_type`,
  `input_id_2`, and `choices_2` is now required for previously implemented
  promptString and two-pickString tasks to work in the real trusted GUI.
- Linux/macOS only; no Windows-specific implementation or test additions.

Still not implemented: unattended true stream subscription, independent
persistent job identities across logout, multi-step task dependencies,
secrets, arbitrary free-form arguments, Git writes, terminal PTY, and
high-volume output export. Production remains NO-GO until the separate
security gates and tests have passed.

## 2026-11-05 — auto polling without exhausting cancellation quota

- The host now emits an explicit authenticated `more` boolean in each
  `tasks.poll` E2EE result: whether the per-session output cursor still
  has buffered, sanitized text. This is separate from `truncated`
  (which indicates the 16-KiB capture limit was exceeded).
- A terminal job may still have unread output. The native UI displays
  `còn output chưa nhận` and keeps Poll available while `more=true`.
  If the operator deliberately chooses to clear first, the button warns
  `Bỏ output còn lại và dọn job` rather than silently losing that data.
- After a user explicitly starts a long job, native GUI automatically
  requests incremental output in bounded E2EE polls; the user can pause
  or resume automatic fetching locally. Automatic polling stops for a
  terminal job when no buffered output remains.
- Auto polling **reserves three of the existing 16 session action rounds**
  for actions such as Cancel and Clear, stopping once remaining<=3. Manual
  Poll may spend those reserved rounds only by an explicit user click.
  The 90-second host limit, 7-second poll wait and 16-KiB capture bound
  remain unchanged.
- Every poll remains a fresh native E2EE operation subject to TaskDeck
  password/key/RBAC and uncached Portal authorization on the host.
  Pausing auto-poll does not cancel the host job. Local logout still
  revokes the session and cancels the process group.
- Added coverage for multi-chunk output after terminal status,
  invalid `more` metadata and native GUI pause/resume, unread-output
  warning, quota reservation and deliberate manual tail retrieval.
- These changes intentionally cover Linux/macOS only, with no Windows
  feature development. The Remote Control production go-live gate
  remains disabled.

Auto polling is bounded repeated requests, **not** an unlimited push
stream. A job which produces more output than the E2EE session quota
permits still requires a new authenticated session or an explicitly
reviewed future resumable job protocol (not yet implemented).

## 2026-11-05 — E2EE escaped-output frame safeguard

JSON control/quote escapes and the default HTML escaping can enlarge a
160-byte raw output beyond the fixed 512-byte AEAD plaintext envelope.
Long-task output chunks are now capped at **128 UTF-8 bytes** and encoded
with HTML escaping disabled (still JSON-encoded, still textContent-only in
the native GUI). The worst-case 128 doubled JSON bytes plus fixed fields
fit in the envelope; remaining bytes stay in the host-owned cursor and
are indicated by `more=true`. Regression tests cover special characters.
The 16 KiB aggregate capture and 90-second job limits remain.

## 2026-11-05 — protect long-task controls during bulk task-page loading

- When the trusted GUI has an uncleared long task, unattended **bulk**
  task-preview paging cannot consume the final three authenticated E2EE
  action rounds. These are kept for cancellation, status checks and cleanup.
  An explicit one-page fetch is still a separate user choice.
- Bulk paging can temporarily occupy the single authenticated action slot
  while an auto-poll timer expires. The GUI now rearms the pending long-task
  poll when bulk loading exits, unless the session has closed or the
  three-action reserve has been reached.
- The disabled bulk button, handler guard and paging-loop boundary agree.
  This avoids bypassing the reserve through programmatic event dispatch.
- Extended the existing native JS execution fixture with a paused bulk
  request, an auto-poll timer firing during that request, stop/restart and
  cancellation-quota assertions. The native launcher text now accurately
  states that host-opted-in task execution exists; it no longer claims
  the GUI is read-only.
- **Implementation complete for this bounded coordination fix.** The
  unsupported broader capabilities are still Git pull/commit/push,
  interactive Terminal, remaining Git operations, other GUI commands,
  task dependencies/secrets and high-volume resumable output. No Windows
  implementation was added. Real-device deployment is a separate gate,
  not a source-code TODO.

## 2026-11-05 — stable task-input controls during remote GUI polling

- The native-only task list now keeps one in-memory draft per already-inspected
  Task.ID for up to two host-approved `pickString` choice indices or one
  bounded non-secret `promptString` token. Changing local sort/filter/page
  does not silently reset an operator's choice to the first option.
- Long-task `start`, `poll`, `cancel` and `clear` progress no longer
  reconstruct the unchanged task DOM. `render()` still updates button
  authorization/disabled state. This preserves the focused editor while
  output arrives, without issuing additional E2EE requests.
- Drafts are strictly transient local JavaScript state, not browser storage;
  they are cleared on every fresh authenticated task snapshot, task
  re-inspection and session close/revocation. Host still verifies exact
  Task.ID, source snapshot, allowed values, RBAC and live Portal grant
  independently. No new actions, privileges or output quotas were added.
- New regression files:
  `cmd/taskdeck_remote_login/trusted_gui_task_drafts_test.go` and
  `trusted_gui_task_drafts_test.cjs`. The fixture executes the shipped
  JavaScript, drives filter/sort/long polling/logout events, and verifies
  two-choice index binding, prompt preservation and snapshot scrubbing.
  JavaScript syntax and simulated UI tests PASS; device deployment remains
  separate from code completion. Production Remote Control remains OFF.

## 2026-11-05 — explicit launch confirmation and Unix task/PTY exclusion

- A trusted native GUI click on **Chạy task** or **Chạy task dài** now
  requires an explicit local confirmation *after* the host-issued Task.ID
  and any selected host-approved inputs have been validated. The dialog
  identifies the task label, stable ID, short/long duration limit and
  fixed pickString choices, and warns that the host task can mutate data.
  A promptString token is deliberately **not** echoed in the dialog.
  Rejecting the confirmation sends no local HTTP or E2EE action.
- Native GUI disables both task-launch buttons whenever a Terminal PTY
  remains opened (including a shell with exited state pending Close).
  A forged click still encounters the same guard before `invoke()`.
- The Unix Agent independently rejects short or long task starts if
  the authenticated session owns a PTY; it does not trust the GUI state.
  The lock order is `terminalMu -> longTaskMu`, identical to PTY-open,
  preventing the opposite-direction race. Short tasks hold the terminal
  exclusion lock until their bounded process group returns. PTY-open
  already rejects a session-owned long job.
- Added Go regression tests for both directions: PTY blocks short/long
  tasks without side effects, task execution resumes after PTY close,
  and a long task blocks PTY until cancelled and cleared. The native
  JavaScript execution fixture also covers decline-confirm (no E2EE
  request), choice display and both launch controls while PTY is open.
- No additional task definitions, shell commands, arguments, permissions,
  Portal capabilities, quotas or Windows-specific features were added.
  Deployment/security acceptance remains separate from source-code
  completion. Remote Control remains default-OFF.

## 2026-11-05 — eliminate short-task versus long-job overlap

The earlier Unix Terminal/task mutual exclusion left one opposite-direction
hole: an authenticated long job returns from `tasks.start` while its host
process is still alive. `tasks.run` only checked `terminalMu`, so a short
task could start alongside the ongoing long job.

- The Agent's bounded short-run path now acquires `terminalMu` followed by
  `longTaskMu`, rejects any session-owned long job, and retains these locks
  through the bounded process execution. This is the same lock order used by
  TerminalOpen and long-task start, preventing concurrent PTY/short/long
  executions without introducing a lock-order inversion.
- The independently trusted native GUI disables and guards **both**
  short and long task launch paths for any uncleared `longTaskID`,
  not merely the long-task button. The Agent check is authoritative.
- A Go Unix regression starts a real long task and checks that attempting
  `tasks.run` returns the denial immediately, before a second process can
  start. The native browser execution fixture checks a forged click cannot
  invoke either task launch while the long job is active.
- `Dọn job` now explicitly asks for confirmation when `more=true`
  indicates unread captured output. Refusing that confirmation never sends
  `tasks.clear` or uses another E2EE action; the user can still manually
  poll the remaining text before clearing. An executable browser fixture
  checks this behavior.
- In-process JavaScript fixture executions PASS for the guarded task UI
  and the unread-output confirmation. The Go regression was committed
  for CI and is not claimed PASS before its workflow finishes. No new
  remote command capability, client-selected executable, arbitrary
  task arguments, permission or Windows implementation was added.

Pending advanced task work remains host-authored dependencies, secret input,
longer/reconnectable output semantics and cross-session job resumption.
Live-device and production security approval are separate gates, not
unfinished source code for the supported bounded operations.

## 2026-11-05 — bounded native task execution history

The native localhost GUI now keeps a small, *ephemeral* history of the six
most recent authenticated task runs for the current browser/E2EE session.
This solves the loss of the last task outcome when an operator checks
permissions, loads another task page, or reads Git status:

- A bounded short task stores only its host-issued Task.ID and label,
  nonzero/success exit status, exit code, truncation flag, and the already
  bounded/decrypted short-output text.
- A long task creates one history entry at successful start and updates
  that entry as polling, cancellation and completion provide new statuses.
  Its full 16 KiB output is **not** duplicated into history; it stays only
  in the existing long-job output pane, which is cleared with the job.
- Entries are rendered with `document.createElement` plus `textContent`;
  even malicious task labels and output looking like HTML cannot execute
  as script. History is capped at six entries, strictly in JS memory,
  never written to cookies, storage, logs, PHP Portal or the Relay.
- "Xóa kết quả" removes every local entry **without an HTTP/E2EE request**;
  Disconnect, expiry and pagehide synchronously scrub the same data.
  Re-fetching the host task list does not rewrite a completed outcome.
- Browser regression tests exercise nonzero-exit output, inert HTML-like
  labels, the six-entry cap, persistent display through unrelated reads,
  explicit local clear, logout, and an independently finalized long-job
  summary. These are new UI affordances only: no host process, transport
  command, role, task argument or security opt-in has changed.

Only complete code features are counted as complete. Actual live-device
deployment and separate remote security release gates do not turn this
local result-history feature into unfinished code.

## 2026-11-05 — correct in-memory short-task history status

The new native-only task outcome history must distinguish an E2EE action
that finished normally from the host process that actually succeeded.

- A short task with `exit_code=0` records **completed**. A short task
  with `exit_code=1..255` records **failed**, consistent with Agent audit
  results and the previously validated exit code.
- Only the local display classification changed. The task command, host
  permissions, E2EE protocol, timeouts, process exit handling, output
  sanitization and existing bounded six-item session-only retention remain
  unchanged. History is still cleared at logout.
- The native browser regression now checks both exit 7 -> failed and exit
  0 -> completed, and all seven relevant native GUI task fixtures
  passed isolated JavaScript execution. CI on this new skip-ci commit
  has not been run; earlier Go/PHP workflow PASS applies to the older
  head `5bf19a6` only.

## 2026-11-05 — incremental native task pagination without losing focus

- When an authenticated `tasks.preview.next` extends the cached task list but
  does not alter the four visible task rows (under the current search/sort),
  the GUI now retains the existing DOM nodes and their focused task-input
  controls. Only pagination, progress, next-page controls and coverage change.
- When the newly received labels actually change the visible page, the
  existing render path still rebuilds those rows; new full snapshots still
  invalidate prior inspections and input drafts for safety. No task command,
  E2EE operation, authorization, timeout, retention or background polling
  semantics change.
- A native GUI JavaScript regression checks both stable row identity and
  page-count updates after an authenticated next-page response.

## 2026-11-05 — native long task final-round quota protection

- The automatic poll and bulk task page buttons protected the final three
  E2EE rounds, but manual pagination, fixed reads and the quick palette
  could still consume the cancellation/cleanup budget.
- With an outstanding long task and three or fewer rounds, the native GUI
  now reserves rounds for cancel → poll → clear; after a terminal status,
  output reads may continue while reserving the clear round.
  Regular controls are disabled and the central `invoke` guard also
  rejects synthetic clicks. Disconnect remains available.
- Browser regression covers disabled controls, direct clicks and quick
  palette bypass attempts followed by output retrieval and clear.
  No E2EE wire or Agent authorization change.
