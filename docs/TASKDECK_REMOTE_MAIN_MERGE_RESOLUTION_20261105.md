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

## 2026-11-06 — bounded staged-only Git patch review

- The merge had excluded main's incompatible commit-review transport. The
  current Remote E2EE dispatcher now exposes two fixed authenticated actions:
  `git.patch.cached.start` and `git.patch.cached.next`. They read only
  `git diff --cached` for the host's exact workspace Git root. The existing
  `git.patch.start/next` continue to read tracked changes against HEAD.
- Both views reuse the current `git.diff` RBAC, independent
  `TASKDECK_REMOTE_GIT_PATCH=1` host opt-in (default disabled), durable
  TaskDeck-local audit, live Portal grant and 512-byte per-action mailbox.
  They accept no paths, arguments, commit message, diff target or command.
- The entire diff must fit the existing 4-KiB bound and is sent in paged,
  text-only responses. The host fingerprints the workspace and complete diff,
  including **which view** it is reading, on every next page. Switching
  from staged to HEAD mode while a cursor is in progress fails closed.
- The trusted native GUI includes a separate **Review staged diff** button.
  Its Next control follows the selected view without copying a client page
  number or path into any E2EE action.
- This is a **read-only informational preview**, not a commit authorization,
  conflict resolution, full repository editor, immutable commit attestation
  or file-transfer implementation. `git.commit` independently rechecks
  host permissions and its staging state at mutation time.
- Regression coverage targets staged/unstaged isolation, stale index denial,
  view mixing, strict no-argument envelopes, UI page routing and logout.
  No Windows-specific behavior is introduced.

### Work remaining from main's incompatible remote APIs

- Compare task report/managed process features against the current
  session-owned long-job lifecycle; do not restore duplicate protocols merely
  because their old files disappeared. Host secrets, dependency execution and
  arbitrary free-form task arguments remain out of scope.
- Port remaining Git interaction/review operations selectively with
  independent typed E2EE schemas and per-action RBAC.
- Add read-only Files discovery, scoped file reading and later explicitly
  approved editing/transfer under `files.read/write/download/upload`
  permissions. Never tunnel the old generic HTTP/API transport via PHP.
- Add native GUI Editor/Files screens only after their matching Agent
  dispatch actions and regression tests exist; no inert remote buttons.
- Production LIVE remains NO-GO until all existing security release gates
  pass. Tests running in CI are code-validation gates, not device test
  requirements for marking an implemented code feature complete.

## 2026-11-06 — first scoped Files E2EE implementation

- `files.root` is a fixed, root-only, read-only authenticated E2EE
  operation on the existing native Agent dispatcher. The host requires the
  local `files.read` RBAC permission and its own opt-in
  `TASKDECK_REMOTE_FILES_ROOT=1` (disabled by default). Each request is
  subject to current password/key, live Portal grant and local audit.
- The browser cannot choose a path, project, filename, offset or command.
  The host opens only the configured workspace root and lists at most
  128 directory entries before refusing oversized directories. It returns
  at most eight sorted safe ASCII file/directory **names**, skipping hidden
  names, symlinks and special files. Contents, absolute host paths,
  arbitrary directories and file transfer are not part of this action.
- The native GUI shows this as an explicit opt-in root preview with
  textContent-only output. There are strict E2EE result bounds and
  filename/type validation on the Agent, native client and local GUI.
- Tests cover opt-out denial, bounded items, no request-supplied paths,
  symlink/hidden exclusion, and absence of recursion. This does NOT
  restore the general Editor, binary transfer, upload/download,
  arbitrary file reads or file mutations from main: those require
  independent typed E2EE operations, authorization and regression tests.

## 2026-11-06 — offline changed-file Pull review

- Ported main's separate changed-file inventory into the current native
  `git.pull_changes` E2EE dispatcher; no main-only HTTP command protocol,
  project path, Git arguments, remote URL or caller-selected ref is restored.
- The host requires current `git.status` permission and both
  `TASKDECK_REMOTE_GIT_PULL_REVIEW=1` and
  `TASKDECK_REMOTE_GIT_PULL_CHANGES=1` (default disabled). It reuses
  the existing read-only clean-worktree, non-shallow, fast-forward ancestry
  and tracking-ref checks and repeats them after generating a result.
- Only local, previously fetched commit trees are compared with a fixed
  trusted Git executable, isolated environment, disabled external diff,
  hooks and protocol network helpers. No fetch, checkout, merge, file
  modification or Git ref update occurs.
