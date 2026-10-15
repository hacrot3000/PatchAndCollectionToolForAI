# Restricted Developer — Development Handoff

**Branch:** `feature/restricted-developer-security`  
**Status:** **IN PROGRESS — NOT READY FOR PRODUCTION**  
**Security priority:** Never widen access to source code to make Git commands work.  
**Branch policy:** **DO NOT COMMIT TO `main`**. Only push incremental commits to this feature branch. Do not merge without explicit user approval.

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

## CLI example (early development only — partial restricted UI is available)

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

**WARNING:** This is intentionally a **lockdown skeleton**. Restricted instances deny legacy APIs; only the isolated restricted gateway and bounded source editor are partially available. Diff/History/Compare and full security verification remain pending. Do not describe it as a production-ready developer mode.

## Continued security hardening (2026-10-15)

- The branch now contains a **strict restricted command endpoint** (`/api/restricted/command`) and a **bounded, quota-charged source window endpoint** (`/api/restricted/file-window`), plus package-level gateway/quota tests. These remain isolated from the legacy Terminal and full-file Editor; the general UI has **not** been reopened for restricted users and `/api/restricted/status` still reports `ready:false`.
- Re-audited the cumulative source disclosure ledger. **Fixed a quota bypass for files with repeated identical lines:** accounting now incorporates both 1-based source line position **and** content, rather than just content text. Re-reading the exact same line stays free; viewing identical text at different positions counts separately; changed text at the same position counts again. Validation rejects windows exceeding file line count.
- Updated the file-window endpoint to forward the authenticated requested start line to the quota store. Updated all tests and added specific regression scenarios for repeated lines, replaying identical windows and invalid ranges.
- Kept Secure Command Gateway limited to reviewed metadata-only `git status`, `git branch`, `git rev-parse --abbrev-ref HEAD`, `ls` and `pwd`; **no unrestricted Git**, raw Bash, SSH, SFTP, FTP, or export is enabled.
- **Testing caveat:** no full native Go test or GitHub Actions run has been verified in this turn. Before releasing, run `go test ./internal/restrictedview ./internal/restrictedcommand ./internal/server ./cmd/vscode_tasks_menu` from `vscode_tasks_menu_go`. Security coverage is incomplete; do **not** enable in production.

## Browser lease interoperability (2026-10-15)

- Corrected the **restricted API gate** so it permits only GET/POST `/api/browser/lease` for the existing single-user browser-control handshake. Without this exception, an active lease prevented the otherwise permitted `/api/restricted/command` and `/api/restricted/file-window` POST calls from operating correctly.
- PUT/DELETE on the lease endpoint and all unrelated legacy dynamic APIs remain denied under restricted mode. This does not enable ordinary Terminal, SSH, Git Quick Actions or full-file APIs.
- Added a narrow-method regression test in `restricted_gate_test.go`. No production/CI pass is claimed; run native Go tests before deployment.

## Protected Editor milestone — range save and isolated UI (2026-10-15)

- Added prior-view proof in internal/restrictedview/ledger.go: source editing is authorized only for the exact original line positions/content already charged to the same account.
- Added internal/restrictededit/range.go: line-count-preserving bounded patch helper, preserving LF/CRLF, rejecting multiline injection, invalid UTF-8, NUL and out-of-range requests.
- Added POST /api/restricted/file-edit: checks file SHA-256, prior-view proof, version conflict and writable file; writes via pinned atomic replacement; never returns full-file source. Per-server mutex prevents concurrent restricted edits overwriting each other.
- Added canonical source path verification for both read and edit, rejecting symlink aliases into .git and other hidden paths.
- Shared-server FilesWrite permission now protects restricted edit; FilesRead remains required for restricted view.
- Added separate Restricted Developer landing page and web/featuremods/restricted.js: reviewed Git/ls/pwd gateway, window-only code view, bounded physical line editing and best-effort copy controls. Stale source is cleared after saves; no legacy full-file/Terminal APIs are called.
- Window response advertises editable=false for read-only or oversized files; the browser respects it.
- Added security regressions for blind edits, source path symlinks, bad patch input, stale hashes, concurrent writes and unchanged source outside the authorized window.
- Six simulated browser interaction tests passed; native Go build/test, browser smoke and security review remain UNVERIFIED.

Next: run native Go tests, verify single-/multi-user authorization, protect Diff/History/Compare/Search under the same cumulative budgets, and audit remaining Git/Task/AI export routes. Keep all legacy broad read/export/SSH/SFTP/FTP endpoints blocked until validated.

## Protected Editor safety follow-up (2026-10-15, latest)

**Implemented / corrected on this branch:**

