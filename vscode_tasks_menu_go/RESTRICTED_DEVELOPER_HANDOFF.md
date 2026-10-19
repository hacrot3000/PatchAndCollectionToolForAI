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

## Durable quota lock-file hardening (2026-10-15)

- Commits `2311f07b` and `eb50c36f`: both `Charge` and `HasViewed` now obtain their cross-process lock through `openLedgerLock`. It rejects a symlink, directory, non-regular file, or group/world-accessible lock and checks opened descriptor identity against the on-disk file, detecting a replacement between pre-open and post-open checks.
- New regression cases cover symlink lock, directory lock, permissive regular file, and repeat use of a valid private lock. Unsafe lock state fails closed with `ErrUnsafeState`.
- This is hardening, not a comprehensive defense against a **local attacker with write access to the private quota directory**. Future review must validate parent directory ownership and protect races after opening/locking (including ledger.json read and replacement); an OS user who can directly alter quota data remains outside the intended remote-user boundary.
- **Go tests and live runtime still NOT VERIFIED**. Run `go test ./internal/restrictedview ./internal/server` and a live restricted single/shared-server smoke before enablement. Keep `ready:false`; legacy endpoints remain blocked. Work remains exclusive to `feature/restricted-developer-security`; do not merge to main.

## Persisted ledger identity check (2026-10-15)

- Commits `3cb4b14b` and `34973e8e`: `readLedger` compares the file descriptor's `Stat` to the initial `Lstat` using `os.SameFile`, and revalidates regular-file type, private permissions and maximum size before reading bytes. This blocks an inspected ledger path being exchanged for a different inode between inspection and opening.
- Added read/write authorization tests for a symlink, directory and group/world-readable `ledger.json`; both quota charge and protected edit proof must fail closed.
- **Not yet a complete TOCTOU solution**: replacing `ledger.json` *after* read or swapping private directory entries after lock acquisition requires more design work and OS-level ownership protections; the current threat model does not promise resistance to a local user with write access to private quota storage. Avoid asserting production readiness.
- Native Go tests/build and real daemon tests remain **UNVERIFIED**. Run `go test ./internal/restrictedview ./internal/server`, then validate Linux permissions and multi-process locking. `ready:false`, branch isolated from `main`.

## Protected Compare API milestone (2026-10-15)

- Commits `42173cbf`, `46f7940c`, `1ab43c43`, `6ca9ad3e`, `b8c50e0d`: introduced separate `POST /api/restricted/compare` (not the legacy project/Git compare). Only two explicitly requested physical source windows are read, with `count <= max_visible_lines`, 64 KiB per side, UTF-8/text validation and the same canonical hidden-path restrictions as Protected Editor.
- Each side is charged through the durable per-user/per-file source exposure ledger **before either side is returned**. Failure to charge the second side can consume the first side's budget but cannot leak its content. The response includes line-by-line equality, without full-file export or invoking Git/shell.
- Shared-server requires a concrete authenticated principal before reading source. Outer restricted gate allows only POST for this exact path, while keeping all legacy compare routes unavailable. Added tests for two-file quotas, hidden/traversal paths, unknown JSON fields, excessive windows, anonymous shared mode and method rejection.
- **Scope limitation:** this is a basic position-aligned *two-window compare*, not Git history, a full diff algorithm, bulk comparison or browser UI. It currently uses two independent quota charges, not a transactional dual-file quota reservation. Source snapshot consistency and full single/shared authorization testing remain necessary.
- **No native Go test/build or real browser smoke has been verified yet.** Run `go test ./internal/restrictedview ./internal/server` and `go test ./...` in `vscode_tasks_menu_go`; do not mark `ready:true` without successful verification.
- **Next implementation:** bounded restricted search (careful: never emit match snippets or line numbers without quota charging, reject bulk enumeration), review source snapshot races, then isolated Diff/History endpoints. Keep `main` unchanged.

## Restricted Search window milestone (2026-10-15)

- Added isolated `POST /api/restricted/search` endpoint. It only searches a single explicit physical line window in one canonical, protected workspace file; requires an exact literal query of 2–128 bytes. No directory traversal, global project scanning, regex execution, unrestricted metadata enumeration or legacy search APIs are opened.
- Every line examined is quota-charged through `restrictedview.Store.Charge` **before returning even match positions**. Results contain only matching text/line positions already within the charged range. Limited to policy `max_visible_lines` and 64 KiB of source per request; shared mode requires authenticated account identity.
- Added route, strict POST-only outer gate and regression tests for charged slices, exhausted quota, hidden paths, traversal, unbounded count, unknown fields and anonymous shared access.
- Commits: `85d6c5e4`, `af398d1e`, `eb0f51f4`, `48ea9375`, `59920813`. The client UI does not yet expose search; API foundation only.
- **Native Go tests/build remain UNVERIFIED.** Run `go test ./internal/server ./internal/restrictedview` and `go test ./...` from `vscode_tasks_menu_go`; audit all permission middleware and compile before allowing production. Keep `ready:false`, no merge to `main`.
- Next: review protected compare/search response shaping and shared permission requirements; implement bounded History/Diff without invoking unrestricted Git content exporters. 

## Search / Compare range-hardening checkpoint (2026-10-16)

- Commits `4c93e2dc` and `c5c87124` replace `start-1+count`-first arithmetic with range-aware clipping in the protected Search and Compare handlers. This avoids integer addition overflow before bounding the slice. Search also explicitly rejects invalid UTF-8 query values.
- Commits `ead8912e` and `807dbf6d` add regression requests using very large starting line numbers and malformed/unsafe search queries. Their purpose is to confirm rejection without source disclosure.
- Source-bearing legacy APIs remain closed and the restricted status remains `ready:false`.
- **Validation caveat:** the Go unit tests and full build have not been run/verified in this session. Run from `vscode_tasks_menu_go`: `go test ./internal/server ./internal/restrictedview ./internal/restrictedpolicy ./internal/restrictededit ./internal/restrictedcommand ./cmd/vscode_tasks_menu`, then `go test ./...` and `node --test test_restricted_ui.cjs`.
- **Next stage:** verify cross-user authorization in the real middleware pipeline, implement a safe quota-charged restricted historical-diff design with no raw Git command output and no bypass through Git metadata, then add UI. Do not merge into `main`.

- Follow-up fixture correction: commit `a3aad958` ensures JSON escaped NUL and newline test inputs decode as actual control characters (rather than literal backslash sequences). These tests still need native Go execution.

## Shared-server authorization fix for restricted Search/Compare (2026-10-16)

