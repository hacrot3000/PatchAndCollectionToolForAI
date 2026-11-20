# TaskDeck Remote UI — Implementation plan (remaining work)

> Audit date: 2026-11-19. Branch: `main`. Repository: `hacrot3000/PatchAndCollectionToolForAI`.
> Audit baseline: the source tree at `7a96b02cb2b54455c386b044183017fad1f5ed21`.
> The companion [feature checklist](TASKDECK_REMOTE_UI_FEATURE_CHECKLIST_20261119.md) is the **single active tracker** for future commits.
> **Scope:** Native Remote UI/client, not the separate AI Agent Chat feature.

## A. Principles and status definitions

**Agreed coding order:** (1) Remote Task GUI, (2) Git Pull → Commit → Push, (3) Terminal, (4) other Git functions, (5) other GUI components, (6) remaining remote capabilities. Work directly on `main`; Windows-specific expansion is deferred.

**Do not count live-only verification as unfinished code.** Each feature carries TWO independent statuses:

- **CODE DONE**: its approved, bounded host/E2EE/native-client functionality and relevant automated tests exist. This does not imply production approval.
- **VERIFY**: code exists; review source/test evidence and add a regression only if there is a reproduced gap. Do not create speculative rewrite work.
- **PARTIAL**: a subset is in code but specifically identified user-facing functionality is missing.
- **TODO**: specified feature is not wired end-to-end through the native Remote GUI.
- **OPTIONAL**: extension requires explicit new scope; it does not downgrade the implemented baseline.
- **LIVE PENDING**: independent production/real-device acceptance not yet complete. Keep a separate list.
- A checklist box `[x]` means **source implementation exists at the audited revision**, not that LIVE has been accepted.

Keep `remote_control_available=false` and host opt-in flags off by default. The existing bounded E2EE operations are not a generic HTTP/WS reverse proxy and must not be described as unlimited Remote Control. Do not move credentials, private key material or privileged GUI code into the PHP Portal. All side-effecting host actions require locally authenticated TaskDeck principal, client-key approval, live grant, current RBAC, audit and host opt-in.

## B. Audit evidence and reconciliation

Reviewed the 2026-11-19 `main` repository tree, current native GUI routes/feature flags, E2EE action schemas, host dispatch/handlers, unit-test files and the mainline workflow/status handoffs. Particularly relevant implementation paths (all paths relative to repository root):

| Feature | Actual source and tests present on main | Conclusion |
| --- | --- | --- |
| Trusted native GUI and E2EE login | `vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui.go`; `trusted_gui_assets.go`; `internal/remoteportal/`; `internal/remotetransport/` | CODE DONE for bounded native client |
| Task preview/select/inspect/prompt/run/report/long jobs | `internal/server/remote_access_task_*.go`; `remote_access_long_task.go`; `remote_portal/tests/native_gui_task_*.test.cjs` | CODE DONE for supported bounded forms |
| Workflow and console | `remote_access_task_workflow.go`, `remote_access_task_workflow_parallel.go`, tests; native GUI assets; 2026-11-18 combined session history copy and adaptive poll | CODE DONE for implemented sequential/fork-join forms |
| Git Pull / Commit / Push | `internal/server/remote_access_git_{pull,commit,push}.go` and tests; typed `internal/remotetransport/action_git_*.go`; native `/api/git-*` | CODE DONE for limited reviewed workflows |
| Terminal PTY Linux/macOS | `internal/server/remote_access_terminal_unix.go`, `remote_access_terminal_session_{linux,darwin}.go`; `internal/remotetransport/action_terminal.go`; native `/api/terminal-{open,poll,write,resize,close}` | CODE DONE for bounded single PTY |
| Other Git | Host handlers for status/log/branches/remotes/tracking/fetch/diff/patch/stage/unstage/branch create/delete/same-HEAD switch and native routes | Existing operations CODE DONE; broader Git UX PARTIAL |
| Remote Files | `internal/server/remote_access_files_{root,directory,read}.go`; typed codecs; native `/api/files-*` | PARTIAL: read/navigation implemented, richer file manager missing |
| Remote Editor GUI | CLI text-edit support exists; dedicated remote native GUI editor route/host editor feature not observed in audited tree | TODO for GUI, do not mistake CLI edit for GUI completion |
| Remote Patch/Collection GUI | No dedicated `remote_access_patch*` / `remote_access_collect*` host handler and no native Patch/Collection route in reviewed GUI | TODO |
| Production evidence | `TASKDECK_REMOTE_GUI_LIVE_ACCEPTANCE_20261101.md` and release-gate handoffs | LIVE PENDING, not a code deficit |

