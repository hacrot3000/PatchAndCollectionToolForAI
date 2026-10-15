# Restricted Developer — Development Handoff

**Branch:** `feature/workspace-auto-init-20261015`  
**Status:** **IN PROGRESS — NOT READY FOR PRODUCTION**  
**Security priority:** Never widen access to source code to make Git commands work.  
**Branch policy:** **DO NOT COMMIT TO `main`**. Development has moved to this specified feature branch; all further commits go here. Do not merge without explicit user approval.

**Repository ref cleanup:** Initial feature commits were mistakenly made on `main` before the user clarified branch isolation. A dedicated feature branch was created preserving those commits, and `main` was restored to pre-feature revision `d27a3c7f34d4a334d0d0c32a89c629a10d540d1b` using a ref update with an expected-HEAD check. All subsequent restricted work goes only to this feature branch.

## Confirmed requirements

1. An instance-wide Restricted Developer mode is selected **only at process startup**, equally for single-user and shared multi-user servers. Use either `--restricted-policy /absolute/file.json` with `--restricted`, or `--restricted-*` numeric flags. No runtime/web UI toggling, permission editing, or role-based disabling.
2. The Git server/repository is responsible for Git permissions. TaskDeck must **not** create a redundant Git ACL; TaskDeck must still prevent source disclosure through Git subcommands/output, aliases, hooks, arbitrary remotes or other command execution. Restricted users must not receive unrestricted Bash. Security takes priority if Git and confidentiality conflict.
3. Protected Editor: chunked viewing/editing and configurable budgets for visible lines, per-copy lines and bytes, unique lines per file/day, and fraction of source disclosed. Server must not send entire file to browser. Save patches with version checks.
4. Same content budget policy for Diff, History, Compare, Search, Project Bytes, Preview and any other source-bearing endpoints; no full-source alternative routes. No bulk export.
5. No SSH, SFTP, FTP for restricted instances. Protect all Task, Patch/Collection, downloads/archives, Database, terminal broadcast, AI context, and Git Quick Actions; default-deny any unreviewed capability.
6. Restricted configuration is immutable until deliberate new startup. An existing daemon/old broker PTY must not be silently reused under the new policy.
7. Unrestricted single-user and multi-user deployments must continue to behave exactly as before.

## Phase 0 implemented on this branch

- `internal/restrictedpolicy/policy.go`: strict, bounded JSON policy loader, immutable value-style struct and validation; rejects unknown fields, oversized policies and unsafe paths/symlinks.
- `internal/config/config.go`: CLI-only `Restricted *restrictedpolicy.Policy`, **not parsed from web-modifiable workspace INI**.
- `cmd/vscode_tasks_menu/restricted_startup.go` and `main.go`: CLI options, policy JSON, per-limit overrides, canonical child daemon arguments and SHA-256 fingerprint validation; preserve across initial daemon spawn and self-update handoff. Reject attaching a Restricted invocation to an already-running daemon.
- Before exposing the web server in restricted mode, reject an existing session broker with any previously created sessions (unrestricted PTY reuse is unsafe).
- `internal/server/restricted_gate.go` and `server.go`: restricted backend HTTP gate applied in **both** auth modes. Currently only allows a health endpoint and a read-only `/api/restricted/status`, plus application static assets. **Legacy dynamic API paths are denied 403** (Git, file read/write, download, history, diff, task, patch, SSH/SFTP/FTP, database, AI, sessions, admin changes). Status explicitly says `ready:false`.
- Tests added for policy parsing/validation, startup arguments, endpoint denylist and unrestricted compatibility. **CI and Go runtime tests still need confirmation.**

## CLI example (early development only — restricted UI is not usable yet)

```sh
# Start/restart only after shutting down prior daemon and removing pre-policy PTYs:
./taskdeck --workspace /absolute/workspace --restricted --restricted-policy /absolute/secure/restricted.json

# Override configurable budgets at launch:
./taskdeck --workspace /absolute/workspace --restricted \
  --restricted-visible-lines 80 \
  --restricted-copy-lines 10 --restricted-copy-bytes 2048 \
  --restricted-per-file-lines 240 --restricted-per-day-lines 2000 \
  --restricted-reveal-percent 80 --restricted-commands git,ls,pwd
```

Example policy file:

```json
{
  "max_visible_lines": 80,
  "max_copy_lines": 10,
  "max_copy_bytes": 2048,
  "max_unique_lines_per_file": 240,
  "max_unique_lines_per_day": 2000,
  "max_reveal_percent": 80,
  "allowed_commands": ["git", "ls", "pwd"]
}
```

**WARNING:** This is intentionally a **lockdown skeleton**. Restricted instances currently deny most API calls and cannot yet edit files or run commands. Do not describe it as a production-ready developer mode.

## Mandatory next implementation stages (small commits only)

