# TaskDeck Remote UI — Active feature & remaining-work checklist

> Updated: 2026-11-19. Source baseline: `main@7a96b02cb2b54455c386b044183017fad1f5ed21`.
> Owner-defined priority: **Remote Task GUI → Git Pull/Commit/Push → Terminal → other Git → Files/Editor/other GUI → remaining Remote**.
> [Full implementation plan](TASKDECK_REMOTE_UI_REMAINING_PLAN_20261119.md).
> **Checkbox `[x]` means implemented in inspected code**, not a new local test PASS and not approval to release LIVE.

## Legend and triage rule

| Status | Meaning | Count as unfinished code? |
| --- | --- | --- |
| `[x] CODE DONE` | Bounded implementation + relevant test source are present on main | **No** |
| `[ ] VERIFY` | Existing code needs targeted source/regression recheck, only fix reproduced defects | Not automatically |
| `[ ] PARTIAL` | Specific requested GUI function missing though base works | **Yes, missing portion only** |
| `[ ] TODO` | Host/E2EE/native GUI path is missing or not integrated | **Yes** |
| `[ ] OPTIONAL` | Not required for currently approved bounded scope | Only after explicit approval |
| `[ ] LIVE PENDING` | On-device / network / independent release acceptance outstanding | **No — separate gate** |

**Evidence:** the source/test paths below existed in the audited repository tree. No full test run or live smoke test was performed by the documentation audit. Later code additions override the older 2026-10/early-11 prose.

## 0. Foundations / login / E2EE

- [x] **REM-001 CODE DONE** — Native trusted client + local GUI (`cmd/taskdeck_remote_login/trusted_gui.go`, `trusted_gui_assets.go`).
- [x] **REM-002 CODE DONE** — Pinned Agent identity, native key material, TaskDeck credential session, approved client key and E2EE action codecs (`internal/remoteportal/`, `internal/remotetransport/`, `remote_access_session*.go`).
- [x] **REM-003 CODE DONE** — Native-only localhost routes with origin/capability checks (`trusted_gui.go`); PHP/Relay is not the native GUI provider.
- [x] **REM-004 CODE DONE** — Host `session.features` capability snapshot, with authoritative per-action authorization (`internal/server/remote_access_features.go`).
- [ ] **REM-005 VERIFY** — Cross-project, multi-user and revoked-session edge cases against latest permissions and tests. Add tests only for uncovered cases.
- [ ] **REM-006 LIVE PENDING** — Production provenance, real host approval, hostile-PHP/Relay and device acceptance.

## 1. Remote Task GUI — first priority

- [x] **TASK-001 CODE DONE** — Preview/list, pagination, selection from signed host snapshot, task inspect and metadata (`remote_access_tasks_preview.go`, `remote_access_task_pages.go`, `remote_access_task_prepare.go`).
- [x] **TASK-002 CODE DONE** — Host-approved short task run, exit code and bounded captured stdout/stderr (`remote_access_task_run.go`, `remote_access_task_report*.go`).
- [x] **TASK-003 CODE DONE** — Long task Start / Poll / Cancel / Clear, process-group cleanup, revoke, timeout and output cursor (`remote_access_long_task.go`).
- [x] **TASK-004 CODE DONE** — Workflow dependency support including bounded sequential/parallel fork/join (`remote_access_task_workflow.go`, `remote_access_task_workflow_parallel.go`).
- [x] **TASK-005 CODE DONE** — Host-controlled workspace cwd, fixed workflow env, safe argv/VS Code process argument objects, step timeout and rolling output (2026-11-16/18 mainline updates).
- [x] **TASK-006 CODE DONE** — Native Task GUI long-task lifecycle, signed progress, console and adaptive idle polling (`trusted_gui_assets.go`, `remote_portal/tests/native_gui_long_task*.test.cjs`).
- [x] **TASK-007 CODE DONE** — Copy concatenated task/workflow consoles from bounded native session history, order/truncation/clear/logout behavior (2026-11-18 mainline update).
- [ ] **TASK-008 VERIFY / P0** — Validate no stale/false task completion, Poll after cursor eviction, Cancel vs finalization, workflow step race and dependency failure; record failing regression only if reproduced.
  - [ ] TASK-008.1 Verify exact state transitions after revocation and cancellation.
  - [ ] TASK-008.2 Verify parallel/serial state, output ordering and final result presentation.
  - [ ] TASK-008.3 Verify Copy history and adaptive polling with UTF-8, Clear/logout, and E2EE quota.
- [ ] **TASK-009 PARTIAL / P1** — Create compatibility inventory for still-unsupported VS Code task inputs/options, then implement only approved missing task variants.
  - [ ] TASK-009.1 Compare current parser/test fixtures for prompt inputs and `continueOnError`.
  - [ ] TASK-009.2 Typed host-input schema, browser input UX and denial of unsafe arbitrary argv/secret expansion if extended.
  - [ ] TASK-009.3 Positive, malformed, stale snapshot, cancel and RBAC tests.