**Stale-doc warning:** [Historical interactive matrix](TASKDECK_REMOTE_INTERACTIVE_FEATURE_MATRIX.md) and [older handoffs](TASKDECK_REMOTE_OPEN_FEATURE_HANDOFF.md) predate substantial implementation. Do **not** mechanically carry their “Terminal PTY/Git write/task dependencies missing” descriptions into the current backlog. [Mainline workflow](TASKDECK_REMOTE_MAINLINE_WORKFLOW.md) and code through 2026-11-18 take precedence. The new checklist intentionally records status separately from deployment.

**Audit limitation:** repository/source inspection only. No real remote login, Linux/macOS client, PHP/Relay or end-to-end deployment was exercised by this documentation task; no current CI result was independently claimed PASS.

## C. Implementation milestones (features → tasks → subtasks)

### M0. Baseline regression and evidence closure — P0 / VERIFY

This milestone finishes the user's **first three priorities without rebuilding completed code**.

**M0.1 Remote Task GUI audit**
- M0.1.1 Check native preview/inspect/select → E2EE → host task source snapshot/ID validation → audit → process launch → signed report/console.
- M0.1.2 Review short/long/parallel/sequential/fork-join lifecycle: RUNNING, completed, failed, cancelled, timed_out, skipped, source/cwd/env updates and dependency failure.
- M0.1.3 Assert stale polling, duplicate cancel, logout, revoked key/grant, race at process launch/finalization, Unicode console, cursor eviction and automatic polling do not invent completion. Add **only missing targeted tests**.
- M0.1.4 Recheck session-history **Copy broadcast group console** equivalent in native Task GUI: every retained task/workflow console in chronological order, explicit truncation, safe cleanup on Clear/logout.

**M0.2 Git Pull → Commit → Push audit**
- M0.2.1 Trace review/confirm native routes to typed E2EE and host handler for each operation.
- M0.2.2 Verify GitWrite + operation permission/flag, current branch/upstream/HEAD/index/repo snapshot, attempted audit, lock, revocation and non-replay after a side effect.
- M0.2.3 Cover stale review after stage/commit/pull/push, redirected remote URL, credential injection, failure/timeout, invalid upstream, non-fast-forward and concurrent local Git changes.

**M0.3 Terminal baseline audit**
- M0.3.1 Verify open/poll/write/resize/close native UI, typed E2EE codec, host permission/opt-in and capture cursor.
- M0.3.2 Test Ctrl+C/EOF, process group/session teardown, background children, expired session, role revoke, workspace lease, bounded output and zero orphaned processes.
- M0.3.3 Keep implemented single-PTY scope marked CODE DONE; streaming/detach belong to optional M3.

**Exit M0:** every baseline row has a source/test reference. A failed or genuinely missing case gets an explicit bug entry plus reproducer. LIVE-only gaps are moved to M7 without downgrading code status.

### M1. Task GUI feature extensions — P1 / PARTIAL or OPTIONAL

- **TASK-01 Compatibility inventory:** enumerate actual VS Code task types and options supported by parser and GUI: `shell`/`process`, static args, host-authorized `pickString`, typed prompt inputs, sequential and bounded parallel dependency graphs, variable expansion, `continueOnError`, per-step deadlines, fixed env and workspace cwd.
  - TASK-01.1 Compare code and tests to representative `.vscode/tasks.json` fixtures; do not claim entire dependency handling is missing.
  - TASK-01.2 Record only unsupported **requested** forms and why (unsupported / unsafe / not specified).
- **TASK-02 Advanced inputs:** if required, define host-owned allowlisted prompt semantics for still-unsupported combinations; no arbitrary browser-supplied argv/secret values.
  - TASK-02.1 Signed schema + strict parser + RBAC/opt-in.
  - TASK-02.2 Native input UI + cancel/validation + CLI compatibility.
  - TASK-02.3 Positive/negative/modified-source tests.
- **TASK-03 Output history extension:** if needed beyond bounded rolling capture, specify retention, cursor, truncation, per-step attribution, stream order and network loss behavior before implementation.
- **TASK-04 Resume/reattach (optional):** new host-owned authenticated job contract with TTL/ownership/revoke and no stale E2EE token reuse; never infer success solely from a persisted display row.

**Exit M1:** users can run every **approved** task form through GUI with accurate job status; unsupported forms are shown as unsupported, not offered as buttons that fail silently.

### M2. Git Pull / Commit / Push polish — P1 / VERIFY