- Audited `sharedAuthorize` and `sharedRoutePermissions`: the new `/api/restricted/search` and `/api/restricted/compare` endpoints were **not mapped**, so shared-server requests were denied as `unmapped_route` even when the authenticated user held `FilesRead`. Existing single-user endpoints were unaffected.
- Commit `f07abd1a` maps both routes to `identity.PermissionFilesRead`, alongside `/api/restricted/file-window`. No broad authorization or privileged Git permission was added. The outer restricted gate remains POST-only and blocks all legacy source/export APIs.
- Commit `65e858b1` adds shared authorization regression scenarios for FilesRead grant, write-only, Git-only, no permissions and no authenticated principal.
- **Go tests remain unverified**; run `go test ./internal/server` and full build. If shared authorization tests need adjustments against real middleware/audit setup, fix on this branch before marking passing.
- Next prioritize native Go verification, then review bounded History/Diff and their cumulative quota usage; do not use unrestricted `git show` output. `ready:false`, do not merge into `main`.

## 2026-10-16 — CI evidence and bounded Diff milestone

### Verified CI failures and corrections
- Inspected GitHub Actions run [38837175568](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/38837175568) (Go 1.19 and 1.23). Both failed `TestMalformedLedgerFailsClosedWithoutResettingQuota`: `readLedger("{}")` mistakenly retained a preinitialized `Version:1,Accounts:{}` and accepted unsafe ledger state. Commit `8094cd1f` now decodes **existing files** into a zero-valued temporary struct; only the missing-file path creates a fresh ledger. Commit `7a716464` covers incomplete existing schema and ensures explicit `{"version":1,"accounts":{}}` remains valid.
- Subsequent CI exposed a stale test expectation: the **outer** restricted gate intentionally responds `403` to unreviewed GET methods, whereas the direct command handler responds `405`. Commit `76983d9b` corrects the integration test and separately checks the direct handler method guard; **no widening** of the gate.
- The next CI stage revealed Smart Terminal keyboard test harness errors (`app.jsonFetch is not a function`, 7 tests). The production completion module already calls a read-only async alias fallback. Commit `65a4a91b` adds a neutral async backend mock to the harness; production terminal code unchanged.
- CI run for the intermediate commit `76983d9b` confirmed Restricted Go tests passed and later failed in this unrelated Smart Terminal test; the newest workflow is still subject to completion. Do not claim full CI pass without inspecting the latest run.

### Restricted Diff implementation
- Added `POST /api/restricted/diff` and `restrictedWindowDiff`: an LCS-based edit script (`equal`, `add`, `remove`) with physical source line numbers. This compares only two explicitly requested **current workspace** line windows; it is **not** Git history or Git diff and never invokes a shell or emits raw Git output.
- The new handler calls the same private source-reading, UTF-8 validation, canonical path validation, byte/window caps, authenticated principal and quota-debit pipeline as Restricted Compare. **Both window debits complete before any content is returned**, including unchanged lines. A second-side quota failure returns no source content. It may still charge the first side without returning output (safe but not atomic).
- New exact route is POST-only at the outer restricted gate. Shared-server `FilesRead` permission is required. Legacy Git/project diff/export routes remain blocked.
- Added pure algorithm tests (insert/delete/equal with 1-based positions), HTTP quota exhaustion regression, hidden path denial, unauthenticated shared denial, and route permission/method checks.
- Major commits: `6b6edf74`, `11f24ed6`, `cabad373`, `48bf7ec7`, `5fa01c81`, `11bfbd93`, `52b1f45a`, `5ab6d3ed`.

### Next priorities
1. Confirm the **latest branch-head** GitHub Actions result on Go 1.19/1.23, including full staged `go test ./...`, JS regressions, vet and build; fix failures and write evidence here.
2. Add the restricted Diff/Search/Compare browser UI using only isolated source-charged endpoints; keep caches in memory and clear stale windows.
3. Design a separate **historical Git file viewer** that cannot leak full historical blobs or Git command outputs. No historical endpoint has been enabled yet. Source disclosure accounting must include historical revision identity and all returned lines.
4. Harden private quota directory ownership and TOCTOU under the stated remote-only threat model; perform real single-/shared-server attack testing. Keep `ready:false` and **never merge to main without explicit user approval**.

## 2026-10-16 — Restricted UI Search/Compare/Diff milestone and CI evidence

- Workflow run [38837979746](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/38837979746) at backend Diff commit `5ab6d3ed` **PASSED** both Go 1.19.x and Go 1.23.x test-build jobs (full configured pipeline, including restricted/server tests, JS regressions, staged Go tests, vet and build). This validates the backend milestone; it does **not** prove complete production security or real-browser acceptance.
- Added a dedicated source-review section to `restricted_ui.go`, and connected it in `web/featuremods/restricted.js` to **only** `POST /api/restricted/search`, `/api/restricted/compare`, and `/api/restricted/diff`. Each operation requires explicit relative path, physical start lines, bounded line count and a browser-lease token. Search uses an exact-text query; Compare displays aligned windows; Diff displays the quota-charged equal/add/remove edit script.
- Review results are rendered using `textContent` rather than `innerHTML`, kept only in the page DOM, and cleared before later requests or upon failure. The UI prevents invalid/oversized ranges but the **server remains the security boundary**; selection/copy/devtools cannot be fully controlled after disclosure.
- Added browser VM contract tests for search request shape, diff/compare endpoints, and client-side rejection of over-limit review requests. Kept the existing editor/terminal-specific UI tests and forbade legacy full-file or Git-history calls from the restricted page. Updated UI wording to distinguish bounded workspace Diff/Compare from locked unrestricted Git history/exports.
- UI commits: `a7621feb`, `78f4d875`, `7a9845f4`, `4775db05`. **Latest UI workflow result must be checked separately**: the backend green run preceded these UI changes.
- Restricted Developer still advertises `ready:false`, main remains unchanged and unrestricted single/multi-user route behavior remains outside these isolated endpoints.
- Next: confirm latest branch-head CI including the new UI, run real Firefox/single-user/shared-server browser+backend permission smoke, address pre-existing quota-directory ownership/TOCTOU issues, and design historical Git views with revision-bound disclosures (never plain `git show` text or unlimited `git diff`).

## 2026-10-16 — Restricted Developer as an assignable shared-server Role

**User instruction:** merge main into feature/restricted-developer-security; then allow assigning a user to a role that cannot receive granular **Edit permissions** and is subject to **all existing Restricted Developer** boundaries. No direct changes to main.

