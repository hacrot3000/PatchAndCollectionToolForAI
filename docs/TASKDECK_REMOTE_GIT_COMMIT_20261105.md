# TaskDeck Remote Git commit — staged-only E2EE (2026-11-05)

This milestone adds a bounded **Git commit** action to the independently
distributed native GUI on Linux/macOS. It is separate from Git pull, task
execution, Remote Workspace over SFTP, and the untrusted PHP Portal.

## Host-side behavior

- Explicit host opt-in: `TASKDECK_REMOTE_GIT_COMMIT=1` (default OFF).
- Requires an authenticated software client key, the TaskDeck user/password
  principal, fresh `git.write` RBAC, and a live Portal grant **per action**.
- The target is the exact canonical TaskDeck workspace Git root, never an
  arbitrary client-provided directory. Detached HEAD and repositories without
  an existing commit are deliberately refused.
- **Only files already staged in the host Git index are committed.** No
  implicit `git add`, `git commit -a`, branch selection, fetch or push.
- A single UTF-8 commit subject (1–120 bytes, one line) crosses the encrypted
  channel. PHP/Relay do not see it. No command, arbitrary argv, shell, path,
  URL, environment or Git config is accepted from the remote client.
- Git is invoked using a fixed argument vector without a shell. Hooks,
  signing and optional fsmonitor execution are disabled for the operation.
  The process has an eight-second timeout and is cancelled when its E2EE
  action is revoked. No automatic retries.
- A mandatory host-local `remote.action.git_commit` attempted audit is
  recorded **before** the mutation, tied to the actual TaskDeck user and
  project. Audit stores no message, file path, credentials or process output.
- The successful E2EE result contains only the previous and new lowercase
  40-character commit IDs. The GUI invalidates the cached Git Status and
  clears the commit message on success and logout. A malformed result closes
  the authenticated channel.
- No Windows-specific code added. Host-local Git identity must already be
  configured (e.g. repository-specific `user.name`/`user.email`). This
  code uses SHA-1 object-ID format and intentionally does not create an
  initial commit, select the index contents or sign commits.

## Tests and remaining priority

Regression tests cover strict request/response parsing and replay boundary,
Unicode message validation, denied viewer permission and disabled opt-in,
root confinement, real Git staged-vs-unstaged behavior, Git hook suppression,
local-only GUI JSON validation, the explicit browser confirmation, and
logout clearing. The Go Agent and PHP Portal CI jobs validate this branch.

Next remote-code priorities: Git push, Terminal, remaining Git operations,
and other GUI components. Advanced task dependencies, secrets and
cross-session resumable output remain separate enhancements and are **not**
represented by the completed bounded task GUI. Live device acceptance and
production security review remain separate deployment gates, not code TODOs.