- [ ] **TASK-010 OPTIONAL** — Persistent resumable task across a newly authenticated session; do not reuse old session tokens.
- [ ] **TASK-011 OPTIONAL** — Unbounded/full transcript archival or extended workflow engine beyond current bounded cursor.

## 2. Git Pull / Commit / Push — second priority

- [x] **GCORE-001 CODE DONE** — Remote Git Pull review/execute with host admission and freshness checks (`remote_access_git_pull*.go`, `native_gui_git_pull_review.test.cjs`).
- [x] **GCORE-002 CODE DONE** — Remote Commit from reviewed staged state with host-owned identity, snapshot/lock (`remote_access_git_commit*.go`, `native_gui_git_commit.test.cjs`).
- [x] **GCORE-003 CODE DONE** — Remote Push with reviewed host remote/ref and scoped authorization (`remote_access_git_push.go`, `native_gui_git_push.test.cjs`).
- [x] **GCORE-004 CODE DONE** — Native UI buttons/routes, typed E2EE action codecs and host opt-in for bounded Pull/Commit/Push (`trusted_gui.go`, `internal/remotetransport/action_git_*.go`).
- [ ] **GCORE-005 VERIFY / P0** — Rerun review invalidation after local Git mutations, post-audit race, remote redirect and credentials/timeout tests; add missing case only on evidence.
  - [ ] GCORE-005.1 Pull dirty worktree/submodule, upstream and HEAD changes.
  - [ ] GCORE-005.2 Commit index/HEAD change, empty stage, malformed message, author identity.
  - [ ] GCORE-005.3 Push ref/destination change, non-fast-forward, no forced update or blind retry.
- [ ] **GCORE-006 VERIFY / P1** — Native Git GUI shows exact target, meaningful error and final commit/push status without stale cached review.

## 3. Remote Terminal — third priority

- [x] **TERM-001 CODE DONE** — Linux/macOS host PTY open/poll/write/resize/close (`remote_access_terminal_unix.go` and platform session files).
- [x] **TERM-002 CODE DONE** — Typed E2EE terminal actions, native `/api/terminal-*` routes, permission and opt-in (`action_terminal.go`, `trusted_gui.go`).
- [x] **TERM-003 CODE DONE** — Captured output, bounded cursor, session/process group teardown, timeout/revoke in host code and tests (`remote_access_terminal_unix_test.go`, `remote_access_terminal_capture_test.go`).
- [ ] **TERM-004 VERIFY / P0** — Recheck shell background children, race between close/final output, Ctrl+C, terminal resize, E2EE quota and permission downgrade; do not reimplement PTY without a failing test.
- [ ] **TERM-005 OPTIONAL** — Full interactive streaming/backpressure, broader keyboard input, reconnect/reattach with new security contract.
- [ ] **TERM-006 LIVE PENDING** — Actual Linux/macOS PTY/TTY/network interruption trials.

## 4. Other Remote Git capabilities — fourth priority

- [x] **GIT-001 CODE DONE** — Git Status, Log, Branches, Remotes, tracking, Fetch, Diff and Patch read-only typed handlers.
- [x] **GIT-002 CODE DONE** — Git Stage/Unstage, reviewed tracked or issued-path workflows with host opt-in and index guards.
- [x] **GIT-003 CODE DONE** — Branch create/delete within approved scope; switch to same-HEAD branch (not checkout arbitrary commit).
- [x] **GIT-004 CODE DONE** — Existing native Git selection/paging/review routes and regression tests (`native_gui_git_*.test.cjs`).
- [ ] **GIT-005 PARTIAL / P2** — Larger multi-file diff/compare, change tree, remote-vs-local comparison display, paged selections and refreshed source proof.
  - [ ] GIT-005.1 UI review tree/navigation with typed Git status snapshot, safe path display.
  - [ ] GIT-005.2 File/page diff cursor and safe stale state/review invalidation.
- [ ] **GIT-006 PARTIAL / P2** — Branch checkout/switch across different commits; current same-HEAD implementation must remain the baseline.
- [ ] **GIT-007 TODO / P2** — Reviewed merge/rebase/cherry-pick/revert and conflict-resolution actions, if in approved UX scope.
  - [ ] GIT-007.1 Host action contracts and fail-closed preflight/rollback policy.
  - [ ] GIT-007.2 Native review/confirm/conflict UI.
  - [ ] GIT-007.3 Race, lock, auth, revoke, conflict/abort/continue regression.
- [ ] **GIT-008 OPTIONAL** — Expanded advanced Git commands beyond agreed typed, scoped operations.

## 5. Remote Files / Editor — fifth priority