- Fixed a compile-blocking missing `user` variable in `restricted_file_edit.go`. File-edit authorization now uses the same user identifier as the quota-charged file-window endpoint; shared mode requires a concrete authenticated principal rather than falling back to `single-instance`.
- Repaired malformed Go string literals in `restricted_file_edit_test.go` and the repeated-line fixture in `restricted_file_window_test.go`; the latter now writes ten actual physical lines instead of literal backslash-n sequences.
- Protected range saves now preserve a pre-existing **UTF-8 BOM**, including when editing line 1, while rejecting embedded or newly injected BOM markers. LF/CRLF preservation and non-insertion/non-deletion semantics remain intact; tests were added at both pure-edit-helper and protected HTTP handler levels.
- **Reduced the HTTP attack surface:** `restrictedGate` no longer permits every GET outside `/api/`. Aside from the isolated root page, the only unreviewed static route allowed is the exact module `/featuremods/restricted.js` (GET/HEAD). All legacy UI bundles, plugin/debug routes, Git exports, SSH/SFTP/FTP and full-file APIs remain blocked. The startup-only policy cannot be changed from the web.
- Updated `restricted_gate_test.go` to assert denial of legacy static/plugin/debug URLs and the new narrow allowlist.
- Wired `go test ./internal/restrictededit` and `node --test test_restricted_ui.cjs` into the restricted feature branch's existing CI workflow, alongside restricted server tests.
- Reran **six browser-interaction regression cases** using a simulated JavaScript environment: all six passed. The tests assert that only the reviewed restricted endpoints are invoked, edits contain only the viewed lines and expected hash, copy button enforces configured limits, and read-only windows disable save.
- Checked Go source files with a lightweight lexical scan for malformed quoted multiline literals. **This is not a Go compiler run.** The Go tests, full application build, live Firefox browser and end-to-end penetration checks **remain unverified**.

**Priority remaining work:**

1. Execute native Go tests/CI: `go test ./internal/restrictedpolicy ./internal/restrictedcommand ./internal/restrictedview ./internal/restrictededit ./internal/server ./cmd/vscode_tasks_menu`; fix any failures and update handoff per commit.
2. Audit authorizations for both single-user and authenticated shared-server (including cross-user source windows, replayed account identity, a stale expected SHA, and direct API bypass attempts).
3. Add separate, quota-charged restricted Diff/History/Compare endpoints and carefully bounded source search. Do **not** re-open legacy APIs. Every line that leaves the backend must count against cumulative per-user limits.
4. Verify feature readiness with a real browser/daemon/sandbox, then decide whether `/api/restricted/status.ready` can be changed from `false`. **Do not merge into main without the user's explicit approval.**

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

## Follow-up — outer HTTP method hardening (2026-10-15)

- Commit `4aadb3506ab76ddcd9f2f6a32642875ca46efc9b` enforces **POST-only** forwarding at the outer restricted gate for `/api/restricted/command`, `/api/restricted/file-window` and `/api/restricted/file-edit`. Previously, unrelated HTTP methods reached their respective handlers, which relied on their own method checks. This is defense in depth, not a claim of a working terminal or unrestricted file access.
- Added `TestRestrictedGateRejectsUnreviewedMethodsOnProtectedAPIs` covering six disallowed verbs and the permitted POST for all three routes in both single-user and shared-server modes. Every forbidden request must be stopped **before** the downstream handler.
- No legacy API was reopened. `/api/restricted/status.ready` remains `false`. Changes are confined to `feature/restricted-developer-security`; `main` is unchanged.
- **Validation not yet completed:** native Go tests/build and live single-/multi-user browser/daemon tests still must be run. Recommended next command:

```sh
cd vscode_tasks_menu_go
go test ./internal/restrictedpolicy ./internal/restrictedcommand ./internal/restrictedview ./internal/restrictededit ./internal/server ./cmd/vscode_tasks_menu
node --test test_restricted_ui.cjs
```

**Next priority:** review file-window quota and editing TOCTOU/path safety; then implement separate bounded Diff/History/Compare/Search APIs with complete disclosure accounting. Do not mark ready or merge until tests and security checks pass.

## Startup policy input hardening (2026-10-15)

- Commits `6276ee28` and `886353cb`: reject startup policy JSON whose top-level value is not an explicit object. Previously `null` decoded into the preinitialized Go struct without error, silently leaving launch defaults active. Non-object arrays, strings, numbers and booleans now fail before decoding. An empty object remains valid and inherits documented defaults.
- Added table tests covering `null`, array, quoted string, number, boolean and whitespace-surrounded object. Native Go execution remains **unverified**; these are committed regressions, not a test pass claim.
- Continue with native Go test execution, source-ledger/atomic edit review and isolated quota-charged Diff/History/Compare/Search. Remain fail-closed (`ready:false`). Do not merge into `main`.

## Persistent quota ledger fail-closed hardening (2026-10-15)

- Commits `01468489` and `3a9a5eb4`: `restrictedview.readLedger` now refuses JSON `null`, empty and non-object documents instead of silently retaining the default initialized empty ledger. This prevents corrupted state from resetting cumulative source-reveal accounting.
- Validate all stored accounts and nested file/day counters: null account, null or missing file/day maps, null usage entry, nonpositive baseline, missing seen map, and null day set are now treated as unsafe state. Both source-window `Charge` and edit proof `HasViewed` fail closed rather than rebuilding fresh counters.
- Added regression cases asserting that malformed quota storage cannot permit either new disclosure or protected editing.
- No changes to `main`, legacy endpoints, or the `ready:false` safety status.
- **Not yet verified:** native Go tests/build. Execute `go test ./internal/restrictedview ./internal/server ./internal/restrictedpolicy` inside `vscode_tasks_menu_go` before considering deployment.
- Next: harden ledger lockfile and filesystem ownership/permissions against symlink and replacement attacks, check pinned-file read/edit races, then build isolated quota-charged Diff/History/Compare/Search gateways.
