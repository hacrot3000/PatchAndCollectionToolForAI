# Main → Remote integration resolution (2026-11-05)

Source main: `2ded4207222acaed4f07f50fc9fd3bdfea5234d5`.
Target remote: `12af56d278f289b7dc8fac47dc49ce4adc6cca69`.
Shared ancestor: `5bf68380dac8b1719d8a493cc9ceb7926da237f1`.

The history-preserving merge commit `7971763d2ba8cd9ad9e9bb88e4c8f7cb256b1db1`
has exactly two parents (remote, main) and the byte-for-byte Git
`merge --no-commit --no-ff -X ours` tree.

## Post-merge compatibility selection

CI on that raw merged tree demonstrated duplicate Go E2EE result functions and
different, incompatible `PingClient` method signatures and request schemas.
In particular, main and remote independently implemented `git.diff`,
`git.log`, `git.commit` and task execution with different typed protocols.
Blindly taking a union would not be a safe authenticated remote API.

This follow-up keeps the **remote architecture implementation** as the
authoritative default under `internal/remotetransport`,
`internal/remoteportal`, `internal/remoterelay`,
`cmd/taskdeck_remote_login` and related `internal/server/remote_*` handlers.
Main's separate native interactive GUI, file-transfer implementation,
review/digest Git API and its dependent server helper files are *not*
enabled in this merged build. Corresponding one-off workflows were
omitted in favor of the existing remote-agent and portal suites.

Main's unrelated improvements (PATCH/COLLECT, database, broker,
self-update, task variables and ordinary application UI) remain in the
merged tree wherever they do not depend on the incompatible alternative
remote transport. Existing remote task GUI, Git pull/commit/push and PTY
remain protected by their original independent E2EE permissions.

## Integration gate

Before promotion to the named remote branch, run Go Agent and PHP Portal
CI plus relevant PATCH/COLLECT, MySQL and self-update checks. No forced
branch update, production remote-control enablement or weakening of
existing authentication is authorized by this merge.

**Deferred code integration**, not a device-test-only gap: porting main's
independently implemented remote file/interactive operations onto the
remote architecture with explicit typed-operation design review and
focused regression tests.

## CLI and minimum Go version

The main-only `taskdeck remote-task` subcommand requires an incompatible
host approval/token protocol; its unused dispatch was removed from the
default executable. The remote architecture's authenticated task GUI
remains available.

The pre-existing remote architecture uses `exec.Cmd.Cancel` and
`exec.Cmd.WaitDelay` (Go 1.20+). The merged module now declares Go 1.20
and self-update CI validates Go 1.20 and 1.23 instead of advertising Go 1.19
compatibility which could not compile.

## 2026-11-06 — incremental port: read-only Git remote names

- Ported main's fixed, offline `git remote` **name-only** view into the
  remote architecture's existing E2EE dispatcher and trusted native GUI.
  No remote URL, credentials, host paths, command arguments or network
  operation can be supplied by the client or returned to PHP/Relay.
- The host independently requires the existing local `git.status` RBAC,
  authenticated client key/password, live Portal grant, audited action and
  **`TASKDECK_REMOTE_GIT_REMOTES=1`** (default disabled).
- The result uses a separate authenticated envelope, allows 0–16 strictly
  sorted ASCII names, and refuses the entire oversized/unsafe result.
  The native GUI renders plain text and also checks the report format.
- The existing `session.features` wire shape and 512-byte E2EE quota
  remain unchanged; the GitStatus advisory permission enables the local
  button, while the new host opt-in is always enforced at execution time.
- Added focused Go E2EE/host/audit tests, a local-browser no-args/no-URL
  test and CI steps. This does not imply that other main-only remote file
  and interactive APIs are already compatible or ported.

## 2026-11-06 — offline remote-tracking refs (paged)

- Ported the main branch's offline remote-tracking ref/OID view into the
  working Remote E2EE architecture with native GUI buttons for first and next
  pages. Uses fixed `git for-each-ref refs/remotes` and no network activity,
  client path/remote name/Git arguments or ref mutation.
- Requires the existing `git.status` TaskDeck-local RBAC and audit,
  authenticated E2EE possession, live grant and
  `TASKDECK_REMOTE_GIT_TRACKING_REFS=1` (default disabled).
- A canonical snapshot is bounded to 4 KiB and 32 sorted ASCII ref names
  with lowercase 40/64-hex OIDs, excluding symbolic remote HEAD aliases.
  Separate strictly typed pages fit the existing 512-byte E2EE plaintext
  limit; the host owns the cursor and recomputes the complete snapshot on
  every next-page action. A changed snapshot fails closed. The browser
  renders only checked, textContent-only output.
- No changes to session.features wire schema or agent encryption frame limits.
  The pre-existing session quota may prevent reading all pages of an unusually
  verbose snapshot. No client-controlled offsets or reusable bearer tokens.

## 2026-11-06 — read-only offline Git Pull Review

Ported the review step from main (not its separate incompatible transport)
as `git.pull_review` in the established TaskDeck Agent E2EE protocol.
The host checks the exact workspace root, clean index/worktree including
untracked files, non-shallow local branch, existing local tracking ref, and
full commit ancestry before displaying either `up-to-date` or
`fast-forward`. Conflicts, divergence, no upstream, dirty or shallow
repositories are refused; no partially approved state is sent.

The command uses fixed trusted Git binary locations, an isolated environment,
no local username/password/URL transfer, no network requests and no Git writes.
Its authenticated response contains **only HEAD and upstream commit OIDs**
plus a strict status enum. Host opt-in
`TASKDECK_REMOTE_GIT_PULL_REVIEW=1` is required; default is disabled.
Permission is local `git.status`, separate from `git.pull` (GitWrite).
The GUI shows this clearly as an informational preview, NOT an approval
or authority to perform a later Pull. Every action is audited locally and
bound to the same client key, local password and current live Portal grant.

The existing 512-byte E2EE envelope and `session.features` shape are
preserved. Added typed codec/forgery, real Git repository/TOCTOU, RBAC,
loopback-route and browser GUI tests.