- [x] **FILE-001 CODE DONE** — Host Files root, folder/directory enumeration and file read typed actions (`remote_access_files_{root,directory,read}.go`).
- [x] **FILE-002 CODE DONE** — Native root/tree-directory selection and authenticated file read routes (`trusted_gui.go`, `native_gui_files_root.test.cjs`).
- [ ] **FILE-003 PARTIAL / P2** — Rich file manager with breadcrumbs, deep folder tree, error/empty states, search, refresh and paged file previews.
  - [ ] FILE-003.1 Native current directory/breadcrumb and host-issued IDs.
  - [ ] FILE-003.2 Next-page cursor and state invalidation across refresh/reconnect.
  - [ ] FILE-003.3 Text search and visual file comparison/diff.
- [ ] **EDIT-001 TODO / P2** — Remote **GUI editor** (current native CLI text edits are not a GUI).
  - [ ] EDIT-001.1 Host typed edit with file-bound SHA-256 CAS, mutation lock, permission/grant/audit, path/symlink/race denials.
  - [ ] EDIT-001.2 Native editor: load/view/edit, before-after preview, explicit confirmation, conflict/reload.
  - [ ] EDIT-001.3 Save/cancel/error and concurrent local edits tests; no silent overwrite.
- [ ] **FILE-004 OPTIONAL / P3** — Large/binary transfer upload/download, rename/move/delete, resumable integrity and multi-file transactions after explicit review.

## 6. Patch, Collection, other Remote UI — sixth priority

- [ ] **PATCH-001 TODO / P3** — Patch queue list/selection/host-issued package IDs and immutable preview/diff.
- [ ] **PATCH-002 TODO / P3** — Patch run/cancel/status/output/artifacts through typed E2EE host actions and native GUI.
  - [ ] PATCH-002.1 Refresh when initial queue is empty and new patches arrive; selection buttons must remain consistent.
  - [ ] PATCH-002.2 Distinguish no active prompt, running, cancelled, failed and completed; no phantom completion.
  - [ ] PATCH-002.3 Show affected-file list and command/output result boundedly.
- [ ] **COLL-001 TODO / P3** — Collection list/configuration, approved run, progress, cancel, output and artifacts.
- [ ] **UI-001 PARTIAL / P3** — Remote vs local navigation, Help (TaskDeck setup / PHP server / initial client), responsive UI, clear permission/host-opt-in diagnostics.
- [ ] **UI-002 TODO / P3** — Explicitly prioritize further screens (workspace snapshots, operation center, project notes, project health, compare) before adding typed E2EE routes; no general arbitrary HTTP proxy.

## 7. CI, safety, and LIVE — separate lane

- [x] **QA-000 CODE DONE** — Go Remote Agent, PHP Portal and native GUI test infrastructure exists (`.github/workflows/taskdeck_remote_agent.yml`, `taskdeck_remote_portal.yml`, `remote_portal/tests/native_gui_*.test.cjs`).
- [ ] **QA-001 VERIFY / ongoing** — For every new code commit, record **same source SHA** and actual local Go/Node/PHP test outcomes and GitHub Actions run conclusions (never assume a queued/cancelled run passed).
- [ ] **SEC-001 VERIFY / ongoing** — Every newly implemented host operation rechecks TaskDeck local account/key/RBAC/grant/host opt-in, audit, bounded E2EE, session ownership, path/CAS and cancellation.
- [ ] **SEC-002 LIVE PENDING** — Real two-device Linux Fedora/macOS, NAT/CGNAT, revoked key/grant, malicious PHP/Relay, offline/wifi drop, process shutdown, audit and CSRF/Origin acceptance.
- [ ] **REL-001 LIVE PENDING** — Independent native binary provenance/update/rollback and security sign-off.
- [ ] **REL-002 LIVE PENDING** — Production Generic Remote Control remains **NO-GO**: keep `remote_control_available=false`; do not treat bounded CODE DONE features as permission to enable full remote.
- [ ] **DOC-001 VERIFY** — Reconcile dated historical feature matrix and handoffs so a reader does not mistake old claims for current missing code. Preserve history rather than deleting it.

## 8. Next implementation order and reporting template

**First code work, conditional on reproduced defects:**
1. TASK-008, GCORE-005 and TERM-004 are **verification**, not a declaration of missing implementation.
2. TASK-009 covers only genuine gaps in selected VS Code task forms.
3. GCORE-006 → GIT-005/006/007 → FILE-003/EDIT-001 → PATCH-001/002 → COLL-001 → UI-001/002.
4. SEC/QA and LIVE acceptance are maintained separately, not used to re-open completed functionality.

**Per-commit update fields:** `date` · `HEAD before` · `feature ID` · `source files` · `test command and result` · `new commit SHA` · `Agent CI result` · `Portal CI result` · `what remains`. Never mark a checkbox based solely on a planning statement. Record new defects with reproducer and expected-vs-actual before assigning a code task.

At the end of each future response, summarize separately: **unfinished code by priority**, **code complete but verification targeted**, and **LIVE/release gates pending**. Windows work is not an active requirement. The separate [AI Agent Chat handoff](../TASKDECK_AI_AGENT_CHAT_HANDOFF_20261119.md) is not part of this Remote UI tracker.