- Output is a bounded, strict, sorted inventory of at most six
  safe ASCII `A:/M:/D:` paths, otherwise the complete request is denied.
  Both old and target commit OIDs are authenticated inside the existing
  512-byte per-action AEAD plaintext limit. A clean already-equal upstream
  returns an empty inventory. The host's audit records the local principal.
- Native GUI offers **Xem file sẽ thay đổi** alongside offline Pull
  preflight, with text-only output and no caller-supplied parameters.
- This is NOT a Pull approval or a guarantee that later Pull will change
  exactly those paths; Git Pull independently revalidates its own state.
  Larger, unusual, renamed, binary or non-ASCII path inventories fail closed
  pending a separate structured paged design.
- Tests cover native transport injection denial, strict result schema,
  change detection, large/unsafe path refusal, host opt-in, no network/write,
  local principal/audit and GUI read-only behavior.

## 2026-11-06 — E2EE Files root pagination and native Files screen

- Added fixed `files.root.next` as a separate **argument-free** E2EE
  operation; it reuses `files.read` RBAC, live local-password/client-key/
  Portal-grant checks and `TASKDECK_REMOTE_FILES_ROOT=1` opt-in.
- The Agent owns the cursor and SHA-256 fingerprint of the full canonical
  workspace root listing. Each next-page request rebuilds the bounded
  source listing and refuses changed names, changed item types, moved
  workspace roots, missing first page, exhausted cursor or revoked session.
  No path, index, offset or cursor token is supplied by the browser/Relay.
- The existing 128-entry source bound and eight displayed items per page
  remain, with the shared 512-byte AEAD action mailbox and 16-round session
  budget. Large roots refuse the request rather than returning partial
  information marked complete.
- The native GUI now has a dedicated **Files · chỉ đọc** navigation page,
  initial list and Next buttons, ordered text-only append, and logout
  cleanup. This screen cannot open/edit contents, follow symlinks, delete
  files, transfer binaries or upload/download; it is NOT a full Explorer.
- Go/Node regression tests cover unordered/forged results, next-before-
  first, cursor exhaustion, stale snapshot denial, TaskDeck audit and GUI
  per-page gating. This is code-only implementation, not LIVE approval.

## 2026-11-06 — Host-fixed same-HEAD branch switch with reviewed plan

- Ported a strictly constrained alternative to main's branch-switch action.
  `git.switch_review` is read-only and requires the current `git.status`
  RBAC; `git.switch_same_head` is a separate **mutating** E2EE action
  requiring `git.write`. Both require host opt-in
  `TASKDECK_REMOTE_GIT_SWITCH=1`, and only the trusted host can define
  `TASKDECK_REMOTE_GIT_SWITCH_NAME` (one validated existing local branch).
- No branch, revision, path, Git options, terminal command or remote URL is
  supplied in the E2EE request; PHP and Relay only forward ciphertext.
  The native GUI first displays the full host-verified source branch, target
  branch and HEAD commit. The user confirms this exact plan before clicking
  the mutating control.
- The Agent stores the audited preview inside the authenticated session,
  not in a transferable continuation token. A switch without a previously
  reviewed plan, a replayed review or any changed target/current/HEAD is
  denied. The host also requires a pristine index and worktree (including
  untracked and submodule status), exact workspace Git root, and equal commit
  OIDs of both local branches. It holds the existing task/PTY Git mutation
  exclusion slot and uses a fixed Git executable with hooks disabled.
  Git's own `switch --no-guess` checks cross-worktree checkout constraints.
- The operation neither creates branches nor fetches, merges, resets,
  force-checkouts or pushes. The local TaskDeck audit never records
  branch names or passwords.
- E2EE `session.features` still has a strict 512-byte plaintext ceiling;
  no new feature flag is added to the already bounded status message.
  The GUI exposes *preview* when its Git branches read feature is enabled.
  The actual write is independently gated by host opt-in + `git.write`,
  not by that visibility hint.
- Regression tests cover host opt-in, clean equal-tip success, dirty/missing/
  changed-tip denial, inspected plan drift, viewer inability to mutate,
  strict E2EE typed result, malicious caller arguments, and native GUI
  confirmation with the named source/target.
- This does **not** yet restore arbitrary branch switching, branch deletion
  or any operation that changes checked-out file content. Such operations
  require independent, individually reviewed E2EE schemas and safety gates.