- **GCORE-01 Pull**: preflight, dirty index/worktree/submodule, upstream/remote URL change, ff-only/ref correctness, identity, network failure and cancellation.
- **GCORE-02 Commit**: staged snapshot, bounded commit text, host-owned identity, HEAD/index recheck after audit, clear resulting SHA, no commit if review invalidates.
- **GCORE-03 Push**: exact authorized remote/ref, credential isolation, HTTPS redirect/TLS policy, rejected non-fast-forward, no implicit force, status/error rendering.
- **GCORE-04 UX**: review/confirm clearly displays action target and consequences; refresh invalidates cached Git status/diff/pull/commit/push review; no duplicate POST after timeout.

**Exit M2:** these **already implemented** limited Git operations retain CODE DONE after targeted regression. Do not count extra Git operations in M4 as missing core Pull/Commit/Push.

### M3. Terminal GUI enhancements — P2 / OPTIONAL

- **TERM-01** Existing single bounded Linux/macOS PTY: CODE DONE; preserve 90-second deadline, output cap, line-length/round quotas, host permissions and safe cleanup.
- **TERM-02** True higher-throughput streaming (if approved): ordered frames, bounded buffering/backpressure, cursor and viewport, out-of-order/replay prevention, explicit user control; no raw cross-origin socket proxy.
- **TERM-03** Detach/reattach (if approved): host-owned session identity, TTL/ownership limits, revoke/network partition/agent restart cleanup; explicitly distinguish disconnect from kill.
- **TERM-04** Input UX (if approved): terminal key events, paste safety, resize frequency, Unicode/control handling and native renderer sandboxing.

### M4. Remaining Git UI and actions — P2 / PARTIAL

- **GIT-01** Multi-file and paged read-only reviews, changed-path navigation, cached/untracked distinctions and visible “diff with remote” review when grounded by a confirmed fetched ref.
  - GIT-01.1 Reuse existing status/log/diff/patch/tracking cursors rather than reintroducing shell-based Git commands.
  - GIT-01.2 Respect E2EE quota and immutable snapshot IDs; Next must fail closed when stale.
- **GIT-02** Additional branch operations beyond existing create/delete same-HEAD/switch same-HEAD; moving to a different commit needs clean-state guarantees, review and rollback.
- **GIT-03** Merge/rebase/cherry-pick/revert and conflict UI: separately approved typed host contracts, scoped flags, preview, CAS/mutation lock, abort/continue semantics and tests; not present as generic commands.
- **GIT-04** Remote Git GUI parity: branch chooser, conflict visualisation, staged-vs-unstaged review, safe refresh and useful errors.

### M5. Remote Files + Editor GUI — P2 / PARTIAL + TODO

- **FILE-01** Finish tree/breadcrumb/folder paging and selection UX on existing `files.root` / directory navigation. Directory list and file read **already exist**.
  - FILE-01.1 Empty/error/permission/symlink states; project-bound canonical path and selected entry ID.
  - FILE-01.2 Paging cursor, stale snapshot invalidation, no free-form path trusted from browser.
- **FILE-02** File view/compare: read-only UTF-8, large-file chunk navigation, search, left/right compare and diff; propagate signed version/size/truncation state.
- **EDIT-01** New Remote GUI editor: design a typed E2EE mutation using host-issued file identifiers, full-file SHA-256 CAS, bounded change, preview/diff, user confirmation, single-writer lock, FilesWrite + live grant and audit.
  - EDIT-01.1 Host contract, negative tests for symlink/path/mutation race.
  - EDIT-01.2 Native editor GUI and conflict/“reload and reapply” workflow.
  - EDIT-01.3 End-to-end read → modify → confirm → save with remote version validation. Native CLI edit does **not** complete this.
- **FILE-03** Transfer and management (explicitly scoped later): upload/download larger/binary files with integrity/checkpoints and owned destination, rename/move/delete and rollback where feasible. Do not silently add arbitrary filesystem writes.

### M6. Patch, Collection and remaining Remote GUI — P3 / TODO

- **PATCH-01** Host-scoped list/detail/review for queued patches and affected files using immutable patch identifiers and source proof; no arbitrary package path or ZIP execution.
- **PATCH-02** Review → Run selected → Poll console/output → Cancel → finished status; integrate with host Patch dispatcher under new scoped E2EE actions.
  - PATCH-02.1 Implement exact state transitions when queue is empty at page load and later receives new patch.
  - PATCH-02.2 Ensure Run selected/Cancel presence reflects current selection and active host session; avoid stale “no active prompt”.
  - PATCH-02.3 No false success, explicit artifact/affected file list and error recovery.
