# Remote Task selection / inspection — 2026-11-04

Branch: `feature/taskdeck-remote-access-architecture`.
The work in this checkpoint implements authenticated task *inspection*, not
remote task execution.

## Implemented in source

- `tasks.preview` and `tasks.preview.next` return display labels with
  parallel stable host-derived Task.IDs, bounded by the pre-existing 512-byte
  encrypted plaintext limit. The adaptive envelope fitter trims labels and
  IDs together. A duplicate-definition ID is zero (nonselectable). No
  commands, process environment, task parameters, secrets, workspace paths or
  source hashes leave the host.
- The trusted, locally bundled GUI (127.0.0.1 only) displays those IDs and a
  per-task **Kiểm tra ID** operation. Unknown, missing, noncanonical, extra-
  argument and PHP-origin routes fail without reaching the authenticated
  E2EE TaskDeck action.
- The authenticated `tasks.inspect` record carries only one bounded numeric
  ID, already published from *this* host's task preview and tracked in the
  in-memory TaskDeck principal/session. The task source must still match
  the host-owned workspace+file hash and pass its execution preflight. Every
  inspection rechecks current local TaskDeck permissions, owner-approved
  client-key proof, live Portal grant, account and audit. The returned record
  only confirms the selected ID is eligible for later review, NOT that the
  task was executed or that Remote Control is enabled.
- Existing 16-round stop-and-wait AEAD quota, independent Client/Agent,
  PHP opacity and 5-minute native session expiry are unchanged.

## Important limitations / next implementation

1. **Not yet implemented:** E2EE `tasks.run` command, explicit per-task
   host approval, execution/cancellation lifecycle, terminal output stream,
   command arguments and secret-aware input UI. No process is started by the
   new code. The preflight uses local host task definitions only.
2. Then implement user-prioritized Git pull / commit / push with distinct
   per-operation RBAC and safe, auditable host-side execution.
3. Then remote interactive Terminal, remaining Git operations, other GUI and
   remaining functions.
4. Keep `remote_control_available=false`, Remote OFF by default and
   production LIVE NO-GO until real end-to-end security and cross-device
   acceptance. CI source PASS is not a production deployment approval.
5. When updating the 56-task paging test, count pages dynamically: including
   IDs in authenticated plaintext legitimately reduces labels per frame,
   but must never skip or duplicate tasks.

Do not treat a code-complete feature as unfinished solely because a physical
environment test was not run. Conversely, task execution is **not code-complete**
while only selection and inspection exist.