1. **Secure Command Gateway:** parse typed commands as structured argv, never invoke a general-purpose shell. Implement allowlisted Git operations with bounded/redacted results; block source exfil via Git aliases, hooks, helpers, pager, diff drivers, arbitrary Git config/options, unsafe push remotes, environment overrides, shell metacharacters and output streaming. Deny unknown operations. Use a sandboxed OS user with minimal filesystem/network rights.
2. **Identity-bound, durable source-reveal budgets:** shared-mode principal or authenticated single-user identity; database-backed counters by canonical file, unique line range, user and day, replay-safe accounting. Do not allow users to reset counters via browser session/cookies or restart.
3. **Protected Editor:** pinned safe path resolution, limited line-window requests, no full file contents to browser, segment patch save with hash/version preconditions; restrict client state persistence/caches.
4. **Diff/History/Compare/Search:** server-side content slicing with cumulative budgets; mask restricted areas and refuse any output/export that would circumvent the budget.
5. **Cross-module lockdown:** inspect all API routes and alternate source/export paths. Reopen only explicitly validated endpoints; write tests for direct HTTP/WebSocket calls bypassing UI, symlink/path traversal, read budget resets, role differences and multiple browser sessions.
6. **UI/UX:** usable restricted terminal-like gateway and protected editor; clear quota/copy warnings; display only startup policy status (no runtime editing). Handle policy launch when a daemon is already active with an explicit error.
7. **Go tests / CI / security review / Linux + Windows verification.** Never merge into `main` without explicit approval.

## Open technical risks

- CLI flags are not an OS security boundary; host users who can execute arbitrary processes or edit the server binary are outside the remote restricted threat model.
- Do not confuse the current API gate (fail-closed but non-functional) with completion of Restricted Developer.
- Existing standalone session broker and its data must not outlive policy transitions with controllable PTYs.
- A user allowed to edit/build code can add data-exfiltration code; protect CI credentials, deployment and branch/merge workflows separately.
- Source display can be photographed or scraped from individual visible chunks; cumulative server-side limits make it harder, not impossible.


## Progress on feature/workspace-auto-init-20261015 — 2026-10-15

- Moved the restricted startup-policy package, CLI flags, daemon handoff pinning and default-deny HTTP gate from `feature/restricted-developer-security` onto this requested branch. The existing workspace auto-init feature remains intact. `main` is not modified.
- Added `internal/restrictedpolicy/command.go`: initial **structured metadata-only Secure Command Gateway**. It never invokes Bash, a PTY, Git subprocess, hooks, aliases or arbitrary scripts. It supports `pwd`, a bounded current-folder `ls` and **only** `git branch --show-current` / `git rev-parse --abbrev-ref HEAD` by reading safe `.git/HEAD` metadata. Even with `git` enabled in the launch allowlist, all other Git operations (including diff, show, cat-file, archive and push) are denied pending separate audits.
- Added `/api/restricted/command` HTTP handler; explicit method, origin/cross-site protection from existing middleware, JSON schema and request-size limits, and restrictions for anonymous remote access in single-user mode. Shared-server authorization requires both `terminal.control_own` and `files.read`. Unreviewed legacy APIs remain blocked.
- Added `command_test.go` and `restricted_command_test.go` for no-shell metadata, exfil/injection denial, symlink-safe Git metadata, authentication and HTTP route restrictions.
- Enabled GitHub Actions on this feature branch and moved targeted Restricted Go tests/build ahead of unrelated UI tests. CI first confirmed `internal/restrictedpolicy` and `cmd/vscode_tasks_menu` tests PASS, but `internal/server` initially failed to compile due an unrelated preexisting `strings.Index` misuse in `editor_ui_test.go`; that regression test was fixed on this branch.
- **Restricted UI remains intentionally unavailable**, because the protected editor, quota accounting, diff/history/compare, and proper isolated Git execution have not been implemented. The status endpoint reports `ready:false`. Do not release or claim source-exfiltration protection complete.

**Next:** confirm the newest Go CI result, implement durable identity-bound line exposure budgets and protected file window/edit APIs, use the same budgets for Git diff/history/compare/search, build frontend gateway UX, then audit every export and session path before lifting readiness. Keep commit-by-commit handoff and do not merge into `main`.


## Durable exposure accounting preparation (2026-10-15)

- `internal/restrictedpolicy/ledger.go`: **unexposed backend quota ledger**, with `ReserveWindow` admission performed *before* returning any protected source window. It tracks line numbers per stable user and canonical file for a **lifetime per-file limit**, and per user across all files for each UTC day's limit. Re-reading already counted lines does not consume additional unique-line budget. A separate file-reveal percent ceiling applies across days.
- State is persisted in a server-owned absolute-path JSON file (user/file identities hashed) by atomic temp-file sync/rename; exclusive lockfile prevents concurrent admission, with fail-closed errors when locked, corrupted, unreadable or exceeding storage limits. The policy now correctly permits a daily limit lower than the lifetime per-file limit, provided both cover at least one maximum-sized viewing window.
- `ledger_test.go`: verifies persistence across object/restart reconstruction, two users' independent quotas, cross-file daily limits, file-percentage ceiling, corrupt/locked ledger denial, and invalid windows.
- **Security boundary still closed:** No HTTP protected-file read/edit endpoint is enabled yet. Before exposing the reader, obtain stable per-user identity, immutable canonical file identity (including hardlink/symlink concerns), derive total lines from trusted server-side file bytes, and store the ledger outside all user-editable project directories. Verify locking/durability on Windows and Linux. Never trust line-count or canonical-file ID from client JSON.
- **CI baseline:** GitHub Actions run `38722504834` succeeded on Go 1.19.x and 1.23.x (complete workflow, before durable-ledger addition). Newer commits require a fresh green workflow result.