### Main synchronization
- Main was merged into feature/restricted-developer-security with an actual two-parent merge commit `7b12f21ae2cb2e9333d127b29bd96d8e9376c57d` via [PR #9](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/9), targeted **into the feature branch only**. No feature work was merged to main. Follow-up role changes are commits on the feature branch.
- Note: main is active and may gain later independent commits. The merge records the main SHA at that moment; recheck ancestry/CI after subsequent merges.

### Role and permission changes
- Reserved immutable system role **`system:restricted-developer`** (`restricted-developer` in Roles list). Admin selects it when creating a project user or editing their project membership via **Users & Access → Users**. No custom role name or client assertion can impersonate it. Role permissions are pinned to the minimum backend requirements for the protected gateways: `files.read`, `files.write`, `git.status`; this does NOT grant legacy file endpoints, Git APIs or unrestricted terminal.
- Identity database **role seed v5** migrates existing databases, inserts/verifies this system role and its small role permission list, and purges legacy member overrides for members already assigned that role. A membership change into restricted role atomically removes all pre-existing per-user ALLOW/DENY records in the same transaction. Assignment to another role later cannot regain those stale overrides.
- `SetMemberPermission` checks current role under `BEGIN IMMEDIATE` and rejects attempts to assign granular permissions to restricted members. `DeleteMemberPermission` additionally scopes its SQL mutation to members not assigned the restricted role. Administration PUT/DELETE `/api/admin/users/permission` rejects restricted members with a clear 409 and an audit event, even when the caller is an administrator.
- Server authorization resolves restricted status from **authenticated project membership on every request**, never from a user-supplied request parameter. `EffectivePermissions` ignores any stale override rows and returns only the canonical restricted permissions, while `Principal.Allowed` adds a second role-aware ceiling against unexpectedly privileged maps.
- Admin UI shows **Locked by Restricted Developer role** instead of detailed Edit permissions controls for such users; Roles page explains that the reserved role's permissions are not editable. Other roles/users retain their existing permissions interface.

### Applying existing Restricted Developer policy per role (shared server)
- Added `restrictedPolicyForRequest`. Explicit CLI `--restricted` still enforces its pinned launch policy globally. Without that process flag, a shared-server request whose server-authenticated principal belongs to `system:restricted-developer` receives the immutable `restrictedpolicy.Default()` (80 lines/window, 10 copy lines, 2048 copy bytes, 240 unique lines/file, 2000 unique lines/day, 80% file visibility ceiling, only reviewed git/ls/pwd command groups).
- `restrictedGate` now checks **each authenticated request**. Role members receive the isolated Protected Code View and the existing bounded, quota-charged `/api/restricted/{command,file-window,file-edit,search,compare,diff}` operations only. Old terminal/SSH/Git APIs, legacy editor, full-file download, raw task runner, browser bundles, unrestricted export/history and arbitrary static routes remain HTTP 403. The role never toggles the global process mode and does not affect normal admins/developers.
- All existing protected handlers use the policy resolved for the current request, not just `s.Config.Restricted`. Source disclosures remain keyed to the **project ID + user ID** in the persistent quota ledger. Browser reload or multi-tab concurrency does not reset the account's quota. Protected edit authorization remains tied to lines previously viewed by that user and an expected file SHA.
- The startup-only CLI policy remains separate and cannot be enabled/disabled through HTTP admin settings. Shared role assignment is a server-side authorization capability, not a web preference that can remove restrictions from the daemon.

### New/updated regression tests
- `identity/restricted_role_test.go`: role seeded and selectable; system-role permissions not editable; transition clears prior ALLOW; effective permissions minimal; subsequent override attempts fail; transition away does not restore stale permissions; principal cannot use injected privileged map.
- `server/restricted_role_test.go`: a restricted project principal on an otherwise normal shared server gets the isolated page, 403 on legacy endpoints and persistent per-account source quotas; admin without restricted role retains normal routes.
- `server/restricted_role_integration_test.go`: actual HTTP admin role assignment, direct override API 409, login as restricted member and deny legacy routes, normal admin unaffected.
- Updated fake DB role-member fixture to account for role lookup in permission override setter.
- **Validation still required:** Execute actual Go 1.19/1.23 build, `go test ./internal/identity ./internal/server ./...`, Node restricted VM tests and real browser shared server role/permission smoke. This assistant session has not run native Go tests. User previously required GitHub Actions manual-only, so do not enable push-trigger CI or claim tests passed without executing them.

### Remaining security acceptance
- Restricting a member via an HTTP role does not solve the older, documented gaps in quota-directory ownership/TOCTOU, historical Git source review, or true screenshot/clipboard prevention in a browser. All preexisting security limits and risk disclosures remain applicable.
- **ready:false** pending tests, deployment review, and real role-based browser smoke; do not merge feature branch to main without the user's explicit approval.

### Merge catch-up and security review notes
- Main changed while role implementation was underway; a second clean merge brought its subsequent shared-terminal isolation improvements into the same feature branch, merge commit `b2f9d3f589b23ef51f5dd390c677755260bb5d5e` ([PR #10](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/10)). The main branch itself was **not** altered by either merge.
- Added `/api/restricted/status` to the shared-server permission router (requires `files.read`), preventing an otherwise fully isolated member UI from being unable to display its policy. This does **not** grant arbitrary `/api/restricted/*` capabilities.
- The SQL delete of an individual permission override now additionally requires the target member's role **not** equal the reserved restricted role, so even a lower-level caller cannot delete a restricted member's detailed permissions.
- Tightened effective role capabilities in `Principal.Allowed` as a defense in depth, so a mistakenly permissive in-memory map cannot grant restricted members legacy Git, task, SSH, admin or full-file actions.
- Verified the 20 modified/new identity/server Go files for balanced delimiters and literals; the embedded administration JS parses in V8. **Static verification only**, not `gofmt`, native Go compilation, SQLite migration smoke or full end-to-end regression. The branch must not be declared release-ready until these run.


## 2026-10-16 — Assign members directly from Roles to Restricted Developer

**Request:** Sync latest `main` into `feature/restricted-developer-security`; let an administrator choose existing users belonging to the Restricted Developer role; members of that role must not be assigned individual "Edit permissions" overrides and must obey all Restricted Developer protections. Publish ONLY on the feature branch.

### Branch sync verification
- GitHub compare `main...feature/restricted-developer-security` before modifications returned `status=ahead`, `ahead_by=177`, `behind_by=0`, `merge_base=main HEAD d67f1a8c`. **The feature branch already contained the complete current main history**, including all recent main commits; merging again would be a no-op. No merge conflict or force ref update is required, and main was not modified.
- Explicit attempts to open a PR from main into the feature branch returned `No commits between`; attempting a backwards ref update was rejected as non-fast-forward, confirming the right action is to keep the feature branch lineage intact, not rewind it.

### Implemented
- Added **Restricted Developer · Members** panel to the existing **Roles** page (`internal/server/shared_admin_ui.go`). The panel displays already-assigned usernames and an `Assign Restricted Developer` action to select an existing user, with an explicit confirmation warning that previous individual overrides will be cleared.
- Assignment requires both `users.view` and `users.manage` in the UI, excludes self-modification, and delegates to the existing authorized `PATCH /api/admin/users/access` with `role_id=system:restricted-developer`. Preserves the member's current `enabled` flag. Does not invent a second assignment/authentication path.
- The pre-existing server-guarded system role already provides the essential non-bypassable boundary: `UpsertProjectMember` clears prior per-user ALLOW/DENY overrides transactionally; `SetMemberPermission` rejects attempts to add overrides, `DeleteMemberPermission` cannot modify this role's overrides, `SetProjectRolePermissions` cannot edit immutable system role grants; `EffectivePermissions` is pinned to the restricted allowlist and `Principal.Allowed` fails closed. On every authenticated request, the per-role restricted policy routes the user through the protected source-window, quota, command and mutation gates, independent of UI state.
- Existing **Users** page continues to hide Edit permissions controls for assigned Restricted Developer members. Users can later be reassigned to another role by an authorized administrator through the ordinary project-access workflow.
- Added `TestRestrictedRoleMembershipSelectionUsesExistingServerAuthoritativeAccessEndpoint` and adjusted the established UI contract test in `shared_admin_ui_test.go`.

### Verification / limits
- Extracted and parsed the actual updated embedded `sharedAdminJS` code with V8: **syntax PASS**.
- Prior full HTTP/SQLite regression `TestSharedAdminAssignRestrictedRoleWithoutEditPermissions` already checks real role reassignment, clearing an old git.push ALLOW override, denied override APIs, blocked unrestricted endpoints, protected-code UI, and role-scoped quota policy. No backend privilege rules were relaxed.
- **Go full unit suite and Firefox UI smoke not executed in this conversation**, because the linked GitHub checkout is not locally materialized. GitHub Actions remain manual-only, and no workflow was automatically started.
- Release readiness remains **false** pending full CI and end-to-end validation; user-requested branch is `feature/restricted-developer-security`. Do not merge this feature into `main` without permission.

### Next validation
```bash
cd vscode_tasks_menu_go
go test ./internal/identity ./internal/server
node --test test_restricted_ui.cjs
go test ./...
```
Open `/admin/access` with a project administrator → **Roles** → **Restricted Developer · Members** → select a different user → confirm. Verify that user is now restricted, sees **Protected Code View**, cannot edit individual permissions or use legacy terminal/file/Git/admin routes, and that quota windows remain account-specific.

### Follow-up: real main merge commit (2026-10-16)

While this feature was being updated, another agent pushed additional changes to `main` after the initial ancestry comparison. The latest fully inspected pinned main SHA was `b1d68ac5dd999b6ad76a1e7a6908e90fb594154c`, five commits after the earlier common ancestor. GitHub compare showed exactly five changed JS files (addons, commandpresets, database, database_workbench, filewatcher); each was **unchanged on the feature branch since the common ancestor**, so a mechanical merge was conflict-free. Created a new tree from the feature branch snapshot with those five original main blobs, then a **two-parent Git merge commit** `5e941ea5c428cf89e0585fbb63c708c3f407e6d6` (parents: feature `6d9bc9ac` and pinned main `b1d68ac5`). Updated the feature branch by non-force fast-forward with an expected-SHA lease. No file changes were pushed to `main`.

Because other work may still be committing to `main` concurrently, this statement pins the exact merged main SHA rather than claiming the feature branch will contain every future main commit.

## 2026-10-16 — Browser-test mode enabled (not production-certified)

**User request:** Enable Restricted Developer to be ready for testing in a browser, using the feature branch only.

- `GET /api/restricted/status` for an already authenticated restricted member or an explicit global `--restricted` instance now returns `enabled:true`, `ready:true`, `status:"browser_test_ready"`, plus `release_ready:false` and `policy_source:"restricted_role" | "startup"`. The original limits (`max_visible_lines`, `max_copy_lines`, `max_copy_bytes`) remain.
- The isolated page header identifies browser-test readiness and continued server-side enforcement; the JavaScript shows which policy source is active. It keeps ordinary commands, source exports, terminal, full-file access and any unreviewed routes blocked.
- Added assertions to restricted Go endpoint/integration tests and the VM JavaScript browser UI regression that verify the test-ready state is visible, without advertising production certification.
- **Test prerequisites:** The deployed TaskDeck binary must actually be built from `feature/restricted-developer-security` and restarted. In multi-user shared-server mode, log into `/admin/access` as admin, assign the fixed `system:restricted-developer` role to a test user, then sign out and sign in with that account. That account will see the isolated Protected Code View; `/api/restricted/status` should show `ready:true`. An ordinary admin remains unrestricted unless the daemon is explicitly started with `--restricted`.
- **Standalone mode:** Use the existing `--restricted` launch flag to activate the global startup-only policy; users must restart to change that immutable launch policy. No web-facing setting has been added that can disable it.
- This enables a browser QA session **but does not claim completed security acceptance**. Existing quota-directory ownership/TOCTOU and browser clipboard/screenshot limitations still require a dedicated security assessment before `release_ready` can become true.
- GitHub Actions stay **manual-only**; native Go build and real browser smoke are **not yet confirmed in this response**. This branch has not been merged to main.

Suggested smoke: verify `/api/restricted/status`, protected view of 1-3 lines, persistent quota after reload, rejected hidden file, rejected arbitrary terminal/Git/export APIs, source-edit requires prior window and original SHA, role switch removes old permission overrides, and nonrestricted admin retains normal UI.


## 2026-10-18 — P1 Schedule RBAC and legacy V5 migration regression

- `schedule_api.go` now treats Schedule mutation and execution as a paired privileged capability: in shared mode, **both** `schedules.manage` and `schedules.run` must be granted. Neither permission alone may create or trigger a job running as the TaskDeck daemon. Admin/local behavior is unchanged.
- `schedules.view` alone now receives a minimal allowlisted schedule summary, never full command/URL/notification configuration, trigger diagnostics, run output or log contents. Inspect/simulate/preview, log and repository discovery endpoints are privileged.
- Tests: `schedule_permissions_test.go` checks each grant combination and payload redaction; `schedule_rbac_http_test.go` checks actual handler HTTP 403 responses and log/metadata exclusions.
- `seed_roles_upgrade_v5_test.go` now exercises real SQLite migrations for **both incompatible V5 histories**, including deliberate main-branch Schedule revocations, preexisting restricted membership, stale member ALLOW overrides, and repeated V6/V7 seeds. It must be supplemented by testing backups of actual legacy V5 database files.
- `restricted_live_role_change_test.go` covers existing browser cookies after Developer→Restricted→Developer transition and verifies old ALLOW overrides never reappear.
- CI remains **workflow_dispatch only**. The manual GitHub Actions workflow now includes the new migration, Schedule RBAC, and Settings Role visibility regressions. **No exact-HEAD CI PASS or live Firefox pass has been obtained in this work session.**
- Deployment reference: `deploy/taskdeck-restricted.service.example` (dedicated Linux OS account, limited workspace/filesystem/network access); full conditions and Firefox smoke checklist are in `docs/RESTRICTED_DEVELOPER_SECURITY_RELEASE_GATE.md`.
- Critical unsolved limitation: roles sharing a single TaskDeck daemon do **not** have distinct OS process privileges. Restrict source workspaces from privileged Schedules, CI runners and deployment credentials, or run an isolated restricted daemon. No automatic OS sandbox is claimed.
- Keep `release_ready:false`; do **not** merge into `main` until live three-account Firefox tests, OS isolation and exact-HEAD CI succeed.


## 2026-10-18 — Candidate update Go test failure at 109724d2

The user's self-update candidate validation at `109724d2363a25abc1008bb7eac56444b7fb0250` failed before activation. The old TaskDeck process remained usable. Two immediate causes were fixed on the feature branch only:

1. `internal/server/file_transfer_dual_pane_ui_test.go:1237` incorrectly invoked `strings.Index(js, token, remoteStart)`. Go's standard `strings.Index` accepts exactly two arguments. It now checks that `remoteStart` exists, searches `js[remoteStart:]`, and translates the relative result back to an absolute offset, retaining the missing-anchor failure condition.
2. `internal/identity/sqlite_admin_role_member_test.go` matched the fake-SQLite query with spaces around `=`, but `SetMemberPermission` now executes `project_id=? AND user_id=?`. The test fixture was updated to match the production SQL; no permission enforcement code was loosened.

**Validation:** Both modified files were fetched back from GitHub at the new HEAD and the edits verified. Native Go/Node tests and exact-HEAD GitHub Actions have **not** been executed by this session. The connected GitHub API does not provide an execution/dispatch facility here and no local checkout of this repository is available. This is a source fix, **not a claim of a passing update/build**.

Run from the updated branch checkout:

```sh
cd vscode_tasks_menu_go
go test ./internal/identity ./internal/server -count=1
go test ./... -count=1
go vet ./...
go build ./...
node --test test_restricted_ui.cjs tests/settings_role_visibility.test.mjs tests/schedule_role_security.test.mjs
```

If any further test fails, retain the running TaskDeck; do not activate the candidate or promote `release_ready`. Main is unchanged.


## 2026-10-18 — Fix self-update server test failures after c5785963

User-reported candidate update at `c5785963a11b8cfed494bccb117802b8b64f2e9d` passed all identity tests but failed these nine server tests, mostly due to stale source-string contracts after valid UI changes:

- `connections_ui_test.go`: profile loading now uses `loadAuthorizedConnectionSource(permission,path)`, which checks shared-mode privileges and stops retrying HTTP 401/403; assert secure helper call rather than direct fetch.
- `editor_ui_test.go`: Config language supports `xcconfig`, `cfg`, `.editorconfig`, and `.gitconfig` in addition to the older mappings.
- `file_transfer_dual_pane_ui_test.go`: server-side scan entries are transient and disappear after finish or failure; old assertions erroneously required persistent failed/empty-scan rows. Preserve active scan and browser download checks.
- `file_watcher_ui_test.go`: bounded metadata polling now also checks `canWatchFiles()` for role permission safety.
- `ios_syntax_ui_test.go`: compare indentation language set now begins with `gradle` before Swift/C++; Swift remains covered.
- `markdown_preview_ui_test.go`: editor toolbar now includes `structuredSelect`; preview still uses unsaved CodeMirror buffer.
- `project_file_actions_ui_test.go`: host panel passes an explicit joined path and entry-type mapping to shared project file actions.
- `schedule_inspection_test.go`: `replace` is deliberately implemented and supported by schedule validation/runtime. Assert `skip`, `parallel`, `queue`, `replace` as valid and reject an unknown policy.
- `workspace_utilities_ui_test.go`: sidebar icon minimum width is now 26px (rather than 29px) to fit more controls.

All changes were confined to Go regression tests; no production RBAC or business logic was weakened. Twelve changed contracts were fetched from branch test and implementation files and matched by direct string checks (12/12). All previously passing package tests remain unverified against the new branch HEAD until a native Go test run. `release_ready` is unchanged; do not merge into main or activate without passing candidate validation.

Suggested native verification:

```sh
cd vscode_tasks_menu_go
go test ./internal/server -count=1
go test ./... -count=1
```


## 2026-10-18 — Self-update failures at c91920b3: final two stale UI test assertions

User-reported candidate `c91920b3c5184db9bbc6b1145c064534265b8da6` failed exactly two additional `internal/server` source-contract assertions:

1. `connections_ui_test.go` expected a direct `app.jsonFetch('/api/config/terminal-cwds')`. The Connections page now correctly uses `loadAuthorizedConnectionSource('settings.read','/api/config/terminal-cwds')` to respect per-user settings permissions, including revocation while the page is active. The test now asserts the RBAC-aware invocation.
2. `project_file_actions_ui_test.go` expected `addHeading('Project')` inside the editor tab context menu. The current implementation consolidates actions in `...(projectActions.length?[{label:'Project',children:projectActions}]:[])`. The test now asserts the submenu and preserved shared project actions.

Source-side validation fetched fresh files from the updated GitHub branch and matched **all 106 literal source strings** asserted by the Connections main test, plus **all four Project File Actions integration cases** (Explorer, host file transfer, Git, tab context). The RBAC check `app.sharedMode&&!app.hasPermission?.(permission)` and shared action invocation remain intact. No production code or security checks were modified.

**Native Go test execution remains pending**; the user can retry self-update (which runs `go test ./...`) or run `cd vscode_tasks_menu_go && go test ./... -count=1` from a branch checkout. Keep candidate inactive if validation fails; retain `release_ready:false`.


## 2026-10-18 — Security follow-up after reported full self-update test PASS

User confirmed the previous TaskDeck candidate updated successfully and passed all its tests. Security review continued on `feature/restricted-developer-security` **without merging main**.

Confirmed backend authorization flaw: `users.manage` allowed a lower-privilege manager to create an admin-controlled new account with Role `system:admin`, or assign a stronger role/ALLOW override to another account; `roles.manage` allowed broader grants to existing custom roles. This bypassed intended least-privilege separation without requiring the attacker to change their own account.

Mitigation in `internal/server/shared_admin.go`:
- Role assignments, member overrides and custom Role permission edits are bounded by the acting principal's current privileges, unless `project.admin`.
- Existing target accounts and roles with higher grants are protected from delegated modification. The existing Role baseline is checked even when a member is disabled.
- Both denial and allowed lower-privilege delegation are tested through real shared-server routes in `internal/server/shared_admin_delegation_test.go`.

A second cross-user issue involved `/api/file-transfer/jobs/control` allowing `transfer.read` to manipulate a process-global job queue. Handler and route policy now require `project.admin` for queue-wide control in shared mode; single-user remains unchanged. File Transfer job creation also enforces `files.read` on background host uploads and `files.write` on background host downloads. See `internal/server/shared_transfer_job_security_test.go`.

**Validation state:** New tests were committed and the expected security gates verified by re-fetching source from GitHub; **native Go/Node tests have not been executed on the new HEAD**. A prior pass at `42b34887` is not a pass for these commits. Update candidate and Firefox live multi-user smoke must be rerun. Maintain `release_ready:false`.


## 2026-10-18 — Continue security audit: owner isolation and remaining queue persistence disclosure

On feature branch only (no main merge), continued the release-gate audit after prior reported updater PASS.

New code:
- `internal/dbsession/manager.go` stores immutable hidden project/user owner metadata on live connections; GET now copies metadata under the manager lock.
- `internal/server/db_sessions.go` filters the session list and rejects read/query/import/close across user or project boundaries, unless the caller has `project.admin`. It binds owner after opening the adapter. `db_connection_probe.go` and database session creation require `ssh.use` for an SSH tunnel.
- `internal/server/file_transfer_jobs.go` stores server-derived hidden job owner, filters GET snapshots and job items by owner (admin sees everything). It maintains existing admin-only global control.
- `internal/server/file_transfer_persistence.go` writes owner to separate optional fields in disk state and restores it. Legacy jobs without owner are admin-only.
- Tests: `internal/dbsession/manager_owner_test.go`, `internal/server/db_session_ownership_test.go`, `internal/server/file_transfer_owner_security_test.go`; targeted cases added to manual workflow.

**Important unsolved release blocker:** the file transfer queue disk state is still in the workspace's `.vscode/vscode_tasks_menu.filetransfer.jobs.json`, which can be read through normal project file access by a user with `files.read` (project-file API does not currently deny managed state names). This bypasses queue snapshot isolation and could expose other users' job metadata. Search, archive and other managed state files need audit. Do NOT claim queue secrecy, merge to main or release. Required next implementation: move private state outside served workspace with safe migration, block legacy copies/all alternative APIs, add two-user regression tests.

This environment did **not** execute the new Go/Node suites or real Firefox tests. Latest previous PASS was for older revision only. Keep `release_ready:false`.


## 2026-10-18 — Shared queue state moved out of workspace (follow-up to confidentiality blocker)

The queue-state issue described above was **subsequently addressed in code**. In shared Multi-user mode, `fileTransferQueueStatePath` now resolves an external private directory keyed by workspace and project ID alongside the secure identity DB parent, not under the served workspace. Legacy `.vscode/vscode_tasks_menu.filetransfer.jobs.json` is migrated into a private 0600 file before the HTTP listener serves requests, after safety checks. The code deliberately refuses conflicting old/private states instead of losing jobs. Single-user retains its existing project-local state path. Added Go tests for successful migration, untrusted legacy symlink, workspace-resident identity paths and divergent state conflicts; included in manual CI.

**Still not release-ready:** Neither these fresh tests nor a real daemon restart migration has been executed in this conversation. Back up an existing queue before evaluating this candidate. Other managed TaskDeck configuration/state files in `.vscode` remain possible disclosure paths via project read/search/archive routes. OS isolation, exhaustive Firefox multi-account tests, and tests against the exact new commit remain required. This note supersedes the earlier statement that *the queue file itself* remains inside `.vscode` for shared mode, but does not supersede the broader managed-state confidentiality blocker.

Run the candidate's full `go test ./...` check before activation. Keep `release_ready:false`; no merge into main.


## 2026-10-18 — Managed-state confidentiality guard across project APIs

Implemented source-side hiding of TaskDeck internal `vscode_tasks_menu.*` artifacts in shared Multi-user mode. New helper `internal/server/shared_managed_state_guard.go` recognizes private basename components after path normalization, and canonical targets after symlink resolution. Updated `project_files.go`, `project_search.go`, `project_content_search.go`, `project_content_replace.go`, `project_symbols.go`, `project_completions.go`, `project_archive_create.go`, and `file_transfer_host.go`. Restrictions cover Explorer, file reads, binary reads, filename/content search, replace preview, symbols, autocomplete, ZIP/tar exports, File Transfer host operations, and aliasing into private state. Admin settings APIs still use dedicated backend functionality; ordinary source-file access in shared mode is blocked even for admins when accessing a TaskDeck managed-state file.

Regression cases in `shared_managed_state_guard_test.go` and manual-only Actions workflow. Static inspection matched 13/13 guard contracts; native test execution and Firefox session checks are NOT yet verified on this commit.

**Remaining:** prior archives/history may contain secrets; broader Git/terminal/CI trust boundary and OS isolation must be assessed. Do not merge into main or enable `release_ready`.


### New upstream main commits after this audit

At the end of this pass, `main` had advanced to `9357cb6eb34a4bf97baa6402d422aa6c17e1ed7f`; the restricted feature branch was **38 commits behind main**. These upstream commits mainly concern macOS portability and include changes to `.github/workflows/vscode_tasks_menu_go.yml`. This security work was deliberately **not** merged or rebased in this pass: first reconcile the workflow so none of the targeted security tests are lost, review any cross-platform project-file behavior, and rerun a complete exact-HEAD candidate validation. Never interpret the existing base/main comparison as a release approval.


## 2026-10-18 — Integrated upstream macOS branch into Restricted Developer feature

The 38 outstanding main-only commits at `9357cb6eb34a4bf97baa6402d422aa6c17e1ed7f` were merged into `feature/restricted-developer-security` via two-parent commit `c0dca96677cf6360d984d68adce21b62933b7a10`. **No commit was written to main.** The merge tree preserves exactly the upstream SHA and executable mode for all 26 unchanged macOS/new documentation paths. Only `.github/workflows/vscode_tasks_menu_go.yml` overlapped with security branch edits; the resulting manual-only workflow includes BOTH the existing Linux Restricted/Schedule/DB/FTP managed-state tests and upstream `macos-native` job for macOS Intel/Apple Silicon with Go 1.25.

Post-merge review also closed a cross-project admin metadata disclosure corner case: `internal/server/db_sessions.go` does not expose owned database sessions to a `project.admin` from another project; `internal/server/file_transfer_jobs.go` filters even privileged admin queue snapshots to the matching Project ID while retaining explicit admin-only access to legacy ownerless jobs for recovery. Both already-registered test files now assert project-admin boundaries.

Verification performed: compare with main shows `behind_by=0` at the time of merge; object-level verification shows correct two parents, 26/26 upstream file blobs/modes, and both classes of CI checks preserved. Native `go test ./...`, Go vet, macOS runner, self-update candidate activation and interactive multi-user Firefox smoke **have not been executed at this new HEAD** in this chat. Do not mark release-ready or merge into main without them.


## 2026-10-18 — Follow-up main sync: Schedule remote Git triggers

While finalizing the upstream macOS merge, main advanced from `9357cb6e` to `2ae90b69b3dd7edcf4d4409b1c256fb2a47529e4` with 11 additional Schedule commits. The feature branch integrated these in merge commit `ce5afd43fc225e7a1bec442eb655cb301f07ba31`, with main as the second parent.

For the eight upstream-only Schedule files, the feature merge copied the exact upstream blobs. Two overlapping frontend modules needed a hand merge: `scheduleviews.js` now displays remote Git conditions while retaining Restricted Developer `redacted` metadata-only display, and `schedulespanel.js` includes remote Git defaults and condition selection while retaining the earlier role-specific UI checks. Static assertions verified all six relevant feature and upstream markers.

The new Schedule remote Git checker can contact remote repositories and, if configured, perform a privileged fast-forward-only pull before task execution. Its authorization boundary must be reviewed in shared-server role testing: `schedules.manage`/`schedules.run` can schedule arbitrary task/command execution, but unattended Git/SSH credentials and an automatically updated worktree warrant explicit approval and OS isolation. No policy relaxation or automatic CI triggers were added.

Full native Go tests, frontend tests, and macOS runner checks have NOT been run at the resulting feature HEAD. The release gate remains closed.


### Finalized sync snapshot — 2026-10-18

Three additional main-only fixes for remote-Git Schedule runtime, regression tests, and the expansion handoff were integrated with second-parent `main` commit `0286e36378dcc62c7b72df1e2c538a54230bd912` via feature-only merge `b23fd512bdecf3f67d11c2eff73c69b28683a0d5`. No protected RBAC or Restricted Developer source file was overwritten by this follow-up. The feature branch was 0 commits behind that explicit main snapshot immediately after merge.

**Validation checkpoint:** require native `cd vscode_tasks_menu_go && go test ./... -count=1`, `go vet ./...`, JS regression suite, and macOS Actions before release. This chat did not execute those tests or obtain an Actions result on the new SHA; prior user-confirmed green update belongs to an older commit. Review auto-pull Git/SSH privileges in multi-user roles. Keep `release_ready:false` and do not merge into main.


## 2026-10-18 — Closed direct historic-blob bypasses on managed TaskDeck state

Added managed-path checks to `project_file_history.go`: server-side Local File History's saved index and historical blob retrieval did not previously reuse the live Project-file restriction and could disclose historical `vscode_tasks_menu.*` config to `files.read` roles. Now protected path requests (including a resolvable symlink alias) are denied in shared mode; single-user remains unchanged.

Added equivalent checks to `githistory.go` (Git file history, historical/current index Git blobs, Git blame), `gitquick.go` (Git diff), and `git_conflict_file.go` (read/write conflict stages). Created `shared_managed_history_security_test.go` and `shared_managed_git_security_test.go` and wired those tests into the manual Actions source-confidentiality preflight. Reviewed changed Go source for missing/duplicate imports (zero duplicates found).

**Validation:** no native `go test ./...` or real Firefox/macos run occurred during this pass; the branch contains new unexecuted regression tests. Other content/history aliases, privileged terminal/schedule execution, Git metadata and OS-level isolation remain release blockers; maintain `release_ready:false` and do not merge into main.


## 2026-10-18 — Shared transfer queue control and Git view RBAC follow-up

Inspected `fileTransferJobsControl` and confirmed cross-project side effects remained despite project-filtered response JSON. Added `fileTransferQueueProjectAuthorizedLocked` to reject queue mutations while holding `queue.mu` if any job is outside the active project or has unknown ownership. Added the same in-lock authorization to `resolveServerConflicts` (retained its old two-argument test interface using an optional project ID). Created `file_transfer_cross_project_control_test.go` and added it to manual Actions preflight.

Inspected `sharedRoutePermissions` and found Git `view=blame`, `view=file-history` and `view=compare-tree` fell through to `git.status`. Assigned actual `git.diff`/`git.log` capabilities to these and related views, expanded `shared_authorization_test.go`, and added `shared_git_view_security_test.go` middleware assertions.

**Pending:** full CI and live Firefox validation on new feature revision. Current queue control policy is intentionally conservative on mixed-owner queues; owner-scoped job-control design must precede feature completeness claims. Review privileged Schedule/task/terminal execution and asynchronous worker tenant isolation next. No main merge; `release_ready:false`.


## 2026-10-18 — Git tree/commit metadata redaction continuation

Added `shared_managed_git_metadata.go` to filter TaskDeck-managed paths from Git commit-file lists and tree diff summaries. `git_tree_compare.go` now suppresses protected tree entries and diff changes. `gitgraph.go` uses a common shared-aware parser to redact protected file paths and either old/new rename paths. If an ordinary ancestor directory includes a private child, its aggregate Git tree object ID is cleared while safe children remain visible.

Added `shared_managed_git_metadata_test.go` and manual-only targeted CI tests. Existing project Admin / transfer queue code was concurrently updated in another session to handle cross-project mutations; do not silently overwrite that work. Latest changes have source checks only; **no native Go tests or Firefox tests were run** in this conversation. Refresh branch HEAD before any further work due to concurrent writes.

Release gate remains false pending exact-HEAD build/tests, terminal/Schedule privilege isolation and historical-secret containment.


## 2026-10-18 — Source audit: Git Changes and conflicts

Added `filterManagedGitStatusChanges` and `filterManagedGitConflictFiles` to `internal/server/shared_managed_git_metadata.go`. The `gitChanges` API now hides TaskDeck-managed pathnames and rename originals from Git porcelain status, and `gitConflictState` applies filtering before multiple HTTP consumers receive conflict metadata. Existing Single-user handling remains unchanged.

Added `TestSharedGitStatusMetadataRedactsRenamedManagedPaths` and `TestSharedGitConflictMetadataExcludesProtectedPaths`; both included in the manual-only workflow. Verified eight source-level guard assertions from the latest branch snapshot, **not** a native Go test pass.

Schedule remains a separate security decision: `scheduleElevatedAllowed` permits non-admin users with BOTH `schedules.manage` and `schedules.run` to initiate daemon-credential shell/task execution. Merely having RBAC cannot sandbox these processes. Assess whether to require admin for unattended execution or implement isolated OS user/container worker. Continue to hold `release_ready:false`.


## 2026-10-18 — No-op main sync, complete-owner-only visibility for DB and FTP

Verified branch against current `main`; main had zero commits missing from the Restricted Developer feature branch. No merge commit was created, and no write to main occurred.

Fixed cross-project admin visibility of legacy ownerless/partially owned data in `internal/server/db_sessions.go` and `internal/server/file_transfer_jobs.go`. Even `project.admin` must now have a complete matching `OwnerProjectID` **and** nonempty `OwnerUserID` to see a shared DB session or saved-profile transfer job. Existing private state/queue data are preserved, not deleted. Updated `db_session_ownership_test.go` and `file_transfer_owner_security_test.go` for missing/partial metadata and browser isolation. Existing queue mutation checks deny operations on foreign/unknown owners; recovery remains out-of-band.

Static source inspection checked the relevant server gates and tests; **exact-HEAD `go test ./... -count=1`, Go vet, macOS runner and Firefox dual-user tests remain unexecuted**. Maintain `release_ready:false`, and address elevated unattended Schedule/Terminal OS permissions before release.


## 2026-10-19 — Continued review excluding Schedule/Terminal OS isolation

Per user request, postpone further work on Schedule and Terminal daemon process permissions and continue checking remaining RBAC, data isolation, recovery, and build-test risks.

Verified main is an ancestor of `feature/restricted-developer-security` (0 commits behind); no merge was required. Tightened DB session and File Transfer queue project-admin read gates so they expose **only completely owned resources**: project ID must match and user ID must be nonempty. Old ownerless and partially owned jobs remain in protected private persistence but are invisible to all browser-level project administrators. Updated related Go tests, including checks that serialized admin queue responses do not include private historical job paths or errors.

Changed `requireSharedActionPermission` to deny a missing authenticated principal in shared mode with 401 rather than silently treating the handler call as Single-user. The local Single-user behavior is preserved. Added `shared_action_gate_security_test.go` and manual workflow targeted test.

**Do not claim test PASS yet.** Re-run `cd vscode_tasks_menu_go && go test ./... -count=1 && go vet ./...`, the manual security workflow, and Firefox multi-user tests on the exact new branch SHA. Keep `release_ready:false`; do not merge feature into main.


## 2026-10-19 — Continue permission and task-history checks (process isolation deferred)

Maintained the user's explicit decision to defer Schedule/Terminal OS process-privilege changes.

`internal/server/task_run_history.go` now fails closed when a shared task-run list/log request lacks an authenticated principal. Task-run POST authorizes its project principal *before* session metadata lookup, and now requires the session owner or same-project admin (as well as task view/run grants) to write a durable task log; a different same-project user with task execution permission cannot overwrite it. This preserves normal read-only shared history semantics.

Created `internal/server/shared_task_run_history_security_test.go` for anonymous, cross-project, local Single-user, same-project originator/admin and unrelated-user cases. Added `TestBrowserSessionReflectsRolePermissionRevocationImmediately` in identity authentication tests, proving that role grants refresh on each HTTP request without session logout. Both tests were wired into the manual-only GitHub security workflow.

Validated eight static source/test checks from GitHub; full Go/Node CI, macOS and Firefox acceptance have **not** been run. Exact-HEAD `go test ./... -count=1`, `go vet ./...` and multi-user browser tests remain required. Keep `release_ready:false`, no main merge.


## 2026-10-19 — Close Download/Upload/Explorer Git bypass paths

Audited general workspace routes in addition to previously protected Project files. In shared mode `files.go:resolveDownloadPath` now blocks TaskDeck-managed state using canonical workspace-relative path checks; `filePreviewInfo` and terminal file-selection use that same resolver. `upload.go:fileUpload` blocks uploads and overwrites targeting internal `vscode_tasks_menu.*` state, with a regression asserting `.vscode/tasks.json` is not misclassified. `project_git_status.go:projectGitStatus` now filters Git status paths and rename originals just like the Git panel.

Added regression files `shared_files_download_security_test.go`, `shared_files_upload_security_test.go` and `TestSharedProjectGitStatusRedactsManagedFiles` in `project_git_status_test.go`. Added them to the existing manual-only GitHub Actions security suite. Source review verified all 11 integration assertions, not native Go execution.

Retain this branch exclusively for review and tests. `main` has not been changed, Schedule/Terminal OS privilege isolation remains deferred per user request, and release_ready remains false until exact-HEAD tests and Firefox multi-account validation succeed.


## 2026-10-19 — Resumed FTP/SFTP download target protection

Found a second host-file mutation path after the direct `/api/files/upload` guard: background transfer-queue downloads can resume after a daemon restart and call `backgroundHostTarget` directly. Added shared-mode TaskDeck managed-path checks to `backgroundHostTarget`, `ensureBackgroundHostDirectory` and `backgroundHostExistingMeta`, so scan/conflict evaluation and final file writes are protected independently of browser-side checks. Public project files (`.vscode/tasks.json`) and Single-user remain compatible.

New test `shared_background_download_security_test.go:TestSharedBackgroundHostDownloadRejectsManagedDestinations` was registered with existing manual Go security workflow. 9/9 static wiring checks passed; **native `go test` and real FTP/SFTP / Firefox checks are not run** on this HEAD. Do not merge into main or declare release-ready without full tests. Schedule/Terminal OS isolation remains postponed at the user's request.