- **COLL-01** Collection list/configuration/project scope read-only, then reviewed bounded run, progress and result/artifacts.
- **GUI-01** Bring remaining Remote navigation, Help/install/setup, mobile resizing, error messages, permissions and refresh closer to local TaskDeck, without unreviewed HTTP/WS proxying.
- **GUI-02** Further screens (workspace snapshots, project notes, operation center, project health, compare) only after prioritized capability design; this document does not falsely claim they already have Remote routes.

### M7. Continuous security, CI and deployment — P0 for LIVE / LIVE PENDING

This is a **parallel release lane**; lack of a physical test does not convert CODE DONE back to TODO.

- **SEC-01** Every new action: client key approved by host, pinned Agent ID, TaskDeck password session, per-project RBAC, current grant, host opt-in, audit and typed E2EE framing.
- **SEC-02** Lost session/key/account/permission, cancelled task, compromised PHP/Relay, wrong-origin browser, replay/duplicate frames, concurrency and process cleanup must all fail closed.
- **QA-01** Run Go host/transport/client and race tests, Node native GUI mock-DOM suites, PHP Portal tests, static asset checks and Linux/macOS builds; record exact commit SHA and both workflows' results.
- **QA-02** Perform the real Linux Fedora/macOS/NAT-CGNAT matrix, disconnection, key and grant revoke, E2EE session expiration, binary provenance/rollback and independent hostile-PHP review. See [LIVE acceptance](TASKDECK_REMOTE_GUI_LIVE_ACCEPTANCE_20261101.md).
- **REL-01** Independent release sign-off: never change `remote_control_available` just to get a demo working; the full production security gate is separate from developing bounded features.

## D. Execution protocol on main

1. Fetch latest `main` before touching code; read and update the companion checklist. Never overwrite another AI's newer changes or rewrite history.
2. Pick **one code gap** in the priority order above; if source is already complete, add targeted verification only. Mark old “pending” prose as obsolete when a later code file disproves it.
3. Create a minimal failing test for a real gap, implement typed action + host + client/UI together, then add grant/permission/CAS/cancel/error regressions.
4. Execute relevant tests locally. Commit and push directly to `main` with a minimal, reviewable message. Verify both [Agent CI](../../.github/workflows/taskdeck_remote_agent.yml) and [Portal CI](../../.github/workflows/taskdeck_remote_portal.yml) for the **same HEAD**; pending/cancelled jobs are not PASS.
5. Update checklist IDs, source/test paths, commit SHA, exact next unfinished scope and unchanged LIVE NO-GO. At end of each response summarize **only unfinished code tasks**, with live-only items reported separately.

**Recommended test examples** (commands documented, **not executed in this audit**):

```sh
cd vscode_tasks_menu_go
go test ./internal/server ./internal/remotetransport ./cmd/taskdeck_remote_login -run 'TestRemote|TestTrustedGUI' -count=1
go test -race ./internal/server -run TestRemote -count=1
cd ..
node --test remote_portal/tests/native_gui_*.test.cjs
```

Additional regression targets: `remote_portal/tests/native_gui_long_task*.test.cjs`, `native_gui_git_*.test.cjs`, `native_gui_terminal*.test.cjs`, plus `.github/workflows/taskdeck_remote_agent.yml` and `taskdeck_remote_portal.yml`. Inspect the actual test names before invoking narrow `-run` expressions.

## E. Documentation ownership

- This **plan** describes dependencies and acceptance criteria.
- The [feature checklist](TASKDECK_REMOTE_UI_FEATURE_CHECKLIST_20261119.md) owns live statuses and individual task IDs.
- [Mainline workflow](TASKDECK_REMOTE_MAINLINE_WORKFLOW.md) contains chronological implementation notes.
- [Implementation status](TASKDECK_REMOTE_IMPLEMENTATION_STATUS.md) is historical cumulative handoff.
- [Interactive matrix](TASKDECK_REMOTE_INTERACTIVE_FEATURE_MATRIX.md) remains historically useful but contains stale “not supported” lines.
- [LIVE matrix](TASKDECK_REMOTE_GUI_LIVE_ACCEPTANCE_20261101.md) owns physical/deployment acceptance.
- [AI Agent Chat handoff](../TASKDECK_AI_AGENT_CHAT_HANDOFF_20261119.md) is out of this Remote UI scope.

After the owner's documentation move, all new Remote UI planning files live in `docs/taskdeck_remote/`. No new `TASKDECK_REMOTE_*` document should be added at the `docs/` root.

**Evidence caveat:** This checklist and plan are source-based, not newly run local/remote tests or a security certification.
